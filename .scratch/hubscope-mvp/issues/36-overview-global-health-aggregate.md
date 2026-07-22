# 36 — overview API 扩展全局健康聚合

**What to build:** `GET /api/overview` 响应新增全局聚合字段:`availability_24h`(全局 24h 可用率,按探测次数加权,口径与既有分组可用率一致)与启用端点总数,供 Dashboard 健康横幅展示「全部正常 / N 个端点异常 + 24h 可用率」。口径定义先于实现;TDD 黑盒测试走 W1 唯一接缝(HTTP API 层 + stub Hub + 假时钟 + 真 SQLite 临时库)。规格:docs/specs/0003-ui-redesign.md §5.1 数据来源节。

**Blocked by:** None — can start immediately

**Status:** done

- [ ] 响应含全局 availability_24h 与启用端点总数,禁用端点不计入
- [ ] 加权口径与分组可用率一致(按探测次数加权)
- [ ] 空库/无探测数据时字段行为明确(不报错、不产出假数据)
- [ ] 黑盒测试:加权计算正确、禁用端点排除、空数据场景
- [ ] `make test` 全绿
