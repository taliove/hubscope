# 51 — Nadir 归一化定标与维度分同屏

**What to build:** 按 ADR 0009 / spec 0004 落地跑分式刻度:nadir 常数存于能力点(随 Suite Version 锁定),能力点分 =(case 均分 − nadir)/(1 − nadir)× 100 并 clamp [0,100];总分 = 各能力点分加权平均(沿用 `suite_weights`)。榜单维度分同屏(总分 + 各能力点列,不再只靠 Suite 切换);每个分数带置信标记(判分覆盖率 X/Y 题 + 采样数),覆盖率不满的分数视觉区分;未判分能力点不计入总分(分子分母同剔除)。ADR 0005 内核不动:非 Elo、非排名、确定性。承重墙关联:W7——四问已在 ADR 0009 书面回答;UI 部分过 design-owner 评审并回写 ui-guidelines.md(置信标记、维度分栏)。

**Blocked by:** 49(口径版本)、50(nadir 常数随题库 v3 入库)

**Status:** done

- [x] nadir 归一公式与 clamp 行为正确(含 nadir=0 退化等同旧口径)
- [x] nadir 随 suite_version 锁定,跨版本走断点不比
- [x] 报告 API 同屏返回维度分 + 覆盖率 + 采样数
- [x] 榜单维度分同屏展示,置信标记视觉区分(过 design-owner)
- [x] 未判分能力点不计入总分
- [x] 黑盒测试覆盖上述全部
