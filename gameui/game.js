// Session and HTMX WebSocket adapter. Game components are rendered in Go.
(() => {
  const live = document.getElementById('live');
  const connection = document.getElementById('connection');
  let room = new URLSearchParams(location.search).get('room') || 'default';
  let session = null;
  let connected = false;
  let lastUpdate = 0;

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
