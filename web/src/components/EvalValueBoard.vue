<!-- EvalValueBoard — the batch's cost-and-value ledger (spec 0020, GH #179
     view C「账本」): a KPI strip over the registry-priced estimate plus a
     per-model value ranking (score vs measured speed vs exam cost, sorted
     by cost per point). Mounted above the cost matrix on the report page's
     cost view. Unknown prices are named, never zeroed. -->
<script setup lang="ts">
import { computed } from 'vue'
import type { CampaignReport } from '@/api/types'

const props = defineProps<{ report: CampaignReport }>()

interface ValueRow {
  model: string
  score: number | null
  tps: number | null
  examCost: number | null
  perPoint: number | null
  probeOk: boolean | null
  probeText: string
}

const estimated = computed(() => props.report.estimated_cost ?? null)

// The KPI totals only render when at least one run is priced: an
// all-unknown batch reads "—" (nothing measured), never "$0.0000"
// (which would read as free).
const priced = computed(() => {
  const e = estimated.value
  if (!e) return false
  return e.exam > 0 || e.judge > 0 || e.unknown_runs === 0
})

const avgTps = computed(() => {
  const vals = (props.report.cost_rows ?? [])
    .map((r) => r.avg_tps)
    .filter((v): v is number => v !== null && v > 0)
  if (vals.length === 0) return null
  return vals.reduce((a, b) => a + b, 0) / vals.length
})

const rows = computed<ValueRow[]>(() => {
  const tpsByModel = new Map<string, { sum: number; n: number }>()
  const costByModel = new Map<string, { sum: number; unknown: boolean }>()
  for (const cr of props.report.cost_rows ?? []) {
    if (cr.avg_tps !== null) {
      const t = tpsByModel.get(cr.model_id) ?? { sum: 0, n: 0 }
      t.sum += cr.avg_tps
      t.n++
      tpsByModel.set(cr.model_id, t)
    }
    const c = costByModel.get(cr.model_id) ?? { sum: 0, unknown: false }
    if (cr.exam_cost === null) c.unknown = true
    else c.sum += cr.exam_cost
    costByModel.set(cr.model_id, c)
  }
  const probe = props.report.jury?.probe ?? {}
  const out: ValueRow[] = props.report.rows.map((r) => {
    const t = tpsByModel.get(r.model_id)
    const c = costByModel.get(r.model_id)
    const examCost = c && !c.unknown ? c.sum : null
    const score = r.total_score
    const p = probe[r.model_id]
    return {
      model: r.model_id,
      score,
      tps: t && t.n > 0 ? t.sum / t.n : null,
      examCost,
      perPoint: examCost !== null && score !== null && score > 0 ? examCost / score : null,
      probeOk: p ? p.ok : null,
      probeText: p ? (p.ok ? `✓ ${p.succ}/${p.rounds} · ${p.tps.toFixed(0)} tps` : `✗ ${p.succ}/${p.rounds} 不可达`) : '—',
    }
  })
  return out.sort((a, b) => (a.perPoint ?? 9e9) - (b.perPoint ?? 9e9))
})

const maxTps = computed(() => Math.max(...rows.value.map((r) => r.tps ?? 0), 1))

function fmtCost(v: number | null): string {
  return v === null ? '价格未登记' : `$${v.toFixed(4)}`
}
function fmtScore(v: number | null): string {
  return v === null ? '—' : v.toFixed(1)
}
</script>

<template>
  <div class="value-board">
    <div class="kpis">
      <div class="kpi">
        <div class="kpi-label">估算总成本</div>
        <div class="kpi-big">
          {{ estimated && priced ? `$${(estimated.exam + estimated.judge).toFixed(4)}` : '—' }}
        </div>
        <div v-if="estimated && estimated.unknown_runs > 0" class="kpi-sub warn">
          {{ estimated.unknown_runs }} 个 Run 价格未登记
        </div>
      </div>
      <div class="kpi">
        <div class="kpi-label">考试成本</div>
        <div class="kpi-big">{{ estimated && priced ? `$${estimated.exam.toFixed(4)}` : '—' }}</div>
        <div class="kpi-sub">答题调用(被评模型侧)</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">裁判成本</div>
        <div class="kpi-big">{{ estimated && priced ? `$${estimated.judge.toFixed(4)}` : '—' }}</div>
        <div class="kpi-sub">3 票/答案(裁判团侧)</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">平均 TPS</div>
        <div class="kpi-big">{{ avgTps === null ? '—' : avgTps.toFixed(0) }}</div>
        <div class="kpi-sub">output tokens / 答题延迟</div>
      </div>
      <div class="kpi" :class="{ warn: (estimated?.unknown_runs ?? 0) > 0 }">
        <div class="kpi-label">价格未登记</div>
        <div class="kpi-big">{{ estimated?.unknown_runs ?? 0 }}</div>
        <div class="kpi-sub">未登记 Run 的成本不计入合计</div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-title">性价比榜 <span class="hint">按每分成本升序(花一分钱买多少分)</span></div>
      <table>
        <thead>
          <tr>
            <th>#</th><th>模型</th><th>总分</th><th>TPS</th>
            <th>答题成本</th><th>每分成本</th><th>预检</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(r, i) in rows" :key="r.model">
            <td class="rank">{{ r.perPoint === null ? '—' : i + 1 }}</td>
            <td class="mono">{{ r.model }}</td>
            <td class="score">{{ fmtScore(r.score) }}</td>
            <td>
              <div class="tpsbar">
                <div class="tpsbar-fill" :style="{ width: ((r.tps ?? 0) / maxTps) * 100 + '%' }" />
              </div>
              <span class="tpsnum">{{ r.tps === null ? '—' : r.tps.toFixed(0) }}</span>
            </td>
            <td>{{ fmtCost(r.examCost) }}</td>
            <td class="perpoint">{{ r.perPoint === null ? '—' : `$${r.perPoint.toFixed(5)}` }}</td>
            <td class="probe" :class="{ bad: r.probeOk === false }">{{ r.probeText }}</td>
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
.value-board {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-4);
  margin-bottom: var(--hs-space-4);
}
.kpis {
  display: flex;
  gap: var(--hs-space-3);
}
.kpi {
  flex: 1;
  background: #fff;
  border: 1px solid var(--hs-gray-100);
  border-radius: 12px;
  padding: var(--hs-space-3) var(--hs-space-4);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.kpi-label {
  font-size: 12px;
  color: var(--hs-gray-500);
}
.kpi.warn {
  border-color: #ffe1c2;
  background: #fffaf5;
}
.kpi-big {
  font-size: 24px;
  font-weight: 700;
  color: var(--hs-gray-900);
}
.kpi-sub {
  font-size: 12px;
  color: var(--hs-gray-500);
}
.kpi-sub.warn {
  color: var(--hs-warning-text-base);
}
.panel {
  background: #fff;
  border: 1px solid var(--hs-gray-100);
  border-radius: 12px;
  padding: var(--hs-space-4);
}
.panel-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--hs-gray-900);
  margin-bottom: var(--hs-space-3);
}
.hint {
  font-size: 12px;
  font-weight: 400;
  color: var(--hs-gray-500);
  margin-left: 8px;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
th {
  text-align: left;
  color: var(--hs-gray-500);
  font-weight: 500;
  padding: 6px 8px;
  border-bottom: 1px solid var(--hs-gray-100);
  font-size: 12px;
}
td {
  padding: 8px;
  border-bottom: 1px solid var(--hs-gray-50);
  color: var(--hs-gray-800);
}
.rank {
  color: var(--hs-gray-400);
  width: 28px;
}
.mono {
  font-family: ui-monospace, monospace;
}
.score {
  font-weight: 700;
  color: var(--hs-gray-900);
}
.perpoint {
  font-weight: 700;
  color: var(--hs-blue-700);
}
.tpsbar {
  width: 80px;
  height: 5px;
  background: var(--hs-gray-100);
  border-radius: 3px;
  overflow: hidden;
  display: inline-block;
  vertical-align: middle;
}
.tpsbar-fill {
  height: 100%;
  background: var(--hs-success-base);
}
.tpsnum {
  margin-left: 6px;
  font-size: 12px;
  color: var(--hs-gray-500);
}
.probe {
  font-size: 12px;
  color: var(--hs-success-text-base);
}
.probe.bad {
  color: var(--hs-danger-text-base);
}
.footnote {
  font-size: 12px;
  color: var(--hs-gray-500);
  line-height: 1.8;
}
</style>
