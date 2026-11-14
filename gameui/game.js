// Session and HTMX WebSocket adapter. Game components are rendered in Go.
(() => {
  const live = document.getElementById('live');
  const connection = document.getElementById('connection');
  let room = new URLSearchParams(location.search).get('room') || 'default';
  let session = null;
  let connected = false;
  let lastUpdate = 0;
  let lastSceneUpdate = 0;
  let table = null;
  let runtimePromise = null;
  let selections = new Map();
  let displayedSeats = [];
  let rulesScroll = 0;
  let submittedUpdate = 0;
  let sceneQueue = [];
  let presenting = false;
  let presentationEpoch = 0;
  let restoring = true;
  const seenEvents = new Set();
  const presentationLog = [];
  const tableShell = document.getElementById('table3d');
  const canvas = document.getElementById('table3d-canvas');
  const seatLabels = document.getElementById('table3d-seats');
  const dropHint = document.getElementById('table3d-drop-hint');
  const markPresentation = (kind, id, ok = true) => {
    presentationLog.push({kind, id, ok, at: performance.now()});
    if (presentationLog.length > 40) presentationLog.shift();
  };
  window.__trickPresentation = () => ({pending: sceneQueue.length, presenting, history: presentationLog});

  function positionSeatLabels() {
    if (!table) return;
    const frame = tableShell.getBoundingClientRect();
    displayedSeats.forEach((seat, index) => {
      const label = seatLabels.children[index];
      const point = table.projectPoint(seat.labelPosition);
      const left = label.offsetWidth / 2 + 6, top = label.offsetHeight / 2 + 6;
      label.style.left = `${Math.max(left, Math.min(frame.width - left, point.x - frame.left))}px`;
      label.style.top = `${Math.max(top, Math.min(frame.height - top, point.y - frame.top))}px`;
    });
  }

  function showSeats(seats) {
    displayedSeats = seats;
    seatLabels.replaceChildren(...seats.map(seat => {
      const label = document.createElement('div');
      label.className = seat.turn ? 'table-seat current' : 'table-seat';
      label.title = seat.name;
      const name = document.createElement('strong');
      name.textContent = seat.name;
      const score = document.createElement('span');
      score.textContent = `· ${seat.score} pts`;
      label.setAttribute('aria-label', `${seat.name}, ${seat.score} points${seat.turn ? ', to play' : ''}`);
      label.append(name, score);
      return label;
    }));
    positionSeatLabels();
  }

  function inputBlocked() {
    return !connected || presenting || submittedUpdate === lastSceneUpdate;
  }

  function submit(id) {
    const entry = selections.get(id);
    if (!entry || inputBlocked()) return;
    if (!entry.legal) {
      if (displayedSeats.some(seat => seat.self && seat.turn)) notice('Choose a legal card for this trick.');
      return;
    }
    const screen = document.getElementById('screen');
    if (Number(screen.dataset.update) !== lastSceneUpdate) return;
    const form = [...screen.querySelectorAll('.play-commands form')].find(f => f.elements.card.value === entry.card);
    const button = form?.querySelector('button');
    if (!button || button.disabled) return;
    submittedUpdate = lastSceneUpdate;
    form.requestSubmit(button);
  }
  const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
  async function drainScenes() {
    if (presenting || !table) return;
    presenting = true;
    const epoch = presentationEpoch;
    try {
      while (sceneQueue.length && epoch === presentationEpoch) {
        const payload = sceneQueue.shift();
        const events = (payload.presentations || []).filter(item => !seenEvents.has(item.id));
        events.forEach(item => seenEvents.add(item.id));
        const cards = payload.selections || [];
        selections = new Map(cards.map(item => [item.pickId, item]));
        tableShell.hidden = false;
        if (!events.length || restoring || document.hidden) { table.apply(payload.scene, cards); showSeats(payload.seats || []); restoring = false; continue; }
        table.apply(payload.presentationScene || payload.scene, cards);
        for (const event of events) {
          if (epoch !== presentationEpoch) break;
          if (event.kind === 'card_played') {
            markPresentation('landing', event.id);
            const landed = await Promise.race([table.animate(event.motion), wait(1800).then(() => false)]);
            markPresentation('landed', event.id, landed);
            if (epoch !== presentationEpoch) break;
            // The pause follows the visible landing, including when the next update is queued.
            await wait(event.seat === payload.seats?.find(s => s.self)?.id ? 180 : 420);
            markPresentation('landing-pause-end', event.id);
          } else if (event.kind === 'trick_won') {
            markPresentation('collection-hold', event.id);
            await wait(650);
            if (epoch !== presentationEpoch) break;
            markPresentation('collection-start', event.id);
            if (event.motion?.steps?.length) await Promise.race([table.animate(event.motion), wait(1200)]);
          }
        }
        if (epoch === presentationEpoch) {
          table.apply(payload.scene, cards);
          showSeats(payload.seats || []);
        }
      }
    } finally { if (epoch === presentationEpoch) { presenting = false; if (sceneQueue.length) drainScenes(); } }
  }
  function resetPresentation() {
    presentationEpoch++;
    sceneQueue = [];
    presenting = false;
    restoring = true;
    seenEvents.clear();
    submittedUpdate = 0;
    table?.cancel();
  }
  async function applyScene() {
    const screen = document.querySelector('#screen[data-update]');
    if (!screen) return;
    const update = Number(screen.dataset.update);
    if (!Number.isSafeInteger(update) || update <= lastSceneUpdate) return;
    if (!screen.dataset.scene) {
      rulesScroll = 0;
      showSeats([]);
      lastSceneUpdate = update;
      resetPresentation();
      tableShell.hidden = true;
      selections = new Map();
      return;
    }
    const rules = screen.querySelector('#game-rules');
    if (rules) rules.scrollLeft = rulesScroll;
    try {
      const payload = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(screen.dataset.scene), c => c.charCodeAt(0))));
      if (payload.version !== 1) throw new Error('Unsupported card table payload');
      tableShell.hidden = false;
      if (!table) {
        runtimePromise ||= import('/game/browser3d/runtime.js');
        const {createRuntime} = await runtimePromise;
        if (document.querySelector('#screen[data-update]') !== screen) { applyScene(); return; }
        if (!table) {
          table = createRuntime(canvas, {onSelect: submit, onDrop: submit, inputBlocked, onResize: positionSeatLabels, onDragState: valid => dropHint.classList.toggle('active', valid)});
          window.__trick3d = table; // Read-only browser smoke diagnostics.
        }
      }
      if (update <= lastSceneUpdate) return;
      lastSceneUpdate = update;
      sceneQueue.push(payload);
      drainScenes();
    } catch (error) {
      lastSceneUpdate = update;
      tableShell.hidden = true;
      notice(`3D table unavailable: ${error.message}`);
    }
  }
  new MutationObserver(applyScene).observe(live, {childList: true, subtree: true});
  window.addEventListener('pagehide', () => table?.dispose());

  const key = () => `trick-evolution:${room}`;
  const notice = text => { document.getElementById('notice').textContent = text || ''; };
  const setConnection = online => {
    connected = online;
    connection.textContent = online ? 'Connected' : 'Offline';
    connection.classList.toggle('online', online);
  };

  async function join(name, token = '', bots = 0) {
    try {
      const response = await fetch('/game/join', {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room, name, token, bots})
      });
      if (!response.ok) {
        const message = (await response.text()).trim();
        if (token) sessionStorage.removeItem(key());
        throw new Error(message || `Join failed (${response.status})`);
      }
      session = await response.json();
      sessionStorage.setItem(key(), session.token);
      sessionStorage.setItem(`${key()}:name`, session.name);
      lastUpdate = 0;
      lastSceneUpdate = 0;
      resetPresentation();
      notice('');
      live.setAttribute('hx-ext', 'ws');
      live.setAttribute('ws-connect', `/ws/game?room=${encodeURIComponent(room)}&token=${encodeURIComponent(session.token)}&format=html`);
      htmx.process(live);
    } catch (error) { notice(error.message); }
  }

  document.getElementById('join-form').addEventListener('submit', async event => {
    event.preventDefault();
    const form = event.currentTarget;
    const chosenRoom = form.elements.room.value.trim();
    if (!/^[A-Za-z0-9_-]{1,40}$/.test(chosenRoom)) {
      notice('Use 1–40 letters, numbers, _ or - for the room code.');
      return;
    }
    room = chosenRoom;
    history.replaceState(null, '', `/game?room=${encodeURIComponent(room)}`);
    const submit = form.querySelector('button[type=submit]');
    submit.disabled = true;
    try { await join(form.elements.name.value.trim(), '', Number(form.elements.bots.value)); }
    finally { submit.disabled = false; }
  });

  document.addEventListener('click', event => {
    const action = event.target.closest('[data-action]')?.dataset.action;
    if (action === 'leave') {
      sessionStorage.removeItem(key());
      location.reload();
    } else if (action === 'end-confirm' || action === 'end-cancel') {
      const confirm = document.getElementById('end-confirm');
      if (confirm) confirm.hidden = action !== 'end-confirm';
    }
  });

  document.body.addEventListener('htmx:wsOpen', event => {
    if (event.detail.elt !== live) return;
    setConnection(true);
    notice('');
  });
  document.body.addEventListener('htmx:wsClose', event => {
    if (event.detail.elt !== live) return;
    setConnection(false);
    resetPresentation();
    if (session) notice('Connection lost. Reconnecting…');
  });
  document.body.addEventListener('htmx:wsConfigSend', event => {
    if (!connected) {
      event.preventDefault();
      notice('Waiting for connection…');
      return;
    }
    event.detail.parameters.id = crypto.randomUUID();
  });
  document.body.addEventListener('htmx:wsBeforeMessage', event => {
    const documentFragment = new DOMParser().parseFromString(event.detail.message, 'text/html');
    const screen = documentFragment.querySelector('#screen[data-update]');
    if (!screen) {
      if (documentFragment.querySelector('#notice')) submittedUpdate = 0;
      return;
    }
    const update = Number(screen.dataset.update);
    if (!Number.isSafeInteger(update) || update <= lastUpdate) {
      event.preventDefault();
      return;
    }
    rulesScroll = document.getElementById('game-rules')?.scrollLeft || 0;
    lastUpdate = update;
  });

  const form = document.getElementById('join-form');
  form.elements.name.value = sessionStorage.getItem(`${key()}:name`) || '';
  window.__trickUIReady = true;
  const savedToken = sessionStorage.getItem(key());
  if (savedToken) join(form.elements.name.value || 'Guest', savedToken);
})();
