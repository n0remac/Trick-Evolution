// Bounded, fixed-step card motion. All distances are in table world units.
const STEP = 1 / 120;
const HALF_WIDTH = .43;
const HALF_THICKNESS = .0175;
const HALF_LENGTH = .61;
const GRAVITY = 9.8;
const MAX_SPEED = 4.5;
const clamp = (value, low, high) => Math.max(low, Math.min(high, value));

export function releaseVelocity(samples) {
  if (samples.length < 2) return {x: 0, z: 0};
  const last = samples[samples.length - 1];
  const first = samples.find(sample => last.time - sample.time <= 130) || last;
  const seconds = Math.max(.025, (last.time - first.time) / 1000);
  let x = (last.x - first.x) / seconds;
  let z = (last.z - first.z) / seconds;
  const speed = Math.hypot(x, z);
  if (speed > MAX_SPEED) { x *= MAX_SPEED / speed; z *= MAX_SPEED / speed; }
  return {x, z};
}

export function cardSupport(rotation) {
  const [pitch, , roll] = rotation;
  const cp = Math.cos(pitch);
  return HALF_WIDTH * Math.abs(cp * Math.sin(roll)) +
    HALF_THICKNESS * Math.abs(cp * Math.cos(roll)) +
    HALF_LENGTH * Math.abs(Math.sin(pitch));
}

export function createDropState(from, target, velocity) {
  return {
    position: [...from.position], rotation: [...from.rotation], scale: [...from.scale],
    target, vx: velocity.x, vz: velocity.z, vy: -.25,
    xLimit: Math.max(1.82, Math.abs(from.position[0])),
    zLimit: Math.max(3.8, Math.abs(from.position[2])),
    releaseSpeed: Math.hypot(velocity.x, velocity.z),
    wx: clamp(-velocity.z * .28, -1.6, 1.6),
    wz: clamp(-velocity.x * .42, -2.2, 2.2),
    contact: false, contactTilt: null, elapsed: 0, accumulator: 0, done: false,
    maxTilt: Math.abs(from.rotation[2]),
  };
}

function step(state) {
  const p = state.position, r = state.rotation, target = state.target;
  if (state.contact) {
    // A damped spring draws the card into the exact slot; friction removes slide.
    state.vx += (target.position[0] - p[0]) * 150 * STEP;
    state.vz += (target.position[2] - p[2]) * 150 * STEP;
    const friction = Math.exp(-23 * STEP);
    state.vx *= friction; state.vz *= friction;
    state.wx += (target.rotation[0] - r[0]) * 170 * STEP;
    state.wz += (target.rotation[2] - r[2]) * 170 * STEP;
    const angularFriction = Math.exp(-25 * STEP);
    state.wx *= angularFriction; state.wz *= angularFriction;
  } else {
    const air = Math.exp(-1.15 * STEP);
    state.vx *= air; state.vz *= air;
    // A released hand card turns face-up while falling. Fast releases retain
    // enough angular velocity to tip and slide; a gentle release levels out.
    const momentum = clamp(state.releaseSpeed / MAX_SPEED, 0, 1);
    const alignment = 100 - momentum * 70;
    state.wx += (target.rotation[0] - r[0]) * alignment * STEP;
    state.wz += (target.rotation[2] - r[2]) * alignment * STEP;
    const angularAir = Math.exp(-(14 - momentum * 8) * STEP);
    state.wx *= angularAir; state.wz *= angularAir;
    p[1] += state.vy * STEP - GRAVITY * STEP * STEP / 2;
    state.vy -= GRAVITY * STEP;
  }
  p[0] += state.vx * STEP;
  p[2] += state.vz * STEP;
  r[0] = clamp(r[0] + state.wx * STEP, -.85, .85);
  r[2] = clamp(r[2] + state.wz * STEP, -.8, .8);
  state.maxTilt = Math.max(state.maxTilt, Math.abs(r[2]));
  for (const [axis, limit, velocity] of [[0, state.xLimit, 'vx'], [2, state.zLimit, 'vz']]) {
    if (Math.abs(p[axis]) > limit) {
      p[axis] = clamp(p[axis], -limit, limit);
      state[velocity] = 0;
    }
  }
  // The target card's bottom defines the contact plane. Its rotated support
  // height keeps every corner above the felt, including at the field edge.
  const floor = target.position[1] - HALF_THICKNESS + cardSupport(r);
  if (state.contact || p[1] <= floor) {
    if (!state.contact) state.contactTilt = Math.hypot(r[0] - target.rotation[0], r[2] - target.rotation[2]);
    p[1] = floor;
    state.vy = 0;
    state.contact = true;
  }
  state.elapsed += STEP;
  const distance = Math.hypot(p[0] - target.position[0], p[2] - target.position[2]);
  const tilt = Math.hypot(r[0] - target.rotation[0], r[2] - target.rotation[2]);
  if (state.contact && state.elapsed >= .48 && distance < .008 && tilt < .012 &&
      Math.hypot(state.vx, state.vz) < .09 && Math.hypot(state.wx, state.wz) < .12) {
    state.done = true;
  }
  if (state.elapsed >= 1.1) state.done = true;
  if (state.done) {
    state.position = [...target.position];
    state.rotation = [...target.rotation];
    state.scale = [...target.scale];
  }
}

export function advanceDrop(state, seconds) {
  if (state.done) return state;
  state.accumulator += Math.max(0, Math.min(seconds, 1.1));
  while (state.accumulator >= STEP && !state.done) {
    step(state);
    state.accumulator -= STEP;
  }
  return state;
}
