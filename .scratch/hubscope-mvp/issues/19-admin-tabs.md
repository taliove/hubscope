# 19 — 管理视图 Tabs 重构

**What to build:** AdminView 由竖排长页改为 el-tabs 分页:"资源"(Hub 管理 + 添加模型 + Endpoint 列表)、"分类规则"、"操作日志"、"设置"四页;各组件事件接线(@changed/@sync-settled/@added 等)与轮询逻辑保持不变,纯前端重组,无 API 变更。

**Blocked by:** 无

**Status:** done

- [x] 管理视图四个 tab 页,资源页含原前三区块
- [x] 各组件功能(增删改/轮询/刷新)在 tabs 下行为不变
- [x] 前端类型检查与构建通过
