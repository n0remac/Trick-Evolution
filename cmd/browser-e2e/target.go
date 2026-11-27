package main

import (
	"fmt"
	"time"
)

func (r *runner) targetSmoke() error {
	var visible bool
	if err := r.bob.eval(r.ctx, `(() => {const target=document.querySelector('.target-indicator'),scores=document.querySelector('.target-progress'),box=target?.getBoundingClientRect();return Boolean(target?.textContent.includes('first to 5') && scores?.textContent.includes('/5') && box.left>=-1 && box.right<=innerWidth+1)})()`, &visible); err != nil {
		return err
	}
	if !visible {
		return fmt.Errorf("target progress does not fit portrait view")
	}
	if err := r.autoplay(0); err != nil {
		return err
	}
	a, _, err := r.waitBoth("target game result", 65*time.Second, func(a, b uiState) bool {
		return a.Status == "Game complete" && b.Status == "Game complete" && a.Winner != "" && b.Winner != ""
	})
	if err != nil {
		return err
	}
	if err := r.stopAutoplay(); err != nil {
		return err
	}
	if err := r.bob.eval(r.ctx, `Boolean(document.querySelector('.target-result')?.textContent.includes('Game 1 won') && document.querySelector('.scorecard-result')?.textContent.includes('first to 5'))`, &visible); err != nil {
		return err
	}
	if !visible {
		return fmt.Errorf("target result missing from portrait scorecard")
	}
	r.result.Final = a
	r.steps = append(r.steps, "portrait target progress stayed visible; target result and scorecard survived the final closed-room snapshot")
	return r.checkConsole()
}
