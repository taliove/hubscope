---
name: backend
description: 后端开发流程:契约先行 → 影响分析(调 plan)→ 黑盒 TDD at W1 唯一接缝 → dto/handler/路由 → 输入校验与凭证边界 → 前端同步 → 三层测试。新增后端接口、改后端逻辑、补测试时使用。
---

# 后端开发流程

1. **契约先行**:在对应 ticket / spec 中写明方法、路径、请求/响应 JSON、错误码、鉴权档位(公开 GET / 需登录 / 需管理员,见 ticket 16 分档)。
2. **影响分析**:调 `plan` agent,确认不动承重墙(尤其 W1 测试接缝、W6 凭证边界);dto 字段是否与前端 `web/src/api/types.ts` 冲突。
3. **TDD at 唯一接缝(承重墙 W1)** — 测试一律走 HTTP API 层:httptest + stub Hub + 假时钟 + 真 SQLite 临时库;禁止 mock 内部模块,禁止断言内部状态。先写黑盒 HTTP 测试跑红 → 最小实现跑绿 → 重构。

   **stub Hub 纪律(历史教训):** stub 必须校验请求字段(model、stream、协议格式),否则硬编码字段的实现 bug 测不出来;流式响应要覆盖「200 但零内容」与 reasoning 增量(thinking/reasoning_content)两类真实故障样本。

   **时序:** 任何时间相关断言用假时钟推进,禁止 `time.Sleep`;涉及中途取消的场景用 `blockCalls`/`release` 阻塞门(票 18 引入的惯例)保证确定性。

   **断言面:** 状态码 + 响应体 + 必要的库内副作用(经 API 读回验证,不直查表);错误路径(4xx/5xx、超时、半断流)与成功路径都要覆盖。

   **测试 helper 状态纪律(ticket 83 登记):** 测试 helper 禁止按可复用标识(origin/端口/URL 等)跨测试缓存有状态对象(已登录 client、连接、句柄)——httptest 端口会被 OS 回收复用,按 origin 缓存的 client 会把旧服务器的 session cookie 打到复用端口的新服务器上,造成整包轮换随机 FAIL(401/截断 JSON/DB closed),侵蚀门禁可信度。确需缓存时键必须含测试实例(如 `*testing.T`),并用 `t.Cleanup` 配对清理;包级可变状态只允许不可变常量与类型。

4. **实现**:dto 进 `dto.go`(或对应领域文件),handler 进领域文件,路由注册进 `server.go`;三态可选字段用 `json.RawMessage` 区分 absent/null(既有惯例)。
5. **安全自查**:输入边界校验;错误信息不泄漏内部细节;写接口过鉴权中间件;敏感字段(token、webhook)不回明文。测试数据一律假值(假 token、假口令),触发凭证扫描的值要含 test/fake/example 字样。**结构改动后核「入口覆盖清单」:** 移除构造属性级兜底(如 `http.Client.Timeout`、默认值、全局开关)时,必须枚举该类型全部公开入口,确认每个入口都有等价预算/兜底(GH #31 check 登记:超时改 per-call 后 `ListModels` 漏网,挂死 Hub 可永久占住 discovery 同步锁——与「stub 不校验字段」同属结构改动后覆盖清单漏项,第二次出现故成文)。
6. **前端同步**:更新 `web/src/api/types.ts` 与对应 api 模块,交 check agent 前端维度。规则类常量(阈值/系数/档界)若被前端镜像(阈值线、档色等),契约中写明口径与出处,改动时检索前端 utils 同步(数字语义双端同源,见 frontend skill)。**行为变更检索提示文案(GH #34 check 沉淀):** 改试通/建端点/协议词表/状态词类行为时,grep `web/src` 中相关关键词的提示文案(hint/empty-text/描述行)——行为描述性文案散落在组件里,后端行为变更不同步即失真(ModelAdder 试通提示同类失真第二次发生故成文)。
7. **三层测试**:当前功能层(新增黑盒测试全绿)→ 关联功能层(触及模块及调用方包回测)→ 核心闭环层(`make test` 全量:后端全部测试 + 前端 typecheck + build)。
8. **审查**:交 check agent(规范双轴 + 测试三层 + 沉淀建议)后英文 Conventional Commits commit。
