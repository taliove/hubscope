# 83 — 修复 authedClient 跨测试缓存导致的整包随机 FAIL

**What to build:** 修复 W1 测试基建缺陷:`internal/server/auth_test.go:51-86` 的 `authedClient` 用 `testClients sync.Map` 按 **origin** 跨测试缓存已登录 client。httptest 端口被 OS 回收复用后,新测试的服务器拿到相同 origin 即命中旧缓存,旧 cookie 对不上新服务器随机生成的 session secret → 随机 401,表现为 `go test ./internal/server` 约 50% 随机 FAIL(失败测试每次不同:TestOverviewWindowStats/TestEvalLatestScores/TestReadAuthTiers 等)。修复方向(check 建议):缓存键改为 server 实例标识(如 t 指针或 db 路径),或去掉缓存每次登录——实施时二选一,倾向「去缓存每次登录」(最简、无共享可变状态;若登录开销实测可感知再改实例键)。同时在 backend skill 或 AGENTS.md 测试纪律补一条:「测试 helper 禁止按可复用标识(origin/端口)跨测试缓存有状态 client」。

**Blocked by:** 无(票外存量,ticket 80 check 发现;证据:双 worktree 对照,base commit d729057 同率复现)

**Status:** done(commit 4032ef3;check 三维度 PASS:3×-count=1 全绿 + -count=2 全绿 + make test exit 0;check 自身纪律「稳定性复验禁缓存」已落 check.md)

## 验收清单

- [ ] `go test ./internal/server` 连跑 5 次全绿(修复前约 50% 随机 FAIL)
- [ ] `go test -count=2 ./internal/server`(端口复用压力)全绿
- [ ] `make test` 全绿(install-test 存量失败除外)
- [ ] 测试纪律文本补登记(AGENTS.md 或 backend skill)
- [ ] 无生产代码行为变化(纯测试基建)

## 风险登记

1. 去缓存会增加每测试一次登录往返——internal/server 包测试基数大,实测总时长变化;若不可接受改实例键方案
2. 同类缓存模式可能不止 authedClient 一处——grep 类似 sync.Map/包级可变状态 helper 一并核
