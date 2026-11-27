package main

import (
	"fmt"
	"time"
)

func (r *runner) biddingSmoke() error {
	if err := r.waitPage(r.bob, "bid controls", 15*time.Second, func(s uiState) bool { return s.TurnIndicator == "Your bid or pass" }); err != nil {
		return err
	}
	var fits bool
	if err := r.bob.eval(r.ctx, `(() => {const panel=document.querySelector('.auction-panel'), button=panel?.querySelector('button[type=submit]'), box=panel?.getBoundingClientRect(); return Boolean(box && box.left>=-1 && box.right<=innerWidth+1 && box.bottom<=innerHeight+1 && button.getBoundingClientRect().height>=44)})()`, &fits); err != nil {
		return err
	}
	if !fits {
		return fmt.Errorf("bidding controls do not fit a portrait phone")
	}
	if err := r.bob.clickSelector(r.ctx, `.auction-actions form:first-child button[type="submit"]`); err != nil {
		return err
	}
	if _, _, err := r.waitBoth("auction contract", 15*time.Second, func(a, b uiState) bool { return a.Revision == 2 && b.Revision == 2 && a.TurnIndicator != "" }); err != nil {
		return err
	}
	var visible bool
	if err := r.bob.eval(r.ctx, `Boolean(document.querySelector('.contract-indicator')?.textContent.includes('Bob bid 1 points'))`, &visible); err != nil {
		return err
	}
	if !visible {
		return fmt.Errorf("winning bid was not shown at the table")
	}
	if err := r.autoplay(4); err != nil {
		return err
	}
	a, _, err := r.waitBoth("settled contract", 20*time.Second, func(a, b uiState) bool { return a.Revision >= 4 && b.Revision >= 4 && a.Winner != "" && b.Winner != "" })
	if err != nil {
		return err
	}
	if err := r.stopAutoplay(); err != nil {
		return err
	}
	if err := r.alice.eval(r.ctx, `Boolean(document.querySelector('.scorecard-contract')?.textContent.includes('bid 1 points'))`, &visible); err != nil {
		return err
	}
	if !visible {
		return fmt.Errorf("settled contract missing from scorecard")
	}
	r.result.Final = a
	r.steps = append(r.steps, "bid control submitted, winner led, one-trick contract settled on both screens")
	return r.checkConsole()
}
