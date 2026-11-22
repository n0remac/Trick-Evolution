package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
	"github.com/n0remac/trick-evolution/gameui"
)

func benchmarkBrowser(t *testing.T) *runner {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	s, err := gameui.NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return domain.SeededShuffle(5101) }, gameui.BotOptions{Mode: gameui.BotModeHeuristic})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	web := httptest.NewServer(s.Handler())
	t.Cleanup(web.Close)
	path, err := browserBinary("")
	if err != nil {
		t.Fatal(err)
	}
	b, err := launchBrowser(ctx, path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.close)
	return &runner{ctx: ctx, browser: b, server: s, httpServer: web}
}

func startBenchmarkEndless(t *testing.T, r *runner) *page {
	t.Helper()
	p, err := r.browser.newPage(r.ctx, "Alice", r.httpServer.URL+"/game?endless=1")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.waitPage(p, "Endless start form", 20*time.Second, func(s uiState) bool { return s.Ready && s.Hand == 0 }); err != nil {
		t.Fatal(err)
	}
	var submitted bool
	if err := p.eval(r.ctx, `(() => {const form=document.getElementById('endless-form'); if (!form) return false; form.elements.name.value='Alice'; form.requestSubmit(); return true})()`, &submitted); err != nil || !submitted {
		t.Fatalf("start Endless form: %v, submitted=%t", err, submitted)
	}
	if err := r.waitPage(p, "first Endless hand", 30*time.Second, func(s uiState) bool { return s.Connection == "Connected" && s.Hand == 1 && len(s.Scores) == 2 }); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBenchmarkBrowserInvite(t *testing.T) {
	r := benchmarkBrowser(t)
	alice := startBenchmarkEndless(t, r)
	if err := alice.clickSelector(r.ctx, `[data-action="share-endless"], [data-action="invite-endless"]`); err != nil {
		t.Fatal(err)
	}
	var link string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if err := alice.eval(r.ctx, `document.getElementById('endless-share-link')?.value || document.getElementById('invite-link')?.value || ''`, &link); err != nil {
			t.Fatal(err)
		}
		if link != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	u, err := url.Parse(link)
	if err != nil || u.Query().Get("room") == "" || u.Query().Get("invite") == "" {
		t.Fatalf("share control did not produce an invite link: %q, %v", link, err)
	}
	bob, err := r.browser.newPage(r.ctx, "Bob", link)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.waitPage(bob, "invited join form", 20*time.Second, func(s uiState) bool { return s.Ready && s.Hand == 0 }); err != nil {
		t.Fatal(err)
	}
	var submitted bool
	if err := bob.eval(r.ctx, `(() => {const form=document.getElementById('join-form'); if (!form) return false; form.elements.name.value='Bob'; form.requestSubmit(); return true})()`, &submitted); err != nil || !submitted {
		t.Fatalf("invited join form: %v, submitted=%t", err, submitted)
	}
	if err := r.waitPage(bob, "invited live hand", 30*time.Second, func(s uiState) bool { return s.Connection == "Connected" && s.Hand == 1 && len(s.Scores) == 2 }); err != nil {
		t.Fatal(err)
	}
	r.alice, r.bob = alice, bob
	a, b, err := r.waitBoth("shared live hand", 30*time.Second, func(a, b uiState) bool {
		return a.Hand == 1 && b.Hand == 1 && len(a.Scores) == 2 && len(b.Scores) == 2 && a.Scores[0].Name == "Alice" && a.Scores[1].Name == "Bob" && b.Scores[0].Name == "Alice" && b.Scores[1].Name == "Bob"
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Hand != b.Hand || a.Revision != b.Revision {
		t.Fatalf("pages observe different hands: Alice=%+v Bob=%+v", a, b)
	}
}

func TestBenchmarkBrowserRemoval(t *testing.T) {
	r := benchmarkBrowser(t)
	alice := startBenchmarkEndless(t, r)
	if err := alice.startAutoplay(r.ctx, 0); err != nil {
		t.Fatal(err)
	}
	defer alice.stopAutoplay(r.ctx)
	deadline := time.Now().Add(90 * time.Second)
	var ready bool
	for time.Now().Before(deadline) {
		if err := alice.eval(r.ctx, `Boolean(document.querySelector('.draft-choices'))`, &ready); err != nil {
			t.Fatal(err)
		}
		if ready {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		state, _ := r.state(alice)
		t.Fatalf("never reached a draft: %+v", state)
	}
	before, err := r.state(alice)
	if err != nil {
		t.Fatal(err)
	}
	var choices string
	if err := alice.eval(r.ctx, `document.querySelector('.draft-choices')?.textContent || ''`, &choices); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(choices, "Remove Expanded Hand Size") {
		t.Fatalf("draft did not offer eligible removal: %q", choices)
	}
	if err := alice.click(r.ctx, "Remove Expanded Hand Size"); err != nil {
		t.Fatal(err)
	}
	if err := r.waitPage(alice, "removed rule and settled draft", 20*time.Second, func(s uiState) bool {
		return s.Revision == before.Revision+1 && !strings.Contains(strings.Join(s.Rules, ","), "Expanded Hand Size")
	}); err != nil {
		t.Fatal(fmt.Errorf("removal UI: %w", err))
	}
}
