#!/usr/bin/env python3
import json
from pathlib import Path
import sys

root = Path(sys.argv[1])
for task in ("invite", "removal"):
    for layout in ("historical", "colocated"):
        for fixture, expected in (("missing", False), ("reference", True)):
            folder = root / task / layout / fixture
            result = json.loads((folder / "result.json").read_text())
            markdown = (folder / "result.md").read_text()
            if result["passed"] != expected or result.get("acceptance", {}).get("passed") != expected:
                raise SystemExit(f"bad result: {task}/{layout}/{fixture}: {result}")
            if result["status"] not in markdown or not result.get("candidate_sha256"):
                raise SystemExit(f"incomplete report: {folder}")
            print(f"{task}/{layout}/{fixture}: {result['status']} acceptance_exit={result['acceptance']['exit_code']} elapsed_ms={result['elapsed_ms']}")
