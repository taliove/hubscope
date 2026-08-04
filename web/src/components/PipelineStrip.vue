<!-- PipelineStrip — the running batch's two-stage queue state (spec 0020,
     GH #179 view A「管线指挥舱」): probe gate → exam pool → judge pool →
     median aggregate, fed by the report's live queue_depth. Rendered only
     while the batch runs; it disappears at settle. -->
<script setup lang="ts">
import { computed } from 'vue'
import type { CampaignReport } from '@/api/types'

const props = defineProps<{ report: CampaignReport }>()

const depth = computed(() => props.report.queue_depth ?? null)

const examDone = computed(() => {
  if (!depth.value) return 0
  const d = depth.value
  // The exam total is the run cell count: models × suites.
  const total = props.report.rows.length * props.report.suites.length
  return Math.max(total - d.exam_pending - d.exam_inflight, 0)
})
const examTotal = computed(() => props.report.rows.length * props.report.suites.length)

const progress = computed(() => props.report.progress)

interface MonitorRow {
  model: string
  probeOk: boolean | null // null = no probe data (pre-jury batch)
  probeSucc: string
  probeTps: number | null
  examJudged: number
  examExpected: number
  judgeDone: number
  judgeTotal: number
  examCost: number | null
}

// The ops monitor table (prototype A): one row per batch member — probe
// gate outcome, exam progress from the progress cells, judge progress from
// the live queue, measured speed, and the registry-priced exam cost.
const monitorRows = computed<MonitorRow[]>(() => {
  const probe = props.report.jury?.probe ?? {}
  const judgeByModel = new Map<number, { done: number; total: number }>()
  for (const m of depth.value?.models ?? []) {
    judgeByModel.set(m.model_db_id, { done: m.judge_done, total: m.judge_total })
  }
  const costByModel = new Map<string, { sum: number; unknown: boolean }>()
  for (const cr of props.report.cost_rows ?? []) {
    const c = costByModel.get(cr.model_id) ?? { sum: 0, unknown: false }
    if (cr.exam_cost === null) c.unknown = true
    else c.sum += cr.exam_cost
    costByModel.set(cr.model_id, c)
  }
  return props.report.rows.map((r) => {
    const p = probe[r.model_id]
    const judged = r.cells.reduce((a, c) => a + c.judged_cases, 0)
    const expected = r.cells.reduce((a, c) => a + c.expected_cases, 0)
    const j = judgeByModel.get(r.model_db_id)
    const c = costByModel.get(r.model_id)
    return {
      model: r.model_id,
      probeOk: p ? p.ok : null,
      probeSucc: p ? `${p.succ}/${p.rounds}` : '—',
      probeTps: p && p.ok ? p.tps : null,
      examJudged: judged,
      examExpected: expected,
      judgeDone: j?.done ?? 0,
      judgeTotal: j?.total ?? 0,
      examCost: c && !c.unknown ? c.sum : null,
    }
  })
})

function pct(done: number, total: number): number {
  return total === 0 ? 0 : Math.round((done / total) * 100)
}
</script>

<template>
  <div v-if="depth" class="pipeline-strip">
    <div class="pipe-node done">
      <div class="node-title">刺探门控</div>
      <div class="node-big">✓</div>
      <div class="node-sub">不可达模型已跳过</div>
    </div>
    <div class="pipe-link" />
    <div class="pipe-node active">
      <div class="node-title">考试池</div>
      <div class="node-big">
        {{ examDone }}<span class="dim">/{{ examTotal }}</span>
      </div>
      <div class="node-sub">队列 {{ depth.exam_pending }} · 在飞 {{ depth.exam_inflight }}</div>
    </div>
    <div class="pipe-link" />
    <div class="pipe-node active">
      <div class="node-title">裁判池 ×3 票</div>
      <div class="node-big">
          {{ depth.judge_inflight }}<span class="dim"> 在飞</span>
      </div>
      <div class="node-sub">积压 {{ depth.judge_pending }}</div>
    </div>
    <div class="pipe-link" />
    <div class="pipe-node">
      <div class="node-title">中位数聚合</div>
      <div class="node-big">
        {{ progress.done }}<span class="dim">/{{ progress.total }}</span>
      </div>
      <div class="node-sub">Run 完成 · 3 票取中位</div>
    </div>
  </div>

  <!-- Jury strip: the batch's judge panel (GH #179). -->
  <div v-if="depth && report.jury" class="jury-strip">
    <span class="strip-label">裁判团({{ report.jury.policy }}):</span>
    <span v-for="j in report.jury.judges" :key="j" class="judge-chip">{{ j }}</span>
  </div>

  <!-- Ops monitor table: one row per batch member. -->
  <div v-if="depth && monitorRows.length > 0" class="monitor">
    <table>
      <thead>
        <tr><th>模型</th><th>刺探</th><th>考试</th><th>裁判</th><th>TPS</th><th>考试成本</th></tr>
      </thead>
      <tbody>
        <tr v-for="m in monitorRows" :key="m.model" :class="{ skipped: m.probeOk === false }">
          <td class="mono">{{ m.model }}</td>
          <td>
            <span v-if="m.probeOk === null" class="dim2">—</span>
            <span v-else-if="m.probeOk" class="ok">✓ {{ m.probeSucc }}<template v-if="m.probeTps"> · {{ m.probeTps.toFixed(0) }} tps</template></span>
            <span v-else class="bad">✗ {{ m.probeSucc }} 不可达,已跳过</span>
          </td>
          <td>
            <div class="cellbar"><div class="cellbar-fill" :style="{ width: pct(m.examJudged, m.examExpected) + '%' }" /></div>
            <span class="cellnum">{{ m.examJudged }}/{{ m.examExpected }}</span>
          </td>
          <td>
            <template v-if="m.judgeTotal > 0">
              <div class="cellbar"><div class="cellbar-fill judge" :style="{ width: pct(m.judgeDone, m.judgeTotal) + '%' }" /></div>
              <span class="cellnum">{{ m.judgeDone }}/{{ m.judgeTotal }}</span>
            </template>
            <span v-else class="dim2">—</span>
          </td>
          <td>{{ m.probeTps === null ? '—' : m.probeTps.toFixed(0) }}</td>
          <td>{{ m.examCost === null ? '未登记' : '$' + m.examCost.toFixed(4) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.pipeline-strip {
  display: flex;
  align-items: stretch;
  gap: var(--hs-space-2);
  margin-bottom: var(--hs-space-4);
}
.pipe-node {
  flex: 1;
  background: #fff;
  border: 1px solid var(--hs-gray-100);
  border-radius: 12px;
  padding: var(--hs-space-3) var(--hs-space-4);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.pipe-node.active {
  border-color: var(--hs-blue-200);
}
.pipe-node.done {
  border-color: var(--hs-success-base);
}
.pipe-link {
  align-self: center;
  width: 24px;
  height: 2px;
  background: var(--hs-gray-200);
  position: relative;
}
.pipe-link::after {
  content: '';
  position: absolute;
  right: 0;
  top: -3px;
  border-left: 7px solid var(--hs-gray-200);
  border-top: 4px solid transparent;
  border-bottom: 4px solid transparent;
}
.node-title {
  font-size: 12px;
  color: var(--hs-gray-500);
}
.node-big {
  font-size: 24px;
  font-weight: 700;
  color: var(--hs-gray-900);
  line-height: 1.2;
}
.dim {
  color: var(--hs-gray-400);
  font-size: 15px;
  font-weight: 500;
}
.node-sub {
  font-size: 12px;
  color: var(--hs-gray-500);
}
.jury-strip {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--hs-space-2);
  background: #fff;
  border: 1px solid var(--hs-gray-100);
  border-radius: 12px;
  padding: var(--hs-space-2) var(--hs-space-4);
  margin-bottom: var(--hs-space-4);
  font-size: 13px;
}
.strip-label {
  color: var(--hs-gray-700);
  font-weight: 600;
}
.judge-chip {
  background: var(--hs-blue-50);
  color: var(--hs-gray-900);
  border-radius: 8px;
  padding: 3px 10px;
  font-size: 12px;
}
.monitor {
  background: #fff;
  border: 1px solid var(--hs-gray-100);
  border-radius: 12px;
  padding: var(--hs-space-3) var(--hs-space-4);
  margin-bottom: var(--hs-space-4);
}
.monitor table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.monitor th {
  text-align: left;
  color: var(--hs-gray-500);
  font-weight: 500;
  padding: 6px 8px;
  border-bottom: 1px solid var(--hs-gray-100);
  font-size: 12px;
}
.monitor td {
  padding: 8px;
  border-bottom: 1px solid var(--hs-gray-50);
  color: var(--hs-gray-800);
}
tr.skipped td {
  color: var(--hs-gray-400);
}
.mono {
  font-family: ui-monospace, monospace;
}
.ok {
  color: var(--hs-success-text-base);
}
.bad {
  color: var(--hs-danger-text-base);
}
.dim2 {
  color: var(--hs-gray-400);
}
.cellbar {
  width: 90px;
  height: 5px;
  background: var(--hs-gray-100);
  border-radius: 3px;
  overflow: hidden;
  display: inline-block;
  vertical-align: middle;
}
.cellbar-fill {
  height: 100%;
  background: var(--hs-blue-600);
}
.cellbar-fill.judge {
  background: var(--hs-warning-base);
}
.cellnum {
  margin-left: 6px;
  font-size: 12px;
  color: var(--hs-gray-500);
}
</style>
