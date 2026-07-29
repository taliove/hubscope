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

   **门控步进纪律(GH #26 check 登记,ticket 83 flake 后第二次出现):** 「release 后立即 blockCalls 重臂」的门控存在滑窗——重臂前本地快速调用可漏过,冻结点依赖「其后不得再有调用完成」的场景下构造性不确定(并发执行器跨单元连续喂任务时必现)。此类冻结点必须用**按计数放行的门**(每步恰好放行 N 个调用)或**对目标 prompt/模型的专用门**(stub marker 机制),禁止靠 release 后重臂;flaky 修复的证据要求是 `-count=100` 连跑零失败、禁用缓存。**冻结点三律(同轮复验补全):** ① 计数门替代滑窗步进;② 第 N+1 个调用先记账后阻塞(记账与阈值判断同一临界区);③ cancel 后不提前释放门——「cancel 后立即 release」会把中止语义交给 Transport 取消传播与响应到达的竞态(实测 ~0.4%),正确形态是门保持关闭让 ctx 确定性中止,cleanup 再释放。

   **worker 池取消模式(GH #26 check 登记):** `select` 同时监听 `ctx.Done()` 与任务 channel 时,取消后两分支同时就绪会被 Go 随机选择——可能偷领新任务并以已取消 ctx 执行。worker/feeder 循环在 `select` 前必须先 `if ctx.Err() != nil` 预检;「取消后不再领取新任务」必须有确定性测试(门控使取消恰好发生在任务完成与新任务可领之间)。

   **断言面:** 状态码 + 响应体 + 必要的库内副作用(经 API 读回验证,不直查表);错误路径(4xx/5xx、超时、半断流)与成功路径都要覆盖。

   **测试 helper 状态纪律(ticket 83 登记):** 测试 helper 禁止按可复用标识(origin/端口/URL 等)跨测试缓存有状态对象(已登录 client、连接、句柄)——httptest 端口会被 OS 回收复用,按 origin 缓存的 client 会把旧服务器的 session cookie 打到复用端口的新服务器上,造成整包轮换随机 FAIL(401/截断 JSON/DB closed),侵蚀门禁可信度。确需缓存时键必须含测试实例(如 `*testing.T`),并用 `t.Cleanup` 配对清理;包级可变状态只允许不可变常量与类型。

4. **实现**:dto 进 `dto.go`(或对应领域文件),handler 进领域文件,路由注册进 `server.go`;三态可选字段用 `json.RawMessage` 区分 absent/null(既有惯例)。
5. **安全自查**:输入边界校验;错误信息不泄漏内部细节;写接口过鉴权中间件;敏感字段(token、webhook)不回明文。测试数据一律假值(假 token、假口令),触发凭证扫描的值要含 test/fake/example 字样。**兜底盖章谓词一致性(GH #39 check 登记):** 凡「为防搁浅而兜底盖章」的防御分支(如把某对象标 failed),被盖章对象必须先证明确实被本流程迁移过——谓词必须与迁移守卫同谓词(迁移按谓词 P 挑选对象,兜底不得按全集操作),否则兜底本身成为新的误标源。
6. **前端同步**:更新 `web/src/api/types.ts` 与对应 api 模块,交 check agent 前端维度。规则类常量(阈值/系数/档界)若被前端镜像(阈值线、档色等),契约中写明口径与出处,改动时检索前端 utils 同步(数字语义双端同源,见 frontend skill)。
7. **三层测试**:当前功能层(新增黑盒测试全绿)→ 关联功能层(触及模块及调用方包回测)→ 核心闭环层(`make test` 全量:后端全部测试 + 前端 typecheck + build)。
8. **审查**:交 check agent(规范双轴 + 测试三层 + 沉淀建议)后英文 Conventional Commits commit。
