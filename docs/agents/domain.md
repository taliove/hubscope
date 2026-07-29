# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root(single-context repo,无 CONTEXT-MAP.md)。
- **`docs/adr/`** — read ADRs that touch the area you're about to work in.
- 本仓另有项目治理文档层,优先级高于通用 skill 约定:`AGENTS.md`(宪法)、`.claude/rules/load-bearing-walls.md`(承重墙 W1–W8)、`.claude/rules/ui-guidelines.md`(设计规范)——涉及其管辖领域时以这些文件为准。

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

Single-context repo (this repo):

```
/
├── CONTEXT.md
├── docs/adr/
│   ├── 0001-*.md
│   └── ...
└── internal/ + web/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (eval immutability) — but worth reopening because…_
