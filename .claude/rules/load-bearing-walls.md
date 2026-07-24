# Load-Bearing Walls(承重墙清单)

改错代价极高、撑起系统语义的结构与约定。修改任何一项前,必须按 [AGENTS.md](../../AGENTS.md)「承重墙」一节回答四问(为什么必须改 / 影响哪些调用方 / 有无替代方案 / 回归测试什么),并附 ADR。

## W1. 唯一测试接缝

- **位置:** `internal/server/*_test.go` 的测试模式
- **约定:** 黑盒测试只走 HTTP API 层:httptest + stub Hub + 假时钟 + 真 SQLite 临时库。不 mock 内部模块,不断言内部状态。
- **为何承重:** 全部回归网挂在这一条接缝上;接缝一变,全量测试语义失效。

## W2. 存储层与 Schema

- **位置:** `internal/store/`
- **约定:** SQLite 单连接 `SetMaxOpenConns(1)` 串行化写(防 SQLITE_BUSY);schema 随 `store.Open` 自动迁移,旧库可无脑升级;seed 幂等(按 generation 追踪,不覆盖管理员编辑)。
- **为何承重:** 单二进制交付,数据只此一份;迁移写错即用户数据损坏。

## W3. Endpoint = 模型 × 协议,试通才建

- **位置:** `internal/discovery/`、`internal/prober/`(ADR 0002)
- **约定:** 监控最小单位是模型×协议;只建试通成功的协议端点;discovered 模型禁删(409)只能禁用;同步可补建缺失协议端点。
- **为何承重:** 状态机、评分、告警、报表全部以 endpoint 为粒度;语义一改全链路口径错乱。

## W4. 调度器时钟注入

- **位置:** `internal/scheduler/`(自写调度,非 cron 库)
- **约定:** 所有周期作业走可注入 Clock,测试中可手动推进;真实运行用 `RealClock`。
- **为何承重:** 假时钟是 W1 接缝的一半;换 cron 库则调度行为不可确定性测试。

## W5. 状态机与告警防抖

- **位置:** `internal/status/`、`internal/alerter/`
- **约定:** 红黄绿状态由探测历史推导;告警走 `prober.AfterRound` 钩子(手动/调度同语义)、懒状态重建(重启不重复告警)、发送失败也落事件防重发、全进程单 evaluator/alerter 实例。
- **为何承重:** 误报/轰炸直接损害告警可信度;多实例会产生重复告警。

## W6. 凭证边界

- **位置:** `internal/server`(auth、脱敏)、`cmd/hubscope/admin.go`、`.githooks/`
- **约定:** Hub token 入库脱敏(省略号+后 4 位)、任何接口不回明文;凭证经 `users` 表(bcrypt 哈希)注入,session 签名 key 经 `SESSION_SECRET` env 或 settings 表自动生成(独立 secret,不从密码派生),首个 super_admin 经 CLI `hubscope admin create` 创建(见 ADR 0011),后续用户走已认证的管理 API;`ADMIN_PASSWORD` 已移除(不再读取);凭证扫描门禁 fail-closed;Lark webhook 不进日志。
- **为何承重:** 泄露即事故,不可逆。

## W7. 评估不可变性

- **位置:** `internal/evaluator/`、`internal/store/eval.go`(ADR 0005、0007)
- **约定:** Case 不可变(内容修改 = 退役旧 Case + 铸新 Case),Suite 版本化;绝对分制,不做 Elo;裁判失败不计 0 分。
- **为何承重:** 评估的核心价值是跨时间可比(防供应商作假);可变题库或相对分制会让纵向对比失效。

## W8. 前端嵌入与单二进制

- **位置:** `web/`(embed 声明)、`Makefile`
- **约定:** `web/dist` 经 `go:embed` 进二进制;`make ensure-dist` 保证首次构建前 embed 目标存在;交付物是单二进制,不引入运行时 node 依赖。
- **为何承重:** 部署模型(单二进制 + systemd/nginx)依赖此约定;拆分会推翻部署文档与打包流程。
