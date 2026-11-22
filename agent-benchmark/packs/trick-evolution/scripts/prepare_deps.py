#!/usr/bin/env python3
"""Stage the pinned public Go modules for isolated, offline agent attempts."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


PACK_ROOT = Path(__file__).resolve().parents[1]
GO = "/usr/local/go/bin/go"
PAIRS = (("invite", "historical"), ("invite", "colocated"),
         ("removal", "historical"), ("removal", "colocated"))
PACKAGES = ("./animation", "./cards", "./cardtable", "./cmd/browser-e2e",
            "./domain", "./gameui", "./scene")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(*args, cwd, env=None, quiet=False):
    return subprocess.run(args, cwd=cwd, env=env, check=True,
                          stdout=subprocess.DEVNULL if quiet else None)


def remove_staging(staging):
    if not staging.exists():
        return
    # Go expands modules as read-only trees. Directory write permission is
    # needed to remove a failed private staging area.
    for directory, _, _ in os.walk(staging):
        os.chmod(directory, 0o700)
    shutil.rmtree(staging)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--generated", type=Path, default=PACK_ROOT / "_generated")
    parser.add_argument("--source-cache", type=Path,
                        help="existing local Go module cache (default: go env GOMODCACHE)")
    args = parser.parse_args()
    generated = args.generated.resolve()
    if args.source_cache is None:
        source_cache = Path(subprocess.check_output([GO, "env", "GOMODCACHE"], text=True).strip())
    else:
        source_cache = args.source_cache
    source_cache = source_cache.resolve()
    proxy = source_cache / "cache" / "download"
    if not proxy.is_dir():
        parser.error(f"local module download cache missing: {proxy}")

    snapshots = {}
    signatures = set()
    for task, layout in PAIRS:
        directory = generated / task / layout
        repo = directory / "repo"
        pack = json.loads((directory / "task.json").read_text())
        entry = {"baseline": pack["baseline"], "go_mod_sha256": digest(repo / "go.mod"),
                 "go_sum_sha256": digest(repo / "go.sum")}
        snapshots[f"{task}/{layout}"] = entry
        signatures.add((entry["go_mod_sha256"], entry["go_sum_sha256"]))
    if len(signatures) != 1:
        parser.error("the four snapshots do not share identical go.mod and go.sum")

    destination = generated / "deps"
    if destination.exists():
        parser.error(f"dependency snapshot already exists: {destination}; preserve it for reproducibility")
    staging = Path(tempfile.mkdtemp(prefix=".deps-", dir=generated))
    try:
        module_cache = staging / "modcache"
        module_cache.mkdir()
        repo = generated / "invite" / "historical" / "repo"
        env = os.environ.copy()
        env.update({"GOMODCACHE": str(module_cache), "GOCACHE": str(staging / "buildcache"),
                    "GOPROXY": proxy.as_uri(), "GOSUMDB": "off", "GOTOOLCHAIN": "local",
                    "GOWORK": "off", "GOPRIVATE": "", "GONOPROXY": "",
                    "GIT_TERMINAL_PROMPT": "0"})
        run(GO, "list", "-deps", "-test", *PACKAGES, cwd=repo, env=env, quiet=True)
        env["GOPROXY"] = "off"
        run(GO, "list", "-deps", "-test", *PACKAGES, cwd=repo, env=env, quiet=True)
        shutil.rmtree(staging / "buildcache", ignore_errors=True)
        manifest = {"format": 1, "module_cache": "modcache",
                    "go_version": subprocess.check_output([GO, "version"], text=True).strip(),
                    "verified_packages": list(PACKAGES),
                    "snapshots": snapshots,
                    "source": "local Go module download cache; no network fallback"}
        (staging / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        staging.rename(destination)
    finally:
        remove_staging(staging)
    print(f"Prepared {destination / 'modcache'}")
    print(f"Manifest: {destination / 'manifest.json'}")


if __name__ == "__main__":
    main()
