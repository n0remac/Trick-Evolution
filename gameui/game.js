(() => {
  const screen = document.getElementById('screen');
  const notice = document.getElementById('notice');
  const connection = document.getElementById('connection');
  let room = new URLSearchParams(location.search).get('room') || 'default';
  let session = null;
  let socket = null;
  let revision = -1;
  let reconnectTimer = null;

  const ruleNames = {
    'rule.high_card_wins': 'High Card Wins',
    'rule.low_card_wins': 'Low Card Wins',
    'rule.follow_suit': 'Follow Suit',
    'rule.no_follow_required': 'No Follow Required'
  };

  function key() { return `trick-evolution:${room}`; }
  function element(tag, className, content) {
    const result = document.createElement(tag);
    if (className) result.className = className;
    if (content !== undefined) result.textContent = content;
    return result;
  }
  function setNotice(text) { notice.textContent = text || ''; }
  function setConnection(online) {
    connection.textContent = online ? 'Connected' : 'Offline';
    connection.classList.toggle('online', online);
  }
  function button(label, handler, disabled = false) {
    const result = element('button', '', label);
    result.disabled = disabled;
    result.addEventListener('click', handler);
    return result;
  }

  function showJoin() {
    const panel = element('section', 'panel join');
    panel.append(element('h2', '', 'Join a room'), element('p', 'muted', 'Share the room code with friends, or add computer opponents when creating a room. The first player hosts.'));
    const form = element('form', 'form-grid');
    const roomLabel = element('label', '', 'Room code');
    const roomInput = element('input');
    roomInput.name = 'room'; roomInput.required = true; roomInput.maxLength = 40; roomInput.pattern = '[A-Za-z0-9_-]+'; roomInput.value = room;
    roomLabel.append(roomInput);
    const nameLabel = element('label', '', 'Your name');
    const nameInput = element('input');
    nameInput.name = 'name'; nameInput.required = true; nameInput.maxLength = 32;
    nameInput.value = sessionStorage.getItem(`${key()}:name`) || '';
    nameLabel.append(nameInput);
    const botsLabel = element('label', '', 'Computer opponents (new room)');
    const botsSelect = element('select');
    botsSelect.name = 'bots';
    for (let count = 0; count <= 3; count++) {
      const option = element('option', '', String(count));
      option.value = String(count);
      botsSelect.append(option);
    }
    botsLabel.append(botsSelect);
    const submit = button('Join room', () => {});
    submit.type = 'submit';
    form.append(roomLabel, nameLabel, botsLabel, submit);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      const chosenRoom = roomInput.value.trim();
      if (!/^[A-Za-z0-9_-]{1,40}$/.test(chosenRoom)) { setNotice('Use 1–40 letters, numbers, _ or - for the room code.'); return; }
      room = chosenRoom;
      history.replaceState(null, '', `/game?room=${encodeURIComponent(room)}`);
      submit.disabled = true;
      try { await join(nameInput.value.trim(), '', Number(botsSelect.value)); }
      finally { submit.disabled = false; }
    });
    panel.append(form);
    screen.replaceChildren(panel);
  }

  async function join(name, token = '', bots = 0) {
    try {
      const response = await fetch('/game/join', {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room, name, token, bots})
      });
      if (!response.ok) {
        const message = (await response.text()).trim();
        if (token) { sessionStorage.removeItem(key()); showJoin(); }
        throw new Error(message || `Join failed (${response.status})`);
      }
      session = await response.json();
      sessionStorage.setItem(key(), session.token);
      sessionStorage.setItem(`${key()}:name`, session.name);
      revision = -1;
      setNotice('');
      connect();
    } catch (error) { setNotice(error.message); }
  }

  async function addBot() {
    try {
      const response = await fetch('/game/join', {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room, name: session.name, token: session.token, bots: 1})
      });
      if (!response.ok) throw new Error((await response.text()).trim() || `Add bot failed (${response.status})`);
    } catch (error) { setNotice(error.message); }
  }

  function connect() {
    if (!session || socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) return;
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(`${protocol}//${location.host}/ws/game?room=${encodeURIComponent(room)}&token=${encodeURIComponent(session.token)}`);
    socket.addEventListener('open', () => { setConnection(true); setNotice(''); });
    socket.addEventListener('message', event => {
      let message;
      try { message = JSON.parse(event.data); } catch { return; }
      if (message.type === 'error') { setNotice(message.message || message.code); return; }
      if (message.type === 'ack') return;
      if (message.type === 'lobby') { renderLobby(message); return; }
      if (message.type === 'state' && message.revision >= revision) {
        revision = message.revision;
        renderState(message);
      }
    });
    socket.addEventListener('close', () => {
      setConnection(false);
      if (session) {
        setNotice('Connection lost. Reconnecting…');
        clearTimeout(reconnectTimer);
        reconnectTimer = setTimeout(connect, 1200);
      }
    });
  }

  function send(type, card) {
    if (!socket || socket.readyState !== WebSocket.OPEN) { setNotice('Waiting for connection…'); return; }
    const command = {type, id: crypto.randomUUID()};
    if (card) command.card = card;
    socket.send(JSON.stringify(command));
  }

  function renderLobby(message) {
    const panel = element('section', 'panel');
    panel.append(element('h2', '', `Room ${message.room}`), element('p', 'muted', 'Invite friends with this page URL or add bots. Two to four seats can play.'));
    const scores = element('div', 'scoreboard');
    for (const seat of message.seats || []) {
      const tile = element('div', 'seat');
      tile.append(element('strong', '', seat.name), element('span', 'tag', `${seat.host ? 'Host · ' : ''}${seat.bot ? 'Bot' : seat.online ? 'Online' : 'Offline'}`));
      scores.append(tile);
    }
    panel.append(scores);
    const mine = (message.seats || []).find(seat => seat.id === session.seat);
    if (mine?.host) {
      if (message.seats.length < 4) panel.append(button('Add bot', addBot));
      panel.append(button('Start game', () => send('start'), message.seats.length < 2));
    }
    else panel.append(element('p', 'muted', 'Waiting for the host to start.'));
    screen.replaceChildren(panel);
  }

  function cardLabel(id) {
    const parts = id.split(':');
    const rank = Number(parts[2]);
    const suit = {H:'♥', D:'♦', C:'♣', S:'♠'}[parts[1]] || '?';
    return `${({11:'J',12:'Q',13:'K',14:'A'}[rank] || rank)}${suit}`;
  }
  function cardElement(id, owner, playable) {
    const red = id.includes(':H:') || id.includes(':D:');
    const card = button(cardLabel(id), () => send('play', id), !playable);
    card.className = `card${red ? ' red' : ''}`;
    if (owner) card.append(element('small', '', owner));
    return card;
  }

  function renderState(message) {
    const view = message.view;
    const publicState = view.Public;
    const names = Object.fromEntries((message.seats || []).map(seat => [seat.id, seat.name]));
    const panel = element('section', 'panel');
    const heading = element('div', 'row');
    heading.append(element('h2', '', `Room ${message.room} · Hand ${publicState.HandNumber} of ${publicState.MaxHands}`),
      element('span', 'tag', `Revision ${publicState.Revision}`));
    panel.append(heading);
    const status = publicState.Phase === 'playing_trick'
      ? `${names[publicState.Turn] || 'Player'} to play · Trick ${publicState.TrickNumber}`
      : publicState.Phase === 'hand_complete' ? 'Hand complete' : 'Game complete';
    panel.append(element('p', 'status', status));

    const scoreboard = element('div', 'scoreboard');
    for (const seat of publicState.Seats) {
      const tile = element('div', `seat${seat.ID === publicState.Turn ? ' current' : ''}`);
      tile.append(element('strong', '', names[seat.ID] || seat.ID),
        element('span', '', `${seat.Score} points · ${seat.TricksWon} tricks · ${seat.HandCount} cards`));
      scoreboard.append(tile);
    }
    panel.append(scoreboard);

    const rulesPanel = element('div', 'panel');
    rulesPanel.append(element('h2', '', 'Rules in play'));
    const rules = element('div', 'rules');
    for (const resolved of publicState.Rules) {
      const name = ruleNames[resolved.Instance.DefinitionID] || resolved.Instance.DefinitionID;
      const label = resolved.Active ? name : `${name} · overridden`;
      rules.append(element('span', `rule${resolved.Active ? '' : ' inactive'}`, label));
    }
    rulesPanel.append(rules, element('p', 'muted', `${publicState.RulesRemaining} rule cards left · reveal every ${publicState.RevealEvery} completed hands`));
    panel.append(rulesPanel);

    const table = element('div', 'table');
    if (!publicState.CurrentTrick?.length) table.append(element('span', 'muted', 'The table is clear.'));
    for (const play of publicState.CurrentTrick || []) {
      const card = element('div', 'card', cardLabel(play.Card));
      card.append(element('small', '', names[play.Seat] || play.Seat));
      if (play.Card.includes(':H:') || play.Card.includes(':D:')) card.classList.add('red');
      table.append(card);
    }
    panel.append(table);

    if (publicState.Phase === 'playing_trick') {
      panel.append(element('h2', '', 'Your hand'));
      const hand = element('div', 'hand');
      const canPlay = publicState.Turn === session.seat;
      const activeFollow = (publicState.Rules || []).some(rule => rule.Active && rule.Instance.DefinitionID === 'rule.follow_suit');
      const lead = publicState.CurrentTrick?.[0]?.Card?.split(':')[1];
      const cards = view.Hand || [];
      const haveLead = lead && cards.some(id => id.split(':')[1] === lead);
      for (const id of cards) {
        const allowed = canPlay && (!activeFollow || !haveLead || id.split(':')[1] === lead);
        hand.append(cardElement(id, '', allowed));
      }
      panel.append(hand);
    } else if (publicState.Phase === 'hand_complete' && session.host) {
      panel.append(button('Deal next hand', () => send('next')));
    } else if (publicState.Phase === 'game_complete') {
      const winners = (publicState.Winners || []).map(id => names[id] || id).join(' and ');
      panel.append(element('h2', '', `Winner${publicState.Winners?.length > 1 ? 's' : ''}: ${winners}`));
    }

    const result = [...(message.events || [])].reverse().find(event => event.Kind === 'rule_revealed' || event.Kind === 'trick_won');
    if (result?.Kind === 'rule_revealed') setNotice(`${ruleNames[result.Rule] || result.Rule} revealed after hand ${result.Hand}.`);
    else if (result?.Kind === 'trick_won') setNotice(`${names[result.Seat] || result.Seat} won trick ${result.Trick}.`);
    screen.replaceChildren(panel);
  }

  const savedToken = sessionStorage.getItem(key());
  if (savedToken) join(sessionStorage.getItem(`${key()}:name`) || 'Guest', savedToken);
  else showJoin();
})();
