# 以 Eval Campaign 作为报告与排行榜的聚合单位

一键全量评估(所有 chat Model × 所有 Suite)会产生每 Suite 一个 Eval Run。报告、排行榜与"第 N 周考核"需要把这组 Run 视作同一次考核,为此引入 Campaign 实体聚合同轮 Run,而不是按时间窗口隐式推断批次。备选:按时间窗口聚合(规则模糊,定时与手动撞车即乱)、把 Eval Run 改成多 Suite(改动既有 Run 状态机与数据,迁移成本高)。手动单跑一个 Suite 也归入单 Run 的 Campaign,保持数据模型统一。代价:多一张表与一层关系;换来报告语义明确、告警可按 Campaign 对比。
