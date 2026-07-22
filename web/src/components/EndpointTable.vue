<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>Endpoint 列表</span>
        <el-button size="small" :loading="pruning" @click="onPruneDead">清理无效端点</el-button>
      </div>
    </template>

    <el-table :data="rows" v-loading="loading" size="small" empty-text="暂无 Endpoint">
      <el-table-column label="Hub" prop="hubName" width="140" show-overflow-tooltip />
      <el-table-column label="模型 ID" prop="modelName" min-width="180" show-overflow-tooltip />
      <el-table-column label="厂商" width="110">
        <template #default="{ row }">
          <el-tag :type="row.modelFamily === 'other' ? 'info' : 'primary'" size="small" plain>
            {{ row.modelFamily }}
          </el-tag>
        </template>
      </el-table-column>
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
      <el-table-column label="操作" width="340" fixed="right">
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
          <el-button
            type="danger"
            size="small"
            :loading="deletingId === row.endpoint.id"
            @click="onDeleteEndpoint(row)"
          >
            删端点
          </el-button>
          <el-button
            v-if="row.modelOrigin === 'manual'"
            type="danger"
            size="small"
            plain
            :loading="deletingModelId === row.modelDbId"
            @click="onDeleteModel(row)"
          >
            删模型
          </el-button>
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
import { ElMessage, ElMessageBox } from 'element-plus'
import { triggerProbe, listProbeHistory } from '@/api/probes'
import { deleteEndpoint, pruneDeadEndpoints } from '@/api/endpoints'
import { deleteModel } from '@/api/models'
import type { ProbeRecord } from '@/api/types'
import type { EndpointRow } from '@/composables/useAdminData'
import ProbeRecordTable from './ProbeRecordTable.vue'

// Props: flattened endpoint rows produced by the parent view.
defineProps<{ rows: EndpointRow[]; loading: boolean }>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const probingId = ref<number | null>(null)
const deletingId = ref<number | null>(null)
const deletingModelId = ref<number | null>(null)
const pruning = ref(false)
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

async function onPruneDead() {
  try {
    await ElMessageBox.confirm(
      '将删除所有"从未探测成功且当前停用"的端点及其全部历史(协议试探失败留下的无效端点)。继续?',
      '清理无效端点',
      { type: 'warning', confirmButtonText: '清理', cancelButtonText: '取消' }
    )
  } catch {
    return // user cancelled
  }
  pruning.value = true
  try {
    const { pruned } = await pruneDeadEndpoints()
    ElMessage.success(pruned > 0 ? `已清理 ${pruned} 个无效端点` : '没有需要清理的端点')
    if (pruned > 0) emit('changed')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    pruning.value = false
  }
}

async function onDeleteEndpoint(row: EndpointRow) {
  try {
    await ElMessageBox.confirm(
      `确认删除端点「${row.modelName} / ${row.endpoint.protocol}」?其全部探测历史与告警记录将一并删除。注意:若该协议在 Hub 上实际可用,下次同步会重新试通并补建;只想停止探测请改用"停用"(编辑端点)。`,
      '删除端点',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return // user cancelled
  }
  deletingId.value = row.endpoint.id
  try {
    await deleteEndpoint(row.endpoint.id)
    ElMessage.success('端点已删除')
    emit('changed')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    deletingId.value = null
  }
}

async function onDeleteModel(row: EndpointRow) {
  try {
    await ElMessageBox.confirm(
      `确认删除模型「${row.modelName}」?其全部端点、探测历史与告警记录将一并删除。若该模型仍在此 Hub 的模型列表中,下次同步会以"自动发现"形式重新登记。`,
      '删除模型',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return // user cancelled
  }
  deletingModelId.value = row.modelDbId
  try {
    await deleteModel(row.modelDbId)
    ElMessage.success('模型已删除')
    emit('changed')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    deletingModelId.value = null
  }
}
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
</style>
