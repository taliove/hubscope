# 49 — 判分归一化管道与口径版本化

**What to build:** `internal/evaluator/verdict.go` 引入归一化管道(spec 0004 / ADR 0008):exact/contains 比对前归一——trim、去成对引号(`"…"` `'…'` `「…」` `『…』`)、Unicode NFKC、折叠内部空白;大小写保持敏感;regex 模式不归一。管道版本化为 Verdict Profile(`v1` = 仅 trim,`v2` = 完整管道),eval_results 记录口径版本(存量视为 v1,不回刷)。报告/趋势层:同模型结果序列口径版本变化处形成断点,处理与 suite_version 断点一致(照画、标注「判分口径已变更」、不显涨跌箭头、大跌告警跳过)。承重墙关联:W7(评估不可变性)——四问已在 ADR 0008 书面回答;测试断言走 HTTP 接缝,不断言内部状态。

**Blocked by:** None — can start immediately

**Status:** done

- [x] 带引号 `"e"`、全角 `ｅ`、多余空白作答在 exact 下判对
- [x] 大小写差异(如 `Hello` vs `hello`)仍判错
- [x] contains 模式同样过归一管道;regex 不归一
- [x] eval_results 落口径版本字段,存量数据按 v1 渲染
- [x] 口径断裂处趋势断点:不显涨跌、大跌告警跳过、标注「判分口径已变更」
- [x] 黑盒测试覆盖上述全部(HTTP 接缝 + stub Hub + 假时钟)
