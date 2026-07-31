<template>
  <!-- Alert event history table (extracted from SettingsPanel in GH #112,
       spec 0018 IA: the alerts history moves out of the settings area to the
       first-class 故障记录 page). Shared by AlertsView and SettingsPanel —
       the settings card still embeds it under 近期告警事件 until T11. -->
  <div class="alert-history">
    <el-alert
      v-if="error"
      type="error"
      :closable="false"
      :title="`加载失败:${error}`"
      class="history-error"
    >
      <el-button size="small" @click="reload">重试</el-button>
    </el-alert>
    <el-table v-else v-loading="loading" :data="alerts" size="small" empty-text="暂无告警事件">
      <el-table-column prop="kind" label="类型" width="110">
        <template #default="{ row }">
          <el-tag :type="alertKindTagType(row.kind)" size="small">{{ alertKindLabel(row.kind) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="厂商" width="120">
        <template #default="{ row }">
          <!-- Vendor family name rides group_key (spec 0017 group alerts);
               blank on every endpoint- or hub-scoped event. -->
          <span v-if="row.group_key" class="vendor-cell" :title="row.group_key">{{ row.group_key }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="message" label="内容" min-width="240" show-overflow-tooltip />
      <el-table-column label="发送" width="80">
        <template #default="{ row }">
          <span v-if="row.kind === 'score_drop_skipped'" class="sent-skip">未发送</span>
          <span v-else :class="row.sent_ok ? 'sent-ok' : 'sent-fail'">{{ row.sent_ok ? '成功' : '失败' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { listAlerts, type AlertEvent } from '@/api/settings'
import { formatTime } from '@/utils/format'
import { alertKindLabel, alertKindTagType } from '@/utils/alertKind'

// Kind word/tag mapping lives in utils/alertKind.ts (GH #68): the alert
// event vocabulary is a single source of truth, never component literals.
const alerts = ref<AlertEvent[]>([])
const loading = ref(false)
const error = ref('')

async function reload() {
  loading.value = true
  error.value = ''
  try {
    alerts.value = await listAlerts()
  } catch (err) {
    error.value = (err as Error).message
    ElMessage.error(error.value)
  } finally {
    loading.value = false
  }
}

// SettingsPanel refreshes the embedded table after a Lark test send (the
// attempt lands in the history as kind="test").
defineExpose({ reload })

onMounted(() => {
  void reload()
})
</script>

<style scoped>
.history-error {
  margin-bottom: var(--hs-space-3);
}
/* Delivery outcome maps to the semantic status palette. */
.sent-ok {
  color: var(--hs-success);
}
.sent-fail {
  color: var(--hs-danger);
}
/* A recorded-but-unsent event (skipped comparison) reads neutral. */
.sent-skip {
  color: var(--hs-text-secondary);
}
/* Vendor family name (spec 0017 group alerts): long names truncate with
   title hover carrying the full string. */
.vendor-cell {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
