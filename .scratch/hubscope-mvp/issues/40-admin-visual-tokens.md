# 40 — Admin 及管理组件视觉收拢

**What to build:** Admin 页及 HubManager、ModelAdder、SettingsPanel、AuditLogs、ClassificationRules、EndpointTable 全面 token 化;表单/弹窗全部走 Element Plus 原组件 + token;破坏性操作确认样式统一(ElMessageBox.confirm);凭证脱敏展示(后 4 位)样式不弱化。规格:docs/specs/0003-ui-redesign.md 批 5。

**Blocked by:** 35 — 全局壳

**Status:** blocked

- [ ] 表单/弹窗无自造控件,全部 Element Plus + token
- [ ] 凭证脱敏展示不弱化(W6)
- [ ] 禁用/删除二次确认路径不变
- [ ] 无调色板外硬编码色值残留
- [ ] typecheck + build 通过;frontend-checker 全项 PASS
