package main

import (
	"fmt"
	"strings"
	"time"
)

// rulesSmoke uses one portrait browser page and a server-side bot to keep the
// browser pass bounded on the Pi. Multiplayer review transitions are covered
// by focused WebSocket tests.
func (r *runner) rulesSmoke() error {
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
	if err := r.waitPage(page, "automatic starting rules", 15*time.Second, func(s uiState) bool {
		return s.ReviewOpen && s.ReviewTab == "starting" && s.Revision == 0 && s.Hand == 0
	}); err != nil {
		return err
	}
	var previewOK bool
	const preview = `(() => {const dialog=document.getElementById('rules-review'),box=dialog.getBoundingClientRect(),text=document.getElementById('review-panel-starting').textContent,content=document.getElementById('rules-review-content');return dialog.open && box.left>=-1 && box.right<=innerWidth+1 && box.top>=-1 && box.bottom<=innerHeight+1 && dialog.contains(document.activeElement) && text.includes('Deal 5 cards') && text.includes('after 10 completed hands') && text.includes('every 3 completed hands') && content.scrollHeight>content.clientHeight && dialog.querySelector('.review-header button').getBoundingClientRect().height>=44;})()`
	if err := page.eval(r.ctx, preview, &previewOK); err != nil {
		return err
	}
	if !previewOK {
		return fmt.Errorf("portrait preview or actual configured numbers unavailable")
	}
	if err := r.closeReview(page); err != nil {
		return err
	}
	if err := page.click(r.ctx, "Add bot"); err != nil {
		return err
	}
	if err := r.waitPage(page, "two-seat lobby", 15*time.Second, func(s uiState) bool { return s.SeatTiles == 2 }); err != nil {
		return err
	}
	if err := page.clickSelector(r.ctx, `[data-action="review-start"]`); err != nil {
		return err
	}
	state, err := r.state(page)
	if err != nil {
		return err
	}
	if !state.ReviewOpen || state.Revision != 0 || state.Hand != 0 {
		return fmt.Errorf("opening host start review changed the match")
	}
	if err := page.clickSelector(r.ctx, `#review-start-form button`); err != nil {
		return err
	}
	if err := r.waitPage(page, "started match", 20*time.Second, func(s uiState) bool {
		return s.Hand == 1 && s.HandCards == 5 && s.Revision >= 1 && !s.ReviewOpen && s.TableReady
	}); err != nil {
		return err
	}
	if err := page.eval(r.ctx, `window.__rulesSmokeTable=window.__trick3d; true`, nil); err != nil {
		return err
	}
	before, err := r.state(page)
	if err != nil {
		return err
	}
	if err := page.clickSelector(r.ctx, "#rules-open"); err != nil {
		return err
	}
	if err := r.waitPage(page, "current rules reopened", 10*time.Second, func(s uiState) bool { return s.ReviewOpen && s.ReviewTab == "current" && s.Revision == before.Revision }); err != nil {
		return err
	}
	var tabsOK bool
	const tabs = `(() => {const dialog=document.getElementById('rules-review'),tab=dialog.querySelector('[data-review-tab="current"]');tab.focus();tab.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true}));const selected=dialog.querySelector('[data-review-tab][aria-selected="true"]');const moved=selected?.dataset.reviewTab==='starting' && document.activeElement===selected;dialog.querySelector('[data-review-tab="current"]').click();return moved && dialog.contains(document.activeElement) && window.__rulesSmokeTable===window.__trick3d;})()`
	if err := page.eval(r.ctx, tabs, &tabsOK); err != nil {
		return err
	}
	if !tabsOK {
		return fmt.Errorf("rules tabs, focus, or mounted table changed")
	}
	if err := r.closeReview(page); err != nil {
		return err
	}
	var restored bool
	if err := page.eval(r.ctx, `document.activeElement?.id==='rules-open' && window.__rulesSmokeTable===window.__trick3d`, &restored); err != nil {
		return err
	}
	if !restored {
		return fmt.Errorf("rules close did not restore focus or preserve the table")
	}
	if err := page.startAutoplay(r.ctx, 0); err != nil {
		return err
	}
	if err := r.waitPage(page, "scored play after close", 20*time.Second, func(s uiState) bool { return s.Revision >= 3 && totalPoints(s.Scores) >= 1 }); err != nil {
		return err
	}
	if err := page.stopAutoplay(r.ctx); err != nil {
		return err
	}
	if err := page.click(r.ctx, "End game"); err != nil {
		return err
	}
	if err := page.click(r.ctx, "Confirm end game"); err != nil {
		return err
	}
	if err := r.waitPage(page, "completed game", 15*time.Second, func(s uiState) bool { return strings.Contains(s.Status, "Game ended") && s.Winner != "" }); err != nil {
		return err
	}
	final, err := r.state(page)
	if err != nil {
		return err
	}
	if err := page.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if err := r.waitPage(page, "ended game reconnect", 20*time.Second, func(s uiState) bool {
		return s.Revision == final.Revision && s.Winner == final.Winner && s.Connection == "Connected" && !s.ReviewOpen
	}); err != nil {
		return err
	}
	if err := page.clickSelector(r.ctx, "#rules-open"); err != nil {
		return err
	}
	var reconnectOK bool
	const reconnect = `(() => {const dialog=document.getElementById('rules-review'),changes=document.getElementById('review-panel-changes').textContent,history=window.__trickPresentation().history;return dialog.open && dialog.querySelector('[data-review-tab="current"]').getAttribute('aria-selected')==='true' && changes.includes('No rule changes yet') && document.querySelector('#rules-review-content [data-action="review-close"]').textContent==='Back to game' && !history.some(x=>x.kind==='landing');})()`
	if err := page.eval(r.ctx, reconnect, &reconnectOK); err != nil {
		return err
	}
	if !reconnectOK {
		return fmt.Errorf("reconnect rules or presentation restoration incorrect")
	}
	if err := r.closeReview(page); err != nil {
		return err
	}
	r.result.Final = final
	r.steps = append(r.steps, "portrait automatic preview kept revision zero; host reviewed and started; current rules reopened with keyboard tabs and focus restoration; a trick scored after close; ended-game reconnect restored current rules without replaying landing animation")
	return r.checkConsole()
}
