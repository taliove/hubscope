# CRUXEval Subset Attribution

## Source

- **Dataset:** CRUXEval (Code Reasoning, Understanding, and eXecution Evaluation), Gu et al., Meta AI, ICML 2024. Paper: https://arxiv.org/abs/2401.03065
- **Upstream:** https://github.com/facebookresearch/cruxeval (HuggingFace mirror: https://huggingface.co/datasets/facebook/cruxeval — the GitHub repo is the authoritative source and is what was used)
- **Revision (commit):** `190faf16d175b5847b0af05d937872b1fb395942` (main branch tip, last modified 2024-10-11)
- **Artifact:** `data/cruxeval.jsonl` (800 rows, fields `code` / `input` / `output` / `id`), SHA-256 `8368b81047dc5014e4caf5a2f97604eff7644e0ecd7415e3ceeb184bbc2e0c96`
- **Output-prediction split:** this subset uses the output-prediction task — given `code` and `input`, predict the output of `f(input)`. The official prompt caliber is `prompts.py:make_direct_output_prompt` at the pinned commit; the case prompt cast into the suite is that caliber adapted for chat models (reply with only the output literal).

## License

- **MIT** (https://github.com/facebookresearch/cruxeval/blob/main/LICENSE, verified at the pinned commit). The MIT license permits redistribution of a subset inside the HubScope single binary, provided the copyright notice and permission notice are included. This file serves as the notice; the subset rows are unmodified code/input/output content from the upstream dataset.
- Copyright (c) 2023 Meta. Permission is hereby granted, free of charge, to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the data, subject to including this notice.

## Citation

```
@article{gu2024cruxeval,
  title={CRUXEval: Code Reasoning, Understanding, and Execution Evaluation},
  author={Alex Gu and Baptiste Rozi{\`e}re and Hugh Leather and Armando Solar-Lezama and Gabriel Synnaeve and Sida I. Wang},
  journal={Forty-first International Conference on Machine Learning},
  year={2024}
}
```

## Standard answers: offline precomputation (no runtime execution)

Every row's standard answer was verified by **actually executing** the upstream function on the dev machine, once, at authoring time:

- Tool: `scripts/cruxeval_subset.py` (开发机制题工具,不进交付物 — dev-machine authoring tool, not shipped in the binary or the runtime).
- For each of the 800 upstream rows the script `exec`s the trusted upstream `code`, evaluates `f(input)`, and requires the result to equal the dataset's canonical `output` field **and** `repr(result)` to match it byte-for-byte. All 800 rows passed (Python 3.14.4, `PYTHONHASHSEED=0`; the script re-execs itself with that seed so dict/set iteration is deterministic).
- The shipped runtime contains **no execution path at all**: scoring is pure literal comparison between the model's predicted output and the precomputed answer (`output_match` rule). Zero sandbox, zero Python dependency (W8 and the execution safety boundary are untouched).

Rerun (deterministic, byte-identical output):

```
python3 scripts/cruxeval_subset.py            # fetches the pinned dataset, verifies SHA-256
python3 scripts/cruxeval_subset.py --input /path/to/cruxeval.jsonl   # offline from a local copy
```

## Subset selection (frozen, reproducible, no runtime randomness)

100 questions out of the 800-row dataset:

1. Rows keep the upstream file's physical row order (`sample_0` … `sample_799`).
2. Fixed stride: every 8th row (`rows[::8]`), zero RNG — `sample_0, sample_8, …, sample_792`.
3. IDs `cruxeval-0001..cruxeval-0100` assigned in emission order; `source_id` records the upstream row id.

Subset file SHA-256 `6bfe34e5d12c48df82ef79b62ccddf26603b947f146b5e905c16c3611d330e94`.

## Immutability (W7)

This subset is cast into the `cruxeval` suite once and never edited. Changing the subset means retiring the suite and casting a new one (ADR 0007 / ADR 0013).
