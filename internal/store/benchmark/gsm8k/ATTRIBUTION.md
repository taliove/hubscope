# GSM8K Subset Attribution

## Source

- **Dataset:** GSM8K (Grade School Math 8K), Cobbe et al., 2021. Paper: https://arxiv.org/abs/2110.14168
- **Upstream:** https://huggingface.co/datasets/openai/gsm8k
- **Revision (commit):** `740312add88f781978c0658806c59bc2815b9866` (current `main` head, last modified 2026-03-23)
- **Artifact:** `main/test-00000-of-00001.parquet` (test split, config `main`, 1319 rows), SHA-256 `ee7b8da9e381df27b9e3f7758a159ab2bdaa4dbaa910546cbbc47e0cb44e4f59`
- **Integrity cross-check:** every row's question/answer was decoded independently from the pinned-revision parquet (Go, fraugster/parquet-go) and from the HuggingFace datasets-server JSON API (14 pages of 100 rows); all 1319 rows agreed byte-for-byte. Merged extraction SHA-256 `545c5bec3d319576ea8c954a297c2383889096a91fbfa140e8a533c7b7c48430`.

## License

- **MIT** (dataset card: https://huggingface.co/datasets/openai/gsm8k, `license: mit`). The MIT license permits redistribution of a subset inside the HubScope single binary, provided the copyright notice and permission notice are included. This file serves as the notice; the subset rows are unmodified question text from the upstream test split, with the expected answer taken from the official solution's `#### N` marker (thousands separators removed).
- Copyright (c) 2021 OpenAI (GSM8K authors: Karl Cobbe, Vineet Kosaraju, Mohammad Bavarian, Mark Chen, Heewoo Jun, Lukasz Kaiser, Matthias Plappert, Jerry Tworek, Jacob Hilton, Reiichiro Nakano, Christopher Hesse, John Schulman). Permission is hereby granted, free of charge, to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the data, subject to including this notice.

## Citation

```
@article{cobbe2021gsm8k,
  title={Training Verifiers to Solve Math Word Problems},
  author={Cobbe, Karl and Kosaraju, Vineet and Bavarian, Mohammad and Chen, Mark and Jun, Heewoo and Kaiser, Lukasz and Plappert, Matthias and Tworek, Jerry and Hilton, Jacob and Nakano, Reiichiro and Hesse, Christopher and Schulman, John},
  journal={arXiv preprint arXiv:2110.14168},
  year={2021}
}
```

## Subset selection (frozen, reproducible, no runtime randomness)

100 questions out of the 1319-row test split:

1. Rows keep the parquet's physical row order (verified identical to the datasets-server row order).
2. Pick rows at evenly spaced indices `floor(k * 1319 / 100)`, `k = 0..99` (fixed stride, no RNG): indices 0, 13, 26, …, 1305.
3. Each row's expected answer is the text after the final `#### ` marker of the official solution, with comma thousands separators removed (e.g. `#### 14,000` becomes `14000`); all 100 selected answers are plain non-negative integers.
4. IDs `gsm8k-0001..gsm8k-0100` assigned in emission order; `source_index` records the upstream row index for audit.

Subset file SHA-256 `de97d5c8bc0c5e23623ee9a11198982a0eafbe4281439e1548dc06814d762781`.

## Immutability (W7)

This subset is cast into the `gsm8k` suite once and never edited. Changing the subset means retiring the suite and casting a new one (ADR 0007 / ADR 0013).
