# Collaboration Protocol(Agent 协作协议)

> 本文件定义 AI 团队的协作规则:agent 之间的调用网、跨 agent 调用时必须遵守的派发协议、职责重叠的裁决方式。与 [load-bearing-walls.md](./load-bearing-walls.md)(系统语义)、[ui-guidelines.md](./ui-guidelines.md)(体验一致性)并列,本文件管「协作一致性」。

## 1. 组织原则

- **扁平结构,不设 Lead 岗。** 协调职责由两个机制承担:skills 是流程的 owner(领域 skill 内写明何时调哪个 agent);每个 agent 定义的「协作关系」字段声明可调用谁、被谁调用。
- **main 是总指挥。** 任务拆解、owner 分配、最终验收由 main(读 AGENTS.md 的会话)负责;agent 之间可以在职责范围内互相调用,但跨任务的全局协调回到 main。
- **agent 是成员(3 个),skill 是能力(5 个领域)。** 「谁」写在 agents/(`plan` / `write` / `check`),「怎么做」写在 skills/(`product` / `frontend` / `backend` / `database` / `ops`);agent 定义里不复制 skill 的方法论文本,只引用;领域 skill 按任务组合给 write agent 使用。

## 2. 调用网

```
main(总指挥,AGENTS.md)
 ├─ plan ─────────────── 开工前影响分析 + UI/UX 设计评审(只读,不改代码;维护三层设计规范:DESIGN.md / surface briefs / ui-guidelines 语义手册)
 ├─ write ────────────── ticket 实现(阶段 1 调 plan,阶段 2 组合领域 skill 执行)
 │    └─(放行后)─→ check(经领域 skill 末尾的审查步)
 └─ check ────────────── 提交前三维度验证:测试 + 规范双轴 + 前端细节(三层设计规范多源自查;不改代码,只报告)
```

领域 skill(write agent 阶段 2 按任务组合调用,非独立 agent):

| 领域 skill | 触发 | 内容要点 |
|---|---|---|
| product | 新页面判读者/形态、写或检查 README、对外物料结论呈现 | 产品形态、读者模型、防作假语义、README 对外门面 |
| frontend | 改 `web/` 下任何内容 | 设计评审(按需调 plan)+ 契约核对 + 视图/组件 + UI 细节自查 + typecheck/build |
| backend | 新增后端接口、改后端逻辑、补测试 | 契约先行 + 影响分析(调 plan)+ 黑盒 TDD at W1 + dto/handler/路由 + 安全自查 + 前端同步 |
| database | 改表结构、加字段、改 seed | schema 迁移(只加不删)+ 保留字/单连接坑 + seed 幂等 + 数据隔离 |
| ops | 打包、打 tag、部署(仅用户明确指令) | 发布前检查清单 + 内网部署流水线(交叉编译/docker import/备份/健康检查/自动回滚) |

| 调用方 | 被调用方 | 时机 |
|---|---|---|
| main / write | plan | 非平凡改动开工前,产出影响分析 + 设计评审 |
| write(阶段 2) | 领域 skill + check | 实现完成后,经领域 skill 末尾审查步交 check |
| main | check | 任何 commit 前 |
| 任何人 | product skill | README 相关需求(不再设专职 agent) |

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

- **同一份知识只写一次**,谁 owner 谁写,其余引用(先例:影响分析四节归 plan,write 调用而非复制;测试纪律源头在 AGENTS.md 铁律 + 测试三层,backend skill 只展开后端特定纪律)。
- 横交流程(TDD / 三层测试 / commit 纪律 / 提交前审查)不独立成 skill,纪律在 AGENTS.md,执行展开内置在 write / check agent 定义里。
- 领域知识(产品 / 前端 / 后端 / 数据库 / 运维)以 skill 形式承载,write agent 按任务组合,skill 内不复制 agent 方法论。
- 两个 agent 职责边界模糊时,以「谁的输出更接近最终验收」定归属,另一方的相关内容删除并改引用。
- 裁决结果回写本文件调用网,不留口头约定。
