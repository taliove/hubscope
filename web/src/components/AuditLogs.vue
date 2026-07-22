<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>操作日志</span>
        <el-select
          v-model="actionFilter"
          placeholder="全部动作"
          clearable
          size="small"
          style="width: 200px"
          @change="onFilterChange"
        >
          <el-option v-for="a in actions" :key="a" :label="a" :value="a" />
        </el-select>
      </div>
    </template>

    <el-table :data="logs" v-loading="loading" size="small" empty-text="暂无操作日志">
      <el-table-column label="时间" width="165">
        <template #default="{ row }">{{ formatTime(row.at) }}</template>
      </el-table-column>
      <el-table-column prop="actor" label="操作人" width="80" />
      <el-table-column prop="ip" label="来源 IP" width="120" show-overflow-tooltip />
      <el-table-column prop="action" label="动作" width="130">
        <template #default="{ row }">
          <el-tag size="small" plain>{{ row.action }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="对象" width="140">
        <template #default="{ row }">
          <span v-if="row.object_id">{{ row.object_type }}#{{ row.object_id }}</span>
          <span v-else>{{ row.object_type || '-' }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="detail" label="详情" min-width="220" show-overflow-tooltip />
      <el-table-column label="结果" width="90">
        <template #default="{ row }">
          <el-tag :type="row.result === 'success' || row.result === 'accepted' ? 'success' : 'danger'" size="small">
            {{ row.result }}
          </el-tag>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        background
        layout="total, prev, pager, next, sizes"
        :total="total"
        :page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        :current-page="page"
        @current-change="onPageChange"
        @size-change="onSizeChange"
      />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { listAuditLogs, listAuditActions } from '@/api/audit'
import type { AuditLog } from '@/api/types'
import { formatTime } from '@/utils/format'

const logs = ref<AuditLog[]>([])
const actions = ref<string[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const actionFilter = ref('')

async function reload() {
  loading.value = true
  try {
    const data = await listAuditLogs({
      page: page.value,
      page_size: pageSize.value,
      action: actionFilter.value || undefined,
    })
    logs.value = data.items
    total.value = data.total
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) {
  page.value = p
  reload()
}

function onSizeChange(size: number) {
  pageSize.value = size
  page.value = 1
  reload()
}

function onFilterChange() {
  page.value = 1
  reload()
}

onMounted(async () => {
  await reload()
  try {
    actions.value = await listAuditActions()
  } catch {
    // The filter dropdown degrades to empty; the table still works.
  }
})
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
</style>
