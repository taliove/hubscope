# Collaboration Protocol(Agent 协作协议)

> 本文件定义 AI 团队的协作规则:agent 之间的调用网、跨 agent 调用时必须遵守的派发协议、职责重叠的裁决方式。与 [load-bearing-walls.md](./load-bearing-walls.md)(系统语义)、[ui-guidelines.md](./ui-guidelines.md)(体验一致性)并列,本文件管「协作一致性」。

## 1. 组织原则

- **扁平结构,不设 Lead 岗。** 协调职责由两个机制承担:skills 是流程的 owner(流程步骤里写明何时调哪个 agent);每个 agent 定义的「协作关系」字段声明可调用谁、被谁调用。
- **main 是总指挥。** 任务拆解、owner 分配、最终验收由 main(读 AGENTS.md 的会话)负责;agent 之间可以在职责范围内互相调用,但跨任务的全局协调回到 main。
- **agent 是成员,skill 是能力。** 「谁」写在 agents/,「怎么做」写在 skills/;agent 定义里不复制 skill 的方法论文本,只引用。

## 2. 调用网

```
main(总指挥,AGENTS.md)
 ├─ architect ─────────── 开工前影响分析(只读,不改代码)
 ├─ implementer ───────── ticket 实现(阶段 1 调 architect,阶段 2 走 implement-ticket skill)
 │    └─(放行后)─→ code-reviewer、test-verifier(经 review skill)
 ├─ design-owner ──────── 事前设计评审(维护 ui-guidelines.md)
 ├─ frontend-checker ──── 前端改动后 UI 细节自查
 ├─ code-reviewer ─────── commit 前独立双轴审查 + 沉淀建议
 ├─ test-verifier ─────── 三层测试执行与测试质量审查
 └─ readme-writer ─────── README.md 专科
```

| 调用方 | 被调用方 | 时机 |
|---|---|---|
| main / implementer | architect | 非平凡改动开工前,产出影响分析 |
| implementer(阶段 2,经 implement-ticket) | test-verifier → code-reviewer | 实现完成后,经 review skill 串联 |
| main / implementer(前端新视图/新交互) | design-owner | 动手前,经 design-review skill |
| main / implementer(前端改动后) | frontend-checker | 实现完成后,经 frontend-dev skill |
| main | test-verifier / code-reviewer | 任何 commit 前,经 review skill |
| 任何人 | readme-writer | README.md 相关需求 |

未登记在上表的调用一律不允许;新调用关系出现时先回本文件登记。

## 3. 派发协议(跨 agent 调用必带字段)

main 或 agent 向另一个 agent 派发任务时,prompt 必须包含以下字段(可合并叙述,但信息一项不缺):

| 字段 | 内容 |
|---|---|
| **任务** | 当前目标,一句话 |
| **背景** | 为什么执行(关联的 ticket / spec / 前序结论) |
| **输入** | 已有信息:范围文件、commit 范围、前序 agent 的产出 |
| **执行** | 需要完成什么,约束是什么(只读/可写、时限) |
| **输出** | 返回什么(格式与粒度,如「四节影响分析」「PASS/FAIL + 位置」) |
| **影响** | 涉及哪些模块/文件,被调用方应核对的边界 |
| **风险** | 潜在问题与需要特别留意的点 |

被调用方发现输入不足时,**停下来返回缺口清单**,不凭猜测补齐。

## 4. 职责重叠裁决

- **同一份知识只写一次**,谁 owner 谁写,其余引用(先例:影响分析四节归 architect,implementer 调用而非复制)。
- 两个 agent 职责边界模糊时,以「谁的输出更接近最终验收」定归属,另一方的相关内容删除并改引用。
- 裁决结果回写本文件调用网,不留口头约定。
