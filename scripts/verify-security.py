#!/usr/bin/env python3
"""Run the explicit #121 native security matrix without treating skips as proof.

Raw Go JSON logs stay in the chosen private output directory. Only test names,
counts, source, digests and terminal status enter the reviewable summary.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()
    if os.environ.get("HACKBOX_INTEGRATION") != "1":
        parser.error("HACKBOX_INTEGRATION=1 is required; native skips are not proof")
    root = Path(__file__).resolve().parents[1]
    cases = json.loads((root / "docs/verification/security/cases.json").read_text())
    output = Path(args.output_dir).resolve()
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    summary = {
        "source_commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip(),
        "source_worktree_clean": not subprocess.check_output(["git", "status", "--porcelain"], cwd=root, text=True).strip(),
        "scope": "explicit native security matrix; not full repository CI, browser, deployed manual abuse or pending feature verification",
        "groups": [],
    }
    env = {**os.environ, "GOMAXPROCS": "2"}
    all_ok = True
    for case in cases:
        names = case["tests"]
        if len(names) != len(set(names)) or not all(re.fullmatch(r"Test\w+", name) for name in names):
            raise ValueError("invalid or duplicated security manifest test")
        command = ["go", "test", "-json", "-race", "-p", "1", "-parallel", "1", "./internal/" + case["package"], "-run", "^(" + "|".join(names) + ")$", "-count=1", "-timeout=10m"]
        log = output / (case["package"] + ".jsonl")
        print("Verifying " + case["criterion"], flush=True)
        with log.open("wb") as stream:
            result = subprocess.run(command, cwd=root, env=env, stdout=stream, stderr=subprocess.STDOUT, check=False)
        seen, passed, skipped, failures = [], [], [], []
        for line in log.read_text().splitlines():
            try:
                item = json.loads(line)
            except json.JSONDecodeError:
                continue  # compiler/setup errors still fail via return code
            name, action = item.get("Test"), item.get("Action")
            if name and "/" not in name and action == "run":
                seen.append(name)
            if name and "/" not in name and action == "pass":
                passed.append(name)
            if action == "skip":
                skipped.append(name or "package")
            if action == "fail":
                failures.append(name or "package")
        ok = result.returncode == 0 and sorted(seen) == sorted(names) and sorted(passed) == sorted(names) and not skipped and not failures
        summary["groups"].append({**case, "command": command, "passed_tests": passed, "skips": skipped, "failures": failures, "passed": ok, "log_sha256": hashlib.sha256(log.read_bytes()).hexdigest()})
        all_ok = all_ok and ok
        (output / "summary.json").write_text(json.dumps({**summary, "complete": False}, indent=2) + "\n")
        print(("PASS " if ok else "FAIL ") + case["package"] + ": " + str(len(passed)) + "/" + str(len(names)), flush=True)
        if not ok:
            break  # never spend a broad run hiding an unresolved first failure
    summary["complete"] = all_ok and len(summary["groups"]) == len(cases)
    (output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    return 0 if summary["complete"] else 1


if __name__ == "__main__":
    sys.exit(main())
