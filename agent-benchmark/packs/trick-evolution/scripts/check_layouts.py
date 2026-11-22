#!/usr/bin/env python3
"""Check paired candidate trees differ only in the intended organization."""

import json
from pathlib import Path
import subprocess
import sys

root = Path(sys.argv[1])
allowed = {
    "invite": {"gameui/endless.go", "gameui/feature_endless.go", "gameui/render.go", "gameui/server.go"},
    "removal": {"gameui/feature_draft.go", "gameui/render.go", "gameui/server.go"},
}


def files(repo):
    return {
        str(path.relative_to(repo)): path.read_bytes()
        for path in repo.rglob("*")
        if path.is_file() and ".git" not in path.relative_to(repo).parts
    }


for task in ("invite", "removal"):
    pair = [root / task / layout for layout in ("historical", "colocated")]
    repos = [folder / "repo" for folder in pair]
    trees = [files(repo) for repo in repos]
    changed = {name for name in trees[0].keys() | trees[1].keys() if trees[0].get(name) != trees[1].get(name)}
    if changed != allowed[task]:
        raise SystemExit(f"unexpected layout differences in {task}: {sorted(changed)}")
    packs = [json.loads((folder / "task.json").read_text()) for folder in pair]
    for key in ("name", "repository", "prompt", "budget_seconds", "acceptance_command", "acceptance_timeout_seconds"):
        if packs[0][key] != packs[1][key]:
            raise SystemExit(f"{task}: paired {key} differs")
    for repo, pack in zip(repos, packs):
        head = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()
        count = subprocess.check_output(["git", "-C", str(repo), "rev-list", "--count", "HEAD"], text=True).strip()
        status = subprocess.check_output(["git", "-C", str(repo), "status", "--porcelain"], text=True).strip()
        if head != pack["baseline"] or count != "1" or status:
            raise SystemExit(f"{repo}: baseline history or working tree is wrong")
        if any(name.endswith(".patch") or name.startswith("benchmark_") or "evaluator" in name for name in files(repo)):
            raise SystemExit(f"{repo}: private material entered candidate checkout")
    print(f"{task}: paired prompt, budget, dependencies, documentation, and evaluator match; changed files: {', '.join(sorted(changed))}")
