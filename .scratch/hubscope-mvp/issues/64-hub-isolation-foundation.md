# 64 — 按 Hub 隔离地基

**What to build:** 登录的 Hub 级用户只能看本 Hub 数据，匿名/super_admin 看全局；store 层结构性强制不让漏过滤。

**Blocked by:** 63

**Status:** done

- [ ] store 层 `List*` 去掉无参形态，改 `ListXByHub(hubID)` + `ListXAll()`（`All` 仅 super_admin 路径可达）；先落地 `ListModels` 与 `GetOverview(hubID *int64)`（nil=全局）
- [ ] `GET /api/models`：Hub 级用户只返本 Hub model；super_admin 全部
- [ ] `GET /api/overview`：匿名=全局聚合（公开状态板语义不变）；登录且非 super_admin=本 Hub；super_admin=全局（handler 内按 session 分叉）
- [ ] 公开详情 `/api/endpoints/{id}`（及 `/series`/`/probes`/`eval-summary`）不按 Hub 过滤，保持全局可访问
- [ ] isolation sweep 测试骨架（新文件）：seed 两 Hub 各带数据，Hub-A 用户断言所有已隔离 list 接口不返回 Hub-B 数据，super_admin 两边可见；新增 list 接口须加对应断言行
- [ ] `CLAUDE.md`「开工纪律」第 1 条权限与数据隔离风险项增列：新增 list/query handler 必经 hub 过滤，store 层 List 函数签名强制 hubID 非可选
