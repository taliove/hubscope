# IFEval Subset Attribution

## Source

- **Dataset:** IFEval (Instruction-Following Eval), Zhou et al., 2023. Paper: https://arxiv.org/abs/2311.07911
- **Upstream:** https://huggingface.co/datasets/google/IFEval
- **Revision (commit):** `966cd89545d6b6acfd7638bc708b98261ca58e84`
- **Artifact:** `ifeval_input_data.jsonl` (541 prompts), SHA-256 `6a85310ca8ce15eff755aa08a3a4ff931c7e273e7515ebb3c492ea85fd8288f2`
- **Integrity cross-check:** every row's key/prompt/instruction_id_list/kwargs was decoded independently from the pinned-revision raw JSONL and from the HuggingFace datasets-server rows API (the independent parquet conversion); all 541 rows agreed exactly.
- **Verification code ported from:** https://github.com/google-research/google-research/tree/master/instruction_following_eval (`instructions.py` / `instructions_util.py`, master fetched 2026-07-28), Apache-2.0. See "Checker port" below.

## License

- **Apache-2.0** (dataset card: https://huggingface.co/datasets/google/IFEval, YAML `license: apache-2.0`). The Apache License 2.0 permits redistribution of a subset inside the HubScope single binary, provided the license text and attribution are included. This file serves as the notice; the subset rows are unmodified prompt/instruction content from the upstream file. Copyright 2023 Google LLC. The full license text: https://www.apache.org/licenses/LICENSE-2.0

## Citation

```
@article{zhou2023instruction,
  title={Instruction-Following Evaluation for Large Language Models},
  author={Jeffrey Zhou and Tianjian Lu and Swaroop Mishra and Siddhartha Brahma and Sujoy Basu and Yi Luan and Denny Zhou and Le Hou},
  journal={arXiv preprint arXiv:2311.07911},
  year={2023}
}
```

## Checker port (fidelity statement)

The Go checkers in `internal/evaluator/ifeval/` replicate the official `check_following` implementations class by class (each cites its official class and line anchor). Fidelity evidence:

- **Differential test:** 108 crafted (kwargs, response) cases across the 20 pure-regex instruction types — including the official `instructions_test.py` messages verbatim — produce identical pass/fail on the official Python implementation and the Go port (108/108 agree).
- **Sentence splitter:** the ported `split_into_sentences` (official regex utility) matches the official output exactly on a 13-text battery covering prefixes, acronyms, websites, decimals, quotes and ellipses (13/13 agree).

Two sanctioned approximations (both replace nltk model machinery that cannot ship in the single binary; documented in the package doc):

1. `length_constraints:number_sentences` — official counts with the nltk Punkt tokenizer (`count_sentences`); the port counts with the official deterministic regex splitter `split_into_sentences` from the same `instructions_util.py`.
2. `change_case:capital_word_frequency` — official tokenizes with `nltk.word_tokenize`; the port tokenizes on whitespace (Python `str.isupper` semantics ported exactly).

Three official instruction types are **not ported** because they depend on the langdetect language model (no deterministic single-binary port): `language:response_language`, `change_case:english_capital`, `change_case:english_lowercase`. Rows carrying them are excluded from the subset, and the seed validates fail-closed: an unported type in the data file panics at init (the `mustIFEvalSuite` precedent of `mustMCQSuite`).

## Subset selection (frozen, reproducible, no runtime randomness)

100 prompts out of the 541-row source, stratified proportionally across instruction types:

1. Rows keep the source file order (key ascending).
2. Drop rows carrying any unported (langdetect-dependent) instruction type: 541 → 446 rows.
3. Drop rows whose kwargs would trigger an official `build_description` **random fallback** or error (a randomized check is unmeasurable): out-of-range `letter` (one row uses `!`), out-of-range `nth_paragraph`, empty keyword lists, invalid relations, negative thresholds → 445 rows.
4. Group by the row's first instruction id (22 groups), groups in first-appearance order.
5. Slot allocation: `n_i = max(1, floor(100 * count_i / 445))`; trim the excess by repeatedly decrementing the largest allocation above 1 (ties: lexicographically first instruction id); top up a shortfall by largest fractional remainder (same tie-break, each group at most one extra slot) until the total is exactly 100.
6. Within each group, order rows single-instruction first (source order), then multi-instruction (source order) — so every ported type is exercised by at least one single-instruction prompt — and pick `n_i` rows at evenly spaced indices `floor(k * count_i / n_i)`, `k = 0..n_i-1` (fixed stride, no RNG).
7. IDs `ifeval-0001..ifeval-0100` assigned in emission order.

Subset file SHA-256 `b64e35fafccbfa5ad31f80a706d46fee03af6535f061edc6f8724653ce512533`.

### Per-type allocation (first-instruction group, eligible rows, allocated)

```
punctuation:no_comma,31,7
detectable_content:number_placeholders,12,3
combination:repeat_prompt,35,8
detectable_format:number_bullet_lists,16,4
change_case:capital_word_frequency,17,4
keywords:existence,15,3
length_constraints:number_words,21,5
detectable_format:json_format,17,4
length_constraints:number_paragraphs,18,4
combination:two_responses,20,5
detectable_format:multiple_sections,9,2
startend:end_checker,19,4
keywords:letter_frequency,20,4
keywords:forbidden_words,27,6
keywords:frequency,20,4
startend:quotation,31,7
detectable_format:number_highlighted_sections,37,8
detectable_content:postscript,16,4
detectable_format:title,20,5
length_constraints:number_sentences,24,5
length_constraints:nth_paragraph_first_word,10,2
detectable_format:constrained_response,10,2
```

Every type appears as at least one single-instruction row (72 single-instruction rows, 28 multi-instruction rows in the subset).

## Immutability (W7)

This subset is cast into the `ifeval` suite once and never edited. Changing the subset means retiring the suite and casting a new one (ADR 0007 / ADR 0013).

## 2026-08-04 trim (100 -> 23)

The shipped subset keeps 23 rows: one single-instruction case for each of the 22 ported instruction types plus the keywords:existence + punctuation:no_comma two-instruction case. The trim is coverage-first, not positional: the end-to-end per-checker test contract (TestIFEvalVerdictsPerInstructionType) outranks the round 20. Existing databases converge via the benchmark_trim_20 migration (prompt match, version bump).
