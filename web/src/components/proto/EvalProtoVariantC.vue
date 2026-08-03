<!-- PROTOTYPE — Variant C「账本」: cost & speed ledger.
     Primary affordance: answering "what did this batch cost, and which
     model is the best value" — cost per point as the sort key. -->
<script setup lang="ts">
import { computed } from 'vue'
import { fmtCost, protoSubjects, protoTotals } from './evalProtoMock'

interface LedgerRow {
  id: string
  score: number | null
  tps: number | null
  examCost: number | null
  judgeCost: number | null
  total: number | null
  perPoint: number | null
  probe: string
  skipped: boolean
}

const rows = computed<LedgerRow[]>(() => {
  const out: LedgerRow[] = protoSubjects.map((m) => {
    const total = m.examCost === null || m.judgeCost === null ? null : m.examCost + m.judgeCost
    const perPoint = total !== null && m.score !== null && m.score > 0 ? total / m.score : null
    return {
      id: m.id,
      score: m.score,
      tps: m.tps,
      examCost: m.examCost,
      judgeCost: m.judgeCost,
      total,
      perPoint,
      probe: m.probeOk ? `✓ ${m.probeSucc} · ${m.probeTps} tps` : `✗ ${m.probeSucc} 不可达`,
      skipped: !m.probeOk,
    }
  })
  // Value ranking: known per-point cost ascending, unknowns/unscored last.
  return out.sort((a, b) => (a.perPoint ?? 9e9) - (b.perPoint ?? 9e9))
})

const maxTps = Math.max(...protoSubjects.map((m) => m.tps ?? 0), 1)
</script>

<template>
  <div class="vc">
    <!-- KPI strip -->
    <div class="kpis">
      <div class="kpi">
        <div class="kpi-label">已花总成本</div>
        <div class="kpi-big">${{ (protoTotals.examCost + protoTotals.judgeCost).toFixed(4) }}</div>
        <div class="kpi-sub">预估全程 ≈ ${{ protoTotals.estTotal.toFixed(2) }}</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">考试成本</div>
        <div class="kpi-big">${{ protoTotals.examCost.toFixed(4) }}</div>
        <div class="kpi-sub">答题调用(被评模型侧)</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">裁判成本</div>
        <div class="kpi-big">${{ protoTotals.judgeCost.toFixed(4) }}</div>
        <div class="kpi-sub">3 票/答案,占比 {{ Math.round((protoTotals.judgeCost / (protoTotals.examCost + protoTotals.judgeCost)) * 100) }}%</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">平均 TPS</div>
        <div class="kpi-big">{{ protoTotals.avgTps }}</div>
        <div class="kpi-sub">output tokens / 答题延迟</div>
      </div>
      <div class="kpi warn">
        <div class="kpi-label">价格未登记</div>
        <div class="kpi-big">{{ protoTotals.priceUnknownModels.length }}</div>
        <div class="kpi-sub">{{ protoTotals.priceUnknownModels.join('、') }} 不计入成本</div>
      </div>
    </div>

    <!-- Value ledger -->
    <div class="panel">
      <div class="panel-title">性价比榜 <span class="hint">按每分成本升序(花一分钱买多少分)</span></div>
      <table>
        <thead>
          <tr>
            <th>#</th><th>模型</th><th>中位数分</th><th>TPS</th>
            <th>考试成本</th><th>裁判成本</th><th>合计</th><th>每分成本</th><th>刺探</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(r, i) in rows" :key="r.id" :class="{ skipped: r.skipped }">
            <td class="rank">{{ r.perPoint === null ? '—' : i + 1 }}</td>
            <td class="mono">{{ r.id }}</td>
            <td class="score">{{ r.score === null ? '—' : r.score.toFixed(2) }}</td>
            <td>
              <div class="tpsbar"><div class="tpsbar-fill" :style="{ width: ((r.tps ?? 0) / maxTps) * 100 + '%' }" /></div>
              <span class="tpsnum">{{ r.tps ?? '—' }}</span>
            </td>
            <td>{{ fmtCost(r.examCost) }}</td>
            <td>{{ fmtCost(r.judgeCost) }}</td>
            <td class="total">{{ r.total === null ? '—' : '$' + r.total.toFixed(4) }}</td>
            <td class="perpoint">{{ r.perPoint === null ? '—' : '$' + r.perPoint.toFixed(4) }}</td>
            <td class="probe" :class="{ bad: r.skipped }">{{ r.probe }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="footnote">
      成本按模型登记表公开牌价估算(输入/输出 token 分项计价),牌价随发版更新,管理员可在设置中覆盖;
      未登记价格的模型成本记「价格未登记」,不按 0 计入合计。TPS 为非流式口径(output tokens / 延迟),无 TTFT。
    </div>
  </div>
</template>

<style scoped>
.vc { display: flex; flex-direction: column; gap: var(--hs-space-4); }

.kpis { display: flex; gap: var(--hs-space-3); }
.kpi {
  flex: 1; background: #fff; border: 1px solid var(--hs-gray-100); border-radius: 12px;
  padding: var(--hs-space-4); display: flex; flex-direction: column; gap: 4px;
}
.kpi.warn { border-color: #ffe1c2; background: #fffaf5; }
.kpi-label { font-size: 13px; color: var(--hs-gray-500); }
.kpi-big { font-size: 28px; font-weight: 700; color: var(--hs-gray-900); }
.kpi-sub { font-size: 12px; color: var(--hs-gray-500); }

.panel { background: #fff; border: 1px solid var(--hs-gray-100); border-radius: 12px; padding: var(--hs-space-4); }
.panel-title { font-size: 15px; font-weight: 600; color: var(--hs-gray-900); margin-bottom: var(--hs-space-3); }
.hint { font-size: 12px; font-weight: 400; color: var(--hs-gray-500); margin-left: 8px; }

table { width: 100%; border-collapse: collapse; font-size: 13px; }
th { text-align: left; color: var(--hs-gray-500); font-weight: 500; padding: 6px 8px; border-bottom: 1px solid var(--hs-gray-100); font-size: 12px; }
td { padding: 8px; border-bottom: 1px solid var(--hs-gray-50); color: var(--hs-gray-800); }
tr.skipped td { color: var(--hs-gray-400); }
.rank { color: var(--hs-gray-400); width: 28px; }
.mono { font-family: ui-monospace, monospace; }
.score { font-weight: 700; color: var(--hs-gray-900); }
.total { font-weight: 600; }
.perpoint { font-weight: 700; color: var(--hs-blue-700); }
.tpsbar { width: 80px; height: 5px; background: var(--hs-gray-100); border-radius: 3px; overflow: hidden; display: inline-block; vertical-align: middle; }
.tpsbar-fill { height: 100%; background: var(--hs-success-base); }
.tpsnum { margin-left: 6px; font-size: 12px; color: var(--hs-gray-500); }
.probe { font-size: 12px; color: var(--hs-success-text-base); }
.probe.bad { color: var(--hs-danger-text-base); }

.footnote { font-size: 12px; color: var(--hs-gray-500); line-height: 1.8; }
</style>
