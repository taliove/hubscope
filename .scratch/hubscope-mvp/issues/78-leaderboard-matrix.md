# 78 — 评估榜单矩阵化(Leaderboard 重构)

**What to build:** 按 spec 0009(grill 共识 2026-07-27,8 项决策)把 Leaderboard 从五色堆叠条重构为**矩阵列式**:`名次 │ 模型(+family tag) │ 总分 │ 涨跌 │ 各维度列`,全表 CSS grid 定宽 + 维度列 flex 等宽,**每列 x 位置全表恒定**(解决 family tag 宽度不一导致的 bar 左缘漂移)。**维度格子**抽共享组件 `ScoreCell.vue`(props:`{ score, cell, staticMode? }`,开票时可微调):`formatScore` 数字(`--hs-text-md`/600,档色着色)+ 其下 4px 档色细条(`--hs-brand-soft` 轨道,`--hs-radius-xs`),**条刻度恒 0–100** 不归一化;档色 ≥80 success / ≥50 warning / <50 danger(阈值不动);null → `–` 占位(placeholder 色)+ 空轨道;覆盖率水印 `·8/10`(同色弱化,宽度够则显)+ hover tooltip 恒兜底「能力点名 · 分数 · 判分 X/Y 题 · 采样 N 次」(ticket 51 口径)。**总分列**:`--hs-text-xl`/600 墨色(不染档色)+ 6px 档色条(比维度粗一档);**涨跌列常显**(settled),行级口径不变(▲绿 ▼红,持平/不可比/无基准 `–`);**前 3 名**名次 `--hs-brand` + 600,其余 secondary。**工具条收敛**:废维度切换 radio 与排序 select,只剩 family 筛选 + baselineNote + 分享按钮;**点列头排序**(带 ↓ 指示;**再点当前列 = 回总分降序**——后端恒降序无方向参数,升序另票,2026-07-27 阶段 1 裁决;走现有 `query.sort` 服务端语义,不改 API)。**live 模式**:已判分格子照常;未判分格子 `–` + 空轨道 + tooltip(进行中/等待中);总分墨色数字 + **空轨道不染档色不填充**(半成品构造上读不成「差」);行尾灰注「N 个维度进行中 · N 个失败」;rank `–`、列头禁点、涨跌整列不渲染;「进度网格 / 实时分数」切换保留工具条首位。行点击/键盘下钻 ModelTrendDialog、空态/失败态文案、settle 转场轮询全部不变。**本票不删 ScoreStackBar**(EvalCard 仍在消费,删除归 ticket 79);SuiteLegend/SuiteRuler 从 Leaderboard 摘除(组件文件留待 79 删)。

**Blocked by:** 无(spec 0009 已评审通过)

**Status:** done(5 commit:c28d2d0 → 2b72cff → 6557507 → ed76f3b → d4154e9;check 三维度 PASS,待用户实机视觉验收)

## 执行顺序(票内多 commit 拆分,单 commit ≤8 文件)

1. **utils commit**:`scoreTier(score)` 档色映射纯函数(stackSegments 内已有档位逻辑则迁移)+ 单测(80/50 边界、null);`liveCounts` 从 stackSegments 迁出(新 util 或保留原地,以 EvalCard/本票引用为准)
2. **ScoreCell + settled 矩阵 commit**:`ScoreCell.vue` + Leaderboard 行/列头重构(grid 定宽、总分列、涨跌常显、前 3 名 brand 色、SuiteLegend/SuiteRuler 摘除)
3. **排序 commit**:列头排序状态机(降→升→切列)纯函数 + 单测 + 列头交互接入;工具条收敛(废 radio/select)
4. **live commit**:live 格子/总分空轨道/行尾灰注/列头禁点/涨跌整列隐藏

## 验收清单

- [ ] 全表列对齐:不同 family tag 宽度(kimi/claude)下每列 x 位置恒定;无 SuiteLegend/SuiteRuler 残留引用(Leaderboard 侧)
- [ ] 维度格子:数字+4px 细条,条刻度 0–100(87.5 条恒短于 100);档色三档正确;null → `–` + 空轨道
- [ ] 覆盖率水印 `·8/10` 宽度够则显、tooltip 恒有完整置信信息(能力点/分数/X-Y 题/采样 N 次)
- [ ] 总分列:20px 墨色大数字不染档色 + 6px 档色条;涨跌列常显,▲▼/– 口径回归;前 3 名 brand teal
- [ ] 点列头排序:↓ 指示、再点当前列回总分降序、切列即换列降序(无升序,另票);live 下列头禁点;`query.sort` 参数不变
- [ ] 工具条只剩 family 筛选 + baselineNote + 分享按钮;维度 radio 与排序 select 零残留
- [ ] live:未判分 `–` + tooltip;总分墨色 + 空轨道无档色;行尾「N 个维度进行中 · N 个失败」;rank `–`;涨跌整列不渲染
- [ ] 行点击/Enter/Space 下钻、空态(含 family 筛选空态/全失败/已删除不上榜)、settle 转场 ElMessage 回归
- [ ] 暗色:档色两主题同值可分辨、占位灰可读、轨道 `--hs-brand-soft` 暗色观感
- [ ] `scoreTier` / 排序状态机 / `liveCounts` 单测绿;`make test` 全绿;typecheck/build 通过

## 风险登记

1. **grid 定宽 vs 内容溢出**:模型列定宽 + 长模型名截断已有先例;总分 100.0(5 字符)与涨跌 `▲ +10.0` 的最宽组合需在 1200px 内容区与 6 个维度列(5 suite 现状 + 预留第 6 列)下实测,必要时压缩维度列数字字号(不允许出现横向滚动,§4 硬约束)
2. **EvalCard 过渡态**:本票交付后 EvalCard 仍用 ScoreStackBar(旧五色),页面/物料短暂不同构——已在 spec 0009 登记,79 收口;不要把本票的范围蔓延到 EvalCard
3. **列头排序 × family 筛选的组合态**:排序状态切列时 family 筛选保持(现有 emitQuery 语义),注意 live→settle 转场后排序列头恢复可点的时序
