---
name: frontend
description: 前端开发流程:设计评审(按需调 plan 的 UI 评审子能力)→ 契约核对 → 视图/组件改动 → UI 细节自查 → 类型检查与构建。改 web/ 下任何内容时使用。
---

# 前端开发流程

0. **设计评审(按需)**:若属新增视图/新交互模式/新复用组件/改动语义色或状态表达,先调 `plan` agent 的 UI/UX 设计评审子能力(对照 .claude/rules/ui-guidelines.md),评审意见作为实现约束;其余改动跳过本步。
1. **契约核对**:先确认后端 dto,更新 `web/src/api/types.ts`(单一事实源),再动视图;字段命名/可空与后端严格一致。
2. **改动约束**:沿用既有结构(views / components / api / router);Element Plus 组件优先,不引入新 UI 库;样式 scoped,色板与组件用法遵循 .claude/rules/ui-guidelines.md(语义令牌三层架构 tokens/semantics/ep-theme,页面只消费 semantics 层)。
3. **UI 细节清单**(用户对此敏感,逐项过):
   - 卡片内容溢出与横向滚动条(历史 bug:24h 小点需弹性宽度);
   - 长文本(模型名/Hub 名/错误)截断与 hover 全显;
   - 加载/空/错误三态;
   - 轮询 `setInterval` 配对清理;组件内 tabs 不加 lazy(保轮询事件,见 ticket 19);
   - 汇总卡等可点元素有反馈态(可再点取消,见 ticket fix fc8bdb6)。
4. **验证**:`cd web && pnpm typecheck && pnpm build`;之后交 check agent 的前端维度复核。
5. **闭环**:前端改动也要跑 `make test`(embed 进二进制,构建失败会卡门禁);视觉类改动建议 `make dev` 起服务截图自验。
