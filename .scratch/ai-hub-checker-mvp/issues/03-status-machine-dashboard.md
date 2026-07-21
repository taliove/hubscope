# 03 — 状态机 + 总览 Dashboard

**What to build:** Endpoint 开始有红黄绿状态:最近一次失败为告警态、连续 3 次失败为 Down(红)、24h 成功率 <95% 或 P95 延迟超基线 2 倍为 Degraded(黄)、其余 Healthy(绿)。总览 Dashboard 展示所有 Endpoint 的状态矩阵 + 24h 成功率/P50/P95 延迟摘要,支持按模型名/协议/状态过滤;状态旁可看到判定依据(如"连续 3 次失败,最近错误:no_available_providers")。读接口无需登录。

**Blocked by:** 02 — 定时探测调度

**Status:** done

- [x] 状态机四条规则正确,且每个状态附带人类可读的判定依据
- [x] 总览 API 一次返回全部 Endpoint 的状态与 24h 摘要(成功率、P50、P95)
- [x] Dashboard 状态矩阵渲染红黄绿,支持模型名/协议/状态过滤
- [x] 黑盒测试:用 stub Hub 控制成败序列,推进假时钟,经 API 断言绿→黄→红→恢复的状态迁移与依据文案
