# 82 — 榜单美化三件套(表格化 / 轨道中性化 / 前三名)

**What to build:** 按 spec 0010 对矩阵榜单做视觉精修,Leaderboard/ScoreCell 层改,/eval、/board、EvalCard 三端自动一致:① **表格化精致**——列头下 1px `--hs-border` 分隔(列头 xs secondary + padding-bottom 8px),行间 1px `--hs-border-light` 分隔(替代纯 gap),行高提到 44–48px(落地定稿);② **轨道中性化**——ScoreCell 细条与总分条轨道 `--hs-brand-soft` → 中性灰(候选 `--hs-border-light` / `--hs-bg-hover`,亮暗双主题观感校准后定稿);③ **前三名仪式感**——行左侧 3px `--hs-brand` 竖条(描边语言,不用背景色块)+ 名次数字升一档(`--hs-text-lg` 600),teal 名次保留;EvalCard 物料行同步竖条(页面/物料同构)。

**Blocked by:** 无(81 已交付)

**Status:** done(5 commit:a2012e0 → 7a40b5f → d5bc4a7 → e2f3d3e → d93a37c;check 打回后修复复核通过)

**随票修(ticket 81 check 遗留 LOW):** ① BoardView.vue:96-136 间距 px 字面量 → `--hs-space-*` 令牌;② ui-guidelines §5 /board 条目补登页头构成(「评估榜单」xl 标题行,ticket 81 偏离②未回写)。

## 执行顺序(票内 commit 拆分,单 commit ≤8 文件)

1. **表格化 commit:** 列头 hairline + 行间 hairline + 行高
2. **轨道 commit:** 轨道令牌替换(亮暗校准)
3. **前三名 commit:** 竖条 + 名次放大 + EvalCard 同步
4. **规范 commit:** ui-guidelines §5 Leaderboard/ScoreCell/EvalCard 条目登记三件套 + 轨道令牌定稿

## 验收清单

- [ ] 列头下 hairline + 行间 hairline 落地,行高 44–48px,无横向滚动回归
- [ ] 轨道中性灰亮暗双主题观感校准(档色三档在新轨道上更跳);定稿令牌回写 §5
- [ ] 前 3 名:3px brand 竖条 + 名次 lg/600;第 4 名起无竖条;live 模式(rank `–`)不渲染竖条
- [ ] EvalCard 物料行同步竖条与行高节奏;导出 PNG 回归(亮主题/复制降级不变)
- [ ] 零硬编码色值、零新色相、零渐变;暗色抽查
- [ ] `make test` 全绿;typecheck/build 通过

## 风险登记

1. **轨道令牌选择**:`--hs-border-light` 在亮主题与 `--hs-bg-hover` 同值(#eff4f5),语义取「浅底」——定稿时选语义最贴的一个并回写规范,不留第二处解释
2. **行高与密度档**:消费页 16px 密度下 44–48px 行高与既有节奏的关系,落地时实机校准(15 行一屏的完整性)
3. **竖条与 hover**:行 hover 已有 `--hs-brand-soft` 底,竖条 + hover 底的叠加观感需实机确认(必要时竖条 hover 加深,仍在 brand 刻度内)
