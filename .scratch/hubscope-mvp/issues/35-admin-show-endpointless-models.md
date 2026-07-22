# 35 — 管理后台可见并管理无端点模型

**What to build:** 管理后台的模型/端点表按 `model.endpoints` 展开,零 Endpoint 的模型(active、capability=chat、端点被全部删除或全部停用建不出)在页面上完全不可见:不能查看、不能删除、不能重新触发协议试探。需要让这类模型在管理后台可见并可管理:列表中展示(端点列为空或标注「无端点」),支持删除模型、支持手动重新触发协议试探补建端点。线上实例已出现实际案例(手动登记的 kimi-ki 端点被删后,UI 无任何入口清理该模型,还曾被全量评估空跑——评估侧已在 e92e153 修复)。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] 零 Endpoint 模型出现在管理后台模型/端点列表中,有明确的「无端点」标识
- [ ] 零 Endpoint 模型可删除(沿用 manual 才可删的口径,discovered 模型给禁用入口)
- [ ] 可对零 Endpoint 模型重新触发协议试探(补建试通的端点)
- [ ] 黑盒测试:建模型→删光端点→模型列表 API 仍返回该模型且 endpoints 为空数组(此条或已成立,需钉住);前端展示层由类型检查+构建保证
