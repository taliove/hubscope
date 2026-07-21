<template>
  <el-card shadow="never">
    <template #header>
      <span>Endpoint 列表</span>
    </template>

    <el-table :data="rows" v-loading="loading" size="small" empty-text="暂无 Endpoint">
      <el-table-column label="Hub" prop="hubName" width="140" show-overflow-tooltip />
      <el-table-column label="模型 ID" prop="modelName" min-width="180" show-overflow-tooltip />
      <el-table-column label="协议" width="120">
        <template #default="{ row }">
          <el-tag :type="row.endpoint.protocol === 'anthropic' ? 'success' : 'warning'" size="small">
            {{ row.endpoint.protocol }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.endpoint.enabled ? 'success' : 'info'" size="small">
            {{ row.endpoint.enabled ? '启用' : '停用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button
            type="primary"
            size="small"
            :loading="probingId === row.endpoint.id"
            @click="onProbe(row.endpoint.id)"
          >
            立即探测
          </el-button>
          <el-button size="small" @click="onViewHistory(row)">最近记录</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- Probe run result: shows the two records (non-streaming + streaming). -->
    <el-dialog v-model="probeVisible" title="本轮探测结果" width="920px">
      <ProbeRecordTable :records="probeResults" />
    </el-dialog>

    <!-- Recent probe history for a selected endpoint. -->
    <el-dialog v-model="historyVisible" :title="historyTitle" width="920px">
      <ProbeRecordTable v-loading="historyLoading" :records="historyRecords" />
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { triggerProbe, listProbeHistory } from '@/api/probes'
import type { ProbeRecord } from '@/api/types'
import type { EndpointRow } from '@/composables/useAdminData'
import ProbeRecordTable from './ProbeRecordTable.vue'

// Props: flattened endpoint rows produced by the parent view.
defineProps<{ rows: EndpointRow[]; loading: boolean }>()

const probingId = ref<number | null>(null)
const probeVisible = ref(false)
const probeResults = ref<ProbeRecord[]>([])

const historyVisible = ref(false)
const historyLoading = ref(false)
const historyRecords = ref<ProbeRecord[]>([])
const historyEndpoint = ref<EndpointRow | null>(null)

const historyTitle = computed(() => {
  const row = historyEndpoint.value
  if (!row) return '最近探测记录'
  return `最近探测记录 — ${row.modelName} / ${row.endpoint.protocol}`
})

async function onProbe(endpointId: number) {
  probingId.value = endpointId
  try {
    const result = await triggerProbe(endpointId)
    probeResults.value = result.results
    probeVisible.value = true
  } catch (err) {
    // Hub unreachable / no upstream still returns a message rather than crashing.
    ElMessage.error((err as Error).message)
  } finally {
    probingId.value = null
  }
}

async function onViewHistory(row: EndpointRow) {
  historyEndpoint.value = row
  historyVisible.value = true
  historyLoading.value = true
  historyRecords.value = []
  try {
    historyRecords.value = await listProbeHistory(row.endpoint.id, 50)
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    historyLoading.value = false
  }
}
</script>
