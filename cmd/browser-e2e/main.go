// browser-e2e drives the real game page in two isolated Chromium contexts.
// It is an opt-in release gate; use -mode smoke for a fast local check.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/n0remac/trick-evolution/domain"
	"github.com/n0remac/trick-evolution/gameui"
)

type scoreLine struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
	Tricks int    `json:"tricks"`
	Cards  int    `json:"cards"`
}

type uiState struct {
	Ready      bool        `json:"ready"`
	Heading    string      `json:"heading"`
	Status     string      `json:"status"`
	Notice     string      `json:"notice"`
	Connection string      `json:"connection"`
	Revision   int         `json:"revision"`
	Hand       int         `json:"hand"`
	Rules      []string    `json:"rules"`
	ScoreNames []string    `json:"scoreNames"`
	ScoreText  []string    `json:"scoreText"`
	Scores     []scoreLine `json:"scores"`
	HandCards  int         `json:"handCards"`
	SeatTiles  int         `json:"seatTiles"`
	Winner     string      `json:"winner"`
}

type runResult struct {
	Mode    string    `json:"mode"`
	Status  string    `json:"status"`
	Elapsed string    `json:"elapsed"`
	Hands   []uiState `json:"hands,omitempty"`
	Final   uiState   `json:"final"`
	Error   string    `json:"error,omitempty"`
}

type runner struct {
	ctx        context.Context
	mode       string
	artifacts  string
	browser    *browser
	server     *gameui.Server
	httpServer *httptest.Server
	alice      *page
	bob        *page
	result     runResult
	steps      []string
}

var scorePattern = regexp.MustCompile(`^(\d+) points · (\d+) tricks · (\d+) cards$`)

const stateExpression = `(() => {
	const heading = document.querySelector('.panel .row h2')?.textContent || '';
	const scoreTiles = [...document.querySelectorAll('.panel > .scoreboard .seat')];
	const revision = document.querySelector('.panel .row .tag')?.textContent || '';
	return {
		ready: Boolean(window.__trickUIReady),
		heading, status: document.querySelector('.status')?.textContent || '',
		notice: document.querySelector('#notice')?.textContent || '',
		connection: document.querySelector('#connection')?.textContent || '',
		revision: Number(revision.replace('Revision ', '')) || 0,
		hand: Number(heading.match(/Hand (\d+) of/)?.[1]) || 0,
		rules: [...document.querySelectorAll('.rules .rule')].map(x => x.textContent),
		scoreNames: scoreTiles.map(x => x.querySelector('strong')?.textContent || ''),
		scoreText: scoreTiles.map(x => x.querySelector('span')?.textContent || ''),
		handCards: document.querySelectorAll('.hand .card').length,
		seatTiles: document.querySelectorAll('.scoreboard .seat').length,
		winner: [...document.querySelectorAll('h2')].find(x => x.textContent.startsWith('Winner'))?.textContent || ''
	};
})()`

func main() { os.Exit(runMain()) }

func runMain() int {
	mode := flag.String("mode", "smoke", "smoke, bot-smoke, 3d-smoke, or full")
	browserPath := flag.String("browser", "", "Chromium/Chrome executable (auto-detected if empty)")
	artifactRoot := flag.String("artifacts", "artifacts/browser-e2e", "directory for run reports and failure artifacts")
	timeLimit := flag.Duration("timeout", 0, "overall time bound (default: 45s smoke, 150s full)")
	flag.Parse()
	if *mode != "smoke" && *mode != "bot-smoke" && *mode != "3d-smoke" && *mode != "full" {
		fmt.Fprintln(os.Stderr, "-mode must be smoke, bot-smoke, 3d-smoke, or full")
		return 2
	}
	if *timeLimit == 0 {
		if *mode == "full" {
			*timeLimit = 180 * time.Second
		} else {
			*timeLimit = 45 * time.Second
		}
	}
	if *timeLimit < 5*time.Second {
		fmt.Fprintln(os.Stderr, "-timeout must be at least 5s")
		return 2
	}
	name := time.Now().UTC().Format("20060102T150405") + fmt.Sprintf("-%s-%d", *mode, os.Getpid())
	artifacts := filepath.Join(*artifactRoot, name)
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeLimit)
	defer cancel()
	r := &runner{ctx: ctx, mode: *mode, artifacts: artifacts, result: runResult{Mode: *mode}}
	started := time.Now()
	err := r.run(*browserPath)
	r.result.Elapsed = time.Since(started).Round(time.Millisecond).String()
	if err != nil {
		r.result.Status, r.result.Error = "failed", err.Error()
		r.captureFailure(err)
	} else {
		r.result.Status = "passed"
	}
	r.writeJSON("result.json", r.result)
	r.close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "browser %s FAILED: %v\nArtifacts: %s\n", *mode, err, artifacts)
		return 1
	}
	fmt.Printf("browser %s passed in %s\nReport: %s\n", *mode, r.result.Elapsed, filepath.Join(artifacts, "result.json"))
	return 0
}

func (r *runner) run(explicitBrowser string) error {
	var deal atomic.Int64
	shuffle := func() domain.ShuffleSource {
		return domain.SeededShuffle(5100 + deal.Add(1))
	}
	var server *gameui.Server
	var err error
	if r.mode == "3d-smoke" {
		server, err = gameui.NewServerWithBotOptions(domain.DefaultConfig(), shuffle, gameui.BotOptions{Mode: gameui.BotModeHeuristic})
	} else {
		server, err = gameui.NewServerWithConfig(domain.DefaultConfig(), shuffle)
	}
	if err != nil {
		return err
	}
	r.server = server
	r.httpServer = httptest.NewServer(server.Handler())
	executable, err := browserBinary(explicitBrowser)
	if err != nil {
		return err
	}
	r.browser, err = launchBrowser(r.ctx, executable, r.artifacts)
	if err != nil {
		return err
	}
	address := r.httpServer.URL + "/game?room=d1-browser"
	r.alice, err = r.browser.newPage(r.ctx, "alice", address)
	if err != nil {
		return err
	}
	if r.mode == "3d-smoke" {
		return r.threeDSmoke()
	}
	if r.mode == "bot-smoke" {
		return r.botSmoke()
	}
	r.bob, err = r.browser.newPage(r.ctx, "bob", address)
	if err != nil {
		return err
	}
	if err := r.join(r.alice, "Alice"); err != nil {
		return err
	}
	if err := r.join(r.bob, "Bob"); err != nil {
		return err
	}
	if _, _, err := r.waitBoth("two-seat lobby", 25*time.Second, func(a, b uiState) bool { return a.SeatTiles == 2 && b.SeatTiles == 2 }); err != nil {
		return err
	}
	if err := r.alice.click(r.ctx, "Start game"); err != nil {
		return err
	}
	if _, _, err := r.waitBoth("first deal", 40*time.Second, func(a, b uiState) bool {
		return a.Hand == 1 && b.Hand == 1 && a.HandCards == 5 && b.HandCards == 5 && a.Revision == 1 && b.Revision == 1
	}); err != nil {
		return err
	}
	r.steps = append(r.steps, "two isolated browser contexts joined and received first private hands")
	limit := 0
	if r.mode == "smoke" {
		limit = 3
	}
	if err := r.autoplay(limit); err != nil {
		return err
	}
	if r.mode == "smoke" {
		return r.smoke()
	}
	return r.full()
}

func (r *runner) join(p *page, name string, botCount ...int) error {
	if err := r.waitPage(p, name+" join form", 40*time.Second, func(s uiState) bool { return s.Ready && s.Connection == "Offline" && s.Hand == 0 }); err != nil {
		return err
	}
	quoted, _ := json.Marshal(name)
	count := 0
	if len(botCount) > 0 {
		count = botCount[0]
	}
	expression := fmt.Sprintf(`(() => { const name = document.querySelector('input[name=name]'); const bots = document.querySelector('select[name=bots]'); const form = document.querySelector('form'); if (!name || !bots || !form) return false; name.value = %s; bots.value = %q; form.requestSubmit(); return true; })()`, string(quoted), fmt.Sprint(count))
	var submitted bool
	if err := p.eval(r.ctx, expression, &submitted); err != nil {
		return err
	}
	if !submitted {
		return fmt.Errorf("%s join form was absent", name)
	}
	return r.waitPage(p, name+" connected", 40*time.Second, func(s uiState) bool { return s.Connection == "Connected" && s.SeatTiles > 0 })
}

func (r *runner) botSmoke() error {
	if err := r.join(r.alice, "Alice"); err != nil {
		return err
	}
	if err := r.waitPage(r.alice, "one-seat lobby", 20*time.Second, func(s uiState) bool { return s.SeatTiles == 1 }); err != nil {
		return err
	}
	if err := r.alice.click(r.ctx, "Add bot"); err != nil {
		return err
	}
	if err := r.waitPage(r.alice, "human and bot lobby", 20*time.Second, func(s uiState) bool {
		return s.SeatTiles == 2 && len(s.ScoreNames) == 2 && s.ScoreNames[1] == "Bot 1"
	}); err != nil {
		return err
	}
	if err := r.alice.click(r.ctx, "Start game"); err != nil {
		return err
	}
	if err := r.waitPage(r.alice, "bot first deal", 20*time.Second, func(s uiState) bool {
		return s.Hand == 1 && s.HandCards == 5 && s.Revision >= 1
	}); err != nil {
		return err
	}
	if err := r.alice.startAutoplay(r.ctx, 3); err != nil {
		return err
	}
	if err := r.waitPage(r.alice, "first human-bot trick", 20*time.Second, func(s uiState) bool {
		return s.Revision >= 3 && totalPoints(s.Scores) >= 1
	}); err != nil {
		return err
	}
	if err := r.alice.stopAutoplay(r.ctx); err != nil {
		return err
	}
	if err := r.alice.click(r.ctx, "End game"); err != nil {
		return err
	}
	if err := r.alice.click(r.ctx, "Confirm end game"); err != nil {
		return err
	}
	if err := r.waitPage(r.alice, "host-ended match", 20*time.Second, func(s uiState) bool {
		return s.Status == "Game ended by Alice" && s.Winner != "" && totalPoints(s.Scores) >= 1
	}); err != nil {
		return err
	}
	final, err := r.state(r.alice)
	if err != nil {
		return err
	}
	if err := r.alice.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if err := r.waitPage(r.alice, "ended-game reload", 20*time.Second, func(s uiState) bool {
		return s.Status == "Game ended by Alice" && s.Revision == final.Revision && s.Connection == "Connected"
	}); err != nil {
		return err
	}
	if err := r.alice.click(r.ctx, "Return to join"); err != nil {
		return err
	}
	if err := r.waitPage(r.alice, "return to join", 10*time.Second, func(s uiState) bool {
		return s.Hand == 0 && s.Connection == "Offline"
	}); err != nil {
		return err
	}
	r.result.Final = final
	r.steps = append(r.steps, "one browser added a bot from the lobby, scored a trick, ended the game, reloaded its final state, and returned to join")
	return r.checkConsole()
}

func (r *runner) state(p *page) (uiState, error) {
	var state uiState
	if err := p.eval(r.ctx, stateExpression, &state); err != nil {
		return state, err
	}
	for i, line := range state.ScoreText {
		matches := scorePattern.FindStringSubmatch(line)
		if matches == nil {
			if state.Hand == 0 {
				continue
			}
			return state, fmt.Errorf("%s scoreboard row %d has unexpected text %q", p.name, i, line)
		}
		var score scoreLine
		_, _ = fmt.Sscanf(matches[1], "%d", &score.Points)
		_, _ = fmt.Sscanf(matches[2], "%d", &score.Tricks)
		_, _ = fmt.Sscanf(matches[3], "%d", &score.Cards)
		if i < len(state.ScoreNames) {
			score.Name = state.ScoreNames[i]
		}
		state.Scores = append(state.Scores, score)
	}
	return state, nil
}

func (r *runner) waitPage(p *page, label string, duration time.Duration, ready func(uiState) bool) error {
	deadline := time.Now().Add(duration)
	var last uiState
	var lastErr error
	for time.Now().Before(deadline) && r.ctx.Err() == nil {
		state, err := r.state(p)
		if err == nil {
			last = state
			if ready(state) {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s: last state %+v; last CDP error %v; overall %v", label, last, lastErr, r.ctx.Err())
}

func (r *runner) waitBoth(label string, duration time.Duration, ready func(uiState, uiState) bool) (uiState, uiState, error) {
	deadline := time.Now().Add(duration)
	var a, b uiState
	var lastErr error
	for time.Now().Before(deadline) && r.ctx.Err() == nil {
		var err error
		a, err = r.state(r.alice)
		if err == nil {
			b, err = r.state(r.bob)
		}
		if err == nil && ready(a, b) {
			return a, b, nil
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	return a, b, fmt.Errorf("timed out waiting for %s: Alice=%+v Bob=%+v last CDP error=%v overall=%v", label, a, b, lastErr, r.ctx.Err())
}

func (r *runner) autoplay(maximumRevision int) error {
	if err := r.alice.startAutoplay(r.ctx, maximumRevision); err != nil {
		return err
	}
	return r.bob.startAutoplay(r.ctx, maximumRevision)
}

func (r *runner) stopAutoplay() error {
	if err := r.alice.stopAutoplay(r.ctx); err != nil {
		return err
	}
	return r.bob.stopAutoplay(r.ctx)
}

func (r *runner) smoke() error {
	a, b, err := r.waitBoth("first completed trick", 25*time.Second, func(a, b uiState) bool { return a.Revision == 3 && b.Revision == 3 })
	if err != nil {
		return err
	}
	if err := r.stopAutoplay(); err != nil {
		return err
	}
	if a.Hand != 1 || b.Hand != 1 || len(a.Rules) != 1 || len(b.Rules) != 1 || totalPoints(a.Scores) != 1 || totalPoints(b.Scores) != 1 {
		return fmt.Errorf("first trick did not update both scoreboards: Alice=%+v Bob=%+v", a, b)
	}
	r.result.Final = a
	r.steps = append(r.steps, "first trick completed at revision 3 with one point and no rule reveal")
	return r.checkConsole()
}

func (r *runner) full() error {
	for hand := 1; hand <= 10; hand++ {
		phase := "Hand complete"
		if hand == 10 {
			phase = "Game complete"
		}
		wantRevision := 1 + hand*10 + (hand - 1)
		a, b, err := r.waitBoth(fmt.Sprintf("hand %d boundary", hand), 45*time.Second, func(a, b uiState) bool {
			return a.Hand == hand && b.Hand == hand && a.Status == phase && b.Status == phase && a.Revision == wantRevision && b.Revision == wantRevision
		})
		if err != nil {
			return err
		}
		if err := r.assertHand(hand, a, b); err != nil {
			return err
		}
		r.result.Hands = append(r.result.Hands, a)
		r.steps = append(r.steps, fmt.Sprintf("hand %d completed at revision %d with %d rules and %d total points", hand, a.Revision, len(a.Rules), totalPoints(a.Scores)))
		if hand == 10 {
			r.result.Final = a
			break
		}
		if err := r.stopAutoplay(); err != nil {
			return err
		}
		if err := r.alice.click(r.ctx, "Deal next hand"); err != nil {
			return err
		}
		if _, _, err := r.waitBoth(fmt.Sprintf("hand %d deal", hand+1), 20*time.Second, func(a, b uiState) bool {
			return a.Hand == hand+1 && b.Hand == hand+1 && a.Revision == wantRevision+1 && b.Revision == wantRevision+1 && a.HandCards == 5 && b.HandCards == 5
		}); err != nil {
			return err
		}
		if hand == 4 {
			if err := r.bob.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
				return err
			}
			if err := r.waitPage(r.bob, "midgame reload", 25*time.Second, func(s uiState) bool {
				return s.Hand == 5 && s.Revision == wantRevision+1 && s.HandCards == 5 && s.Connection == "Connected"
			}); err != nil {
				return err
			}
			r.steps = append(r.steps, "Bob reloaded during hand 5 and recovered his private hand")
		}
		if err := r.autoplay(0); err != nil {
			return err
		}
	}
	if err := r.stopAutoplay(); err != nil {
		return err
	}
	if err := r.bob.cdp.call(r.ctx, "Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if err := r.waitPage(r.bob, "completed-game reload", 25*time.Second, func(s uiState) bool {
		return s.Status == "Game complete" && s.Revision == 110 && s.Winner == r.result.Final.Winner && s.Connection == "Connected"
	}); err != nil {
		return err
	}
	r.steps = append(r.steps, "Bob reloaded after game completion and recovered final result")
	return r.checkConsole()
}

func (r *runner) assertHand(hand int, a, b uiState) error {
	if !reflectScores(a.Scores, b.Scores) {
		return fmt.Errorf("hand %d scoreboards disagree: Alice=%+v Bob=%+v", hand, a.Scores, b.Scores)
	}
	if len(a.Rules) != 1+hand/3 || len(b.Rules) != len(a.Rules) {
		return fmt.Errorf("hand %d rule count %d/%d, expected %d", hand, len(a.Rules), len(b.Rules), 1+hand/3)
	}
	if totalPoints(a.Scores) != hand*5 || totalTricks(a.Scores) != hand*5 || a.HandCards != 0 || b.HandCards != 0 {
		return fmt.Errorf("hand %d totals or hands wrong: Alice=%+v Bob=%+v", hand, a, b)
	}
	if hand%3 == 0 && hand < 10 {
		message := fmt.Sprintf("revealed after hand %d", hand)
		if !strings.Contains(a.Notice, message) || !strings.Contains(b.Notice, message) {
			return fmt.Errorf("hand %d reveal notice missing: %q / %q", hand, a.Notice, b.Notice)
		}
	}
	if hand == 10 {
		if !reflectScores(a.Scores, []scoreLine{{Name: "Alice", Points: 28, Tricks: 28, Cards: 0}, {Name: "Bob", Points: 22, Tricks: 22, Cards: 0}}) {
			return fmt.Errorf("deterministic final scores changed: %+v", a.Scores)
		}
		if a.Winner != "Winner: Alice" || b.Winner != a.Winner {
			return fmt.Errorf("final winner incorrect: %q / %q", a.Winner, b.Winner)
		}
	}
	return nil
}

func totalPoints(scores []scoreLine) int {
	total := 0
	for _, s := range scores {
		total += s.Points
	}
	return total
}
func totalTricks(scores []scoreLine) int {
	total := 0
	for _, s := range scores {
		total += s.Tricks
	}
	return total
}
func reflectScores(a, b []scoreLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *runner) checkConsole() error {
	for _, p := range []*page{r.alice, r.bob} {
		if p == nil {
			continue
		}
		var drain bool
		if err := p.eval(r.ctx, `true`, &drain); err != nil {
			return err
		}
		if len(p.cdp.errors) != 0 {
			return fmt.Errorf("%s had JavaScript exceptions: %s", p.name, strings.Join(p.cdp.errors, "; "))
		}
	}
	return nil
}

func (r *runner) writeJSON(name string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(r.artifacts, name), append(data, '\n'), 0o644)
	}
}

func (r *runner) captureFailure(cause error) {
	_ = os.WriteFile(filepath.Join(r.artifacts, "failure.txt"), []byte(cause.Error()+"\n\nSteps:\n"+strings.Join(r.steps, "\n")+"\n"), 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, p := range []*page{r.alice, r.bob} {
		if p == nil {
			continue
		}
		capture := p
		if fresh, err := dialCDP(p.debugURL); err == nil {
			defer fresh.close()
			capture = &page{name: p.name, cdp: fresh, debugURL: p.debugURL}
			_ = fresh.call(ctx, "Page.enable", map[string]any{}, nil)
			_ = fresh.call(ctx, "Runtime.enable", map[string]any{}, nil)
		}
		var shot struct {
			Data string `json:"data"`
		}
		if err := capture.cdp.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png", "captureBeyondViewport": false}, &shot); err == nil {
			if image, err := base64.StdEncoding.DecodeString(shot.Data); err == nil {
				_ = os.WriteFile(filepath.Join(r.artifacts, p.name+".png"), image, 0o644)
			}
		}
		var html string
		if err := capture.eval(ctx, `document.documentElement.outerHTML`, &html); err == nil {
			_ = os.WriteFile(filepath.Join(r.artifacts, p.name+".html"), []byte(html), 0o644)
		}
		var state uiState
		if err := capture.eval(ctx, stateExpression, &state); err == nil {
			r.writeJSON(p.name+"-state.json", state)
		}
		r.writeJSON(p.name+"-console.json", map[string]any{"exceptions": p.cdp.errors, "console": p.cdp.console})
	}
}

func (r *runner) close() {
	if r.browser != nil {
		r.browser.close()
	}
	if r.httpServer != nil {
		r.httpServer.Close()
	}
	if r.server != nil {
		r.server.Close()
	}
}
