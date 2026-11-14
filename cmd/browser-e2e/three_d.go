package main

import (
	"fmt"
	"strings"
	"time"
)

func (r *runner) threeDSmoke() error {
	page := r.alice
	if err := page.cdp.call(r.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 390, "height": 844, "deviceScaleFactor": 2, "mobile": true}, nil); err != nil {
		return err
	}
	if err := page.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if err := r.join(page, "Alice"); err != nil {
		return err
	}
	if err := page.click(r.ctx, "Add bot"); err != nil {
		return err
	}
	if err := r.waitPage(page, "3D bot lobby", 15*time.Second, func(s uiState) bool { return s.SeatTiles == 2 }); err != nil {
		return err
	}
	if err := page.click(r.ctx, "Start game"); err != nil {
		return err
	}
	if err := r.waitPage(page, "3D first deal and Alice turn", 20*time.Second, func(s uiState) bool {
		return s.HandCards == 5 && s.Revision >= 1 && strings.HasPrefix(s.Status, "Alice to play")
	}); err != nil {
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
	const framingJS = `(() => { const b = document.querySelector('#table3d-canvas').getBoundingClientRect(); const payload = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(document.querySelector('#screen').dataset.scene), c => c.charCodeAt(0)))); const points = payload.selections.map(s => window.__trick3d.project(s.pickId)); return b.height >= innerHeight - 2 && points.length === 5 && points.every(p => p && p.x > b.left + 40 && p.x < b.right - 40 && p.y > b.top + b.height * .55 && p.y < b.bottom - b.height * .08) && document.getElementById('end-confirm').getClientRects().length === 0 && !document.querySelector('#table3d-play, .hand-fallback') && document.querySelectorAll('.table-seat').length === 2 && document.querySelector('.table-seat.current').textContent.includes('Alice') && document.querySelector('.end-game').getBoundingClientRect().right > innerWidth - 16 && !document.querySelector('#game-rules').open; })()`
	if err := page.eval(r.ctx, framingJS, &framed); err != nil {
		return err
	}
	if !framed {
		return fmt.Errorf("portrait hand framing or hidden confirmation failed")
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
	const targetJS = `(() => { const bytes = Uint8Array.from(atob(document.querySelector('#screen').dataset.scene), c => c.charCodeAt(0)); const payload = JSON.parse(new TextDecoder().decode(bytes)); const card = payload.selections.find(s => s.legal); if (!card) return null; const point = window.__trick3d.project(card.pickId); return {id: card.pickId, x: point.x, y: point.y}; })()`
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
	// A click on the card itself plays it after the invalid drag.
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
	if err := r.waitPage(page, "valid drag and complete trick", 20*time.Second, func(s uiState) bool { return s.Revision >= 5 && s.HandCards == 3 }); err != nil {
		return err
	}
	var pacing bool
	const pacingJS = `(() => { const log = window.__trickPresentation().history; const by = (kind,id) => log.find(x => x.kind === kind && x.id === id); return log.some(x => x.kind === 'landed' && x.ok && by('landing-pause-end',x.id)?.at - x.at >= 350) && log.some(x => x.kind === 'collection-hold' && by('collection-start',x.id)?.at - x.at >= 600); })()`
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
	r.steps = append(r.steps, "phone-size table rendered; tap-to-play, invalid drag, legal drag, bot reply and complete trick passed; canvas resized and restored on reconnect")
	return r.checkConsole()
}
