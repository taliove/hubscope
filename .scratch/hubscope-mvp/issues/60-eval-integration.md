# Ticket 60: 评估体系集成到状态板与分享

Status: planned

## 背景

现状：
- 探测体系（probe：可用率/延迟/状态）和评估体系（eval：题库打分/能力维度）完全隔离
- 端点详情页只有探测侧数据（图表 + 失败记录），看不到「智商」
- 分享卡只有探测侧指标，看不到评估分
- 单模型评估触发要进 Task Center → 选评估集 → 选模型两步，详情页没有快捷入口
- 分组分享卡的正常端点只有一行汇总（「2 个端点正常，24h 可用率 98.5%–99.2%」），看不出各端点的具体表现

用户诉求（2026-07-23）：
1. 分组分享展示组内所有端点的「稳定在线」+「智商在线」状态（太多则取前 N 项 + 汇总行兜底）
2. 单个模型可以独立分享（带探测 + 评估）
3. 单模型评估触发更方便（不想每次都选模型）
4. 端点详情页加入评估分（「智商稳定性」）

## 决策记录（grilling 2026-07-23）

### Q1: 评分语义
- **结论**：榜单守 ADR 0009（Nadir 归一化 clamp [0,100]）不动；展示侧可派生指标但**先不合成**「智商稳定性」，并列展示探测分和评估分两个独立值，等真实使用反馈后再定合成公式。
- **理由**：ADR 0009 的防作假内核（纵向可比、刻度语义）不能为展示便利推翻；「智商稳定性」合成公式（加权？乘法罚分？两档制？）现在没有用户验证，先并列展示原始值、避免过早抽象。

### Q2: 单模型评估数据源
- **结论**：新增后端 API `GET /api/models/{id}/eval-summary`，返回该模型最近一次评估的汇总（总分 + 各能力分 + 时间戳 + campaign_id）。
- **理由**：端点详情页和单模型分享都需要单模型视角的评估数据，不该为了一个模型去拉整个 campaign 矩阵；且后续单模型分享卡���复用这个接口。

### Q3: 智商稳定性计算公式
- **结论**：先不定公式，**并列展示**探测分（0-100 稳定性）和评估分（0-100 总分）两个独立指标。
- **理由**：加权平均/乘法罚分/两档制等方案都没有真实场景验证，过早合成可能抹掉关键信号或放大噪声；并列展示让用户看清原始值、自己判断，积累反馈后再回来定合成公式。

### Q4: 分组分享内容范围
- **结论**：异常明细保持 cap 10，正常端点**展开列表** cap 15（每条显示名称 + 24h可用率 + 评估分 + 24h打点），超出 15 的用一行兜底汇总。
- **理由**：异常优先级最高必须全列（cap 10 够用）；正常端点现在一行汇总丢失了「哪些模型正常、各自分数多少」的信息，展开有价值；但全展开太长（静态卡片过长影响阅读和传播），cap 15 是平衡点。

### Q5: 单模型分享入口和内容
- **结论**：Dashboard 列表每行 + 端点详情页标题旁都加分享按钮，生成单模型卡片（模型名/协议/Hub + 当前状态 + 24h探测 + 评估分 + 小结）。
- **理由**：两个场景都真实——Dashboard 列表快速截卡、详情页深入看完后分享做记录；两个入口成本低（调同一对话框）且无歧义（卡片相同）。

### Q6: 单模型评估触发便利性
- **结论**：端点详情页控件行加「评估此模型」按钮（模型已锁定，只选 Suite + 可选「一键全量评估」）；Task Center 的 `EvalTriggerDialog` 记住上次选择。
- **理由**：详情页单模型视角，用户看完探测数据想验证智商时从详情页直接触发最自然；Task Center 全局入口保留并优化（覆盖批量评估场景），记住选择减少重复劳动。Dashboard 列表不加评估图标（评估是重操作，不适合列表一键触发）。

### Q7: 端点详情页评估分展示
- **结论**：状态行下方插入「指标卡片行」，左卡片「24h 稳定性 XX 分」（探测 score，三档着色），右卡片「评估总分 XX 分 + 各能力分 tag」或「暂无评估数据」灰卡。
- **理由**：详情页是深入分析场景，稳定性和评估分是两个核心指标，值得显眼位置；卡片形式能承载分级信息（各能力分、时间、无数据状态）；插在状态行和图表之间逻辑清晰。

## 设计方案

### 1. 后端 API

#### 1.1 新增单模型评估汇总接口

```
GET /api/models/{id}/eval-summary
```

**响应**（200 OK）:
```json
{
  "data": {
    "model_id": 123,
    "model_id_str": "gpt-4",
    "campaign_id": 456,
    "campaign_created_at": "2026-07-20T10:00:00Z",
    "total_score": 87.5,
    "suite_scores": [
      {"suite_id": 1, "suite_name": "推理", "score": 90.2, "version": 2},
      {"suite_id": 2, "suite_name": "代码", "score": 85.3, "version": 1}
    ]
  }
}
```

- 无评估记录时返回 `{"data": null}`
- `total_score` 是加权平均后的总分（按 suite 权重，默认等权）
- `suite_scores` 按 suite_id 升序排列
- 只返回**最近一次** campaign 的数据（不返回历史）

**实现要点**：
- `internal/server/models.go` 新增 `handleGetModelEvalSummary`
- `internal/store/campaign.go` 新增 `GetLatestCampaignForModel(modelID int64) (*Campaign, []CampaignSuiteScore, error)`
- 复用既有的 `CampaignSuiteScore` 结构（已有 suite_id/suite_version/score 字段）
- 总分计算复用 `campaign_report.go` 的加权平均逻辑

### 2. 前端组件

#### 2.1 端点详情页（EndpointDetailView.vue）

**新增指标卡片行**（插在状态行和控件行之间）：

```vue
<div class="metrics-row">
  <el-card class="metric-card">
    <div class="metric-label">24h 稳定性</div>
    <div class="metric-value" :class="`tier-${stabilityTier}`">
      {{ detail.score }}<span class="metric-unit">/100</span>
    </div>
    <div class="metric-note">基于探测可用率</div>
  </el-card>
  
  <el-card v-if="evalSummary" class="metric-card">
    <div class="metric-label">评估总分</div>
    <div class="metric-value" :class="`tier-${evalTier}`">
      {{ evalSummary.total_score.toFixed(1) }}<span class="metric-unit">/100</span>
    </div>
    <div class="metric-capabilities">
      <el-tag v-for="s in evalSummary.suite_scores" :key="s.suite_id" size="small">
        {{ s.suite_name }} {{ s.score.toFixed(0) }}
      </el-tag>
    </div>
    <div class="metric-note">{{ formatTime(evalSummary.campaign_created_at) }}</div>
  </el-card>
  
  <el-card v-else class="metric-card metric-card-empty">
    <div class="metric-label">评估总分</div>
    <div class="metric-empty">暂无评估数据</div>
    <el-button size="small" @click="triggerEval">评估此模型</el-button>
  </el-card>
</div>
```

**控件行新增评估按钮**：
```vue
<div class="controls">
  <!-- 既有的窗口选择器和试用按钮 -->
  <el-button @click="triggerEval">评估此模型</el-button>
  <el-button @click="shareModel">分享</el-button>
</div>
```

**script setup**：
```ts
const evalSummary = ref<ModelEvalSummary | null>(null)

async function loadEvalSummary() {
  const id = Number(route.params.id)
  try {
    const resp = await getModelEvalSummary(id)
    evalSummary.value = resp.data
  } catch (err) {
    // 无评估记录时 data 为 null，不报错
    evalSummary.value = null
  }
}

function triggerEval() {
  // 打开 EvalTriggerDialog，传 preselectedModelId = detail.value.id
  // 对话框内模型已锁定、禁用选择器，只能选 Suite
}

function shareModel() {
  // 打开 StatusShareDialog，传单模型 snapshot
}
```

#### 2.2 Dashboard 列表行加分享图标（DashboardView.vue）

在 `OverviewGroupSection.vue` 的端点行右侧加一个小分享图标：

```vue
<div class="endpoint-row">
  <!-- 既有的 StatusBadge、名称、tags、可用率等 -->
  <el-icon class="action-icon" @click.stop="shareEndpoint(entry)">
    <Share />
  </el-icon>
</div>
```

`@click.stop` 防止触发行的折叠/展开。

#### 2.3 单模型分享卡（StatusCard.vue 新 variant）

新增 `variant="single-model"` prop，渲染单模型卡片：

**顶部**：模型名 + 协议 tag + Hub（不是 brand section）
**状态行**：StatusBadge + 原因（镜像详情页）
**指标块**：
- 左：24h 可用率大数字 + 延迟
- 右：评估总分 + 各能力 tag（或「暂无评估数据」）
**24h 打点条**：一根完整的 24 格分段条（高度 16px，带轴标）
**小结行**：一句话总结，比如「24h 可用率 98.5%，评估总分 87，表现稳定」

#### 2.4 分组分享卡正常端点展开（StatusCardDetail.vue）

**当前逻辑**：
```ts
const abnormalEntries = computed(() => entries.filter(e => e.status !== 'healthy'))
const healthyCount = computed(() => entries.length - abnormalEntries.length)
const healthyRangeText = computed(() => {
  const rates = entries.filter(e => e.status === 'healthy').map(e => e.success_rate_24h)
  // ...
  return `${healthyCount.value} 个端点正常，24h 可用率 ${min}–${max}`
})
```

**修改为**：
```ts
const healthyEntries = computed(() => entries.filter(e => e.status === 'healthy'))
const topHealthy = computed(() => healthyEntries.value.slice(0, 15))
const healthyOverflow = computed(() => Math.max(0, healthyEntries.value.length - 15))

// template
<template v-if="topHealthy.length > 0">
  <div class="detail-title">正常端点</div>
  <div v-for="entry in topHealthy" :key="entry.endpoint_id" class="detail-item detail-item-healthy">
    <div class="detail-row">
      <span class="row-name">{{ entry.model_id }} · {{ entry.protocol }}</span>
      <span class="row-rate av-ok">{{ formatPercent(entry.success_rate_24h) }}</span>
      <span v-if="entry.eval_score !== null" class="row-eval">
        评估 {{ entry.eval_score.toFixed(0) }}
      </span>
    </div>
    <div class="row-dots">
      <span v-for="(dot, i) in entry.dots_24h" :key="i" class="dot-slot">
        <span class="dot" :class="`seg-${dotTier(dot.total, dot.failures)}`" />
      </span>
    </div>
  </div>
  <div v-if="healthyOverflow > 0" class="healthy-overflow">
    另有 {{ healthyOverflow }} 个正常端点，24h 可用率 {{ overflowRateRange }}，
    评估分 {{ overflowEvalRange }}
  </div>
</template>
```

**后端改动**：`OverviewEntry` DTO 新增 `eval_score *float64` 字段（从 `GET /api/models/{id}/eval-summary` 的 `total_score` 来），`GET /api/overview` 响应时每个 endpoint 带上最新评估分（无评估记录时为 null）。

#### 2.5 EvalTriggerDialog 优化

**记住上次选择**：
```ts
// localStorage 存上次选择的 suiteId 和 modelIds
const lastSuiteId = ref(localStorage.getItem('eval-last-suite') || '')
const lastModelIds = ref(JSON.parse(localStorage.getItem('eval-last-models') || '[]'))

watch([suiteId, selectedModelIds], () => {
  localStorage.setItem('eval-last-suite', suiteId.value)
  localStorage.setItem('eval-last-models', JSON.stringify(selectedModelIds.value))
})
```

**支持预选模型（从详情页传入）**：
```ts
const props = defineProps<{
  preselectedModelId?: number // 传入时模型已锁定、禁用选择器
}>()

if (props.preselectedModelId) {
  selectedModelIds.value = [props.preselectedModelId]
  // 模型选择器 disabled
}
```

## 验收

- [ ] 后端 `GET /api/models/{id}/eval-summary` 接口可用，返回最新评估汇总或 null
- [ ] 端点详情页状态行下方显示「指标卡片行」：左卡片稳定性分（三档色）、右卡片评估总分+各能力tag 或「暂无评估数据」灰卡
- [ ] 端点详情页控件行有「评估此模型」按钮，点击弹出简化对话框（模型已锁定、只选 Suite）
- [ ] 端点详情页标题旁有「分享」按钮，点击生成单模型卡片（含探测+评估+小结）
- [ ] Dashboard 列表每行右侧有分享图标，点击生成该端点的单模型卡片
- [ ] 分组分享卡正常端点展开为列表（cap 15），每条显示名称+可用率+评估分+24h打点；超出 15 的用汇总行兜底
- [ ] 分组分享卡异常明细保持 cap 10，每条带评估分（如果有）
- [ ] Task Center 的 `EvalTriggerDialog` 打开时默认选中上次的 Suite 和模型
- [ ] 单模型卡片内容完整：模型名/协议/Hub + 状态 + 探测指标 + 评估分 + 小结
- [ ] `GET /api/overview` 响应的每个 endpoint 带 `eval_score` 字段（float64 或 null）

## 相关文档

- ADR 0005: 排行榜用绝对分制而非 Elo
- ADR 0009: Nadir 归一化定标（修正 ADR 0005 的刻度）
- ticket 59: 分组分享 + 卡片重设计（本 ticket 在其基础上扩展评估维度）
- ui-guidelines §5: StatusCard 设计规范（需补充单模型卡片 variant 约定）

## 实现顺序建议

1. 后端 `GET /api/models/{id}/eval-summary` 接口 + `GET /api/overview` 带 eval_score
2. 端点详情页指标卡片行（验证 API 和展示逻辑）
3. 端点详情页「评估此模型」按钮 + EvalTriggerDialog 预选模型支持
4. 单模型分享卡 variant + 两个分享入口（详情页 + Dashboard 行）
5. 分组分享卡正常端点展开（StatusCardDetail.vue 改动）
6. EvalTriggerDialog 记住上次选择

每个子任务独立可测、可增量交付。
