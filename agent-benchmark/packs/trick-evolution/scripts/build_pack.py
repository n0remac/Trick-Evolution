#!/usr/bin/env python3
"""Build four history-free candidate repos and private calibration patches."""

import argparse
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile

INVITE_BASE = "efc7a7f542c049cc4e93703544390c8debbbd1f4"
INVITE_REFERENCE = "9a66f72baea1cfce08201782f5b004d6791768a6"
REMOVAL_BASE = "94d093326a37171b01f5dd0ed7e33b3914c247b6"
ROOT = Path(__file__).resolve().parents[1]


def git(source, *args, capture=True, env=None):
    command = ["git", "-C", str(source), *args]
    result = subprocess.run(command, check=True, stdout=subprocess.PIPE if capture else None, env=env)
    return result.stdout if capture else b""


def read(tree, name):
    return (tree / name).read_text()


def write(tree, name, content):
    path = tree / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def replace(tree, name, old, new):
    content = read(tree, name)
    if content.count(old) != 1:
        raise RuntimeError(f"expected one occurrence in {name}: {old[:100]!r}")
    write(tree, name, content.replace(old, new, 1))


def archive(source, revision, dest):
    dest.mkdir(parents=True)
    data = git(source, "archive", "--format=tar", revision)
    with tarfile.open(fileobj=io.BytesIO(data)) as tar:
        tar.extractall(dest)


def move_function(tree, old_name, signature, end_marker):
    content = read(tree, old_name)
    start = content.index(signature)
    end = content.index(end_marker, start)
    extracted = content[start:end].rstrip() + "\n\n"
    write(tree, old_name, content[:start] + content[end:])
    return extracted


def invite_solution(source, tree):
    for name in ("gameui/endless.go", "gameui/endless_merge.go", "gameui/game.js", "gameui/render.go", "gameui/server.go", "gameui/style.css"):
        (tree / name).write_bytes(git(source, "show", f"{INVITE_REFERENCE}:{name}"))
    current = read(tree, "domain/endless.go")
    reference = git(source, "show", f"{INVITE_REFERENCE}:domain/endless.go").decode()
    addition = reference[reference.index("// ReplaceEndlessBotSeat"):reference.index("// EndlessCompatible")]
    marker = "// EndlessCompatible"
    if marker not in current:
        raise RuntimeError("invite domain insertion point missing")
    write(tree, "domain/endless.go", current.replace(marker, addition + marker, 1))


def invite_colocated(tree):
    screen = move_function(tree, "gameui/render.go", "func joinScreen(", "func commandForm(")
    join = move_function(tree, "gameui/server.go", "func (s *Server) join(", "// Leave is authenticated")
    start = read(tree, "gameui/endless.go")
    start = start[start.index("// startEndless"):]
    (tree / "gameui/endless.go").unlink()
    server = read(tree, "gameui/server.go")
    routes = '\tmux.HandleFunc("POST /game/join", s.join)\n\tmux.HandleFunc("POST /game/endless", s.startEndless)\n'
    with_invite = '\tmux.HandleFunc("POST /game/endless/join", s.joinEndlessInvite)\n' in server
    if with_invite:
        routes += '\tmux.HandleFunc("POST /game/endless/join", s.joinEndlessInvite)\n'
    if server.count(routes) != 1:
        raise RuntimeError("invite route registration changed")
    server = server.replace(routes, "\ts.registerEndlessRoutes(mux)\n", 1)
    server = server.replace('\t"unicode/utf8"\n', "", 1)
    write(tree, "gameui/server.go", server)
    register = 'func (s *Server) registerEndlessRoutes(mux *http.ServeMux) {\n\tmux.HandleFunc("POST /game/join", s.join)\n\tmux.HandleFunc("POST /game/endless", s.startEndless)\n'
    if with_invite:
        register += '\tmux.HandleFunc("POST /game/endless/join", s.joinEndlessInvite)\n'
    register += '}\n\n'
    share = ""
    render = read(tree, "gameui/render.go")
    marker = '\tif public.Mode == domain.ModeEndless && public.Phase != domain.PhaseGameComplete {\n'
    if marker in render:
        end = render.index('\tif public.Phase == domain.PhasePlayingTrick {', render.index(marker))
        block = render[render.index(marker):end]
        if 'h.Class("share-game")' not in block:
            raise RuntimeError("unexpected Endless share block")
        render = render.replace(block, marker + '\t\tparts = append(parts, endlessShareControls())\n\t}\n', 1)
        write(tree, "gameui/render.go", render)
        share = '''func endlessShareControls() *h.Node {
    return h.Div(h.Class("share-game"),
        h.Button(h.Type("button"), h.Attr("data-action", "share-endless"), h.T("Share live game")),
        h.Input(h.Id("endless-share-link"), h.Attr("aria-label", "Live game invitation link"), h.Attr("readonly", ""), h.Attr("hidden", "")))
}
'''
    feature = '''package gameui

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "unicode/utf8"

    h "github.com/n0remac/GoDom/html"
    "github.com/n0remac/trick-evolution/domain"
)

''' + register + screen + join + start + "\n" + share
    write(tree, "gameui/feature_endless.go", feature)


def removal_solution(tree):
    replace(tree, "domain/model.go", '\tActionChooseRule ActionKind = "choose_rule"\n', '\tActionChooseRule ActionKind = "choose_rule"\n\tActionRemoveRule ActionKind = "remove_rule"\n')
    replace(tree, "domain/engine.go", '\tcase ActionChooseRule:\n\t\treturn stepChooseRule(state, action, seatIndex, catalog)\n', '\tcase ActionChooseRule:\n\t\treturn stepChooseRule(state, action, seatIndex, catalog)\n\tcase ActionRemoveRule:\n\t\treturn stepRemoveRule(state, action, seatIndex, catalog)\n')
    addition = '''// EligibleRemovableRules lists active, post-start evolution cards that can be
// removed without invalidating dependencies or the next deal.
func EligibleRemovableRules(state MatchState, catalog Catalog) []RuleID {
    if state.Config.Mode != ModeEndless || state.Phase != PhaseAwaitingChoice { return nil }
    resolved, err := ResolveRules(state.Field, catalog, state.Config)
    if err != nil { return nil }
    var options []RuleID
    for i, rule := range resolved.History {
        instance := rule.Instance
        if !rule.Active || instance.Starting || instance.IntroducedAfterHand <= 0 || instance.Removed { continue }
        candidate := append([]RuleInstance(nil), state.Field...)
        candidate[i].Removed = true
        after, err := ResolveRules(candidate, catalog, state.Config)
        if err != nil || after.ValidateMatchEffects(state.Config) != nil { continue }
        if _, err := HandPlanForRules(state.Config, len(state.Seats), after); err != nil { continue }
        if !preservesDependencies(state.Field, candidate, catalog, state.Config) { continue }
        options = append(options, instance.DefinitionID)
    }
    return options
}

func stepRemoveRule(state MatchState, action Action, seatIndex int, catalog Catalog) (MatchState, []Event, error) {
    if state.Config.Mode != ModeEndless || state.Phase != PhaseAwaitingChoice {
        return state, nil, transitionError(ErrWrongPhase, "no rule choice is pending")
    }
    if seatIndex != 0 { return state, nil, transitionError(ErrNotHost, "only the runner may choose") }
    if action.BoundaryID == "" || action.BoundaryID != state.PendingBoundaryID {
        return state, nil, transitionError(ErrInvalidState, "stale round boundary")
    }
    allowed := false
    for _, id := range EligibleRemovableRules(state, catalog) { allowed = allowed || id == action.Rule }
    if !allowed { return state, nil, transitionError(ErrInvalidState, "rule is not eligible for removal") }
    next := cloneState(state)
    resolved, _ := ResolveRules(next.Field, catalog, next.Config)
    for i, rule := range resolved.History {
        if rule.Active && next.Field[i].DefinitionID == action.Rule && !next.Field[i].Removed && next.Field[i].IntroducedAfterHand > 0 {
            next.Field[i].Removed = true
            break
        }
    }
    definition, _ := catalog.Definition(action.Rule)
    next.TableCue = endlessCue(next, "You removed "+definition.Title+".")
    next.PendingBoundaryID, next.DraftOptions = "", nil
    next.Phase = PhaseHandComplete
    next.Revision++
    return next, []Event{{Kind: EventRoundDecision, Revision: next.Revision, Hand: next.HandNumber, Rule: action.Rule}}, nil
}

'''
    replace(tree, "domain/endless.go", "func stepChooseRule(", addition + "func stepChooseRule(")
    replace(tree, "domain/view.go", "\tDraftOptions      []RuleID\n", "\tDraftOptions      []RuleID\n\tRemovableRules    []RuleID\n")
    replace(tree, "domain/view.go", "\tview.LastScorecard = cloneScorecard(state.LastScorecard)\n", "\tview.LastScorecard = cloneScorecard(state.LastScorecard)\n\tview.RemovableRules = EligibleRemovableRules(state, catalog)\n")
    # The source uses tabs; add a sibling dispatch case with the same authority.
    server = read(tree, "gameui/server.go")
    start = server.index('\tcase "choose_rule":')
    end = server.index('\tcase "next":', start)
    branch = server[start:end].replace('case "choose_rule":', 'case "remove_rule":').replace('domain.ActionChooseRule', 'domain.ActionRemoveRule')
    write(tree, "gameui/server.go", server[:end] + branch + server[end:])
    marker = '\t\tchoices = append(choices, draftForm("", "Keep current rules", public.PendingBoundaryID, public.Revision))\n'
    removal_ui = '''        for _, id := range public.RemovableRules {
            if definition, ok := catalog.Definition(id); ok {
                choices = append(choices, h.Form(h.Attr("ws-send", ""),
                    h.Input(h.Type("hidden"), h.Name("type"), h.Value("remove_rule")),
                    h.Input(h.Type("hidden"), h.Name("revision"), h.Value(fmt.Sprint(public.Revision))),
                    h.Input(h.Type("hidden"), h.Name("boundaryId"), h.Value(public.PendingBoundaryID)),
                    h.Input(h.Type("hidden"), h.Name("rule"), h.Value(string(id))),
                    h.Button(h.Type("submit"), h.T("Remove "+definition.Title))))
            }
        }
'''.replace("    ", "\t")
    replace(tree, "gameui/render.go", marker, removal_ui + marker)


def removal_colocated(tree):
    form = move_function(tree, "gameui/render.go", "func draftForm(", "func renderScreen(")
    render = read(tree, "gameui/render.go")
    marker = '\t} else if public.Phase == domain.PhaseAwaitingChoice {\n'
    start = render.index(marker)
    end = render.index('\t} else if public.Phase == domain.PhaseGameComplete {', start)
    body = render[start + len(marker):end]
    closing = '\t\tparts = append(parts, h.Div(choices...))\n'
    if not body.endswith(closing):
        raise RuntimeError("draft UI branch changed")
    helper = 'func renderDraftChoices(public domain.PublicView, catalog domain.Catalog, scoreName func(domain.SeatID) string) *h.Node {\n' + body[:-len(closing)] + '\treturn h.Div(choices...)\n}\n\n'
    render = render[:start] + marker + '\t\tparts = append(parts, renderDraftChoices(public, catalog, scoreName))\n' + render[end:]
    write(tree, "gameui/render.go", render)
    server = read(tree, "gameui/server.go")
    start = server.index('\tcase "choose_rule":')
    end = server.index('\tcase "next":', start)
    has_remove = '\tcase "remove_rule":' in server[start:end]
    replacement = '\tcase "choose_rule"'
    if has_remove:
        replacement += ', "remove_rule"'
    replacement += ':\n\t\tnext, events, err = r.dispatchDraftCommand(c, cmd)\n'
    server = server[:start] + replacement + server[end:]
    write(tree, "gameui/server.go", server)
    dispatch = '''func (r *room) dispatchDraftCommand(c *client, cmd command) (domain.MatchState, []domain.Event, error) {
    if r.state == nil { return domain.MatchState{}, nil, errors.New("game has not started") }
    actor := c.seat
    if r.config.Mode == domain.ModeEndless && r.seats[0].Bot { actor = r.seats[0].ID }
    kind := domain.ActionChooseRule
'''
    if has_remove:
        dispatch += '    if cmd.Type == "remove_rule" { kind = domain.ActionRemoveRule }\n'
    dispatch += '''    return domain.Step(*r.state, domain.Action{Kind: kind, Seat: actor, Rule: cmd.Rule, BoundaryID: cmd.BoundaryID}, r.server.catalog)
}
'''
    feature = '''package gameui

import (
    "errors"
    "fmt"
    h "github.com/n0remac/GoDom/html"
    "github.com/n0remac/trick-evolution/domain"
)

''' + form + helper + dispatch
    write(tree, "gameui/feature_draft.go", feature)


def format_go(tree):
    files = [str(p) for p in (tree / "gameui").glob("*.go")]
    files += [str(p) for p in (tree / "domain").glob("*.go")]
    subprocess.run(["/usr/local/go/bin/gofmt", "-w", *files], check=True)


def make_repo(path):
    git(path, "init", "--quiet", "-b", "main")
    git(path, "add", "-A")
    env = os.environ.copy()
    env.update(GIT_AUTHOR_NAME="Benchmark", GIT_AUTHOR_EMAIL="benchmark@example.invalid",
               GIT_COMMITTER_NAME="Benchmark", GIT_COMMITTER_EMAIL="benchmark@example.invalid",
               GIT_AUTHOR_DATE="2026-11-21T00:00:00+0000", GIT_COMMITTER_DATE="2026-11-21T00:00:00+0000")
    git(path, "commit", "--quiet", "-m", "benchmark baseline", env=env)
    return git(path, "rev-parse", "HEAD").decode().strip()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", type=Path, default=Path("/media/pi/media1/Projects/trick-evolution"))
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--force", action="store_true", help="replace the generated directory")
    args = parser.parse_args()
    source = args.source.resolve()
    out = args.out.resolve()
    if out.exists() and any(out.iterdir()):
        if not args.force or out != ROOT / "_generated":
            parser.error("--out must be new or empty; --force only replaces this pack's generated directory")
        shutil.rmtree(out)
    out.mkdir(parents=True, exist_ok=True)
    for task, revision in (("invite", INVITE_BASE), ("removal", REMOVAL_BASE)):
        task_dir = out / task
        eval_dir = task_dir / "evaluator"
        eval_dir.mkdir(parents=True)
        shutil.copy2(ROOT / "_evaluator" / "accept.sh", eval_dir / "accept.sh")
        shutil.copy2(ROOT / "_evaluator" / "browser.sh", eval_dir / "browser.sh")
        shutil.copy2(ROOT / "_evaluator" / "browser_test.go", eval_dir / "browser_test.go")
        tests = ["invite_test.go"] if task == "invite" else ["removal_domain_test.go", "removal_server_test.go"]
        for test in tests:
            shutil.copy2(ROOT / "_evaluator" / test, eval_dir / test)
        for layout in ("historical", "colocated"):
            layout_dir = task_dir / layout
            base = layout_dir / "repo"
            archive(source, revision, base)
            if layout == "colocated":
                (invite_colocated if task == "invite" else removal_colocated)(base)
                format_go(base)
            baseline = make_repo(base)
            prompt = (ROOT / "prompts" / f"{task}.md").read_text()
            pack = dict(name=f"trick-evolution-{task}", repository="repo", baseline=baseline,
                        prompt=prompt, budget_seconds=900,
                        acceptance_command=["sh", "{pack_dir}/../evaluator/accept.sh", task],
                        acceptance_timeout_seconds=300)
            (layout_dir / "task.json").write_text(json.dumps(pack, indent=2) + "\n")
            ref = layout_dir / "reference-work"
            git(layout_dir, "clone", "--quiet", "--no-hardlinks", "--", str(base), str(ref))
            solution_tree = layout_dir / "solution-source"
            archive(source, revision, solution_tree)
            if task == "invite":
                invite_solution(source, solution_tree)
            else:
                removal_solution(solution_tree)
            if layout == "colocated":
                (invite_colocated if task == "invite" else removal_colocated)(solution_tree)
                format_go(solution_tree)
            for child in ref.iterdir():
                if child.name == ".git":
                    continue
                if child.is_dir():
                    shutil.rmtree(child)
                else:
                    child.unlink()
            shutil.copytree(solution_tree, ref, dirs_exist_ok=True)
            shutil.rmtree(solution_tree)
            git(ref, "add", "-A")
            patch = git(ref, "diff", "--cached", "--binary", "--full-index")
            if not patch:
                raise RuntimeError("empty reference patch")
            (task_dir / f"{layout}.patch").write_bytes(patch)
            shutil.rmtree(ref)
            print(f"{task}/{layout}: source={revision} baseline={baseline} patch_bytes={len(patch)}")


if __name__ == "__main__":
    main()
