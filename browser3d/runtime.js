// Small reconciler for scene v1. No game rules live here.
import * as THREE from '/game/vendor/three-0.160.1.module.js';

const vec = values => new THREE.Vector3(...values);
const applyTransform = (object, value) => {
  object.position.set(...value.position);
  object.rotation.set(...value.rotation);
  object.scale.set(...value.scale);
};
const ease = (kind, t) => kind === 'easeOutCubic' ? 1 - (1 - t) ** 3 :
  kind === 'easeInOutCubic' ? (t < .5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2) : t;

function cardMaterial(material) {
  const side = new THREE.MeshBasicMaterial({color: material.color});
  if (!material.text) return Array(6).fill(side);
  const canvas = document.createElement('canvas');
  canvas.width = 256; canvas.height = 360;
  const context = canvas.getContext('2d');
  context.fillStyle = material.color; context.fillRect(0, 0, 256, 360);
  context.fillStyle = material.ink || '#18251c';
  context.font = 'bold 96px system-ui'; context.textAlign = 'center'; context.textBaseline = 'middle';
  context.fillText(material.text, 128, 180);
  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  const face = new THREE.MeshBasicMaterial({map: texture});
  return [side, side, side, side, face, side];
}

function dispose(object) {
  object.geometry?.dispose();
  const materials = Array.isArray(object.material) ? new Set(object.material) : new Set(object.material ? [object.material] : []);
  for (const material of materials) { material.map?.dispose(); material.dispose(); }
}

export function createRuntime(canvas, onPick) {
  const renderer = new THREE.WebGLRenderer({canvas, antialias: false});
  const world = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(45, 1, .1, 100);
  const objects = new Map();
  const signatures = new Map();
  const raycaster = new THREE.Raycaster();
  let disposed = false, frame = 0, motion = null, generation = 0, revision = -1;
  let createCount = 0, maxTravel = 0;
  const render = () => { if (!disposed && !frame) frame = requestAnimationFrame(() => { frame = 0; renderer.render(world, camera); }); };
  const resize = () => {
    const box = canvas.getBoundingClientRect();
    if (!box.width || !box.height || disposed) return;
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2, 960 / box.width, 600 / box.height));
    renderer.setSize(box.width, box.height, false);
    camera.aspect = box.width / box.height; camera.updateProjectionMatrix(); render();
  };
  const observer = new ResizeObserver(resize);
  observer.observe(canvas);
  function cancel() { generation++; motion = null; }
  function remove(id) {
    const object = objects.get(id);
    if (!object) return;
    object.removeFromParent(); dispose(object); objects.delete(id); signatures.delete(id);
  }
  function apply(description) {
    if (description.version !== 1 || !Array.isArray(description.nodes)) throw new Error('Unsupported 3D scene');
    cancel(); revision = description.revision;
    world.background = new THREE.Color(description.background);
    camera.fov = description.camera.fov;
    camera.position.copy(vec(description.camera.position));
    camera.lookAt(vec(description.camera.target)); camera.updateProjectionMatrix();
    const next = new Set(description.nodes.map(node => node.id));
    for (const id of objects.keys()) if (!next.has(id)) remove(id);
    for (const node of description.nodes) {
      const identity = JSON.stringify([node.kind, node.size, node.material, node.parent]);
      let object = objects.get(node.id);
      if (object && signatures.get(node.id) !== identity) { remove(node.id); object = null; }
      if (!object) {
        object = node.kind === 'group' ? new THREE.Group() :
          new THREE.Mesh(new THREE.BoxGeometry(...node.size), cardMaterial(node.material));
        object.userData.pickId = node.pickId || '';
        (objects.get(node.parent) || world).add(object);
        objects.set(node.id, object); signatures.set(node.id, identity); createCount++;
      }
      applyTransform(object, node.transform);
    }
    resize(); render();
  }
  function animate(description) {
    if (description.version !== 1 || !description.steps?.length || !objects.has(description.target)) return false;
    cancel();
    const token = generation, object = objects.get(description.target);
    const origin = object.position.clone();
    maxTravel = 0;
    let index = 0, started = performance.now();
    let from = {position: object.position.toArray(), rotation: [object.rotation.x, object.rotation.y, object.rotation.z], scale: object.scale.toArray()};
    motion = description.target;
    const tick = now => {
      if (disposed || token !== generation) return;
      const step = description.steps[index];
      const t = Math.min(1, (now - started) / step.durationMs), k = ease(step.easing, t);
      for (const field of ['position', 'rotation', 'scale']) {
        const values = from[field].map((v, i) => v + (step.to[field][i] - v) * k);
        if (field === 'rotation') object.rotation.set(...values); else object[field].set(...values);
      }
      maxTravel = Math.max(maxTravel, object.position.distanceTo(origin));
      render();
      if (t >= 1) {
        index++;
        if (index >= description.steps.length) { motion = null; return; }
        from = {position: object.position.toArray(), rotation: [object.rotation.x, object.rotation.y, object.rotation.z], scale: object.scale.toArray()};
        started = now;
      }
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
    return true;
  }
  const pointer = event => {
    const box = canvas.getBoundingClientRect();
    if (!box.width || !box.height) return;
    const point = new THREE.Vector2((event.clientX - box.left) / box.width * 2 - 1, -(event.clientY - box.top) / box.height * 2 + 1);
    raycaster.setFromCamera(point, camera);
    const hits = raycaster.intersectObjects([...objects.values()].filter(o => o.userData.pickId), false);
    if (hits.length) onPick(hits[0].object.userData.pickId);
  };
  canvas.addEventListener('pointerup', pointer);
  resize();
  return {
    apply, animate, cancel,
    stats: () => ({objects: objects.size, created: createCount, revision, motion, maxTravel, width: canvas.width, height: canvas.height}),
    project: id => { const object = objects.get(id); if (!object) return null; const p = object.getWorldPosition(new THREE.Vector3()).project(camera); const b = canvas.getBoundingClientRect(); return {x: b.left + (p.x + 1) * b.width / 2, y: b.top + (1 - p.y) * b.height / 2}; },
    dispose: () => { if (disposed) return; disposed = true; cancel(); observer.disconnect(); canvas.removeEventListener('pointerup', pointer); if (frame) cancelAnimationFrame(frame); for (const id of [...objects.keys()]) remove(id); renderer.dispose(); },
  };
}
