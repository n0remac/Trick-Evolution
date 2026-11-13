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
  let selected = null;
  const tableShell = document.getElementById('table3d');
  const canvas = document.getElementById('table3d-canvas');
  const play3d = document.getElementById('table3d-play');
  const selectionLabel = document.getElementById('table3d-selection');

  function clearSelection() {
    selected = null;
    play3d.disabled = true;
    selectionLabel.textContent = 'Select a card on the table.';
  }
  function picked(id) {
    const entry = selections.get(id);
    if (!entry) return;
    selected = entry;
    table.animate(entry.animation);
    const form = [...document.querySelectorAll('.hand form')].find(f => f.querySelector('input[name=card]')?.value === entry.card);
    play3d.disabled = !connected || !entry.legal || !form || !!form.querySelector('button')?.disabled;
    selectionLabel.textContent = entry.legal ? `${entry.card.replaceAll(':', ' ')} selected.` : `${entry.card.replaceAll(':', ' ')} selected. Wait for your legal turn.`;
  }
  async function applyScene() {
    const screen = document.querySelector('#screen[data-update]');
    if (!screen) return;
    const update = Number(screen.dataset.update);
    if (!Number.isSafeInteger(update) || update <= lastSceneUpdate) return;
    if (!screen.dataset.scene) {
      lastSceneUpdate = update;
      clearSelection();
      table?.cancel();
      tableShell.hidden = true;
      selections = new Map();
      return;
    }
    try {
      const payload = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(screen.dataset.scene), c => c.charCodeAt(0))));
      if (payload.version !== 1) throw new Error('Unsupported card table payload');
      if (!table) {
        runtimePromise ||= import('/game/browser3d/runtime.js');
        const {createRuntime} = await runtimePromise;
        if (document.querySelector('#screen[data-update]') !== screen) { applyScene(); return; }
        if (!table) {
          table = createRuntime(canvas, picked);
          window.__trick3d = table; // Read-only browser smoke diagnostics.
        }
      }
      if (update <= lastSceneUpdate) return;
      lastSceneUpdate = update;
      clearSelection();
      selections = new Map(payload.selections.map(item => [item.pickId, item]));
      tableShell.hidden = false;
      table.apply(payload.scene);
    } catch (error) {
      lastSceneUpdate = update;
      tableShell.hidden = true;
      notice(`3D table unavailable: ${error.message}`);
    }
  }
  new MutationObserver(applyScene).observe(live, {childList: true, subtree: true});
  play3d.addEventListener('click', () => {
    if (!selected || !selected.legal || !connected) return;
    const form = [...document.querySelectorAll('.hand form')].find(f => f.querySelector('input[name=card]')?.value === selected.card);
    const button = form?.querySelector('button');
    if (!button || button.disabled) { clearSelection(); return; }
    button.click(); // Existing authenticated HTMX action carries card, revision and fresh ID.
    play3d.disabled = true;
  });
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
      table?.cancel();
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
    table?.cancel();
    play3d.disabled = true;
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
    if (!screen) return; // Errors update only the notice.
    const update = Number(screen.dataset.update);
    if (!Number.isSafeInteger(update) || update <= lastUpdate) {
      event.preventDefault();
      return;
    }
    lastUpdate = update;
  });

  const form = document.getElementById('join-form');
  form.elements.name.value = sessionStorage.getItem(`${key()}:name`) || '';
  window.__trickUIReady = true;
  const savedToken = sessionStorage.getItem(key());
  if (savedToken) join(form.elements.name.value || 'Guest', savedToken);
})();
