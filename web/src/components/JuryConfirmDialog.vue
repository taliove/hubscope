<!-- JuryConfirmDialog — the manual batch's pre-flight plan review
     (2026-08-04 ruling): who judges and why, who plays and why. Auto-starts
     at the 60s countdown; the server deadline is authoritative. -->
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { confirmJury } from '@/api/campaigns'
import type { CampaignReport } from '@/api/types'

const props = defineProps<{ report: CampaignReport }>()
const emit = defineEmits<{ close: [] }>()

const secondsLeft = ref(60)
const confirming = ref(false)
let timer: ReturnType<typeof setInterval> | undefined

onBeforeUnmount(() => clearInterval(timer))
watch(
  () => props.report.id,
  () => {
    secondsLeft.value = 60
    clearInterval(timer)
    timer = setInterval(() => {
      secondsLeft.value--
      if (secondsLeft.value <= 0) {
        clearInterval(timer)
        emit('close') // the server auto-starts the batch at the deadline
      }
    }, 1000)
  },
  { immediate: true },
)

interface Participant {
  model: string
  ok: boolean
  succ: string
  tps: number | null
  jury: string[]
}

const participants = computed<Participant[]>(() => {
  const jury = props.report.jury
  return props.report.rows.map((r) => {
    const p = jury?.probe[r.model_id]
    return {
      model: r.model_id,
      ok: p?.ok ?? true,
      succ: p ? `${p.succ}/${p.rounds}` : '—',
      tps: p && p.ok ? p.tps : null,
      jury: jury?.juries[String(r.model_db_id)] ?? [],
    }
  })
})

const policyNames: Record<string, string> = {
  balanced: '均衡(智商/速度/成本兼顾)',
  speed: '速度优先',
  iq: '智商优先',
  cost: '成本优先',
}

async function onConfirm() {
  confirming.value = true
  try {
    await confirmJury(props.report.id)
    emit('close')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    confirming.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="true"
    title="评估计划确认"
    width="640px"
    :close-on-click-modal="false"
    @close="emit('close')"
  >
    <div class="plan">
      <div class="sec">
        <div class="sec-title">裁判团({{ policyNames[report.jury?.policy ?? 'balanced'] ?? report.jury?.policy }})</div>
        <div class="sec-sub">每份作答由 3 位裁判独立打分,取中位数;同一厂商至多一位裁判。逐模型裁判名单见下表。</div>
      </div>
      <div class="sec">
        <div class="sec-title">参与模型(跑前预检实测)</div>
        <table class="ptable">
          <thead>
            <tr><th>模型</th><th>预检</th><th>实测速度</th><th>裁判团</th></tr>
          </thead>
          <tbody>
            <tr v-for="p in participants" :key="p.model" :class="{ skipped: !p.ok }">
              <td class="mono">{{ p.model }}</td>
              <td>
                <span v-if="p.ok" class="ok">✓ 通({{ p.succ }})</span>
                <span v-else class="bad">✗ 不通,已跳过</span>
              </td>
              <td>{{ p.tps === null ? '—' : p.tps.toFixed(0) + ' tps' }}</td>
              <td class="jury-cell">{{ p.jury.join('、') || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <template #footer>
      <span class="countdown">{{ secondsLeft }}s 后自动开始</span>
      <el-button type="primary" :loading="confirming" @click="onConfirm">立即开始</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.plan {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-4);
}
.sec-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--hs-gray-900);
  margin-bottom: 4px;
}
.sec-sub {
  font-size: 12px;
  color: var(--hs-gray-500);
  line-height: 1.6;
}
.ptable {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.ptable th {
  text-align: left;
  color: var(--hs-gray-500);
  font-weight: 500;
  font-size: 12px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--hs-gray-100);
}
.ptable td {
  padding: 6px 8px;
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
.jury-cell {
  font-size: 12px;
}
.countdown {
  margin-right: var(--hs-space-3);
  color: var(--hs-gray-500);
  font-size: 13px;
}
</style>
