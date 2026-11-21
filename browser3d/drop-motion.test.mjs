import assert from 'node:assert/strict';
import test from 'node:test';
import {advanceDrop, cardSupport, createDropState, releaseVelocity} from './drop-motion.mjs';

const target = {position: [0, .045, 1.463], rotation: [0, 0, 0], scale: [1, 1, 1]};
const held = {position: [.1, .74, .65], rotation: [.61, 0, 0], scale: [1, 1, 1]};
const contactPlane = target.position[1] - cardSupport(target.rotation);

test('release samples bound quick motion and let a paused release slow down', () => {
  const fast = releaseVelocity([{x: 0, z: 0, time: 0}, {x: 2, z: 0, time: 40}]);
  assert.ok(Math.hypot(fast.x, fast.z) <= 4.5 + 1e-9);
  const slow = releaseVelocity([{x: 0, z: 0, time: 0}, {x: .1, z: 0, time: 80}, {x: .1, z: 0, time: 150}]);
  assert.equal(slow.x, 0);
});

test('gravity, contact and friction settle slow and fast drops without penetration', () => {
  const gentle = createDropState(held, target, {x: .12, z: -.08});
  const quick = createDropState(held, target, {x: 4, z: -1.5});
  advanceDrop(gentle, .15);
  advanceDrop(quick, .15);
  assert.ok(quick.position[0] > gentle.position[0] + .35, 'quick release retains linear momentum');
  assert.ok(quick.maxTilt > gentle.maxTilt + .1, 'quick release gains angular momentum');
  for (const state of [gentle, quick]) {
    let contactPitch = null;
    for (let i = 0; i < 140 && !state.done; i++) {
      advanceDrop(state, 1 / 120);
      if (state.contact && contactPitch === null) contactPitch = Math.abs(state.rotation[0]);
      assert.ok(state.position[1] + 1e-9 >= contactPlane + cardSupport(state.rotation), 'card corner entered the table');
      assert.ok(Math.abs(state.position[0]) <= 1.82 + 1e-9);
    }
    if (state === gentle) assert.ok(contactPitch < .08, 'gentle drop first touches nearly flat');
    else assert.ok(contactPitch > .08, 'quick drop retains tilt at contact');
    assert.equal(state.done, true);
    assert.equal(state.contact, true);
    assert.deepEqual(state.position, target.position);
    assert.deepEqual(state.rotation, target.rotation);
  }
});

test('edge drops stay on the table and yield the same pose across frame rates', () => {
  const edge = {position: [1.72, .7, 1.1], rotation: [.61, 0, 0], scale: [1, 1, 1]};
  const fastFrames = createDropState(edge, target, {x: 4.5, z: 1});
  const slowFrames = createDropState(edge, target, {x: 4.5, z: 1});
  for (let i = 0; i < 36; i++) advanceDrop(fastFrames, 1 / 120);
  for (let i = 0; i < 9; i++) advanceDrop(slowFrames, 1 / 30);
  for (const axis of [0, 1, 2]) {
    assert.ok(Math.abs(fastFrames.position[axis] - slowFrames.position[axis]) < .02);
  }
  assert.ok(fastFrames.position[0] <= 1.82);
  for (let i = 0; i < 140; i++) {
    advanceDrop(fastFrames, 1 / 120);
    advanceDrop(slowFrames, 1 / 120);
  }
  assert.deepEqual(fastFrames.position, target.position);
  assert.deepEqual(slowFrames.position, target.position);
  const outsideSafeCenter = createDropState({position: [1.95, .7, 1.1], rotation: [.61, 0, 0], scale: [1, 1, 1]}, target, {x: 4.5, z: 0});
  advanceDrop(outsideSafeCenter, 1 / 120);
  assert.ok(outsideSafeCenter.position[0] >= 1.94, 'edge release snapped inward on its first frame');
});
