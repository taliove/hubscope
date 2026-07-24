# 05 — 单模型分享卡

**What to build:** `StatusCard.vue` 新增 `variant="single-model"` 支持，渲染单模型卡片（不同于分组卡片）。卡片顶部显示模型名/协议/Hub，当前状态（StatusBadge + 原因），24h 探测指标（可用率大数字 + 延迟 + 完整 24h 打点条），评估分（总分 + 各能力 tag 或「暂无评估数据」），底部一句话小结。端点详情页标题旁新增「分享」按钮；Dashboard 列表每行右侧新增分享图标。点击后弹出 `StatusShareDialog`，传入单模型 snapshot，可导出 PNG/复制图片。

**Blocked by:** 02 — Overview API 带评估分（分享卡需要评估分数据），03 — 端点详情页展示评估分（单模型卡片布局复用详情页的指标卡片设计）

**Status:** ready-for-agent

- [ ] `web/src/components/StatusCard.vue` 新增 prop `variant?: 'group' | 'single-model'`（默认 'group'）
- [ ] 当 `variant="single-model"` 时：顶部不渲染 brand section，改为渲染模型名 + 协议 tag + Hub；不渲染 scope chips（单模型没有范围概念）
- [ ] 单模型卡片内容区：状态行（StatusBadge + 原因）→ 指标块（左：可用率+延迟，右：评估分+能力tag）→ 24h 打点条（16px 高，带轴标）→ 小结行（一句话，比如「24h 可用率 98.5%，评估总分 87，表现稳定」）
- [ ] `web/src/utils/statusCardSnapshot.ts` 新增 `createSingleModelSnapshot(entry: OverviewEntry, generatedAt: string, origin: string)`，生成单模型 snapshot（`entries` 数组只有一项，keyword/protocol/status/group 都为空）
- [ ] `web/src/views/EndpointDetailView.vue` 标题行右侧新增 `el-button` 「分享」图标按钮，`@click="shareModel"`
- [ ] `shareModel` 方法：打开 `StatusShareDialog`，传 `snapshot: createSingleModelSnapshot(detail.value, ...)`
- [ ] `web/src/views/DashboardView.vue` 或 `OverviewGroupSection.vue` 的端点行右侧新增 `el-icon` 分享图标（`<Share />`），`@click.stop="shareEndpoint(entry)"`
- [ ] `shareEndpoint(entry)` 方法：打开 `StatusShareDialog`，传 `snapshot: createSingleModelSnapshot(entry, ...)`
- [ ] `StatusShareDialog` 判断 `snapshot.entries.length === 1` 时，内部调用 `StatusCard` 传 `variant="single-model"`
- [ ] 实机验证：从详情页或 Dashboard 行点分享，弹出对话框显示单模型卡片（与分组卡片样式明显不同），能导出 PNG/复制图片
