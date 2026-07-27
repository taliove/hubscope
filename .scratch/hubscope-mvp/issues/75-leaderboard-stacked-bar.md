# 75 — 评估榜单堆叠条榜单行(Leaderboard 行重构)

**What to build:** 按 spec 0007(grill 共识 2026-07-27)把 Leaderboard 行从「总分长条 + 行下 5 个 mini bar strip」两层结构重构为**单条紧凑堆叠条**:各维度段按 `report.suites` 固定顺序从左连续堆叠,段宽 = `score_i × (weight_i / Σweights)`(归一化到总分刻度,条总长与右侧总分数字构造性相等,不产生第二口径),未得分段(suite_scores 为 null)不占宽度、右端统一留空;段色按既有分数档位(≥80 success / ≥50 warning / <50 danger),段间 1–2px 分隔,轨道 `--hs-brand-soft`,条高 20px。段内宽度足够(阈值约 ≥44px,落地定稿)居中显示 `formatScore` 分数(白字压色段,`--hs-text-xs`/600),窄段省略;每段 hover tooltip 恒有「能力点名 · 分数 · 判分 X/Y 题 · 采样 N 次」;覆盖率不满(done 且 judged<expected)的段内分数后随「·8/10」压缩水印(宽度够时),窄段进 tooltip。**live 模式**:已判分段照常堆叠,条右端灰字「N 个维度进行中」(placeholder 色),有失败追加「· N 个失败」(danger 色),不占条宽不进总分;名次 `–`、字典序、禁用排序/切换、隐藏涨跌列等既有 live 约束不变。**维度切换**:切到某能力点 = 行按该维度重排(现有服务端语义)+ 该段不透明、其余段 ~40% 透明 + 右侧数字换该维度分;「总分」视图 = 全段不透明 + 总分 + 涨跌列。堆叠条抽成共享组件 `ScoreStackBar.vue`(props:row/suites/weights/highlight 键/静态模式),Leaderboard 行消费,ticket 76 的 EvalCard 复用——**这是两票之间的共享接缝,必须先抽对**。行点击下钻 ModelTrendDialog、键盘可达、hover 反馈、空态/失败态文案全部不变;行下 mini bar strip 及其样式整体移除。

**Blocked by:** 无(spec 0007 已评审通过)

**Status:** done

## 执行顺序(票内多 commit 拆分,单 commit ≤8 文件)

1. **utils commit**:`web/src/utils/stackSegments.ts` 纯函数(段宽加权归一/零宽段/档位色/live 计数/水印判定)+ `stackSegments.test.ts`(vitest,参照 statusCardSummary.test.ts 先例)
2. **组件 commit**:`ScoreStackBar.vue` 共享组件 + Leaderboard 行接入(settled 模式先行,行重构、strip 移除)
3. **live + 切换 commit**:live 标注(进行中/失败计数)、维度切换高亮(~40% 透明)+ 右侧数字切换
4. **规范 commit**:ui-guidelines §5 改写 ticket 51/52 两条为堆叠条形态、覆盖率水印条款修订登记(宽度自适应可见 + tooltip 兜底)、ScoreStackBar 组件登记;spec 0007 Status 改 accepted

## 验收清单

- [ ] 每行单层:名次 + 模型名 + family tag + 堆叠条 + 分数 + 涨跌箭头;行下 mini bar strip 零残留(模板与样式)
- [ ] 段宽 = 分数 × 归一化权重,条总长与总分数字一致(等权与不等权各验一例);未得分段零宽、右端留空
- [ ] 段内数字宽度阈值生效(宽段显示/窄段省略);hover tooltip 含能力点名/分数/判分 X-Y 题/采样 N 次
- [ ] 覆盖率不满段带「·8/10」压缩水印(宽度够时),满覆盖不显示
- [ ] live 模式:进行中/失败灰字标注正确,不占条宽;名次 `–`、禁用切换、隐藏涨跌列等回归
- [ ] 维度切换:重排 + 高亮段不透明/其余 ~40% + 右侧数字换维度分;回总分视图恢复
- [ ] 行点击/Enter/Space 下钻 ModelTrendDialog 回归;空态/失败态文案回归
- [ ] 暗色主题下堆叠条三档色、轨道底、段内白字可分辨
- [ ] `stackSegments.test.ts` 覆盖:加权归一、全 null、单 suite、Σweights=0 防御、档位色边界(80/50)、live 计数、水印判定
- [ ] ui-guidelines §5 两条改写 + ScoreStackBar 登记落地;spec 0007 Status=accepted
- [ ] `make test` 全绿;typecheck/build 通过

## 风险登记

1. **权重口径**:段宽乘权重是 spec 0007 的正确性约束(条长 ≡ 总分),但当前数据 5 能力点等权,不等权场景无真实数据可验——单测构造不等权用例覆盖,手测留意「条长略短于直觉」是否符合总分数字
2. **窄段数字省略阈值**(≥44px)是目测值,落地后按 15 模型真实数据校一遍,必要时调阈值并回写规范
3. 维度切换的 ~40% 透明在暗色下可能显得「段消失」,check 环节暗色抽查,必要时改降饱和而非降透明(规范修订项,不在本票硬扛)
