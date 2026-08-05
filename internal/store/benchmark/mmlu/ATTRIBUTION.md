# MMLU Subset Attribution

## Source

- **Dataset:** MMLU (Measuring Massive Multitask Language Understanding), Hendrycks et al., ICLR 2021. Paper: https://arxiv.org/abs/2009.03300
- **Upstream:** https://huggingface.co/datasets/cais/mmlu
- **Revision (commit):** `c30699e8356da336a370243923dbaf21066bb9fe` (last modified 2024-03-08)
- **Artifact:** `all/test-00000-of-00001.parquet` (test split, config `all`, 14042 rows), SHA-256 `74a41822ce7d3def56e1682f958469c04642a5336a5ce912fa375fdb90fb25d7`
- **Integrity cross-check:** every row's question/subject/answer was decoded independently from the pinned-revision parquet and from the HuggingFace datasets-server JSON API; all 14042 rows agreed exactly. Merged extraction SHA-256 `94dc59e71b9266edbc33b376bc73b2e985d06e5b6f3790c08d170c796d305520`.

## License

- **MIT** (dataset card: https://huggingface.co/datasets/cais/mmlu). The MIT license permits redistribution of a subset inside the HubScope single binary, provided the copyright notice and permission notice are included. This file serves as the notice; the subset rows are unmodified question/choice/answer content from the upstream test split.
- Copyright (c) 2020 Dan Hendrycks et al. (MMLU authors). Permission is hereby granted, free of charge, to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the data, subject to including this notice.

## Citation

```
@article{hendryckstest2021,
  title={Measuring Massive Multitask Understanding},
  author={Dan Hendrycks and Collin Burns and Steven Basart and Andy Zou and Mantas Mazeika and Dawn Song and Jacob Steinhardt},
  journal={Proceedings of the International Conference on Learning Representations (ICLR)},
  year={2021}
}
```

## Subset selection (frozen, reproducible, no runtime randomness)

100 questions out of the 14042-row test split, stratified proportionally across the 57 subjects:

1. Rows keep the parquet's physical row order.
2. Group by subject (57 subjects), preserving per-subject row order; subjects processed in first-appearance order.
3. Slot allocation: `n_i = max(1, floor(100 * count_i / 14042))`; trim the excess by repeatedly decrementing the largest allocation above 1 (ties: lexicographically first subject name); top up a shortfall by largest fractional remainder (same tie-break, each subject at most one extra slot) until the total is exactly 100.
4. Within each subject, pick `n_i` rows at evenly spaced indices `floor(k * count_i / n_i)`, `k = 0..n_i-1` (fixed stride, no RNG).
5. IDs `mmlu-0001..mmlu-0100` assigned in emission order.

Subset file SHA-256 `1bbc28448fd424f8b10f33eb5d60e01d10b63d031099d1e7d86044a8f6c2d330`.

### Per-subject allocation (subject, test-split rows, allocated)

```
abstract_algebra,100,1
anatomy,135,2
astronomy,152,1
business_ethics,100,1
clinical_knowledge,265,2
college_biology,144,1
college_chemistry,100,1
college_computer_science,100,1
college_mathematics,100,1
college_medicine,173,1
college_physics,102,1
computer_security,100,1
conceptual_physics,235,1
econometrics,114,2
electrical_engineering,145,1
elementary_mathematics,378,2
formal_logic,126,2
global_facts,100,1
high_school_biology,310,2
high_school_chemistry,203,1
high_school_computer_science,100,1
high_school_european_history,165,1
high_school_geography,198,1
high_school_government_and_politics,193,1
high_school_macroeconomics,390,2
high_school_mathematics,270,2
high_school_microeconomics,238,1
high_school_physics,151,1
high_school_psychology,545,4
high_school_statistics,216,1
high_school_us_history,204,1
high_school_world_history,237,1
human_aging,223,1
human_sexuality,131,2
international_law,121,2
jurisprudence,108,1
logical_fallacies,163,1
machine_learning,112,2
management,103,1
marketing,234,1
medical_genetics,100,1
miscellaneous,783,5
moral_disputes,346,2
moral_scenarios,895,6
nutrition,306,2
philosophy,311,2
prehistory,324,2
professional_accounting,282,2
professional_law,1534,11
professional_medicine,272,2
professional_psychology,612,4
public_relations,110,2
security_studies,245,1
sociology,201,1
us_foreign_policy,100,1
virology,166,1
world_religions,171,1
```

## Immutability (W7)

This subset is cast into the `mmlu` suite once and never edited. Changing the subset means retiring the suite and casting a new one (ADR 0007 / ADR 0013).

## 2026-08-04 trim (100 -> 20)

The shipped subset keeps every 5th row of the frozen 100 (systematic subsample; the stratification documented above is preserved proportionally). Databases that already cast the 100-row bank converge via the one-time benchmark_trim_20 migration at Open, which keeps cases by prompt match and bumps the suite version once.
