# 44 — 评估运营与题库挪入管理台

**What to build:** 按 spec 0002(2026-07-22 重组定稿)把评估中心的运营与配置职能收进管理台:/admin 新增「评估运营」「题库」两个 tab。评估运营 tab:顶部「触发评估」主按钮打开对话框(单评估集 + 可搜索模型多选,或一键全量),下方批次列表(一行一批、展开看各 Suite Run),Run 详情沿用现有 EvalRunDetailDialog;触发后轮询批次进度直到落定。题库 tab:现有 CaseLibrary 平移并删除 authed prop(整页已有登录门禁)。/eval 移除运营与题库区块,只留得分矩阵与趋势(ticket 31 再整体重写为榜单页)。

**Blocked by:** None — can start immediately(29 已 done)

**Status:** ready-for-agent

- [ ] /admin tabs 顺序:资源 | 分类规则 | 评估运营 | 题库 | 操作日志 | 设置,不加 lazy
- [ ] 触发评估对话框:评估集单选 + 模型多选(可搜索,非对话模型禁选)+ 一键全量入口;触发后轮询批次进度直到落定并刷新列表
- [ ] 批次/运行记录与 Run 详情弹窗在评估运营 tab 内可用
- [ ] 题库 tab 功能与现 CaseLibrary 一致(新增/编辑 Case),authed prop 删除
- [ ] /eval 不再渲染触发表单、运行记录与题库区块;EvalCenterView 过时注释清理
- [ ] 管理台 12px 紧凑档、弹窗表单、三态、ElMessage 反馈等 UI 规范逐项满足
- [ ] make test 全绿(后端全部测试 + 前端 typecheck + 前端构建)
