<template>
  <el-card shadow="never" class="admin-card">
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
          <el-tag :type="protocolTagType(row.endpoint.protocol)" size="small">
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
            @click="onDeleteModel(row.modelDbId, row.modelName)"
          >
            删模型
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- Models with zero endpoints produce no row above; this section keeps
         them visible and manageable (re-trial / delete). -->
    <template v-if="endpointlessRows.length > 0">
      <el-divider class="endpointless-divider" />
      <div class="endpointless-header">
        <span class="endpointless-title">无端点模型</span>
        <span class="endpointless-hint">
          以下模型没有任何端点:可重新触发协议试探补建端点,或删除(仅手动添加的模型可删)。
        </span>
      </div>
      <el-table :data="endpointlessRows" v-loading="loading" size="small">
        <el-table-column label="Hub" prop="hubName" width="140" show-overflow-tooltip />
        <el-table-column label="模型 ID" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.model.model_id }}</template>
        </el-table-column>
        <el-table-column label="厂商" width="110">
          <template #default="{ row }">
            <el-tag :type="row.model.family === 'other' ? 'info' : 'primary'" size="small" plain>
              {{ row.model.family }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源" width="110">
          <template #default="{ row }">
            <el-tag :type="row.model.origin === 'manual' ? 'primary' : 'info'" size="small">
              {{ row.model.origin === 'manual' ? '手动添加' : '自动发现' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.model.status === 'active' ? 'success' : 'info'" size="small">
              {{ row.model.status === 'active' ? '活跃' : '已退役' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button
              type="primary"
              size="small"
              :loading="trialingId === row.model.id"
              @click="onTrial(row)"
            >
              重新试探
            </el-button>
            <el-button
              v-if="row.model.origin === 'manual'"
              type="danger"
              size="small"
              plain
              :loading="deletingModelId === row.model.id"
              @click="onDeleteModel(row.model.id, row.model.model_id)"
            >
              删模型
            </el-button>
            <el-tooltip
              v-else
              content="自动发现的模型不可删除;从 Hub 消失后下次同步自动退役"
              placement="top"
            >
              <span class="undeletable-hint">不可删除</span>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
    </template>

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
import { deleteModel, trialModel } from '@/api/models'
import type { ProbeRecord } from '@/api/types'
import { protocolTagType } from '@/utils/protocol'
import type { EndpointRow, EndpointlessModelRow } from '@/composables/useAdminData'
import ProbeRecordTable from './ProbeRecordTable.vue'

// Props: flattened endpoint rows plus endpointless-model rows from the parent view.
defineProps<{ rows: EndpointRow[]; endpointlessRows: EndpointlessModelRow[]; loading: boolean }>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const probingId = ref<number | null>(null)
const deletingId = ref<number | null>(null)
const deletingModelId = ref<number | null>(null)
const trialingId = ref<number | null>(null)
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

async function onDeleteModel(modelDbId: number, modelName: string) {
  try {
    await ElMessageBox.confirm(
      `确认删除模型「${modelName}」?其全部端点、探测历史与告警记录将一并删除。若该模型仍在此 Hub 的模型列表中,下次同步会以"自动发现"形式重新登记。`,
      '删除模型',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return // user cancelled
  }
  deletingModelId.value = modelDbId
  try {
    await deleteModel(modelDbId)
    ElMessage.success('模型已删除')
    emit('changed')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    deletingModelId.value = null
  }
}

// Re-run the protocol trial for an endpointless model: answering protocols
// get a fresh enabled endpoint, failed trials create nothing.
async function onTrial(row: EndpointlessModelRow) {
  trialingId.value = row.model.id
  try {
    const result = await trialModel(row.model.id)
    if (result.created_protocols.length > 0) {
      ElMessage.success(
        `协议试探通过,已为「${row.model.model_id}」补建端点:${result.created_protocols.join('、')}`
      )
      emit('changed')
    } else {
      ElMessage.warning(`协议试探未通过,未创建端点。原因:${result.failures || '未知'}`)
    }
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    trialingId.value = null
  }
}
</script>

<style scoped>
/* Admin density tier: compact 12px card padding (ui-guidelines §2). */
.admin-card {
  --el-card-padding: 12px;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.endpointless-divider {
  margin: 16px 0 12px;
}
.endpointless-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 8px;
}
.endpointless-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.endpointless-hint {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.undeletable-hint {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
  margin-left: 8px;
}
</style>
