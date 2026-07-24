# 07 — EvalTriggerDialog 记住上次选择

**What to build:** Task Center 的 `EvalTriggerDialog` 打开时自动选中上次选择的评估集（Suite）和模型。选择通过 localStorage 持久化，跨会话保留。用户重复触发评估时不用每次重新选择相同的 suite 和模型，减少点击次数。注意：当从详情页传入 `preselectedModelId` 时（ticket 04），不应用 localStorage 的模型记忆（因为模型已被外部锁定）。

**Blocked by:** None — can start immediately（独立优化，不依赖其他 ticket）

**Status:** done

- [x] `web/src/components/EvalTriggerDialog.vue` 新增 `lastSuiteId` 和 `lastModelIds` 两个 ref，从 localStorage 读取初始值（key: `eval-last-suite` 和 `eval-last-models`）
- [x] `suiteId` 的初始值改为 `lastSuiteId.value || ''`（如果 localStorage 有上次选择且该 suite 仍在 `enabledSuites` 中，则默认选中）
- [x] `selectedModelIds` 的初始值改为 `lastModelIds.value || []`（如果 localStorage 有上次选择且这些模型仍在 `models` 中，则默认选中）；但当 `preselectedModelId` 存在时，忽略 localStorage、直接用 `[preselectedModelId]`
- [x] 用 `watch([suiteId, selectedModelIds], ...)` 监听选择变化，debounce 500ms 后写入 localStorage（避免每次点击都写）
- [x] localStorage 写入逻辑：`localStorage.setItem('eval-last-suite', suiteId.value)` 和 `localStorage.setItem('eval-last-models', JSON.stringify(selectedModelIds.value))`
- [x] 对话框关闭时不清空记忆（下次打开仍生效）
- [ ] 实机验证：Task Center 触发评估选 suite A + model X，关闭对话框，再打开时默认选中 suite A 和 model X；从详情页触发评估时模型被锁定、不受 localStorage 影响
