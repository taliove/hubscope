# 80 — 公开只读端点 GET /api/public/eval/board

**What to build:** 按 spec 0010 新增公开只读端点 `GET /api/public/eval/board`,匿名可访问(不读 session),返回 `{ "report": <最新 settle 批次报告,与 /api/campaigns/{id}/report 同形状> | null, "running": bool }`。report = 最近一次 settle(done/failed)批次的完整报告(总分降序,复用现有报告序列化与排序);无 settle 批次 → null;running = 是否存在 pending/running 批次。端点不接 sort/family 参数(客户端完成)。既有 eval API(campaigns 列表/报告/趋势/触发)维持 session 不变;写操作面零暴露;响应不引入任何新敏感字段(与 /report/:token 分享面信息同级)。

**Blocked by:** 无(spec 0010 已评审通过)

**Status:** done(4 commit:1fe33da → 73e6af5 → 4f9d40a → 1a26372;check 三维度 PASS,W6 白名单精确;make test 稳定性受 ticket 83 存量 flake 影响,另票修)

## 执行顺序(票内 commit 拆分)

1. **W1 黑盒测试先行(跑红):** `internal/server/eval_board_public_test.go`——匿名 200;无 settle → report null + running 正确;多批次取最新 settle(done 与 failed 都算,running/pending 跳过);running 标志两态;响应形状与既有报告一致(rows/suite_scores/weights/baseline)
2. **最小实现跑绿:** handler + 路由注册(公开组,不挂 session 中间件);复用现有报告构建函数,不复制序列化逻辑
3. **重构收尾:** 若报告构建有「按 id 取」与「取最新 settle」的共用段,抽公共函数

## 验收清单

- [ ] 匿名请求 200(无 cookie/无 session)
- [ ] 无 settle 批次:report 为 null;有 settle:返回最新 settle 批次(done/failed 都算)
- [ ] running:存在 pending/running 批次为 true,否则 false
- [ ] 响应形状与 /api/campaigns/{id}/report 一致(前端同构消费);无 sort/family 参数
- [ ] 既有 session 门禁 API 行为零变化(既有测试全绿)
- [ ] `make test` 全绿(install-test 存量失败除外)

## 风险登记

1. **报告构建复用**:现有报告构建若与「按 campaign id」耦合,取「最新 settle」需要新查询——注意 store 层只加查询不改 schema(W2)
2. **信息边界**:响应字段逐一核对,与 /report/:token 分享面同级,不得多出 task/触发/token 类字段
3. **性能**:单次请求一次报告构建,与现有报告页同级,无 N+1 顾虑;若有缓存先例沿用,无先例不新造
