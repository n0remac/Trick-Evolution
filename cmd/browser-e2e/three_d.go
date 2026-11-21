package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func (r *runner) threeDSmoke() error {
	page := r.alice
	// Opt in to a delayed WebSocket confirmation while the local drop is moving.
	latencyCheck := os.Getenv("TRICK_3D_LATENCY_SMOKE") == "1"
	const delayedMessages = `(() => {
		const original = WebSocket.prototype.addEventListener;
		WebSocket.prototype.addEventListener = function(type, listener, options) {
			if (type !== 'message') return original.call(this, type, listener, options);
			const socket = this;
			return original.call(this, type, event => {
				const delay = window.__testMessageDelayMs || 0;
				if (delay) setTimeout(() => listener.call(socket, event), delay);
				else listener.call(socket, event);
			}, options);
		};
	})()`
	if latencyCheck {
		if err := page.cdp.call(r.ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": delayedMessages}, nil); err != nil {
			return err
		}
	}
	if err := page.cdp.call(r.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 390, "height": 844, "deviceScaleFactor": 2, "mobile": true}, nil); err != nil {
		return err
	}
	if err := page.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if err := r.join(page, "Alice"); err != nil {
		return err
	}
	if err := r.closeReview(page); err != nil {
		return err
	}
	if err := page.click(r.ctx, "Add bot"); err != nil {
		return err
	}
	if err := r.waitPage(page, "3D bot lobby", 15*time.Second, func(s uiState) bool { return s.SeatTiles == 2 }); err != nil {
		return err
	}
	if err := r.startFromLobby(page); err != nil {
		return err
	}
	if err := r.waitPage(page, "3D first deal and Alice turn", 20*time.Second, func(s uiState) bool {
		return s.HandCards == 5 && s.Revision >= 1 && strings.HasPrefix(s.Status, "Alice to play")
	}); err != nil {
		return err
	}
	if err := r.closeReview(page); err != nil {
		return err
	}
	var ready bool
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, `!!window.__trick3d && window.__trick3d.stats().objects >= 10 && window.__trick3d.stats().width > 0`, &ready); err != nil {
			return err
		}
		if ready {
			break
		}
	}
	if !ready {
		return fmt.Errorf("phone canvas did not render table and private cards")
	}
	var settled bool
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, `window.__trick3d.stats().revision === Number(document.querySelector('#screen').dataset.revision) && !window.__trick3d.stats().motion && !window.__trickPresentation().presenting`, &settled); err != nil {
			return err
		}
		if settled {
			break
		}
	}
	if !settled {
		return fmt.Errorf("initial bot landing did not settle")
	}
	var framed bool
	const framingJS = `(() => { const b = document.querySelector('#table3d-canvas').getBoundingClientRect(); const payload = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(document.querySelector('#screen').dataset.scene), c => c.charCodeAt(0)))); const points = payload.selections.map(s => window.__trick3d.project(s.pickId)); return b.height >= 120 && points.length === 5 && points.every(p => p && p.x > b.left + 40 && p.x < b.right - 40 && p.y > b.top + b.height * .55 && p.y < b.bottom - b.height * .08) && document.getElementById('end-confirm').getClientRects().length === 0 && !document.querySelector('#table3d-play, .hand-fallback') && document.querySelectorAll('.table-seat').length === 2 && document.querySelector('.table-seat.current').textContent.includes('Alice') && document.querySelector('.end-game').getBoundingClientRect().right <= document.getElementById('rules-open').getBoundingClientRect().left && document.querySelector('#game-rules .rule').getBoundingClientRect().height > 0 && !document.querySelector('details#game-rules') && document.querySelector('.match-cue').dataset.next.startsWith('Your turn') && document.querySelector('.match-cue').dataset.event.includes('played') && payload.scene.nodes.filter(n => n.pickId).every(n => n.transform.rotation[0] > .5); })()`
	if err := page.eval(r.ctx, framingJS, &framed); err != nil {
		return err
	}
	if !framed {
		return fmt.Errorf("portrait hand framing or hidden confirmation failed")
	}
	var ruleOpened bool
	if err := page.eval(r.ctx, `(() => {
		const rule = document.querySelector('button.rule');
		rule.click();
		const dialog = document.querySelector('#rule-details');
		const open = dialog.open && !!dialog.querySelector('#rule-text').textContent;
		dialog.close();
		return open;
	})()`, &ruleOpened); err != nil {
		return err
	}
	if !ruleOpened {
		return fmt.Errorf("rule card did not open its details")
	}

	var initialWidth int
	if err := page.eval(r.ctx, `window.__trick3d.stats().width`, &initialWidth); err != nil {
		return err
	}
	var initialRevision int
	if err := page.eval(r.ctx, `window.__trick3d.stats().revision`, &initialRevision); err != nil {
		return err
	}
	if err := page.eval(r.ctx, `window.__trick3dInitial = window.__trick3d; true`, nil); err != nil {
		return err
	}
	var target struct {
		ID string  `json:"id"`
		X  float64 `json:"x"`
		Y  float64 `json:"y"`
	}
	const targetJS = `(() => { const bytes = Uint8Array.from(atob(document.querySelector('#screen').dataset.scene), c => c.charCodeAt(0)); const payload = JSON.parse(new TextDecoder().decode(bytes)); const card = payload.selections.find(s => s.legal); if (!card) return null; const point = window.__trick3d.project(card.pickId, [-.36, .02, -.46]); return {id: card.pickId, x: point.x, y: point.y}; })()`
	if err := page.eval(r.ctx, targetJS, &target); err != nil {
		return err
	}
	if target.ID == "" {
		return fmt.Errorf("no legal visible 3D card")
	}
	// Release outside the legal center. The local hand and revision must stay put.
	if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": target.X, "y": target.Y, "button": "left", "clickCount": 1}, nil); err != nil {
		return err
	}
	if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": target.X, "y": target.Y + 35, "button": "left", "buttons": 1}, nil); err != nil {
		return err
	}
	var grabAligned bool
	if err := page.eval(r.ctx, fmt.Sprintf(`(() => { const point = window.__trick3d.dragPoint(); return point && Math.hypot(point.x - %f, point.y - %f) < 2; })()`, target.X, target.Y+35), &grabAligned); err != nil {
		return err
	}
	if !grabAligned {
		return fmt.Errorf("dragged card lost the grabbed point")
	}
	if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": target.X, "y": target.Y + 35, "button": "left", "clickCount": 1}, nil); err != nil {
		return err
	}
	var rejected bool
	if err := page.eval(r.ctx, fmt.Sprintf(`window.__trick3d.stats().revision === %d && !window.__trick3d.stats().dragging`, initialRevision), &rejected); err != nil {
		return err
	}
	if !rejected {
		return fmt.Errorf("invalid drag did not return to hand")
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		if err := page.eval(r.ctx, `!window.__trick3d.stats().motion`, &rejected); err != nil {
			return err
		}
		if rejected {
			break
		}
	}
	if !rejected {
		return fmt.Errorf("invalid drag return did not settle")
	}
	// A finger leaving the gesture cancels without submitting a play.
	for _, event := range []struct {
		Kind   string
		Points []map[string]any
	}{
		{"touchStart", []map[string]any{{"x": target.X, "y": target.Y, "id": 1}}},
		{"touchMove", []map[string]any{{"x": target.X, "y": target.Y - 45, "id": 1}}},
		{"touchCancel", []map[string]any{}},
	} {
		if err := page.cdp.call(r.ctx, "Input.dispatchTouchEvent", map[string]any{"type": event.Kind, "touchPoints": event.Points}, nil); err != nil {
			return err
		}
	}
	var touchReturned bool
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		if err := page.eval(r.ctx, fmt.Sprintf(`!window.__trick3d.stats().dragging && !window.__trick3d.stats().motion && window.__trick3d.stats().revision === %d`, initialRevision), &touchReturned); err != nil {
			return err
		}
		if touchReturned {
			break
		}
	}
	if !touchReturned {
		return fmt.Errorf("canceled finger drag did not return to hand")
	}
	// A stale command must return the tapped card to its hand without consuming a turn.
	if err := page.eval(r.ctx, `document.querySelectorAll('.play-commands input[name="revision"]').forEach(input => input.value = '0'); true`, nil); err != nil {
		return err
	}
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": kind, "x": target.X, "y": target.Y, "button": "left", "clickCount": 1}, nil); err != nil {
			return err
		}
	}
	var staleStarted bool
	if err := page.eval(r.ctx, `!!window.__trick3d.stats().motion`, &staleStarted); err != nil {
		return err
	}
	if !staleStarted {
		return fmt.Errorf("rejected tap never started its local flight")
	}
	var staleRejected bool
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		if err := page.eval(r.ctx, fmt.Sprintf(`document.querySelector('#notice')?.textContent.includes('Game changed') && Number(document.querySelector('#screen').dataset.revision) === %d && document.querySelectorAll('.play-commands form').length > 0`, initialRevision), &staleRejected); err != nil {
			return err
		}
		if staleRejected {
			break
		}
	}
	if !staleRejected {
		return fmt.Errorf("stale tap did not show rejection feedback")
	}
	var returned bool
	for deadline := time.Now().Add(12 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		if err := page.eval(r.ctx, fmt.Sprintf(`!!window.__trick3d.project(%q) && !window.__trick3d.stats().motion`, target.ID), &returned); err != nil {
			return err
		}
		if returned {
			break
		}
	}
	if !returned {
		var stats any
		_ = page.eval(r.ctx, `window.__trick3d.stats()`, &stats)
		return fmt.Errorf("rejected tap did not settle back in hand: %v", stats)
	}
	if err := page.eval(r.ctx, fmt.Sprintf(`document.querySelectorAll('.play-commands input[name="revision"]').forEach(input => input.value = %q); true`, fmt.Sprint(initialRevision)), nil); err != nil {
		return err
	}
	// A click on the card itself plays it after the invalid drag.
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": kind, "x": target.X, "y": target.Y, "button": "left", "clickCount": 1}, nil); err != nil {
			return err
		}
	}
	// A second tap while the first play is in flight must not submit another card.
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": kind, "x": target.X, "y": target.Y, "button": "left", "clickCount": 1}, nil); err != nil {
			return err
		}
	}
	if err := r.waitPage(page, "accepted 3D play and bot reply", 20*time.Second, func(s uiState) bool { return s.Revision >= 3 && s.HandCards == 4 }); err != nil {
		return err
	}
	var sameRenderer bool
	if err := page.eval(r.ctx, `window.__trick3d === window.__trick3dInitial && window.__trick3d.stats().objects >= 10`, &sameRenderer); err != nil {
		return err
	}
	if !sameRenderer {
		return fmt.Errorf("HTMX update recreated renderer or lost table")
	}
	// A legal pointer release submits through the same Go form, then the bot completes the trick.
	target = struct {
		ID string  `json:"id"`
		X  float64 `json:"x"`
		Y  float64 `json:"y"`
	}{}
	for deadline := time.Now().Add(12 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, targetJS, &target); err != nil {
			return err
		}
		var idle bool
		if err := page.eval(r.ctx, `window.__trick3d.stats().revision === Number(document.querySelector('#screen').dataset.revision) && !window.__trick3d.stats().motion && !window.__trickPresentation().presenting`, &idle); err != nil {
			return err
		}
		if target.ID != "" && target.X != 0 && idle {
			break
		}
		target.ID = ""
	}
	var center struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := page.eval(r.ctx, `window.__trick3d.project('dropzone')`, &center); err != nil {
		return err
	}
	if target.ID == "" || center.X == 0 {
		return fmt.Errorf("missing legal card or center drop target")
	}
	// Pause over the field before releasing a stale play. This should carry
	// almost no release momentum, then return smoothly when Go rejects it.
	if err := page.eval(r.ctx, `document.querySelectorAll('.play-commands input[name="revision"]').forEach(input => input.value = '0'); document.querySelector('#notice').textContent = ''; true`, nil); err != nil {
		return err
	}
	for _, event := range []struct {
		Kind    string
		X, Y    float64
		Buttons int
	}{
		{"mousePressed", target.X, target.Y, 1}, {"mouseMoved", center.X, center.Y, 1},
	} {
		if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": event.Kind, "x": event.X, "y": event.Y, "button": "left", "buttons": event.Buttons, "clickCount": 1}, nil); err != nil {
			return err
		}
	}
	time.Sleep(180 * time.Millisecond)
	if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": center.X, "y": center.Y, "button": "left", "buttons": 0, "clickCount": 1}, nil); err != nil {
		return err
	}
	var slowDrop bool
	if err := page.eval(r.ctx, `window.__trick3d.stats().lastDrop?.speed < .5`, &slowDrop); err != nil {
		return err
	}
	if !slowDrop {
		return fmt.Errorf("paused field drop kept fast release momentum")
	}
	var rejectedDrop bool
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		if err := page.eval(r.ctx, fmt.Sprintf(`document.querySelector('#notice')?.textContent.includes('Game changed') && !window.__trick3d.stats().motion && !!window.__trick3d.project(%q)`, target.ID), &rejectedDrop); err != nil {
			return err
		}
		if rejectedDrop {
			break
		}
	}
	if !rejectedDrop {
		return fmt.Errorf("rejected field drop did not return to the hand")
	}
	if err := page.eval(r.ctx, `document.querySelectorAll('.play-commands input[name="revision"]').forEach(input => input.value = document.querySelector('#screen').dataset.revision); true`, nil); err != nil {
		return err
	}
	var edge struct{ X, Y float64 }
	if err := page.eval(r.ctx, `window.__trick3d.projectPoint([1.35, .35, 0])`, &edge); err != nil {
		return err
	}
	if latencyCheck {
		if err := page.eval(r.ctx, fmt.Sprintf(`(() => {
		window.__testMessageDelayMs = 1300;
		window.__latencyProof = false;
		const revision = Number(document.querySelector('#screen').dataset.revision);
		const card = %q;
		function observe() {
			if (Number(document.querySelector('#screen').dataset.revision) !== revision) return;
			const stats = window.__trick3d.stats();
			if (stats.motion === 'drop:' + card && stats.lastDrop?.contact && window.__trick3d.pose(card)) window.__latencyProof = true;
			if (!window.__latencyProof) requestAnimationFrame(observe);
		}
		requestAnimationFrame(observe);
		return true;
	})()`, target.ID), nil); err != nil {
			return err
		}
	}
	for _, event := range []struct {
		Kind    string
		X, Y    float64
		Buttons int
	}{
		{"mousePressed", target.X, target.Y, 1}, {"mouseMoved", edge.X, edge.Y, 1}, {"mouseReleased", edge.X, edge.Y, 0},
	} {
		if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": event.Kind, "x": event.X, "y": event.Y, "button": "left", "buttons": event.Buttons, "clickCount": 1}, nil); err != nil {
			return err
		}
	}
	if err := r.waitPage(page, "valid drag and complete trick", 20*time.Second, func(s uiState) bool { return s.Revision >= 5 && s.HandCards == 3 }); err != nil {
		return err
	}
	if latencyCheck {
		var latencyProof bool
		if err := page.eval(r.ctx, `window.__testMessageDelayMs = 0; window.__latencyProof === true`, &latencyProof); err != nil {
			return err
		}
		if !latencyProof {
			return fmt.Errorf("drop did not keep moving through delayed GoDom confirmation")
		}
	}
	var fastDrop bool
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		if err := page.eval(r.ctx, `(() => { const drop = window.__trick3d.stats().lastDrop; return drop && drop.contact && drop.done && (drop.speed <= .5 || drop.contactTilt > .1 || drop.maxTilt > .1); })()`, &fastDrop); err != nil {
			return err
		}
		if fastDrop {
			break
		}
	}
	if !fastDrop {
		var drop any
		_ = page.eval(r.ctx, `window.__trick3d.stats().lastDrop`, &drop)
		return fmt.Errorf("edge drop did not respond to its release speed and settle: %v", drop)
	}
	var pacing bool
	const pacingJS = `(() => { const log = window.__trickPresentation().history; const by = (kind,id) => log.find(x => x.kind === kind && x.id === id); return log.some(x => x.kind === 'landed' && x.ok && by('landing-pause-end',x.id)?.at - x.at >= 350) && log.some(x => x.kind === 'collection-hold' && by('collection-start',x.id)?.at - x.at >= 480 && by('collection-end',x.id)?.at > by('collection-start',x.id)?.at); })()`
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, pacingJS, &pacing); err != nil {
			return err
		}
		if pacing {
			break
		}
	}
	if !pacing {
		return fmt.Errorf("bot landing pause or trick collection hold was not observed")
	}
	if err := page.cdp.call(r.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 320, "height": 700, "deviceScaleFactor": 3, "mobile": true}, nil); err != nil {
		return err
	}
	var resized bool
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, fmt.Sprintf(`window.__trick3d.stats().width <= 960 && window.__trick3d.stats().height <= 1400 && window.__trick3d.stats().width > 0 && window.__trick3d.stats().width !== %d`, initialWidth), &resized); err != nil {
			return err
		}
		if resized {
			break
		}
	}
	if !resized {
		return fmt.Errorf("resized canvas exceeded resolution cap")
	}
	if err := page.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if err := r.waitPage(page, "3D reconnect", 35*time.Second, func(s uiState) bool { return s.Revision >= 5 && s.HandCards == 3 && s.Connection == "Connected" }); err != nil {
		return err
	}
	var restored bool
	for deadline := time.Now().Add(12 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, `!!window.__trick3d && window.__trick3d.stats().objects >= 9 && !window.__trick3d.stats().motion && !window.__trickPresentation().presenting`, &restored); err != nil {
			return err
		}
		if restored {
			break
		}
	}
	if !restored {
		return fmt.Errorf("reconnect did not restore current 3D table")
	}
	if err := page.cdp.call(r.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 1280, "height": 800, "deviceScaleFactor": 1, "mobile": false}, nil); err != nil {
		return err
	}
	if err := r.waitPage(page, "desktop turn", 15*time.Second, func(s uiState) bool { return s.HandCards == 3 && strings.HasPrefix(s.Status, "Alice to play") }); err != nil {
		return err
	}
	target.ID = ""
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, targetJS, &target); err != nil {
			return err
		}
		var idle bool
		if err := page.eval(r.ctx, `window.__trick3d.stats().revision === Number(document.querySelector('#screen').dataset.revision) && !window.__trick3d.stats().motion && !window.__trickPresentation().presenting`, &idle); err != nil {
			return err
		}
		if target.ID != "" && idle {
			break
		}
		target.ID = ""
	}
	if target.ID == "" {
		return fmt.Errorf("desktop drag had no legal visible card")
	}
	if err := page.eval(r.ctx, `window.__trick3d.project('dropzone')`, &center); err != nil {
		return err
	}
	for _, event := range []struct {
		Kind    string
		X, Y    float64
		Buttons int
	}{
		{"mousePressed", target.X, target.Y, 1}, {"mouseMoved", center.X, center.Y, 1}, {"mouseReleased", center.X, center.Y, 0},
	} {
		if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": event.Kind, "x": event.X, "y": event.Y, "button": "left", "buttons": event.Buttons, "clickCount": 1}, nil); err != nil {
			return err
		}
	}
	if err := r.waitPage(page, "desktop drag accepted", 20*time.Second, func(s uiState) bool { return s.HandCards == 2 }); err != nil {
		return err
	}
	step := "phone tap, canceled finger drag, slow rejected drop, edge drop, desktop drag, rapid repeat input, bot reply, trick collection, resize and reconnect passed"
	if latencyCheck {
		step += "; drop stayed local during 1.3-second delayed confirmation"
	}
	r.steps = append(r.steps, step)
	return r.checkConsole()
}
