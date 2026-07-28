# 79 — EvalCard 矩阵化 + 堆叠条组件退役 + 规范登记

**What to build:** 按 spec 0009 把榜单分享卡 EvalCard 从五色堆叠条切换为**静态矩阵**(与 ticket 78 页面同构):矩阵行 = 名次(前 3 名 brand 色)+ 模型名(截断)+ 总分(xl 墨色 + 6px 档色条)+ 涨跌(基准可比才渲染整列)+ 维度格子(ScoreCell `staticMode`:无 hover/tooltip,水印按页面同规则渲染,tooltip 置信信息不进物料=已登记信息差)。外框约定一律不动:720px 逻辑宽、2x、恒亮主题、离屏双份捕获、复制降级、20 行封顶 + 收尾行、空筛选中性态、failed 警示行。**范围行 chips 修订**:删「维度」chip(无非总分视图)、删「排序 ≠ 维度键」特判;保留批次(恒列)/ 系列(有筛选)/ 排序(非默认)/ 涨跌基准(恒列,缺失不渲染);顺序:批次 → 系列 → 排序 → 涨跌基准。`buildEvalCardSnapshot` 相应修订(viewSuite 参数退役)。**删除退役**:`ScoreStackBar.vue`、`SuiteLegend.vue`、`SuiteRuler.vue`、`utils/stackSegments.ts` 主体(liveCounts 等已迁出的不留)、`--hs-suite-1..6` token(semantics.css 亮暗两块)、相关测试文件。**规范登记**(plan agent 维护 ui-guidelines.md,本票执行时代为落笔):§5 ScoreStackBar 条目整体改写为矩阵条目(以 spec 0009 为准,ticket 75 段宽/高亮联动约定标 superseded)、ticket 52 live 半成品条目按 spec 0009 修订、ticket 77(五色分类色 + Legend/Ruler)整条废止并登记废止理由(对齐结构性不可能 + 强弱不可见 + 功能色撞车)、EvalCard 条目矩阵构成与 chips 修订、`--hs-suite-*` 从语义令牌表删除;spec 0007 标注「已被 spec 0009 supersede」;spec 0009 Status 改 accepted。

**Blocked by:** 78(ScoreCell 共享接缝与矩阵行先行)

**Status:** done(4 commit:6af4ebb → 86682ab → d26a0c3 → 989345f;check 三维度 PASS,3 条文档级 LOW 修复中,待用户实机验收分享卡)

## 执行顺序(票内多 commit 拆分,单 commit ≤8 文件)

1. **快照 commit**:`buildEvalCardSnapshot` 修订(chips 新集合 + 矩阵行构造 + viewSuite 退役)+ 单测更新(无维度 chip / 排序 chip 特判删除 / 基准恒列 / 20 行封顶 / 空筛选)
2. **EvalCard commit**:矩阵行接入(ScoreCell staticMode + 总分列 + 涨跌条件列),旧堆叠条渲染移除
3. **删除 commit**:ScoreStackBar / SuiteLegend / SuiteRuler / stackSegments.ts / `--hs-suite-*` token / 死测试,全仓 grep 零残留
4. **规范 commit**:ui-guidelines §5 改写 + 废止/修订登记 + spec 0007 supersede 标注 + spec 0009 Status=accepted

## 验收清单

- [ ] EvalCard 矩阵行与页面同构:名次(前 3 brand)/ 模型截断 / 总分 xl + 6px 条 / 维度格子数字+4px 条;档色一致
- [ ] 涨跌列:基准可比才整列渲染;不可比/缺失整列不渲染不占位;行级 ▲▼/– 口径同页面
- [ ] chips:批次恒列;系列/排序/涨跌基准按条件;无「维度」chip 残留;chip 顺序 批次→系列→排序→涨跌基准
- [ ] 静态规则:无 hover/tooltip;水印宽度够则显;20 行封顶 + 「另有 N 个模型未列出」收尾;空筛选 chips 保留 + 中性「暂无匹配模型」
- [ ] 导出回归:暗色会话导出亮主题 PNG;HTTP 裸 IP 复制置灰 + 降级提示、下载可用;文件名 `hubscope-eval-批次N-YYYYMMDD-HHmm.png`
- [ ] 删除零残留:`ScoreStackBar|SuiteLegend|SuiteRuler|stackSegments|hs-suite` 全仓 grep 仅出现在规范文本的历史叙述中(若有)
- [ ] ui-guidelines §5 矩阵条目落地、ticket 77 废止登记、spec 0007 supersede、spec 0009 accepted
- [ ] `make test` 全绿;typecheck/build 通过

## 风险登记

1. **720px 物料宽度的列预算**:名次 + 模型 + 总分 + 涨跌 + 5–6 个维度列在 720px 逻辑宽下比页面(1200px)紧张——维度列窄时数字截断规则需在物料上手测(允许缩小模型列宽,不允许横滚);必要时物料省略 family tag(现有条款已省)并压缩名次/涨跌列
2. **快照兼容**:`EvalCardSnapshot` 结构变更(viewSuite 退役、行构造变化)只影响打开弹窗时新建的快照,无持久化兼容问题;注意 EvalShareDialog 对 snapshot 字段的消费同步改
3. **规范改写的口径风险**:§5 改写涉及 ticket 51/52/75/76/77 五处历史约定,改写时逐条核对「保留/修订/废止」三态,避免静默丢约束(尤其覆盖率水印、分享边界、live 口径)
