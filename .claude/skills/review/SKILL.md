---
name: review
description: 提交前审查流程:独立 code-reviewer 双轴审查 + 三层测试 + 门禁确认。任何 commit 前使用。
---

# 提交前审查流程

1. **范围确认**:`git status` + `git diff --stat`,确认改动收敛在任务必要范围,单 commit ≤ 8 文件;混入的无关改动拆出去另记。
2. **三层测试**:调 `test-verifier` 代理——当前功能层、关联功能层、`make test` 闭环层,三层全绿才进下一步。
3. **独立审查**:调 `code-reviewer` 代理(作者不自审),双轴 Standards + Spec;CRITICAL/HIGH 必须修完,MEDIUM 尽量修。
4. **门禁自知**:commit 会触发 pre-commit(凭证扫描 + `make test`)与 commit-msg(Conventional Commits);禁止 `--no-verify`,门禁误报时修门禁并在 message 说明(铁律 6)。
5. **commit**:英文 Conventional Commits,`feat|fix|refactor|docs|test|chore|perf|ci: <description>`;一票一 commit 或一票内多原子 commit;完成票后标记 Status: done。
6. **不 push**:push、tag、发布只在用户明确指令后执行。
