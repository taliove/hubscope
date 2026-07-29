# 92 — 覆盖率门槛:榜单前端呈现(三处消费页同口径)

**What to build:** 按 spec 0014 实现决策 A 的前端半,消费 ticket 91 的完整性字段。行为:/eval、/board、/report/:token 三处榜单中,判分不完整的模型——名次列显 `–` 占位(复用 live 模式既有形态,`--hs-text-placeholder`)、总分列显 `–` + 弱化水印「判分不完整 N/M 维度」(N=有分维度数,M=启用维度总数;水印样式对齐 ScoreCell 覆盖率水印的弱化规格:font-weight 400 + opacity 0.85)、行排在完整模型之后;各维度格子照常渲染真实分数(分数 + 档色 + 覆盖率水印不变);涨跌列显 `–`。行下钻、family 筛选、列头排序行为不变;列头排序点击时,不完整模型恒沉底(任何排序键下完整组优先)。/board 的客户端 sortRows 镜像同一口径(禁第二排序口径)。**开工前经 frontend skill 调 plan 做 UI 设计评审**,新呈现约定登记 ui-guidelines.md(Leaderboard 条目增补「判分不完整模式」,与「运行中半成品模式」并列)。

**Blocked by:** 91(覆盖率门槛:后端报告口径)

**Status:** done(f390e1a + 224c7e5;水印 N=missing_suites 按冻结契约,票正文「N=有分维度数」作废;水印位置经设计评审修订为模型列第二行,登记 ui-guidelines 附录 B 第 12 项)

## 验收清单

- [ ] 三处消费页(/eval、/board、/report/:token)不完整模型:名次 `–`、总分 `–` + 「判分不完整 N/M 维度」水印、排完整模型之后
- [ ] 维度格子照常显示真实分数与覆盖率水印(不掩盖已判分事实)
- [ ] 涨跌列显 `–` 占位
- [ ] 任意列头排序键下不完整模型恒沉底;/board 客户端 sortRows 与服务端同口径(vitest 覆盖)
- [ ] 全部模型不完整时榜单照常渲染(无空态文案冒充)
- [ ] live 模式呈现零变化(回归)
- [ ] 设计评审通过 + ui-guidelines.md 登记;typecheck/build 绿;check 三维度 PASS

## 风险登记

1. **两模式相邻易混**:live 半成品(名次 `–`)与 settle 后不完整(名次 `–`)同形态不同语义——文案与水印须可区分,设计评审重点
2. **/board 客户端排序**:sortRows 镜像须随服务端口径同步改,单测覆盖「不完整沉底 × 各排序键」
3. **EvalCard/StatusCard 物料**:静态物料若渲染不完整模型,水印规则与页面一致(staticMode),设计评审一并过
