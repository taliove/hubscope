<!-- EvalLiveBoard — the running batch's single model table (2026-08-04
     review, view A rework): probe gate, exam and judge progress, live
     per-suite scores and totals, speed, cost and jury — one row per model.
     Row click opens a drawer with the per-suite detail; a suite there
     drills into the full per-case dialog. Replaces the old
     monitor-table + progress-grid + live-leaderboard stack. -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import VendorTile from '@/components/VendorTile.vue'
import type { CampaignReport, ReportRow } from '@/api/types'

const props = defineProps<{ report: CampaignReport }>()
const emit = defineEmits<{
  cellSelect: [payload: { row: ReportRow; suiteKey: string }]
}>()

interface LiveRow {
  row: ReportRow
  model: string
  family: string
  probeOk: boolean | null
  probeSucc: string
  probeTps: number | null
  examJudged: number
  examExpected: number
  judgeDone: number
  judgeTotal: number
  jury: string[]
  examCost: number | null
}

const depth = computed(() => props.report.queue_depth ?? null)

const rows = computed<LiveRow[]>(() => {
  const probe = props.report.jury?.probe ?? {}
  const juries = props.report.jury?.juries ?? {}
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
    const j = judgeByModel.get(r.model_db_id)
    const c = costByModel.get(r.model_id)
    return {
      row: r,
      model: r.model_id,
      family: r.family,
      probeOk: p ? p.ok : null,
      probeSucc: p ? `${p.succ}/${p.rounds}` : '—',
      probeTps: p && p.ok ? p.tps : null,
      examJudged: r.cells.reduce((a, c) => a + c.judged_cases, 0),
      examExpected: r.cells.reduce((a, c) => a + c.expected_cases, 0),
      judgeDone: j?.done ?? 0,
      judgeTotal: j?.total ?? 0,
      jury: juries[String(r.model_db_id)] ?? [],
      examCost: c && !c.unknown ? c.sum : null,
    }
  })
})

function pct(done: number, total: number): number {
  return total === 0 ? 0 : Math.round((done / total) * 100)
}
function fmtScore(v: number | null): string {
  return v === null ? '—' : v.toFixed(1)
}

// Drawer state: the clicked model's per-suite detail.
const drawerRow = ref<LiveRow | null>(null)
const suites = computed(() => props.report.suites)

function cellOf(r: ReportRow, key: string) {
  return r.cells.find((c) => c.suite_key === key) ?? null
}
function cellStatusLabel(status: string): string {
  switch (status) {
    case 'done':
      return '完成'
    case 'running':
      return '进行中'
    case 'failed':
      return '失败'
    default:
      return '等待中'
  }
}
function openSuite(suiteKey: string) {
  if (!drawerRow.value) return
  emit('cellSelect', { row: drawerRow.value.row, suiteKey })
}
</script>

<template>
  <div class="live-board">
    <table>
      <thead>
        <tr>
          <th>模型</th>
          <th>预检</th>
          <th>答题</th>
          <th>裁判</th>
          <th v-for="s in suites" :key="s.key">{{ s.name }}</th>
          <th>总分</th>
          <th>TPS</th>
          <th>答题成本</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="m in rows" :key="m.model" class="model-row" @click="drawerRow = m">
          <td class="mono"><VendorTile :family="m.family" /> {{ m.model }}</td>
          <td>
            <span v-if="m.probeOk === null" class="dim">—</span>
            <span v-else-if="m.probeOk" class="ok">✓ {{ m.probeSucc }}</span>
            <span v-else class="bad">✗ 已跳过</span>
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
            <span v-else class="dim">—</span>
          </td>
          <td v-for="s in suites" :key="s.key" class="score-cell">
            {{ fmtScore(m.row.suite_scores[s.key] ?? null) }}
          </td>
          <td class="total">{{ fmtScore(m.row.total_score) }}</td>
          <td>{{ m.probeTps === null ? '—' : m.probeTps.toFixed(0) }}</td>
          <td>{{ m.examCost === null ? '未登记' : '$' + m.examCost.toFixed(4) }}</td>
        </tr>
      </tbody>
    </table>

    <el-drawer
      :model-value="drawerRow !== null"
      :title="drawerRow?.model ?? ''"
      direction="rtl"
      size="420px"
      @close="drawerRow = null"
    >
      <div v-if="drawerRow" class="drawer-body">
        <div class="d-sec">
          <div class="d-label">预检</div>
          <div v-if="drawerRow.probeOk" class="ok">
            ✓ 通({{ drawerRow.probeSucc }})<template v-if="drawerRow.probeTps"> · {{ drawerRow.probeTps.toFixed(0) }} tps</template>
          </div>
          <div v-else-if="drawerRow.probeOk === false" class="bad">✗ 不通,已跳过</div>
          <div v-else class="dim">—</div>
        </div>
        <div class="d-sec">
          <div class="d-label">裁判团</div>
          <div class="jury-list">{{ drawerRow.jury.join('、') || '—' }}</div>
        </div>
        <div class="d-sec">
          <div class="d-label">各评估集(点击查看逐题作答)</div>
          <div
            v-for="s in suites"
            :key="s.key"
            class="suite-line"
            @click="openSuite(s.key)"
          >
            <span class="suite-name">{{ s.name }}</span>
            <template v-if="cellOf(drawerRow.row, s.key)">
              <span class="suite-status" :class="cellOf(drawerRow.row, s.key)!.status">
                {{ cellStatusLabel(cellOf(drawerRow.row, s.key)!.status) }}
              </span>
              <span class="suite-cov">
                {{ cellOf(drawerRow.row, s.key)!.judged_cases }}/{{ cellOf(drawerRow.row, s.key)!.expected_cases }}
              </span>
            </template>
            <span class="suite-score">{{ fmtScore(drawerRow.row.suite_scores[s.key] ?? null) }}</span>
          </div>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.live-board {
  background: #fff;
  border: 1px solid var(--hs-gray-100);
  border-radius: 12px;
  padding: var(--hs-space-3) var(--hs-space-4);
  margin-bottom: var(--hs-space-4);
  overflow-x: auto;
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
  white-space: nowrap;
}
td {
  padding: 8px;
  border-bottom: 1px solid var(--hs-gray-50);
  color: var(--hs-gray-800);
  white-space: nowrap;
}
.model-row {
  cursor: pointer;
}
.model-row:hover td {
  background: var(--hs-gray-50);
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
.dim {
  color: var(--hs-gray-400);
}
.cellbar {
  width: 80px;
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
.score-cell {
  text-align: center;
}
.total {
  font-weight: 700;
  color: var(--hs-gray-900);
}
.drawer-body {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-4);
}
.d-label {
  font-size: 12px;
  color: var(--hs-gray-500);
  margin-bottom: 4px;
}
.jury-list {
  font-size: 13px;
  color: var(--hs-gray-800);
  line-height: 1.7;
}
.suite-line {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  padding: 8px 6px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
}
.suite-line:hover {
  background: var(--hs-gray-50);
}
.suite-name {
  flex: 1;
  color: var(--hs-gray-900);
}
.suite-status.done {
  color: var(--hs-success-text-base);
}
.suite-status.running {
  color: var(--hs-blue-600);
}
.suite-status.failed {
  color: var(--hs-danger-text-base);
}
.suite-status.pending {
  color: var(--hs-gray-400);
}
.suite-cov {
  color: var(--hs-gray-500);
  font-size: 12px;
}
.suite-score {
  font-weight: 700;
  color: var(--hs-gray-900);
  min-width: 40px;
  text-align: right;
}
</style>
