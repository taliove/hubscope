# 01 — Walking skeleton:单 Endpoint 手动探测

**What to build:** 项目骨架跑通全链路:管理员能在页面上录入一个 Hub(base URL + token,token 脱敏展示)、手工添加一个模型 ID,系统自动为该模型建立 anthropic 与 openai 两条 Endpoint;点击"立即探测"后,对该 Endpoint 各发一次非流式与流式极简请求,页面上看到本轮 Probe 的成败、HTTP 状态码、错误摘要、总延迟、TTFT、token 用量。Go(chi)+ SQLite(modernc,迁移内嵌)+ Vue 3(构建产物 go:embed)在此票立起,stub Hub + HTTP API 黑盒测试接缝也在此建立。

**Blocked by:** None — can start immediately

**Status:** done

- [ ] 可通过写 API 创建/修改/删除 Hub,任何读接口不回传 token 明文
- [ ] 可通过写 API 手工添加模型,自动建立双协议 Endpoint
- [ ] 页面对单个 Endpoint 触发手动 Probe,展示非流式与流式两条结果(成败/延迟/TTFT/token)
- [ ] Hub 不可达或模型无可用上游时,页面展示 HTTP 码与错误摘要而非崩溃
- [ ] 黑盒测试经 stub Hub 覆盖:建 Hub/模型 → 触发 Probe → 读接口断言记录落库与字段正确
- [ ] `go build` 产出含内嵌前端的单二进制,启动即可用
