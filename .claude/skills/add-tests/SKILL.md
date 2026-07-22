---
name: add-tests
description: 补测试流程:只走唯一接缝(HTTP API 黑盒),stub Hub 要校验请求字段,时序用假时钟。为新功能补测试或提升覆盖率时使用。
---

# 补测试流程

1. **唯一接缝**(承重墙 W1):测试一律走 HTTP API 层——httptest + stub Hub + 假时钟 + 真 SQLite 临时库。禁止 mock 内部模块,禁止断言内部状态。
2. **stub Hub 纪律**(历史教训):stub 必须校验请求字段(model、stream、协议格式),否则硬编码字段的实现 bug 测不出来;流式响应要覆盖"200 但零内容"与 reasoning 增量(thinking/reasoning_content)两类真实故障样本。
3. **时序**:任何时间相关断言用假时钟推进,禁止 `time.Sleep`;涉及中途取消的场景用 `blockCalls`/`release` 阻塞门(票 18 引入的惯例)保证确定性。
4. **断言面**:状态码 + 响应体 + 必要的库内副作用(经 API 读回验证,不直查表);错误路径(4xx/5xx、超时、半断流)与成功路径都要覆盖。
5. **三层自检**:新测试先单跑,再跑关联包,最后 `make test`;`test-verifier` 代理复核。
6. **测试数据**:一律假值(假 token、假口令),触发凭证扫描的值要含 test/fake/example 字样。
