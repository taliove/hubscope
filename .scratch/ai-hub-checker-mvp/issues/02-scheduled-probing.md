# 02 — 定时探测调度

**What to build:** Probe 从手动变成自动:所有启用的 Endpoint 默认每 5 分钟执行一轮(非流式+流式,同 Endpoint 内串行),Endpoint 之间并发上限 8、单请求超时 60 秒;管理员可为单个 Endpoint 覆盖探测间隔(如贵模型降到 15 分钟)。调度经由 Clock 接口,测试用假时钟手动推进。验证方式:服务启动后无需任何操作,探测记录持续增长,页面刷新可见最新结果。

**Blocked by:** 01 — Walking skeleton:单 Endpoint 手动探测

**Status:** done

- [ ] 启动后启用中的 Endpoint 按间隔自动产生 Probe 记录(默认 5 分钟)
- [ ] 同一 Endpoint 上一轮未完成不会重入;不同 Endpoint 并发执行且不超过上限
- [ ] 单请求超过 60 秒记为超时失败,错误摘要可辨识
- [ ] 可通过写 API 为单个 Endpoint 设置/清除间隔覆盖,下一轮即生效
- [ ] 假时钟测试:推进时间 → 经 API 断言 Probe 记录按预期节奏出现;间隔覆盖与防重入行为正确
