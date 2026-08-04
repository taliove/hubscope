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

// The probe gate is the batch's first visible stage: while it runs the
// node shows live completion instead of a bare checkmark that only
// appears after the fact (2026-08-04 UX ruling).
const probing = computed(() => {
  const d = depth.value
  return d !== null && d.probe_total > 0 && d.probe_done < d.probe_total
})
const probeDone = computed(() => depth.value?.probe_done ?? 0)
const probeTotal = computed(() => depth.value?.probe_total ?? 0)


</script>

<template>
  <div v-if="depth" class="pipeline-strip">
    <div class="pipe-node" :class="probing ? 'active' : 'done'">
      <div class="node-title">跑前预检</div>
      <div class="node-big" v-if="probing">
        {{ probeDone }}<span class="dim">/{{ probeTotal }}</span>
      </div>
      <div class="node-big" v-else>✓</div>
      <div class="node-sub">{{ probing ? '实测每个模型的通断与速度' : '不通的模型已跳过' }}</div>
    </div>
    <div class="pipe-link" />
    <div class="pipe-node active">
      <div class="node-title">答题</div>
      <div class="node-big">
        {{ examDone }}<span class="dim">/{{ examTotal }}</span>
      </div>
      <div class="node-sub">排队 {{ depth.exam_pending }} · 进行中 {{ depth.exam_inflight }}</div>
    </div>
    <div class="pipe-link" />
    <div class="pipe-node active">
      <div class="node-title">裁判打分(3 票)</div>
      <div class="node-big">
          {{ depth.judge_inflight }}<span class="dim"> 进行中</span>
      </div>
      <div class="node-sub">排队 {{ depth.judge_pending }}</div>
    </div>
    <div class="pipe-link" />
    <div class="pipe-node">
      <div class="node-title">出分(取中位数)</div>
      <div class="node-big">
        {{ progress.done }}<span class="dim">/{{ progress.total }}</span>
      </div>
      <div class="node-sub">Run 完成数</div>
    </div>
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
