// Reconciler and local presentation runtime. Rules and legal cards arrive from Go.
import * as THREE from '/game/vendor/three-0.160.1.module.js';
import {advanceDrop, createDropState, releaseVelocity} from '/game/browser3d/drop-motion.mjs';

const vec = values => new THREE.Vector3(...values);
const ease = (kind, t) => kind === 'easeOutCubic' ? 1 - (1 - t) ** 3 :
  kind === 'easeInOutCubic' ? (t < .5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2) : t;
function transform(object, value) {
  object.position.set(...value.position);
  object.rotation.set(...value.rotation);
  object.scale.set(...value.scale);
}
function materialFor(spec, round) {
  if (!spec.text) return new THREE.MeshStandardMaterial({color: spec.color, roughness: .78});
  const canvas = document.createElement('canvas');
  canvas.width = 256; canvas.height = 360;
  const ctx = canvas.getContext('2d');
  ctx.fillStyle = spec.color; ctx.fillRect(0, 0, 256, 360);
  ctx.strokeStyle = '#d7d0b7'; ctx.lineWidth = 9; ctx.beginPath(); ctx.roundRect(10, 10, 236, 340, 12); ctx.stroke();
  ctx.fillStyle = spec.ink || '#18251c';
  ctx.textBaseline = 'middle';
  ctx.font = 'bold 55px system-ui'; ctx.textAlign = 'left'; ctx.fillText(spec.text, 18, 51);
  ctx.textAlign = 'right'; ctx.fillText(spec.text, 238, 309);
  ctx.font = 'bold 88px system-ui'; ctx.textAlign = 'center'; ctx.fillText(spec.text, 128, 180);
  const texture = new THREE.CanvasTexture(canvas); texture.colorSpace = THREE.SRGBColorSpace;
  const side = new THREE.MeshStandardMaterial({color: spec.color, roughness: .67});
  const face = new THREE.MeshStandardMaterial({map: texture, roughness: .67});
  return round ? [face, side] : [side, side, face, side, side, side];
}
function geometryFor(node) {
  const [width, height, depth] = node.size;
  if (!node.radius) return new THREE.BoxGeometry(width, height, depth);

  const bevel = Math.min(height * .24, node.radius * .22);
  const x = width / 2 - bevel, y = depth / 2 - bevel, r = node.radius - bevel;
  const shape = new THREE.Shape();
  shape.moveTo(x - r, y);
  shape.absarc(-x + r, y - r, r, Math.PI / 2, Math.PI, false);
  shape.absarc(-x + r, -y + r, r, Math.PI, 3 * Math.PI / 2, false);
  shape.absarc(x - r, -y + r, r, 3 * Math.PI / 2, 2 * Math.PI, false);
  shape.absarc(x - r, y - r, r, 0, Math.PI / 2, false);
  shape.closePath();

  const geometry = new THREE.ExtrudeGeometry(shape, {
    depth: height - 2 * bevel, curveSegments: 16, bevelSize: bevel,
    bevelThickness: bevel, bevelSegments: 3,
  });
  const positions = geometry.getAttribute('position'), uv = geometry.getAttribute('uv');
  for (let i = 0; i < positions.count; i++) {
    uv.setXY(i, .5 + positions.getX(i) / width, .5 + positions.getY(i) / depth);
  }
  geometry.translate(0, 0, -(height - 2 * bevel) / 2);
  geometry.rotateX(-Math.PI / 2);
  return geometry;
}
function dispose(object) {
  object.geometry?.dispose();
  const materials = Array.isArray(object.material) ? new Set(object.material) : new Set(object.material ? [object.material] : []);
  for (const material of materials) { material.map?.dispose(); material.dispose(); }
}
const snapshot = object => ({position: object.position.toArray(), rotation: [object.rotation.x, object.rotation.y, object.rotation.z], scale: object.scale.toArray()});

export function createRuntime(canvas, callbacks) {
  const renderer = new THREE.WebGLRenderer({canvas, antialias: true});
  renderer.shadowMap.enabled = true; renderer.shadowMap.type = THREE.PCFSoftShadowMap;
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  const world = new THREE.Scene();
  world.add(new THREE.HemisphereLight('#ecf4da', '#153528', 2.2));
  const sun = new THREE.DirectionalLight('#fff0ca', 2.4);
  sun.position.set(-3, 8, 4); sun.castShadow = true; sun.shadow.mapSize.set(512, 512);
  Object.assign(sun.shadow.camera, {left:-6,right:6,top:6,bottom:-6});
  world.add(sun);
  const camera = new THREE.PerspectiveCamera(44, 1, .1, 100);
  const objects = new Map(), signatures = new Map(), bases = new Map(), legal = new Set(), promoted = new Set();
  const lifts = new Map();
  const raycaster = new THREE.Raycaster(), plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), -.35);
  const dragPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0));
  let disposed = false, frame = 0, motion = null, generation = 0, revision = -1, active = null, hover = null;
  let createCount = 0, maxTravel = 0, lastDrop = null, lastWidth = 0, lastHeight = 0, lastRatio = 0;
  let baseCamera = [0, 6.4, 7.1], targetCamera = [0, 0, 0];
  const render = () => { if (!disposed && !frame) frame = requestAnimationFrame(() => { frame = 0; renderer.render(world, camera); }); };
  function resize() {
    const box = canvas.getBoundingClientRect(); if (!box.width || !box.height || disposed) return;
    const ratio = Math.min(window.devicePixelRatio || 1, 2, 1600 / box.width, 1400 / box.height);
    if (box.width !== lastWidth || box.height !== lastHeight || ratio !== lastRatio) {
      renderer.setPixelRatio(ratio); renderer.setSize(box.width, box.height, false);
      camera.aspect = box.width / box.height;
      // Fit a 4.7-unit-wide fan in portrait while retaining a 45-degree table view.
      const scale = Math.max(1.13, .57 / camera.aspect);
      camera.position.set(baseCamera[0] * scale, baseCamera[1] * scale, baseCamera[2] * scale);
      camera.lookAt(vec(targetCamera));
      camera.updateMatrixWorld();
      camera.updateProjectionMatrix();
      lastWidth = box.width; lastHeight = box.height; lastRatio = ratio;
      callbacks.onResize?.();
    }
    render();
  }
  const observer = new ResizeObserver(resize); observer.observe(canvas);
  function cancel() { generation++; motion = null; if (active) { active = null; callbacks.onDragState?.(false); } }
  function remove(id) { const object = objects.get(id); if (!object) return; object.removeFromParent(); dispose(object); objects.delete(id); signatures.delete(id); bases.delete(id); }
  function apply(description, cards = []) {
    if (description.version !== 1 || !Array.isArray(description.nodes)) throw new Error('Unsupported 3D scene');
    cancel(); revision = description.revision; legal.clear(); cards.filter(c => c.legal).forEach(c => legal.add(c.pickId));
    lifts.clear();
    for (const card of cards) lifts.set(card.pickId, card.animation.steps[0].to);
    world.background = new THREE.Color(description.background);
    camera.fov = description.camera.fov; baseCamera = description.camera.position; targetCamera = description.camera.target;
    // A light fog and felt plane give the shallow cards readable depth.
    world.fog = new THREE.Fog(description.background, 16, 36);
    const next = new Set(description.nodes.map(node => node.id));
    for (const id of objects.keys()) if (!next.has(id)) remove(id);
    for (const node of description.nodes) {
      const signature = JSON.stringify([node.kind, node.size, node.radius, node.material, node.parent, node.pickId]);
      let object = objects.get(node.id);
      if (object && signatures.get(node.id) !== signature && !promoted.has(node.id)) { remove(node.id); object = null; }
      if (!object) {
        object = node.kind === 'group' ? new THREE.Group() : new THREE.Mesh(geometryFor(node), materialFor(node.material, Boolean(node.radius)));
        object.userData.pickId = node.pickId || '';
        if (node.kind !== 'group') { object.castShadow = node.id.startsWith('hand_') || node.id.startsWith('card_'); object.receiveShadow = node.id === 'surface' || node.id === 'dropzone'; }
        (objects.get(node.parent) || world).add(object); objects.set(node.id, object); signatures.set(node.id, signature); createCount++;
      }
      signatures.set(node.id, signature); promoted.delete(node.id);
      transform(object, node.transform); bases.set(node.id, snapshot(object));
    }
    hover = null; resize(); render();
  }
  function setHover(id) {
    if (hover === id || active || (id && (motion || callbacks.inputBlocked?.()))) return;
    if (hover && objects.has(hover) && bases.has(hover)) transform(objects.get(hover), bases.get(hover));
    hover = id;
    if (id && lifts.has(id)) { transform(objects.get(id), lifts.get(id)); canvas.style.cursor = 'pointer'; }
    else canvas.style.cursor = 'default';
    render();
  }
  function animateAll(descriptions) {
    const tracks = descriptions.filter(item => item.version === 1 && item.steps?.length && objects.has(item.target))
      .map(item => ({item, object: objects.get(item.target), from: snapshot(objects.get(item.target)), origin: objects.get(item.target).position.clone()}));
    if (!tracks.length) return Promise.resolve(false);
    cancel(); const token = generation;
    maxTravel = 0; let index = 0, started = performance.now();
    motion = tracks.map(track => track.item.target).join(',');
    return new Promise(resolve => {
      const tick = now => {
        if (disposed || token !== generation || tracks.some(track => !objects.has(track.item.target))) { resolve(false); return; }
        let complete = true;
        for (const track of tracks) {
          const step = track.item.steps[index];
          if (!step) continue;
          const t = Math.min(1, (now - started) / step.durationMs), k = ease(step.easing, t);
          for (const field of ['position', 'rotation', 'scale']) {
            const values = track.from[field].map((v, i) => v + (step.to[field][i] - v) * k);
            if (field === 'rotation') track.object.rotation.set(...values); else track.object[field].set(...values);
          }
          maxTravel = Math.max(maxTravel, track.object.position.distanceTo(track.origin));
          if (t < 1) complete = false;
        }
        render();
        if (complete) {
          index++; if (tracks.every(track => index >= track.item.steps.length)) { motion = null; resolve(true); return; }
          tracks.forEach(track => { track.from = snapshot(track.object); }); started = now;
        }
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
  }
  const animate = description => animateAll([description]);
  function tap(id, target, afterPlay, reduced) {
    const object = objects.get(id);
    if (!object) return Promise.resolve(false);
    const start = snapshot(object);
    const duration = reduced ? 90 : 300;
    const steps = reduced ? [{to: target, durationMs: duration, easing: 'easeOutCubic'}] : [
      {to: {position: [(start.position[0] + target.position[0]) / 2,
        Math.max(start.position[1], target.position[1]) + .42,
        (start.position[2] + target.position[2]) / 2],
        rotation: [start.rotation[0] * .45, 0, 0], scale: target.scale}, durationMs: 135, easing: 'easeOutCubic'},
      {to: target, durationMs: 165, easing: 'easeInOutCubic'},
    ];
    return animateAll([{version: 1, target: id, steps}, ...afterPlay.map(item => ({
      version: 1, target: item.target, steps: [{to: item.to, durationMs: duration, easing: 'easeInOutCubic'}],
    }))]);
  }
  function drop(id, target, afterPlay, samples, reduced) {
    const object = objects.get(id);
    if (!object) return Promise.resolve(false);
    const velocity = releaseVelocity(samples);
    lastDrop = {speed: Math.hypot(velocity.x, velocity.z), maxTilt: 0, contactTilt: null, contact: false, done: false};
    if (reduced) {
      return animateAll([{version: 1, target: id, steps: [{to: target, durationMs: 90, easing: 'easeOutCubic'}]},
        ...afterPlay.map(item => ({version: 1, target: item.target,
          steps: [{to: item.to, durationMs: 90, easing: 'easeOutCubic'}]}))]).then(ok => {
        lastDrop = {...lastDrop, contact: ok, done: ok, contactTilt: 0};
        return ok;
      });
    }
    const origin = object.position.clone();
    const state = createDropState(snapshot(object), target, velocity);
    const closing = afterPlay.filter(item => objects.has(item.target)).map(item => ({
      object: objects.get(item.target), from: snapshot(objects.get(item.target)), to: item.to,
    }));
    cancel(); const token = generation;
    motion = `drop:${id}`; maxTravel = 0;
    let previous = performance.now();
    return new Promise(resolve => {
      const tick = now => {
        if (disposed || token !== generation || objects.get(id) !== object) { resolve(false); return; }
        advanceDrop(state, (now - previous) / 1000); previous = now;
        transform(object, state);
        const k = ease('easeInOutCubic', Math.min(1, state.elapsed / .3));
        for (const track of closing) {
          const pose = {};
          for (const field of ['position', 'rotation', 'scale']) {
            pose[field] = track.from[field].map((value, index) => value + (track.to[field][index] - value) * k);
          }
          transform(track.object, pose);
        }
        maxTravel = Math.max(maxTravel, object.position.distanceTo(origin));
        lastDrop = {speed: Math.hypot(velocity.x, velocity.z), maxTilt: state.maxTilt, contactTilt: state.contactTilt,
          contact: state.contact, done: state.done};
        render();
        if (state.done) { motion = null; resolve(true); return; }
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
  }
  function restorePlay(id, afterPlay, reduced) {
    const ids = [id, ...afterPlay.map(item => item.target)];
    return animateAll(ids.filter(key => objects.has(key) && bases.has(key)).map(key => ({
      version: 1, target: key, steps: [{to: bases.get(key), durationMs: reduced ? 90 : 220, easing: 'easeOutCubic'}],
    })));
  }
  function promotePlay(id) {
    const object = objects.get(id);
    if (!object) return;
    const slot = id.replace(/^hand_/, 'card_');
    objects.delete(id); signatures.delete(id); bases.delete(id); lifts.delete(id); legal.delete(id);
    object.userData.pickId = '';
    objects.set(slot, object);
    promoted.add(slot);
  }
  function ray(event) {
    const box = canvas.getBoundingClientRect(); if (!box.width || !box.height) return false;
    raycaster.setFromCamera(new THREE.Vector2((event.clientX-box.left)/box.width*2-1, -(event.clientY-box.top)/box.height*2+1), camera);
    return true;
  }
  function hit(event) {
    if (!ray(event)) return null;
    return raycaster.intersectObjects([...objects.values()].filter(o => o.userData.pickId), false)[0]?.object.userData.pickId || null;
  }
  function hitDetail(event) {
    if (!ray(event)) return null;
    return raycaster.intersectObjects([...objects.values()].filter(o => o.userData.pickId), false)[0] || null;
  }
  function planePoint(event, surface = plane) { if (!ray(event)) return null; return raycaster.ray.intersectPlane(surface, new THREE.Vector3()); }
  const validPoint = p => !!p && Math.abs(p.x) < 1.55 && Math.abs(p.z) < 1.2;
  function sample(current, event) {
    const object = objects.get(current.id);
    if (!object) return;
    const last = current.samples[current.samples.length - 1];
    const now = Math.max(Number.isFinite(event.timeStamp) ? event.timeStamp : performance.now(), last?.time || 0);
    current.samples.push({x: object.position.x, z: object.position.z, time: now});
    current.samples = current.samples.filter(point => now - point.time <= 160).slice(-10);
  }
  function followPointer(current, event) {
    const object = objects.get(current.id);
    const point = planePoint(event, dragPlane);
    if (!object || !point) return false;
    object.position.x = point.x - current.grabOffset.x;
    object.position.z = point.z - current.grabOffset.z;
    sample(current, event);
    return true;
  }
  function resetDrag() {
    if (!active) return;
    const id = active.id;
    active = null; callbacks.onDragState?.(false); render();
    restorePlay(id, [], callbacks.reducedMotion?.());
  }
  function down(event) {
    if (active || motion || callbacks.inputBlocked?.() || event.button !== 0) return;
    const contact = hitDetail(event), id = contact?.object.userData.pickId; if (!id) return;
    const grabLocal = contact.object.worldToLocal(contact.point.clone());
    cancel();
    const object = objects.get(id);
    setHover(null);
    const lifted = lifts.get(id);
    if (!object || !lifted) return;
    transform(object, lifted);
    object.updateMatrixWorld(true);
    const grabOffset = object.localToWorld(grabLocal.clone()).sub(object.position);
    dragPlane.constant = -(object.position.y + grabOffset.y);
    active = {pointerId:event.pointerId, id, startX:event.clientX, startY:event.clientY,
      moved:false, grabLocal, grabOffset, samples:[]};
    if (!followPointer(active, event)) { active = null; transform(object, bases.get(id)); return; }
    render();
    canvas.setPointerCapture(event.pointerId);
    event.preventDefault();
  }
  function move(event) {
    if (!active) { if (!motion && !callbacks.inputBlocked?.()) setHover(hit(event)); return; }
    if (event.pointerId !== active.pointerId) return;
    if (!active.moved && Math.hypot(event.clientX-active.startX,event.clientY-active.startY) < 8) return;
    active.moved = true;
    if (!followPointer(active, event)) return;
    const valid = legal.has(active.id) && validPoint(planePoint(event));
    callbacks.onDragState?.(valid); render(); event.preventDefault();
  }
  function up(event) {
    if (!active || event.pointerId !== active.pointerId) return;
    const current = active; const point = planePoint(event);
    if (current.moved) followPointer(current, event);
    active = null; callbacks.onDragState?.(false);
    if (canvas.hasPointerCapture(event.pointerId)) canvas.releasePointerCapture(event.pointerId);
    if (current.moved && legal.has(current.id) && validPoint(point)) {
      const accepted = callbacks.onDrop?.(current.id, current.samples);
      if (!accepted) restorePlay(current.id, [], callbacks.reducedMotion?.());
    }
    else {
      const accepted = !current.moved && callbacks.onSelect?.(current.id);
      if (!accepted) restorePlay(current.id, [], callbacks.reducedMotion?.());
    }
    render(); event.preventDefault();
  }
  const interrupted = event => { if (active && event.pointerId === active.pointerId) resetDrag(); };
  canvas.addEventListener('pointerdown',down); canvas.addEventListener('pointermove',move);
  canvas.addEventListener('pointerup',up); canvas.addEventListener('pointercancel',interrupted);
  canvas.addEventListener('lostpointercapture',interrupted); canvas.addEventListener('pointerleave',() => { if (!active) setHover(null); });
  function projectPoint(position) {
    const p = position.project(camera), b = canvas.getBoundingClientRect();
    return {x: b.left + (p.x + 1) * b.width / 2, y: b.top + (1 - p.y) * b.height / 2};
  }
  resize();
  return {apply,animate,animateAll,tap,drop,restorePlay,promotePlay,cancel,resetDrag,
    stats: () => ({objects:objects.size,created:createCount,revision,motion,maxTravel,lastDrop,width:canvas.width,height:canvas.height,dragging:!!active}),
    pose: id => objects.has(id) ? snapshot(objects.get(id)) : null,
    dragPoint: () => active && objects.has(active.id)
      ? projectPoint(objects.get(active.id).localToWorld(active.grabLocal.clone())) : null,
    projectPoint: position => projectPoint(vec(position)),
    project: (id, offset = [0, 0, 0]) => {
      const object = objects.get(id);
      return object ? projectPoint(object.localToWorld(vec(offset))) : null;
    },
    dispose: () => { if(disposed)return;disposed=true;cancel();observer.disconnect();
      canvas.removeEventListener('pointerdown',down);canvas.removeEventListener('pointermove',move);canvas.removeEventListener('pointerup',up);
      canvas.removeEventListener('pointercancel',interrupted);canvas.removeEventListener('lostpointercapture',interrupted);
      if(frame)cancelAnimationFrame(frame);for(const id of [...objects.keys()])remove(id);renderer.dispose(); },
  };
}
