# 04 — Endpoint 详情页 + rollup 与数据清理

**What to build:** 单个 Endpoint 的详情页:延迟与 TTFT 随时间曲线(可切换流式/非流式)、成功率趋势、近期失败 Probe 列表(含 HTTP 码、错误摘要、时间)。后端引入聚合:小时级 rollup 永久保留,原始 Probe 记录只保留 90 天,由调度器每日清理。排障者凭此页可区分"整体慢"与"卡住不出字",并能拿到错误证据。

**Blocked by:** 03 — 状态机 + 总览 Dashboard

**Status:** ready-for-agent

- [ ] 详情 API 返回曲线数据(按时间桶)、成功率趋势与近期失败列表
- [ ] 页面渲染延迟/TTFT 曲线并可切换流式/非流式
- [ ] 失败记录展示 HTTP 码与错误摘要
- [ ] rollup 每日生成且旧原始数据被清理;rollup 后历史曲线仍可查
- [ ] 假时钟测试:跨天推进后断言 rollup 落库、超期原始记录被删、曲线 API 仍返回 rollup 数据
