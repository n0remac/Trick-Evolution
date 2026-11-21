package main

import (
	"fmt"
	"time"
)

// inviteSmoke checks that the visible link joins the same live Endless hand
// from another isolated browser context, while the control row survives updates.
func (r *runner) inviteSmoke() error {
	page := r.alice
	if err := r.waitPage(page, "Endless start form", 15*time.Second, func(s uiState) bool { return s.Ready }); err != nil {
		return err
	}
	if err := page.eval(r.ctx, `(() => {const form=document.getElementById('endless-form');form.elements.name.value='Alice';form.requestSubmit();return true})()`, nil); err != nil {
		return err
	}
	if err := r.waitPage(page, "live Endless hand", 25*time.Second, func(s uiState) bool { return s.Hand == 1 && s.TableReady && s.Connection == "Connected" }); err != nil {
		return err
	}
	for _, size := range [][3]int{{320, 700, 1}, {844, 390, 1}, {1280, 800, 0}} {
		if err := page.cdp.call(r.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": size[0], "height": size[1], "deviceScaleFactor": 1, "mobile": size[2] == 1}, nil); err != nil {
			return err
		}
		var ok bool
		if err := page.eval(r.ctx, `(async()=>{await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));const bar=document.querySelector('.game-top-bar'),r=bar.getBoundingClientRect(),parts=[...bar.children].map(x=>x.getBoundingClientRect());return parts.length===4&&parts.every((x,i)=>x.height>=44&&x.left>=-1&&x.right<=innerWidth+1&&(i===0||x.left>=parts[i-1].right-1))&&r.bottom<=document.querySelector('.game-hud').getBoundingClientRect().top+1&&document.querySelector('[data-action="share-endless"]').getClientRects().length>0})()`, &ok); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("Endless control row overlaps at %dx%d", size[0], size[1])
		}
	}
	if err := page.clickSelector(r.ctx, `[data-action="share-endless"]`); err != nil {
		return err
	}
	var link string
	if err := page.eval(r.ctx, `document.getElementById('endless-share-link').value`, &link); err != nil {
		return err
	}
	if link == "" {
		return fmt.Errorf("Invite did not expose the live link")
	}
	if err := page.clickSelector(r.ctx, `.scores-control > summary`); err != nil {
		return err
	}
	if err := page.eval(r.ctx, `document.querySelector('.scores-control > summary').focus();true`, nil); err != nil {
		return err
	}
	var before struct{ Room, Seat string }
	if err := page.eval(r.ctx, `(()=>{const s=JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(document.querySelector('#screen').dataset.scene),c=>c.charCodeAt(0))));return {room:JSON.parse(sessionStorage.getItem('trick-evolution:endless')).room,seat:s.seats.find(x=>x.self).id}})()`, &before); err != nil {
		return err
	}
	guest, err := r.browser.newPage(r.ctx, "bob", link)
	if err != nil {
		return err
	}
	r.bob = guest
	if err := r.waitPage(guest, "invited join form", 15*time.Second, func(s uiState) bool { return s.Ready }); err != nil {
		return err
	}
	if err := guest.eval(r.ctx, `(()=>{const f=document.getElementById('join-form');f.elements.name.value='Bob';f.requestSubmit();return true})()`, nil); err != nil {
		return err
	}
	if _, _, err := r.waitBoth("same live table", 30*time.Second, func(a, b uiState) bool {
		return a.Hand == 1 && b.Hand == 1 && len(a.Scores) == 2 && len(b.Scores) == 2 && a.Connection == "Connected" && b.Connection == "Connected"
	}); err != nil {
		return err
	}
	var after struct {
		Room, Seat                    string
		ScoresOpen, InviteOpen, Focus bool
	}
	if err := page.eval(r.ctx, `(()=>{const s=JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(document.querySelector('#screen').dataset.scene),c=>c.charCodeAt(0))));return {room:JSON.parse(sessionStorage.getItem('trick-evolution:endless')).room,seat:s.seats.find(x=>x.self).id,scoresOpen:document.querySelector('.scores-control').open,inviteOpen:!document.querySelector('.invite-panel').hidden,focus:document.activeElement===document.querySelector('.scores-control > summary')}})()`, &after); err != nil {
		return err
	}
	var guestSeat struct{ Room, Seat string }
	if err := guest.eval(r.ctx, `(()=>{const s=JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(document.querySelector('#screen').dataset.scene),c=>c.charCodeAt(0))));return {room:JSON.parse(sessionStorage.getItem('trick-evolution:endless')).room,seat:s.seats.find(x=>x.self).id}})()`, &guestSeat); err != nil {
		return err
	}
	if before.Room != after.Room || before.Seat != after.Seat || after.Room != guestSeat.Room || after.Seat == guestSeat.Seat || !after.ScoresOpen || !after.InviteOpen || !after.Focus {
		return fmt.Errorf("invite join or control state mismatch: before=%+v host=%+v guest=%+v", before, after, guestSeat)
	}
	if err := page.clickSelector(r.ctx, `.more-control > summary`); err != nil {
		return err
	}
	var moreOK bool
	if err := page.eval(r.ctx, `(()=>{const bar=document.querySelector('.game-top-bar').getBoundingClientRect(),panel=document.querySelector('.more-panel').getBoundingClientRect();return panel.top>=bar.bottom&&document.querySelector('[data-action="end-confirm"]').getClientRects().length>0&&document.querySelector('[data-action="leave-prompt"]').getClientRects().length>0})()`, &moreOK); err != nil {
		return err
	}
	if !moreOK {
		return fmt.Errorf("More actions did not open below the bar")
	}
	var fallbackOK bool
	if err := page.eval(r.ctx, `(async()=>{document.querySelector('.shell').classList.add('table-fallback');document.getElementById('table3d').hidden=true;document.querySelector('.play-commands').hidden=false;await new Promise(done=>requestAnimationFrame(done));const bar=document.querySelector('.game-top-bar').getBoundingClientRect(),hud=document.querySelector('.game-hud').getBoundingClientRect();return bar.height>=44&&bar.bottom<=hud.top&&document.querySelector('.fallback-seats').getClientRects().length>0&&document.querySelector('.play-commands button').getBoundingClientRect().height>=44&&document.getElementById('rules-open').getClientRects().length===0})()`, &fallbackOK); err != nil {
		return err
	}
	if !fallbackOK {
		return fmt.Errorf("fallback layout lost the control row or play actions")
	}
	r.steps = append(r.steps, "Invite joined the exact live Endless hand from a distinct browser seat; top bar fit portrait, landscape, desktop, and fallback; open controls and focus survived the roster update")
	return r.checkConsole()
}
