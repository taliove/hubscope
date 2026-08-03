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
</style>
