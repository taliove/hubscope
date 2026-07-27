---
name: database
description: 数据库变更流程:schema 迁移、seed 幂等、保留字与单连接约束。改表结构、加字段、改 seed 时使用。
---

# 数据库变更流程

承重墙 W2(存储层)适用——先按宪法回答四问再动手。

1. **迁移方式**:schema 随 `store.Open` 自动迁移,旧库必须无脑升级:只加不删(新表 / `ALTER TABLE ADD COLUMN` 带默认值),不改写存量行语义;迁移要幂等(重复执行不出错)。
2. **约束清单**(历史坑):
   - 全程单连接 `SetMaxOpenConns(1)`,不在查询里假设多连接;
   - `trigger` 是 SQLite 保留字,作列名必须加引号;
   - RFC3339 秒级精度,时间排序用 `created_at DESC, id DESC` 打破并列;
   - 时间统一 UTC RFC3339 字符串存储。
3. **seed 变更**:按 generation 追踪(settings 表 `seed_gen_<key>`),只增量补新代,绝不覆盖管理员编辑过的行;删光的默认规则不复活。
4. **TDD**:黑盒测试走 HTTP 接缝验证迁移后行为;如需直测 store,用真 SQLite 临时库,不 mock。
5. **数据隔离**:确认新表/字段不含跨 Hub 泄漏面;含凭证的字段一律不脱敏不出库(回包走 dto 脱敏层)。
6. **验证**:三层测试 + check agent 审查;commit 前用本地真实库 `make dev` 起一次确认迁移无碍。
