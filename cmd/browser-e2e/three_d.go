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
		if err := page.eval(r.ctx, `!!window.__trick3d && window.__trick3d.stats().objects === 6 && window.__trick3d.stats().width > 0`, &ready); err != nil {
			return err
		}
		if ready {
			break
		}
	}
	if !ready {
		return fmt.Errorf("phone canvas did not render five private cards")
	}
	var initialWidth int
	if err := page.eval(r.ctx, `window.__trick3d.stats().width`, &initialWidth); err != nil {
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
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		if err := page.cdp.call(r.ctx, "Input.dispatchMouseEvent", map[string]any{"type": kind, "x": target.X, "y": target.Y, "button": "left", "clickCount": 1}, nil); err != nil {
			return err
		}
	}
	var selected bool
	if err := page.eval(r.ctx, `!document.querySelector('#table3d-play').disabled && !!window.__trick3d.stats().motion`, &selected); err != nil {
		return err
	}
	if !selected {
		return fmt.Errorf("pointer did not select and animate a legal rendered card")
	}
	var moved bool
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		if err := page.eval(r.ctx, `window.__trick3d.stats().maxTravel > 0.1`, &moved); err != nil {
			return err
		}
		if moved {
			break
		}
	}
	if !moved {
		return fmt.Errorf("selected 3D card did not move")
	}
	if err := page.click(r.ctx, "Play selected card"); err != nil {
		return err
	}
	if err := r.waitPage(page, "accepted 3D play and bot reply", 20*time.Second, func(s uiState) bool { return s.Revision >= 3 && s.HandCards == 4 }); err != nil {
		return err
	}
	var sameRenderer bool
	if err := page.eval(r.ctx, `window.__trick3d === window.__trick3dInitial && window.__trick3d.stats().objects === 5`, &sameRenderer); err != nil {
		return err
	}
	if !sameRenderer {
		return fmt.Errorf("HTMX update recreated renderer or retained played card")
	}
	if err := page.cdp.call(r.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 320, "height": 700, "deviceScaleFactor": 3, "mobile": true}, nil); err != nil {
		return err
	}
	var resized bool
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := page.eval(r.ctx, fmt.Sprintf(`window.__trick3d.stats().width <= 960 && window.__trick3d.stats().height <= 600 && window.__trick3d.stats().width > 0 && window.__trick3d.stats().width !== %d`, initialWidth), &resized); err != nil {
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
	if err := r.waitPage(page, "3D reconnect", 20*time.Second, func(s uiState) bool { return s.Revision >= 3 && s.HandCards == 4 && s.Connection == "Connected" }); err != nil {
		return err
	}
	var restored bool
	if err := page.eval(r.ctx, `!!window.__trick3d && window.__trick3d.stats().objects === 5 && !window.__trick3d.stats().motion`, &restored); err != nil {
		return err
	}
	if !restored {
		return fmt.Errorf("reconnect did not restore current 3D scene")
	}
	r.steps = append(r.steps, "phone-size 3D card rendered, pointer-picked, animated, played through HTMX against a bot; canvas reconciled, resized and restored on reconnect")
	return r.checkConsole()
}
