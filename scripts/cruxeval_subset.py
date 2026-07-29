#!/usr/bin/env python3
"""CRUXEval frozen-subset authoring tool (ticket 98, spec 0014 decision C).

开发机制题工具,不进交付物:runs once on a dev machine to cast the frozen
100-question subset embedded into the binary; the shipped runtime never
executes Python and never sees this script's behavior (W8).

What it does:
  1. Fetches the official CRUXEval dataset at a pinned commit (or reads a
     local copy via --input) and verifies its SHA-256.
  2. Executes every row's trusted upstream function `f` on its input and
     checks the result equals the dataset's canonical `output` field — the
     standard answers baked into the subset are thus precomputed offline,
     never at runtime.
  3. Selects 100 rows at a fixed stride (every 8th of 800, zero RNG) and
     writes internal/store/benchmark/cruxeval/subset.jsonl.

Determinism: the script re-execs itself with PYTHONHASHSEED=0 so dict/set
iteration order inside the executed functions is stable; rerunning it must
produce a byte-identical subset file.

Usage:
  python3 scripts/cruxeval_subset.py [--input PATH] [--output PATH]

Safety: only the official dataset's own functions are executed (trusted,
curated upstream code), each with a 5s alarm, on the dev machine. No model
output is ever executed here or anywhere else.
"""

import argparse
import ast
import hashlib
import json
import os
import signal
import sys
import urllib.request

PINNED_COMMIT = "190faf16d175b5847b0af05d937872b1fb395942"
DATASET_URL = (
    "https://raw.githubusercontent.com/facebookresearch/cruxeval/"
    f"{PINNED_COMMIT}/data/cruxeval.jsonl"
)
# SHA-256 of data/cruxeval.jsonl at the pinned commit (800 rows).
DATASET_SHA256 = "8368b81047dc5014e4caf5a2f97604eff7644e0ecd7415e3ceeb184bbc2e0c96"

SUBSET_SIZE = 100
DEFAULT_OUTPUT = "internal/store/benchmark/cruxeval/subset.jsonl"


def ensure_deterministic_hash_seed():
    """Re-exec with PYTHONHASHSEED=0 so reruns are byte-identical."""
    if os.environ.get("PYTHONHASHSEED") != "0":
        os.environ["PYTHONHASHSEED"] = "0"
        os.execv(sys.executable, [sys.executable] + sys.argv)


class Timeout(Exception):
    pass


def alarm_handler(signum, frame):
    raise Timeout()


def load_rows(path):
    if path:
        with open(path, "rb") as fh:
            raw = fh.read()
    else:
        with urllib.request.urlopen(DATASET_URL) as resp:
            raw = resp.read()
    digest = hashlib.sha256(raw).hexdigest()
    if digest != DATASET_SHA256:
        sys.exit(
            f"dataset SHA-256 mismatch: got {digest}, want {DATASET_SHA256} "
            f"(pinned commit {PINNED_COMMIT})"
        )
    rows = [json.loads(line) for line in raw.decode("utf-8").splitlines() if line.strip()]
    if len(rows) != 800:
        sys.exit(f"expected 800 rows, got {len(rows)}")
    return rows


def execute_and_verify(rows):
    """Run each row's trusted function; verify against the canonical output.

    Returns the rows unchanged — the canonical `output` strings are the
    standard answers — but only after proving each one equals the value the
    function actually computes. Any mismatch aborts the authoring run.
    """
    signal.signal(signal.SIGALRM, alarm_handler)
    for i, row in enumerate(rows):
        namespace = {}
        signal.alarm(5)
        try:
            exec(row["code"], namespace)
            result = eval(f"f({row['input']})", namespace)
        finally:
            signal.alarm(0)
        canonical = ast.literal_eval(row["output"])
        if result != canonical or type(result) is not type(canonical):
            sys.exit(
                f"row {row['id']}: executed {result!r} != canonical {row['output']}"
            )
        if repr(result) != row["output"]:
            sys.exit(
                f"row {row['id']}: repr drift {repr(result)!r} vs {row['output']!r} "
                "(canonical output must be the exact repr; check PYTHONHASHSEED)"
            )
        if (i + 1) % 100 == 0:
            print(f"verified {i + 1}/{len(rows)}", file=sys.stderr)


def build_subset(rows):
    """Fixed-stride selection: every 8th row of 800, zero RNG, frozen forever."""
    stride = len(rows) // SUBSET_SIZE
    picked = rows[::stride][:SUBSET_SIZE]
    if len(picked) != SUBSET_SIZE:
        sys.exit(f"stride selection yielded {len(picked)}, want {SUBSET_SIZE}")
    out = []
    for n, row in enumerate(picked, start=1):
        out.append(
            {
                "id": f"cruxeval-{n:04d}",
                "source_id": row["id"],
                "code": row["code"],
                "input": row["input"],
                "output": row["output"],
            }
        )
    return out


def main():
    ensure_deterministic_hash_seed()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", help="local cruxeval.jsonl (default: fetch pinned URL)")
    parser.add_argument("--output", default=DEFAULT_OUTPUT)
    args = parser.parse_args()

    rows = load_rows(args.input)
    execute_and_verify(rows)
    subset = build_subset(rows)

    text = "".join(
        json.dumps(row, ensure_ascii=False) + "\n" for row in subset
    )
    with open(args.output, "w", encoding="utf-8") as fh:
        fh.write(text)
    digest = hashlib.sha256(text.encode("utf-8")).hexdigest()
    print(f"wrote {len(subset)} rows to {args.output}")
    print(f"subset SHA-256: {digest}")


if __name__ == "__main__":
    main()
