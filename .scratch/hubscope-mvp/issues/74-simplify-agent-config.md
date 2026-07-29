# 74 — Simplify agent config: 3 agents (Plan/Write/Check) + 5 domain skills

**What to build:** 将 HubScope 的 Claude Code 增强层从「7 agent + 9 skill」简化为「3 agent(Plan / Write / Check)+ 5 领域 skill(产品 / 前端 / 后端 / 数据库 / 运维)」。AGENTS.md 为唯一主控,CLAUDE.md 仅引用它;横交流程(TDD/三层测试/commit 纪律/提交前审查)回归 agent 定义与 AGENTS.md,不再独立成 skill;领域知识以 skill 形式按需组合给 Write agent 使用。不碰 W1–W8 承重墙、不碰代码与测试、不碰历史 ticket/spec/ADR。

**Blocked by:** None — can start immediately

**Status:** done(2026-07-28 核对销账:.claude 已是 plan/write/check 三 agent + 五领域 skill 扁平架构,format-web.sh 已补)

## Problem Statement

当前 Claude Code 增强层配置复杂、职责重叠,新会话(含 AFK agent)难以快速找准「该调谁、走哪条流程」:

- 7 个 agent 中,`code-reviewer` / `test-verifier` / `frontend-checker` 三者都是「不改代码、只报告 PASS/FAIL」的验证角色,仅维度不同;`architect` 与 `design-owner` 都是「开工前只读产出分析与方案」的角色。
- 9 个 skill 中,`review` skill 与 `code-reviewer`+`test-verifier` agent 是同一件事的两面;`implement-ticket` skill 的 TDD/三层/commit 纪律与 AGENTS.md 铁律+工作流重复;`add-tests` skill 的接缝纪律与 W1 承重墙重复;`frontend-dev` skill 与 `frontend-checker` agent 拆成两处。
- `CLAUDE.md` 声明「治理正文在 AGENTS.md」却仍重复登记 agent 目录表与协作叙述,两处需同步维护、易漂移。
- `CLAUDE.md` 引用的 `format-web.sh` hook 文件实际不存在(已知 gap),settings.json 与文档不一致。

结果是:协作摩擦高、引用漂移风险高、新人/onboarding 成本高,且「持续进化」沉淀时该进 agent 还是 skill 边界模糊。

## Solution

从使用者视角:开工时按固定的三段走——

1. **Plan**(分析):开工前产出影响分析四节(直接/间接/公共调用方/权限隔离)+ 承重墙四问 + UI/UX 设计评审(对照 ui-guidelines.md)。不动手实现。
2. **Write**(实现):按 Plan 放行方案,组合所需领域 skill(产品/前端/后端/数据库/运维)实现 ticket——TDD at W1 唯一接缝、小步验证、三层测试、英文 Conventional Commits。README 按「产品」skill 形态约定写。
3. **Check**(审查):三维度全 PASS 才放行 commit——测试(三层 + 接缝质量)+ 规范(Standards+Spec 双轴 + 沉淀建议)+ 前端细节(typecheck/build/溢出/截断/三态/轮询)。不改代码。

AGENTS.md 是唯一主控(铁律/承重墙/测试三层/工作流/3+5 分工骨架),CLAUDE.md 仅引用它并登记 `.claude` 新结构 + `make hooks`。领域知识以 skill 形式承载,Write agent 按任务组合(如「后端 + 数据库」新接口涉及表变更、「前端 + 产品」新状态板视图)。

## User Stories

1. 作为会话指挥(main),我想用一个固定的三段流程(Plan→Write→Check)派发任何 ticket,这样我不必在 7 个 agent 之间选择该调谁。
2. 作为会话指挥,我想让影响分析与设计评审由同一个 Plan agent 产出,这样我不必在 architect 与 design-owner 之间判断该调哪个。
3. 作为会话指挥,我想让三层测试、规范审查、前端细节自查由同一个 Check agent 一次跑完,这样审查环节不再需要我串三个 agent。
4. 作为 Write agent,我想在实现时按领域组合 skill(如 backend + database),这样同一套实现纪律能套用不同领域的具体操作。
5. 作为 Write agent,我想让 TDD/三层测试/commit 纪律内置在我的 agent 定义里而不是分散在 implement-ticket skill,这样我不必每次先读一个 skill 才知道怎么做。
6. 作为 Plan agent,我想保留对 ui-guidelines.md 的写权限(仅限该文件),这样设计评审产生的新约定能由我直接回写规范,不必绕一圈让 main 落盘。
7. 作为 Check agent,我想只报告 PASS/FAIL + 位置,不修改代码,这样审查的独立性(作者不自审)不被破坏。
8. 作为新会话的读者,我想 CLAUDE.md 只指向 AGENTS.md 并登记一张目录表,这样我 30 秒内知道增强层结构而不读重复叙述。
9. 作为新会话的读者,我想 collaboration.md 调用网表用 Plan/Write/Check + 5 领域 skill 表达,这样调用关系与我实际派发的方式一致。
10. 作为维护者,我想「持续进化」时该进 agent 还是 skill 的边界清晰(横交流程进 agent,领域知识进 skill),这样沉淀不再边界模糊。
11. 作为维护者,我想 settings.json 引用的 hook 脚本都真实存在,这样 PostToolUse 自动格式化在 web/ 改动后真的生效(format-web.sh 补建)。
12. 作为维护者,我想落地后全仓 grep 旧 7 agent 名零活引用残留(历史 ticket/spec/ADR/memory 除外),这样旧名不再误导新会话。
13. 作为维护者,我想 `make hooks` 安装后 pre-commit/commit-msg/pre-push 三个 git hook 仍工作,这样门禁链不被配置重构破坏。
14. 作为维护者,我想 `make test` 在落地后仍全绿,这样证明配置重构未误碰代码与测试。
15. 作为领域 skill 使用者,我想 frontend skill 内置「按需调 Plan 的 UI 评审子能力」一步,这样我不必在 frontend-dev 与 design-review 两个 skill 之间判断该走哪个。
16. 作为领域 skill 使用者,我想 backend skill 内置测试纪律(stub 校验请求字段/假时钟/流式故障样本),这样 add-tests 的纪律不丢失。
17. 作为领域 skill 使用者,我想 ops skill 合并发布前检查与内网部署流水线,这样发布相关动作有单一入口。
18. 作为领域 skill 使用者,我想 product skill 承载产品形态/读者模型/防作假语义/README 对外门面形态,这样产品决策有单一事实源。
19. 作为领域 skill 使用者,我想 database skill 原样保留(只更名),这样 W2 存储层的既有约束与历史坑不动。
20. 作为落地执行者,我想落地顺序是「先建新文件 → 改引用 → grep 校验 → 最后删旧文件」,这样回滚成本低、中途任何一步都可停。
21. 作为落地执行者,我想不主动 git commit/push/发布,这样每一步都在用户明确指令后才推进。
22. 作为历史记录的读者,我想历史 ticket/spec/ADR/memory 里提到的旧 agent 名保留不改,这样历史可追溯、不被重写。

## Implementation Decisions

- **3 个新 agent**(位于 `.claude/agents/`):
  - **Plan**:tools = Read, Grep, Glob, Bash(+ Edit/Write,硬约束仅限 `.claude/rules/ui-guidelines.md` 的规范回写,其余只读)。合并来源:architect + design-owner。内置:影响分析四节 + 承重墙四问 + UI/UX 设计评审标准(对照 ui-guidelines.md)。产出:分析 + 评审结论 + 待确认决策点 + 预计改动清单。
  - **Write**:tools = Read, Grep, Glob, Bash, Edit, Write。合并来源:implementer + readme-writer。内置:TDD at W1 / 小步验证 / 三层测试 / 英文 Conventional Commits(纪律来自 AGENTS.md,执行展开内置)。README 编写按 product skill 形态约定。不主动 push/tag/部署。
  - **Check**:tools = Read, Grep, Glob, Bash。合并来源:code-reviewer + test-verifier + frontend-checker。内置三维度:① 测试(三层 + 接缝质量)② 规范(Standards+Spec 双轴 + 沉淀建议)③ 前端(typecheck/build/溢出/截断/三态/轮询)。不改代码,只报告 PASS/FAIL + 位置。
- **5 个新领域 skill**(位于 `.claude/skills/<name>/SKILL.md`):
  - **product**:合并 design-review 的产品判断节 + readme-writer 的 README 形态约定。内容:产品形态(状态板/管理台双形态)、读者模型(3 秒 vs 高效)、防作假语义边界、README 对外门面形态(双语/三问/单二进制使用者路径优先于 git clone)。
  - **frontend**:合并 frontend-dev + design-review。内容:按需调 Plan 的 UI 评审子能力 → 契约核对(api/types.ts)→ 视图/组件改动约束(Element Plus + 语义令牌)→ UI 细节自查清单 → pnpm typecheck && build。
  - **backend**:合并 new-api + implement-ticket 的 TDD 节 + add-tests 的测试纪律。内容:契约先行(路径/JSON/鉴权档位)→ dto/handler/路由 → 黑盒 TDD at W1(stub 校验请求字段/假时钟/流式故障样本)→ 输入校验/凭证不回明文 → 前端同步 → 三层测试。
  - **database**:db-change 原样更名。内容:schema 自动迁移(只加不删/幂等)/ 单连接与保留字坑 / seed 按 generation 追踪 / 含凭证字段不出库 / 迁移后 make dev 验证。
  - **ops**:合并 release-check + deploy。内容:发布前检查(门禁全绿/打包/部署要件/数据面/配置面/发布后核心闭环抽查)+ 内网部署流水线(tag 提议+确认/交叉编译/docker import scratch/备份/健康检查/自动回滚)。
- **横交流程去向**:`implement-ticket`(通用 TDD/三层/commit)纪律已在 AGENTS.md,执行展开进 Write agent 定义,不独立成 skill;`review`(提交前审查)进 Check agent 定义;`add-tests`(补测试纪律)进 backend skill 测试节(W1 承重墙为源)。
- **Rules 处理**:`load-bearing-walls.md` / `ui-guidelines.md` / `graph-tools.md` 不动;`collaboration.md` 调用网表与调用关系表从 7 agent 同步为 3 agent + 5 领域 skill 组合表,派发协议七字段保留,职责重叠裁决条目措辞与新架构一致。
- **CLAUDE.md 简化**:删去与 AGENTS.md 重复的铁律/工作流叙述;一句话指向 AGENTS.md;`.claude` 目录表更新为 3 agent / 5 skill / 4 rules / 2 hooks;保留 `make hooks` 段(Claude Code 特有);设计规范段一句指向 ui-guidelines.md(由 Plan agent 维护)。
- **AGENTS.md 更新**:仅两处,不动铁律/承重墙/测试三层——「开工纪律 §3 Agent 分工与协作」7 agent 列表改为 3 agent + 5 领域 skill 骨架(详细调用网指向 collaboration.md);「持续进化」表确认「新增 agent 须在 collaboration.md 调用网登记」措辞与新架构一致。
- **Hooks 决断(方案 A)**:补建 `.claude/hooks/format-web.sh`(PostToolUse Edit|Write,对 `web/src/**` 跑 prettier/eslint --fix),`.claude/settings.json` 加第二条 PostToolUse matcher(Edit|Write 对 web/ 路径)。
- **ui-guidelines.md 头注**:维护者从 design-owner 改为 Plan agent(一行注释)。
- **memory 更新**:`ai-org-refactor-2026-07.md` 不删,追加一条新 memory 记录此次简化(旧 memory 标注 supersede)。
- **落地顺序**:① 新建 3 agent + 5 skill 文件 → ② 改 collaboration.md 调用网 → ③ 改 CLAUDE.md / AGENTS.md 引用 → ④ 补建 format-web.sh + settings.json → ⑤ grep 校验残留 → ⑥ 删除旧 7 agent + 4 横流 skill(implement-ticket / review / add-tests / design-review,design-review 内容已并入 frontend 与 product)。
- **不动**:W1–W8 承重墙、代码逻辑、测试、`docs/specs/`、`docs/adr/`、`CONTEXT.md`、历史 ticket 里的旧 agent 名引用。
- **不主动**:`git commit` / `git push` / 打 tag / 部署——等用户明确指令。

## Testing Decisions

这是配置/文档重构,不引入新测试代码。验证只走一道接缝(复用现有门禁,不新增):

- **好的测试只测外部行为,不测实现细节**:本任务的「外部行为」= 增强层配置在落地后仍让门禁链工作、引用自洽、无悬空引用。不测 agent 定义文件的「内部结构」。
- **接缝(唯一一道)**:
  1. `make hooks` 安装后,pre-commit(凭证扫描 + make test)、commit-msg(Conventional Commits)、pre-push(保护 main)三个 git hook 仍工作(block-no-verify.sh 不受影响)。
  2. `make test` 全绿(后端全部测试 + 前端 typecheck + build)——证明配置重构未误碰代码与测试。
  3. 全仓 grep 旧 7 agent 名(architect / implementer / design-owner / frontend-checker / code-reviewer / test-verifier / readme-writer),活引用(agents / skills / rules / CLAUDE.md / AGENTS.md / settings.json)零残留;历史引用(ticket / spec / ADR / memory)允许保留。
  4. 3 个新 agent + 5 个新 skill 的 frontmatter(name / description / tools)被 Claude Code 识别(无 schema 报错,agent 列表正确显示)。
  5. 引用自洽:CLAUDE.md 目录表 / AGENTS.md §3 / collaboration.md 调用网 / 各 skill 内部引用 / 各 agent 内部引用,五处交叉引用一致;settings.json 引用的 hook 脚本(format-web.sh 补建后)都存在且可执行。
- **Prior art**:全仓 grep 校验活引用残留的手法在 ticket 73(品牌并入 ProxyHub teal)的迁移验收 checklist 中已用过(grep `#` 十六进制色值 + `--el-*` + 刻度外 z-index);本任务沿用同一手法(grep 旧 agent 名)。
- **不测**:agent prompt 的「输出质量」不在本任务验证范围(那是持续进化机制 + Check agent 的职责);ui-guidelines.md 内容不动,不测视觉回归。

## Out of Scope

- 不改 W1–W8 承重墙(系统语义)。
- 不改代码逻辑、测试代码、前端源码。
- 不改 `docs/specs/`、`docs/adr/`、`CONTEXT.md`(历史文档)。
- 不改历史 ticket / spec / ADR / memory 里提到的旧 agent 名引用(历史记录,保留可追溯)。
- 不主动 `git commit` / `git push` / 打 tag / 部署。
- 不评估 agent prompt 的输出质量(持续进化机制 + Check agent 负责)。
- 不动 ui-guidelines.md 正文(只改头注一行维护者署名)。
- 不新建 CONTRIBUTING.md 或其他对外文档(readme-writer 形态约定进 product skill,不在此票落地对外 README 改动)。

## Further Notes

- **回滚成本低**:全是文档/agent 定义/hook 脚本文件,git 可一键回退;不涉及代码与测试。
- **风险点**:collaboration.md 调用网表是协作一致性的单一事实源,改它须一次改全(7→3 + 5 组合表),否则调用网与实际派发漂移;落地步骤 ② 必须与 ③ 同批。
- **关键裁决记录**:
  - Plan agent 保留对 ui-guidelines.md 的写权限(方案 A),在 agent 定义里硬约束写权限边界到该单文件;权衡了纯只读(方案 B,规范回写交 main 落盘)后选 A,保持 design-owner 原本自回写规范的效率。
  - 横交流程(implement-ticket / review / add-tests)解散进 agent + AGENTS.md + backend skill,不保留为独立 skill,避免「同一纪律写两处」的漂移;纪律源头统一在 AGENTS.md 铁律 + 测试三层 + 工作流。
  - frontend-checker 不再独立,清单并入 Check agent 前端维度 + frontend skill,保持「执法依据」与「执法动作」同处。
- **memory 同步**:落地完成后追加一条 memory 记录「3+5 架构简化」,旧 `ai-org-refactor-2026-07.md`(7 agent 架构)标注 supersede,不删。
