# AGIEval Chinese (Gaokao) Subset Attribution

## Source

- **Dataset:** AGIEval (A Human-Centric Benchmark for Evaluating Foundation Models), Zhong et al., 2023. Paper: https://arxiv.org/abs/2304.06364
- **Upstream:** https://github.com/ruixiangcui/AGIEval (canonical copy of the retired `microsoft/AGIEval` repository; same authors, referenced by the lm-evaluation-harness mirrors)
- **Revision (commit):** `84ab72d94318290aad2e4ec820d535a95a1f7552` (last modified 2024-06-13)
- **Artifacts:** `data/v1_1/gaokao-chinese.jsonl` (246 rows), `data/v1_1/gaokao-history.jsonl` (235 rows), `data/v1_1/gaokao-geography.jsonl` (199 rows) — the Chinese-language, four-option, single-answer MCQ tasks of AGIEval v1.1 whose upstream license permits redistribution (see below).
- **Integrity cross-check:** every row of all three subjects was decoded independently from the pinned-revision GitHub JSONL and from the HuggingFace datasets-server JSON API of the lm-evaluation-harness mirrors (`hails/agieval-gaokao-chinese` / `-history` / `-geography`, themselves derived from the same upstream). All 680 rows agreed exactly on question text, passage, options, and gold label (whitespace-insensitive comparison).

## License

- **MIT** for the Gaokao subsets (`data/v1_1/LICENSE`, header `# Gaokao, SAT`: "MIT License, Copyright (c) Microsoft Corporation"). The MIT license permits redistribution of a subset inside the HubScope single binary, provided the copyright notice and permission notice are included. This file serves as the notice; the subset rows are unmodified question/choice/answer content from the upstream files, apart from the documented option-prefix normalization below.
- Copyright (c) Microsoft Corporation. Permission is hereby granted, free of charge, to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the data, subject to including this notice.
- **Excluded upstream subsets (license incompatibility, ticket 96 verification):** `logiqa-zh` is CC BY-NC-SA 4.0 and `jec-qa-*` is "academic research only, commercial use strictly prohibited" (both per `data/v1_1/LICENSE`); CMMLU and C-Eval (the ticket's original candidates) are likewise CC BY-NC-SA 4.0. None of these are redistributable inside an MIT-licensed binary, so the Chinese suite draws only on the MIT-licensed Gaokao subsets.

## Citation

```
@misc{zhong2023agieval,
  title={AGIEval: A Human-Centric Benchmark for Evaluating Foundation Models},
  author={Wanjun Zhong and Ruixiang Cui and Yiduo Guo and Yaobo Liang and Shuai Lu and Yanlin Wang and Amin Saied and Weizhu Chen and Nan Duan},
  year={2023},
  eprint={2304.06364},
  archivePrefix={arXiv}
}
```

## Subset selection (frozen, reproducible, no runtime randomness)

100 questions out of the three eligible Gaokao subjects, stratified proportionally:

1. Rows keep the upstream JSONL physical row order.
2. Eligibility filter: a row must have exactly 4 options, a single-letter `label` in A-D, and every option must carry a prefix matching its positional letter (`(A)`/`(B)`/…, or the full-width `A．`/`B．` variants, or one observed ASCII `B.` variant). Three `gaokao-history` rows (upstream lines 79, 81, 84, 0-indexed 78/80/83) fail this filter — their option prefixes contain upstream typos (e.g. the fourth option printed `(C)` instead of `(D)`) — and are excluded rather than repaired (no content is ever edited). Eligible counts: gaokao-chinese 246, gaokao-history 232, gaokao-geography 199 (total 677).
3. Slot allocation: `n_i = max(1, floor(100 * count_i / 677))`; top up a shortfall by largest fractional remainder (ties: lexicographically first subject name, each subject at most one extra slot) until the total is exactly 100. Result: gaokao-chinese 36, gaokao-history 34, gaokao-geography 30.
4. Within each subject, pick `n_i` rows at evenly spaced indices `floor(k * count_i / n_i)`, `k = 0..n_i-1` (fixed stride, no RNG).
5. IDs `agieval_zh-0001..agieval_zh-0100` assigned in emission order (subjects in the order listed above).
6. Recorded transformations (nothing else is modified): option letter prefixes are stripped and surrounding whitespace trimmed (the prompt template re-emits `A. ` markers); the `passage` field (reading material, gaokao-chinese only) is kept verbatim and composed into the prompt before the question; the upstream `other.source` exam-provenance metadata and the always-null cloze `answer` field are dropped. Question and passage text are verbatim, including original line endings.

Subset file SHA-256 `dcb963f0d943c53bb65a57e79b177453b5f40207acd661177b3babedd72b5e14`.

### Per-subject allocation (subject, eligible rows, allocated)

```
gaokao-chinese,246,36
gaokao-history,232,34
gaokao-geography,199,30
```

## Immutability (W7)

This subset is cast into the `agieval_zh` suite once and never edited. Changing the subset means retiring the suite and casting a new one (ADR 0007 / ADR 0013).
