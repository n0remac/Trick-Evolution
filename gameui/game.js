// Session and HTMX WebSocket adapter. Game components are rendered in Go.
(() => {
  const live = document.getElementById('live');
  const connection = document.getElementById('connection');
  const route = new URLSearchParams(location.search);
  let room = route.get('room') || 'default';
  let session = null;
  let terminal = false;
  let leavePending = false;
  let connected = false;
  let lastUpdate = 0;
  let lastSceneUpdate = 0;
  let table = null;
  let runtimePromise = null;
  let selections = new Map();
  let displayedSeats = [];
  let rulesScrollLeft = 0;
  let submittedUpdate = 0;
  let sceneQueue = [];
  let presenting = false;
  let presentationEpoch = 0;
  let restoring = true;
  let pendingDeal = false;
  const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');
  const seenEvents = new Set();
  const presentationLog = [];
  const tableShell = document.getElementById('table3d');
  const shell = document.querySelector('.shell');
  let measuredHud = null;
  const hudObserver = new ResizeObserver(() => measureTableSpace());
  function measureTableSpace() {
    const hud = document.querySelector('#screen .game-hud');
    if (measuredHud !== hud) {
      if (measuredHud) hudObserver.unobserve(measuredHud);
      measuredHud = hud;
      if (hud) hudObserver.observe(hud);
    }
    if (!hud || tableShell.hidden) return;
    const top = Math.ceil(hud.getBoundingClientRect().bottom - shell.getBoundingClientRect().top + 8);
    shell.style.setProperty('--table-top', `${top}px`);
  }
  new MutationObserver(() => requestAnimationFrame(measureTableSpace)).observe(live, {childList: true, subtree: true});
  window.addEventListener('resize', measureTableSpace);
  const canvas = document.getElementById('table3d-canvas');
  const seatLabels = document.getElementById('table3d-seats');
  const dropHint = document.getElementById('table3d-drop-hint');
  const reviewDialog = document.getElementById('rules-review');
  const rulesButton = document.getElementById('rules-open');
  let reviewMode = 'closed';
  let reviewTab = 'starting';
  let reviewUpdate = 0;
  let reviewFocus = '';
  let reviewScroll = 0;
  let reviewOpener = 'rules-open';
  let autoReviewPending = false;
  let closeAfterStart = false;
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
      score.textContent = `· ${seat.score}`;
      label.setAttribute('aria-label', `${seat.name}, ${seat.scoreLabel || 'score'} ${seat.score}${seat.turn ? ', to play' : ''}`);
      label.append(name, score);
      return label;
    }));
    positionSeatLabels();
  }

  function inputBlocked() {
    return reviewDialog.open || !connected || presenting || submittedUpdate === lastSceneUpdate;
  }

  function submit(id) {
    const entry = selections.get(id);
    if (!entry || inputBlocked()) return;
    if (!entry.legal) {
      if (displayedSeats.some(seat => seat.self && seat.turn)) notice(entry.reason || 'Choose a legal card for this trick.');
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
  function finishPendingDeal() {
    if (!pendingDeal || presenting || sceneQueue.length || !connected) return;
    pendingDeal = false;
    const form = document.querySelector('#screen .next-hand form');
    if (form) form.requestSubmit(form.querySelector('button'));
  }
  async function drainScenes() {
    if (presenting || !table) return;
    presenting = true;
    shell.classList.add('presentation-active');
    const epoch = presentationEpoch;
    try {
      while (sceneQueue.length && epoch === presentationEpoch) {
        const payload = sceneQueue.shift();
        const events = (payload.presentations || []).filter(item => !seenEvents.has(item.id));
        events.forEach(item => seenEvents.add(item.id));
        const cards = payload.selections || [];
        selections = new Map(cards.map(item => [item.pickId, item]));
        tableShell.hidden = false;
        measureTableSpace();
        if (!events.length || restoring || document.hidden || reducedMotion.matches) { table.apply(payload.scene, cards); showSeats(payload.seats || []); restoring = false; continue; }
        table.apply(payload.presentationScene || payload.scene, cards);
        const completesTrick = events.some(event => event.kind === 'trick_won');
        for (const event of events) {
          if (epoch !== presentationEpoch) break;
          if (event.kind === 'card_played') {
            markPresentation('landing', event.id);
            const landed = await Promise.race([table.animate(event.motion), wait(1800).then(() => false)]);
            markPresentation('landed', event.id, landed);
            if (epoch !== presentationEpoch) break;
            // A completed trick uses its shared face-up hold after the final landing.
            if (!completesTrick) await wait(event.seat === payload.seats?.find(s => s.self)?.id ? 180 : 420);
            markPresentation('landing-pause-end', event.id);
          } else if (event.kind === 'trick_won') {
            markPresentation('collection-hold', event.id);
            await wait(520);
            if (epoch !== presentationEpoch) break;
            markPresentation('collection-start', event.id);
            const motions = event.motions || (event.motion?.steps?.length ? [event.motion] : []);
            if (motions.length) await Promise.race([table.animateAll(motions), wait(1400)]);
            if (epoch !== presentationEpoch) break;
            markPresentation('collection-end', event.id);
            const winner = (payload.seats || []).findIndex(seat => seat.id === event.seat);
            seatLabels.children[winner]?.classList.add('winner');
            await wait(320);
          }
        }
        if (epoch === presentationEpoch) {
          table.apply(payload.scene, cards);
          showSeats(payload.seats || []);
        }
      }
    } catch (error) {
      fallbackTable(error);
    } finally { if (epoch === presentationEpoch) { presenting = false; shell.classList.remove('presentation-active'); if (sceneQueue.length && table) drainScenes(); else finishPendingDeal(); } }
  }
  function fallbackTable(error) {
    table?.dispose(); table = null; runtimePromise = null;
    tableShell.hidden = true;
    shell.classList.add('table-fallback');
    document.querySelectorAll('.play-commands').forEach(node => { node.hidden = false; });
    notice(`3D table unavailable: ${error.message}`);
  }
  function resetPresentation() {
    presentationEpoch++;
    sceneQueue = [];
    presenting = false;
    restoring = true;
    seenEvents.clear();
    submittedUpdate = 0;
    pendingDeal = false;
    shell.classList.remove('presentation-active');
    table?.cancel();
  }
  async function applyScene() {
    const screen = document.querySelector('#screen[data-update]');
    if (!screen) return;
    const update = Number(screen.dataset.update);
    if (!Number.isSafeInteger(update) || update <= lastSceneUpdate) return;
    if (!screen.dataset.scene) {
      rulesScrollLeft = 0;
      showSeats([]);
      lastSceneUpdate = update;
      resetPresentation();
      tableShell.hidden = true;
      selections = new Map();
      return;
    }
    const rules = screen.querySelector('#game-rules');
    if (rules) rules.scrollLeft = rulesScrollLeft;
    try {
      const payload = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(screen.dataset.scene), c => c.charCodeAt(0))));
      if (payload.version !== 1) throw new Error('Unsupported card table payload');
      tableShell.hidden = false;
      shell.classList.remove('table-fallback');
      measureTableSpace();
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
      fallbackTable(error);
    }
  }
  new MutationObserver(applyScene).observe(live, {childList: true, subtree: true});
  let cueTimer = 0;
  const announcedResults = new Set();
  function syncCue() {
    const cue = document.querySelector('#screen .match-cue');
    if (!cue || cue.dataset.ready) return;
    cue.dataset.ready = 'true';
    clearTimeout(cueTimer);
    let immediate = cue.dataset.event;
    if (cue.dataset.resultNumber && !announcedResults.has(cue.dataset.resultNumber)) {
      announcedResults.add(cue.dataset.resultNumber);
      immediate = cue.dataset.result;
    }
    cue.textContent = immediate || cue.dataset.next || '';
    if (immediate) cueTimer = setTimeout(() => { if (cue.isConnected) cue.textContent = cue.dataset.next || ''; }, 1400);
  }
  new MutationObserver(syncCue).observe(live, {childList: true, subtree: true});
  let scoresOpen = false;
  live.addEventListener('toggle', event => {
    if (event.target.matches?.('.scores-control')) scoresOpen = event.target.open;
  }, true);
  new MutationObserver(() => {
    const scores = document.querySelector('#screen .scores-control');
    if (scores && scoresOpen) scores.open = true;
  }).observe(live, {childList: true, subtree: true});
  window.addEventListener('pagehide', () => table?.dispose());

  const key = () => `trick-evolution:${room}`;
  const endlessKey = 'trick-evolution:endless';
  const notice = text => { document.getElementById('notice').textContent = text || ''; };
  const setConnection = online => {
    connected = online;
    connection.textContent = online ? 'Connected' : 'Offline';
    connection.classList.toggle('online', online);
  };

  function activateSession(data, fresh = false, endless = false) {
      room = data.room;
      session = data;
      terminal = false;
      autoReviewPending = fresh && !endless;
      sessionStorage.setItem(key(), session.token);
      sessionStorage.setItem(`${key()}:name`, session.name);
      if (endless) sessionStorage.setItem(endlessKey, JSON.stringify(session));
      lastUpdate = 0;
      lastSceneUpdate = 0;
      resetPresentation();
      notice('');
      live.setAttribute('hx-ext', 'ws');
      live.setAttribute('ws-connect', `/ws/game?room=${encodeURIComponent(room)}&token=${encodeURIComponent(session.token)}&format=html`);
      htmx.process(live);
  }

  async function join(name, token = '', bots = 0) {
    try {
      const response = await fetch('/game/join', {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room, name, token, bots})
      });
      if (!response.ok) {
        const message = (await response.text()).trim();
        if (token && response.status === 410) {
          sessionStorage.removeItem(key());
          if (room.startsWith('endless-')) sessionStorage.removeItem(endlessKey);
        }
        throw new Error(message || `Join failed (${response.status})`);
      }
      activateSession(await response.json(), !token);
    } catch (error) { notice(error.message); }
  }

  document.getElementById('endless-form').addEventListener('submit', async event => {
    event.preventDefault();
    const form = event.currentTarget;
    const name = form.elements.name.value.trim();
    if (!name) { form.elements.name.focus(); return; }
    const submit = form.querySelector('button[type=submit]');
    submit.disabled = true;
    try {
      const saved = sessionStorage.getItem(endlessKey);
      if (saved) {
        const prior = JSON.parse(saved);
        room = prior.room;
        history.replaceState(null, '', '/game?endless=1');
        await join(prior.name, prior.token);
        return;
      }
      const response = await fetch('/game/endless', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({name})});
      if (!response.ok) throw new Error((await response.text()).trim() || 'Could not start run.');
      history.replaceState(null, '', '/game?endless=1');
      activateSession(await response.json(), true, true);
    } catch (error) { notice(error.message); }
    finally { submit.disabled = false; }
  });

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

  async function leaveRoom() {
    if (!session || leavePending) return;
    leavePending = true;
    const lobby = document.querySelector('#screen')?.dataset.kind === 'lobby';
    try {
      const response = await fetch('/game/leave', {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room: session.room, seat: session.seat, token: session.token})
      });
      if (!response.ok && response.status !== 410) throw new Error((await response.text()).trim() || 'Could not leave room.');
      sessionStorage.removeItem(key());
      if (room.startsWith('endless-')) sessionStorage.removeItem(endlessKey);
      session = null;
      terminal = true;
      live.removeAttribute('ws-connect');
      if (lobby || response.status === 410) location.reload();
    } catch (error) { notice(error.message); }
    finally { leavePending = false; }
  }

  const ruleDetails = document.getElementById('rule-details');
  ruleDetails.addEventListener('click', event => {
    if (event.target === ruleDetails) ruleDetails.close();
  });
  ruleDetails.addEventListener('close', () => {
    const card = [...document.querySelectorAll('#game-rules [data-rule-instance]')]
      .find(node => node.dataset.ruleInstance === ruleDetails.dataset.instance);
    (card || rulesButton).focus();
  });
  live.addEventListener('click', event => {
    const rule = event.target.closest('.rule');
    if (!rule) return;
    ruleDetails.dataset.instance = rule.dataset.ruleInstance;
    ruleDetails.dataset.title = rule.dataset.ruleTitle;
    ruleDetails.querySelector('#rule-title').textContent = rule.dataset.ruleTitle;
    ruleDetails.querySelector('#rule-text').textContent = rule.dataset.ruleDetail;
    ruleDetails.showModal();
  });
  function syncRuleDetails() {
    if (!ruleDetails.open) return;
    const rule = [...document.querySelectorAll('#game-rules [data-rule-instance]')]
      .find(card => card.dataset.ruleInstance === ruleDetails.dataset.instance);
    ruleDetails.querySelector('#rule-title').textContent = rule?.dataset.ruleTitle || `${ruleDetails.dataset.title} · no longer active`;
    ruleDetails.querySelector('#rule-text').textContent = rule?.dataset.ruleDetail ||
      'This field card is no longer active. Open Rules, then Changes, to see what replaced it.';
  }
  new MutationObserver(syncRuleDetails).observe(live, {childList: true, subtree: true});

  function setReviewTab(tab, moveFocus = false) {
    if (!['current', 'starting', 'changes'].includes(tab)) return;
    reviewTab = tab;
    const content = document.getElementById('rules-review-content');
    if (!content) return;
    content.querySelectorAll('[data-review-tab]').forEach(button => {
      const active = button.dataset.reviewTab === tab;
      button.setAttribute('aria-selected', String(active));
      button.tabIndex = active ? 0 : -1;
      if (active && moveFocus) button.focus();
    });
    content.querySelectorAll('[data-review-panel]').forEach(panel => { panel.hidden = panel.dataset.reviewPanel !== tab; });
  }

  function syncReview() {
    const content = document.getElementById('rules-review-content');
    const update = Number(content?.dataset.update);
    if (!Number.isSafeInteger(update) || update <= reviewUpdate) return;
    reviewUpdate = update;
    rulesButton.hidden = false;
    if (autoReviewPending) {
      autoReviewPending = false;
      if (content.dataset.kind === 'lobby') openReview('auto', 'starting', 'rules-open');
    }
    if (reviewDialog.open) {
      if (closeAfterStart && content.dataset.kind === 'state') {
        closeAfterStart = false;
        reviewDialog.close();
        return;
      }
      if (content.dataset.kind !== 'lobby' && reviewMode === 'start') reviewMode = 'manual';
      setReviewTab(reviewTab);
      content.scrollTop = reviewScroll;
      const target = content.querySelector(`[data-review-focus="${reviewFocus}"]`);
      requestAnimationFrame(() => {
        if (reviewDialog.open && !content.contains(document.activeElement)) (target || reviewDialog.querySelector('.review-header button')).focus();
      });
    }
    const startWrap = content.querySelector('#review-start-wrap');
    if (startWrap) startWrap.hidden = reviewMode !== 'start' || content.dataset.kind !== 'lobby';
  }
  new MutationObserver(syncReview).observe(live, {childList: true, subtree: true});

  function openReview(mode, tab, opener) {
    const content = document.getElementById('rules-review-content');
    if (!content?.dataset.update) return;
    reviewMode = mode;
    reviewOpener = opener;
    reviewFocus = '';
    reviewScroll = 0;
    setReviewTab(tab);
    const startWrap = content.querySelector('#review-start-wrap');
    if (startWrap) startWrap.hidden = mode !== 'start' || content.dataset.kind !== 'lobby';
    if (!reviewDialog.open) reviewDialog.showModal();
    const primary = mode === 'start' ? content.querySelector('#review-start-form button:not(:disabled)') : null;
    (primary || content.querySelector(`[data-review-tab="${tab}"]`) || reviewDialog.querySelector('.review-header button')).focus();
  }
  reviewDialog.addEventListener('close', () => {
    reviewMode = 'closed';
    closeAfterStart = false;
    const focusTarget = reviewOpener === 'review-start' ? document.querySelector('[data-action="review-start"]:not(:disabled)') : rulesButton;
    (focusTarget || rulesButton).focus();
  });
  reviewDialog.addEventListener('click', event => { if (event.target === reviewDialog) reviewDialog.close(); });
  reviewDialog.addEventListener('focusin', event => { reviewFocus = event.target.dataset.reviewFocus || ''; });
  reviewDialog.addEventListener('keydown', event => {
    const current = event.target.closest('[data-review-tab]');
    if (!current || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    const tabs = [...reviewDialog.querySelectorAll('[data-review-tab]')];
    let index = tabs.indexOf(current);
    if (event.key === 'Home') index = 0;
    else if (event.key === 'End') index = tabs.length - 1;
    else index = (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
    setReviewTab(tabs[index].dataset.reviewTab, true);
    event.preventDefault();
  });
  reviewDialog.addEventListener('submit', event => {
    if (event.target.id === 'review-start-form') closeAfterStart = true;
  });
  reviewDialog.addEventListener('input', event => {
    if (!event.target.closest('#review-settings-form')) return;
    const feedback = document.getElementById('review-settings-feedback');
    if (feedback) feedback.textContent = 'Unsaved changes. Save game settings before starting.';
    const start = document.querySelector('#review-start-form button');
    if (start) start.disabled = true;
  });

  document.addEventListener('click', event => {
    const action = event.target.closest('[data-action]')?.dataset.action;
    if (action === 'review-start') {
      openReview('start', 'starting', 'review-start');
    } else if (action === 'review-close') {
      reviewDialog.close();
    } else if (action === 'return-to-join') {
      sessionStorage.removeItem(key());
      if (room.startsWith('endless-')) {
        sessionStorage.removeItem(endlessKey);
        location.href = '/game';
      } else {
        location.reload();
      }
    } else if (action === 'leave-prompt' || action === 'leave-cancel') {
      const confirm = document.getElementById('leave-confirm');
      if (confirm) confirm.hidden = action !== 'leave-prompt';
    } else if (action === 'leave-room') {
      leaveRoom();
    } else if (action === 'end-confirm' || action === 'end-cancel') {
      const confirm = document.getElementById('end-confirm');
      if (confirm) confirm.hidden = action !== 'end-confirm';
    }
  });
  rulesButton.addEventListener('click', () => {
    const kind = document.getElementById('rules-review-content')?.dataset.kind;
    openReview('manual', kind === 'lobby' ? 'starting' : 'current', 'rules-open');
  });
  reviewDialog.addEventListener('click', event => {
    const tab = event.target.closest('[data-review-tab]')?.dataset.reviewTab;
    if (tab) setReviewTab(tab, true);
  });

  document.body.addEventListener('htmx:wsOpen', event => {
    if (event.detail.elt !== live) return;
    setConnection(true);
    notice('');
  });
  document.body.addEventListener('htmx:wsClose', event => {
    if (event.detail.elt !== live) return;
    setConnection(false);
    if (terminal) return;
    resetPresentation();
    if (session) notice('Connection lost. Reconnecting…');
  });
  document.body.addEventListener('htmx:wsConfigSend', event => {
    if (terminal || leavePending) { event.preventDefault(); return; }
    if (!connected) {
      event.preventDefault();
      notice('Waiting for connection…');
      return;
    }
    if (event.target.closest?.('.next-hand') && (presenting || sceneQueue.length)) {
      event.preventDefault();
      pendingDeal = true;
      return;
    }
    if (event.target.id === 'review-settings-form') {
      const form = event.target;
      if (!form.reportValidity()) { event.preventDefault(); return; }
      const handSize = form.querySelector('[data-setting="hand-size"]').valueAsNumber;
      const revealEvery = form.querySelector('[data-setting="reveal-every"]').valueAsNumber;
      if (!Number.isInteger(handSize) || !Number.isInteger(revealEvery)) {
        event.preventDefault();
        const feedback = document.getElementById('review-settings-feedback');
        if (feedback) feedback.textContent = 'Enter whole numbers for cards and reveal interval.';
        return;
      }
      event.detail.parameters.settings = JSON.stringify({
        handSize, revealEvery,
        initialRules: Array.from(form.querySelectorAll('[data-initial-rule]:checked'), input => input.value)
      });
      const feedback = document.getElementById('review-settings-feedback');
      if (feedback) feedback.textContent = 'Saving settings…';
    }
    event.detail.parameters.id = crypto.randomUUID();
  });
  document.body.addEventListener('htmx:wsBeforeMessage', event => {
    const documentFragment = new DOMParser().parseFromString(event.detail.message, 'text/html');
    const closure = documentFragment.querySelector('#room-closed[data-terminal]');
    if (closure) {
      terminal = true;
      sessionStorage.removeItem(key());
      if (room.startsWith('endless-')) sessionStorage.removeItem(endlessKey);
      session = null;
      live.removeAttribute('ws-connect');
      setConnection(false);
      notice(closure.dataset.message || 'Room closed.');
    }
    const errorText = documentFragment.querySelector('#notice')?.textContent?.trim();
    if (errorText && reviewDialog.open) {
      const feedback = document.getElementById('review-settings-feedback');
      if (feedback) feedback.textContent = errorText;
    }
    const screen = documentFragment.querySelector('#screen[data-update]');
    if (!screen) {
      if (documentFragment.querySelector('#notice')) { submittedUpdate = 0; closeAfterStart = false; }
      return;
    }
    const update = Number(screen.dataset.update);
    if (!Number.isSafeInteger(update) || update <= lastUpdate) {
      event.preventDefault();
      return;
    }
    rulesScrollLeft = document.getElementById('game-rules')?.scrollLeft || 0;
    if (reviewDialog.open) {
      reviewScroll = document.getElementById('rules-review-content')?.scrollTop || 0;
      reviewFocus = document.activeElement?.dataset.reviewFocus || reviewFocus;
    }
    lastUpdate = update;
  });

  const form = document.getElementById('join-form');
  const endlessForm = document.getElementById('endless-form');
  form.elements.name.value = sessionStorage.getItem(`${key()}:name`) || '';
  endlessForm.elements.name.value = form.elements.name.value;
  for (const [source, target] of [[form.elements.name, endlessForm.elements.name], [endlessForm.elements.name, form.elements.name]]) {
    source.addEventListener('input', () => { target.value = source.value; });
  }
  window.__trickUIReady = true;
  const savedEndless = sessionStorage.getItem(endlessKey);
  if (route.get('endless') === '1' && !route.has('room') && savedEndless) {
    try {
      const prior = JSON.parse(savedEndless);
      room = prior.room;
      join(prior.name, prior.token);
    } catch (_) { sessionStorage.removeItem(endlessKey); }
  } else {
    const savedToken = sessionStorage.getItem(key());
    if (savedToken) join(form.elements.name.value || 'Guest', savedToken);
  }
})();
