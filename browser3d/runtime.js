// Reconciler and local presentation runtime. Rules and legal cards arrive from Go.
import * as THREE from '/game/vendor/three-0.160.1.module.js';

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
  const objects = new Map(), signatures = new Map(), bases = new Map(), legal = new Set();
  const raycaster = new THREE.Raycaster(), plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), -.35);
  let disposed = false, frame = 0, motion = null, generation = 0, revision = -1, active = null, hover = null;
  let createCount = 0, maxTravel = 0, lastWidth = 0, lastHeight = 0, lastRatio = 0;
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
    world.background = new THREE.Color(description.background);
    camera.fov = description.camera.fov; baseCamera = description.camera.position; targetCamera = description.camera.target;
    // A light fog and felt plane give the shallow cards readable depth.
    world.fog = new THREE.Fog(description.background, 16, 36);
    const next = new Set(description.nodes.map(node => node.id));
    for (const id of objects.keys()) if (!next.has(id)) remove(id);
    for (const node of description.nodes) {
      const signature = JSON.stringify([node.kind, node.size, node.radius, node.material, node.parent, node.pickId]);
      let object = objects.get(node.id);
      if (object && signatures.get(node.id) !== signature) { remove(node.id); object = null; }
      if (!object) {
        object = node.kind === 'group' ? new THREE.Group() : new THREE.Mesh(geometryFor(node), materialFor(node.material, Boolean(node.radius)));
        object.userData.pickId = node.pickId || '';
        if (node.kind !== 'group') { object.castShadow = node.id.startsWith('hand_') || node.id.startsWith('card_'); object.receiveShadow = node.id === 'surface' || node.id === 'dropzone'; }
        (objects.get(node.parent) || world).add(object); objects.set(node.id, object); signatures.set(node.id, signature); createCount++;
      }
      transform(object, node.transform); bases.set(node.id, snapshot(object));
    }
    hover = null; resize(); render();
  }
  function setHover(id) {
    if (hover === id || active) return;
    if (hover && objects.has(hover) && bases.has(hover)) transform(objects.get(hover), bases.get(hover));
    hover = id;
    if (id && objects.has(id)) { objects.get(id).position.y += .16; canvas.style.cursor = 'pointer'; }
    else canvas.style.cursor = 'default';
    render();
  }
  function animate(description) {
    if (description.version !== 1 || !description.steps?.length || !objects.has(description.target)) return Promise.resolve(false);
    cancel(); const token = generation, object = objects.get(description.target), origin = object.position.clone();
    maxTravel = 0; let index = 0, started = performance.now(), from = snapshot(object);
    motion = description.target;
    return new Promise(resolve => {
      const tick = now => {
        if (disposed || token !== generation || !objects.has(description.target)) { resolve(false); return; }
        const step = description.steps[index], t = Math.min(1, (now - started) / step.durationMs), k = ease(step.easing, t);
        for (const field of ['position', 'rotation', 'scale']) {
          const values = from[field].map((v, i) => v + (step.to[field][i] - v) * k);
          if (field === 'rotation') object.rotation.set(...values); else object[field].set(...values);
        }
        maxTravel = Math.max(maxTravel, object.position.distanceTo(origin)); render();
        if (t >= 1) {
          index++; if (index >= description.steps.length) { motion = null; resolve(true); return; }
          from = snapshot(object); started = now;
        }
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
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
  function planePoint(event) { if (!ray(event)) return null; return raycaster.ray.intersectPlane(plane, new THREE.Vector3()); }
  const validPoint = p => !!p && Math.abs(p.x) < 1.55 && Math.abs(p.z) < 1.2;
  function resetDrag() {
    if (!active) return;
    const id = active.id;
    if (objects.has(id) && bases.has(id)) transform(objects.get(id), bases.get(id));
    active = null; callbacks.onDragState?.(false); render();
  }
  function down(event) {
    if (active || callbacks.inputBlocked?.() || event.button !== 0) return;
    const id = hit(event); if (!id) return;
    cancel();
    const point = planePoint(event), object = objects.get(id); if (!point || !object) return;
    setHover(null);
    active = {pointerId:event.pointerId, id, startX:event.clientX, startY:event.clientY, moved:false,
      offsetX:object.position.x-point.x, offsetZ:object.position.z-point.z};
    canvas.setPointerCapture(event.pointerId);
    event.preventDefault();
  }
  function move(event) {
    if (!active) { setHover(hit(event)); return; }
    if (event.pointerId !== active.pointerId) return;
    if (!active.moved && Math.hypot(event.clientX-active.startX,event.clientY-active.startY) < 8) return;
    active.moved = true;
    const point = planePoint(event), object = objects.get(active.id); if (!point || !object) return;
    object.position.set(point.x+active.offsetX,.39,point.z+active.offsetZ);
    object.rotation.x = -.12;
    const valid = legal.has(active.id) && validPoint(point);
    callbacks.onDragState?.(valid); render(); event.preventDefault();
  }
  function up(event) {
    if (!active || event.pointerId !== active.pointerId) return;
    const current = active; const point = planePoint(event);
    active = null; callbacks.onDragState?.(false);
    if (canvas.hasPointerCapture(event.pointerId)) canvas.releasePointerCapture(event.pointerId);
    if (current.moved && legal.has(current.id) && validPoint(point)) {
      callbacks.onDrop?.(current.id);
      // The server may reject a stale command; never leave its card stranded.
      if (objects.has(current.id) && bases.has(current.id)) transform(objects.get(current.id), bases.get(current.id));
    }
    else {
      if (objects.has(current.id) && bases.has(current.id)) transform(objects.get(current.id),bases.get(current.id));
      if (!current.moved) callbacks.onSelect?.(current.id);
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
  return {apply,animate,cancel,resetDrag,
    stats: () => ({objects:objects.size,created:createCount,revision,motion,maxTravel,width:canvas.width,height:canvas.height,dragging:!!active}),
    projectPoint: position => projectPoint(vec(position)),
    project: id => {
      const object = objects.get(id);
      return object ? projectPoint(object.getWorldPosition(new THREE.Vector3())) : null;
    },
    dispose: () => { if(disposed)return;disposed=true;cancel();observer.disconnect();
      canvas.removeEventListener('pointerdown',down);canvas.removeEventListener('pointermove',move);canvas.removeEventListener('pointerup',up);
      canvas.removeEventListener('pointercancel',interrupted);canvas.removeEventListener('lostpointercapture',interrupted);
      if(frame)cancelAnimationFrame(frame);for(const id of [...objects.keys()])remove(id);renderer.dispose(); },
  };
}
