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
	if err := page.eval(r.ctx, `(() => {const form=document.getElementById('review-settings-form'); const hand=form.querySelector('[data-setting="hand-size"]'),reveal=form.querySelector('[data-setting="reveal-every"]'),game=form.querySelector('[data-initial-rule="rule.game_point"]'); hand.value='2'; hand.dispatchEvent(new Event('input',{bubbles:true})); reveal.value='1'; reveal.dispatchEvent(new Event('input',{bubbles:true})); game.checked=true; game.dispatchEvent(new Event('input',{bubbles:true})); return document.getElementById('review-settings-feedback').textContent.includes('Unsaved');})()`, &previewOK); err != nil {
		return err
	}
	if !previewOK {
		return fmt.Errorf("settings edits were not marked unsaved")
	}
	if err := page.clickSelector(r.ctx, `#review-settings-form button[type="submit"]`); err != nil {
		return err
	}
	if err := r.waitPage(page, "saved room settings", 12*time.Second, func(s uiState) bool { return strings.Contains(s.SettingsFeedback, "Settings saved") }); err != nil {
		return err
	}
	var saved bool
	if err := page.eval(r.ctx, `(() => {const form=document.getElementById('review-settings-form'),preview=document.getElementById('review-panel-starting').textContent;return form.querySelector('[data-setting="hand-size"]').value==='2' && form.querySelector('[data-setting="reveal-every"]').value==='1' && form.querySelector('[data-initial-rule="rule.game_point"]').checked && preview.includes('Deal 2 cards') && preview.includes('every 1 completed hands');})()`, &saved); err != nil {
		return err
	}
	if !saved {
		return fmt.Errorf("saved room settings did not refresh the rules preview")
	}
	if err := page.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if err := r.waitPage(page, "saved lobby reconnect", 15*time.Second, func(s uiState) bool { return s.Connection == "Connected" && s.SeatTiles == 1 }); err != nil {
		return err
	}
	if err := page.clickSelector(r.ctx, "#rules-open"); err != nil {
		return err
	}
	if err := page.eval(r.ctx, `(() => {const form=document.getElementById('review-settings-form');return !!form && form.querySelector('[data-setting="hand-size"]').value==='2' && form.querySelector('[data-setting="reveal-every"]').value==='1' && form.querySelector('[data-initial-rule="rule.game_point"]').checked;})()`, &saved); err != nil {
		return err
	}
	if !saved {
		return fmt.Errorf("saved room settings were lost on reload")
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
		return s.Hand == 1 && s.HandCards == 2 && s.Revision >= 1 && !s.ReviewOpen && s.TableReady
	}); err != nil {
		return err
	}
	for _, viewport := range []struct {
		width, height int
		mobile        bool
	}{{390, 844, true}, {844, 390, true}, {1280, 800, false}, {390, 844, true}} {
		if err := r.checkFieldViewport(page, viewport.width, viewport.height, viewport.mobile); err != nil {
			return err
		}
	}
	if err := page.clickSelector(r.ctx, `#game-rules .rule-card`); err != nil {
		return err
	}
	var detailsOK bool
	if err := page.eval(r.ctx, `document.getElementById('rule-details').open && document.getElementById('rule-text').textContent.includes('No prerequisite rules')`, &detailsOK); err != nil {
		return err
	}
	if !detailsOK {
		return fmt.Errorf("starting card full details unavailable")
	}
	if err := page.clickSelector(r.ctx, `#rule-details button`); err != nil {
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
	if err := r.waitPage(page, "closed game result", 20*time.Second, func(s uiState) bool {
		return s.Revision == final.Revision && s.Winner == final.Winner && s.Connection == "Offline" && !s.ReviewOpen
	}); err != nil {
		return err
	}
	if err := page.clickSelector(r.ctx, "#rules-open"); err != nil {
		return err
	}
	var finalReviewOK bool
	const finalReview = `(() => {const dialog=document.getElementById('rules-review'),changes=document.getElementById('review-panel-changes').textContent;return dialog.open && dialog.querySelector('[data-review-tab="current"]').getAttribute('aria-selected')==='true' && changes.includes('No rule changes yet') && document.querySelector('#rules-review-content [data-action="review-close"]').textContent==='Back to game';})()`
	if err := page.eval(r.ctx, finalReview, &finalReviewOK); err != nil {
		return err
	}
	if !finalReviewOK {
		return fmt.Errorf("final rules review incorrect")
	}
	if err := r.closeReview(page); err != nil {
		return err
	}
	r.result.Final = final
	r.steps = append(r.steps, "portrait automatic preview kept revision zero; host reviewed and started; four zones and measured table space fit portrait, landscape and desktop; card details and rules remained accessible; a trick scored after close; final rules remained available after room closure")
	return r.checkConsole()
}

func (r *runner) checkFieldViewport(page *page, width, height int, mobile bool) error {
	if err := page.cdp.call(r.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": 1, "mobile": mobile}, nil); err != nil {
		return err
	}
	var result struct {
		OK          bool   `json:"ok"`
		Reason      string `json:"reason"`
		Top         int    `json:"top"`
		Height      int    `json:"height"`
		FieldHeight int    `json:"fieldHeight"`
	}
	const expression = `(async () => {
		await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		const field=document.getElementById('game-rules'),hud=document.querySelector('.game-hud'),table=document.getElementById('table3d');
		const zones=[...field.querySelectorAll('.rule-zone')],cards=[...field.querySelectorAll('.rule-card')];
		const box=table.getBoundingClientRect(),hudBox=hud.getBoundingClientRect();
		const fieldBox=field.getBoundingClientRect();
		const source=document.querySelector('#screen')?.dataset.scene;
		const payload=source && JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(source), c => c.charCodeAt(0))));
		const handPoint=window.__trick3d?.project(payload?.selections?.[0]?.pickId);
		const center=window.__trick3d?.project('dropzone');
		const within=p=>p && p.x>=box.left && p.x<=box.right && p.y>=box.top && p.y<=box.bottom;
		const ruleButton=document.getElementById('rules-open').getBoundingClientRect();
		const endButton=document.querySelector('.end-game > button')?.getBoundingClientRect();
		const separated=!endButton || endButton.right<=ruleButton.left+2;
		const compact=cards.every(card=>{const rect=card.getBoundingClientRect();return rect.width<=150 && rect.height>=70 && rect.height<=132;});
		const strip=getComputedStyle(field).display==='flex' && fieldBox.height<=Math.min(144,innerHeight*.24)+3;
		const cue=document.querySelector('.match-cue');
		const turnOK=cue && cue.dataset.next.length>0 && payload?.seats?.some(seat=>seat.turn);
		const target=field.querySelector('.rule-zone:last-child .rule-zone-cards');
		const clones=[];
		for(let i=0;i<12;i++){const clone=cards[2].cloneNode(true);clone.dataset.ruleInstance='test-extra-'+i;target.append(clone);clones.push(clone);}
		await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		const manyBox=field.getBoundingClientRect(),manyTable=table.getBoundingClientRect();
		const manyOK=manyBox.height<=Math.min(144,innerHeight*.24)+3 && manyTable.top>=manyBox.bottom+4 && manyTable.height>=120 && field.scrollWidth>field.clientWidth;
		clones.forEach(clone=>clone.remove());
		const ok=zones.map(z=>z.dataset.category).join(',')==='hand,play,trick,score_victory' &&
			cards.length===3 && cards[0].dataset.ruleTitle==='Follow Suit' && cards[1].dataset.ruleTitle==='High Card Wins' && cards[2].dataset.ruleTitle==='Game Point' &&
			strip && compact && turnOK && manyOK && box.top>=hudBox.bottom+4 && box.height>=120 && box.bottom<=innerHeight+1 &&
			within(handPoint) && within(center) && separated;
		return {ok,reason:ok?'':'zones, compact strip, turn, many-card bound, table projection or controls failed',top:Math.round(box.top),height:Math.round(box.height),fieldHeight:Math.round(fieldBox.height)};
	})()`
	if err := page.eval(r.ctx, expression, &result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("%dx%d field viewport: %s (table top %d, height %d, field height %d)", width, height, result.Reason, result.Top, result.Height, result.FieldHeight)
	}
	return nil
}
