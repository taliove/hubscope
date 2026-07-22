# 16 — 读接口分档鉴权 + 限流 + 安全加固

**What to build:** 安全加固一层到位:①读接口分档——状态看板所需 GET(overview/models/hubs/endpoint 详情/series/probes)保持公开,管理/评测/设置/告警/审计相关 GET 全部要求登录 session;②内置 per-IP 令牌桶限流(x/time/rate),三档:登录接口 5 次/分/IP(防爆破)、写操作与手动触发 30 次/分、公开 GET 20 次/秒,超限 429;TRUST_PROXY 环境变量决定是否信任 X-Forwarded-For(默认不信);③安全响应头(X-Content-Type-Options、frame-ancestors CSP、Referrer-Policy)、请求体大小限制;④失败登录写入审计日志。

**Blocked by:** 14 — 结构化日志 + 操作审计(失败登录入审计依赖审计设施);建议最后实施避免反复改路由

**Status:** todo

- [ ] 未登录可访问状态看板全部数据,访问管理/评测/设置/审计 GET 返回 401
- [ ] 登录接口 per-IP 限流生效,连续爆破收到 429;写操作与公开 GET 各按档位限流
- [ ] 响应含安全头,超大请求体被拒
- [ ] 失败登录入审计日志(时间/IP/结果)
- [ ] 黑盒测试:经 API 断言分档鉴权 401/200 行为与限流 429 行为
