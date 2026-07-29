<template>
  <el-card shadow="never" class="admin-card">
    <template #header>
      <div class="card-header">
        <span>Hub 管理</span>
        <el-button type="primary" size="small" @click="openCreate">新建 Hub</el-button>
      </div>
    </template>

    <el-table :data="hubs" v-loading="loading" empty-text="暂无 Hub" size="small">
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="base_url" label="Base URL" min-width="240" show-overflow-tooltip />
      <el-table-column label="Token" width="120">
        <template #default="{ row }">
          <span class="token-hint">{{ row.token_hint }}</span>
        </template>
      </el-table-column>
      <el-table-column label="同步状态" width="110">
        <template #default="{ row }">
          <el-tooltip
            :content="row.sync_status === 'failed' ? row.last_sync_error || '同步失败' : ''"
            :disabled="row.sync_status !== 'failed'"
            placement="top"
          >
            <el-tag :type="syncStatusTagType(row.sync_status)" size="small" disable-transitions>
              {{ syncStatusLabel(row.sync_status) }}
            </el-tag>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column label="最近同步" width="165">
        <template #default="{ row }">{{ formatTime(row.last_synced_at) }}</template>
      </el-table-column>
      <el-table-column label="创建时间" width="165">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-button
            link
            type="primary"
            size="small"
            :disabled="row.sync_status === 'syncing'"
            @click="onSync(row)"
          >同步</el-button>
          <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" size="small" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="460px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="Hub 名称" />
        </el-form-item>
        <el-form-item label="Base URL">
          <el-input v-model="form.base_url" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="Token">
          <el-input
            v-model="form.token"
            type="password"
            show-password
            :placeholder="editingId === null ? 'Hub 访问凭证' : '留空表示不修改'"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { createHub, updateHub, deleteHub, syncHub } from '@/api/hubs'
import type { Hub } from '@/api/types'
import { formatTime } from '@/utils/format'

// Props: hub list and loading flag are owned by the parent view.
const props = defineProps<{ hubs: Hub[]; loading: boolean }>()
const emit = defineEmits<{
  (e: 'changed'): void
  // Fired when the last in-flight sync settles; the parent reloads models so
  // freshly discovered ones show up without a manual refresh.
  (e: 'sync-settled'): void
}>()

const dialogVisible = ref(false)
const submitting = ref(false)
const editingId = ref<number | null>(null)
const form = reactive({ name: '', base_url: '', token: '' })

const dialogTitle = computed(() => (editingId.value === null ? '新建 Hub' : '编辑 Hub'))

function syncStatusLabel(status: Hub['sync_status']): string {
  switch (status) {
    case 'syncing': return '同步中'
    case 'succeeded': return '成功'
    case 'failed': return '失败'
    default: return '未同步'
  }
}

function syncStatusTagType(status: Hub['sync_status']): 'warning' | 'success' | 'danger' | 'info' {
  switch (status) {
    case 'syncing': return 'warning'
    case 'succeeded': return 'success'
    case 'failed': return 'danger'
    default: return 'info'
  }
}

// Poll the parent for fresh hub data while any sync is in flight; when the
// last one settles, ask the parent to reload models as well.
let pollTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.hubs.some(h => h.sync_status === 'syncing'),
  (anySyncing, wasSyncing) => {
    if (anySyncing && pollTimer === null) {
      const tick = () => {
        emit('changed')
        pollTimer = setTimeout(tick, 2000)
      }
      pollTimer = setTimeout(tick, 2000)
    } else if (!anySyncing && pollTimer !== null) {
      clearTimeout(pollTimer)
      pollTimer = null
      if (wasSyncing) emit('sync-settled')
    }
  },
  // A sync may already be in flight when the page mounts (triggered from
  // another session, or the hourly full sync) — start polling immediately.
  { immediate: true },
)

onBeforeUnmount(() => {
  if (pollTimer !== null) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
})

async function onSync(row: Hub) {
  try {
    await syncHub(row.id)
    ElMessage.success(`已触发「${row.name}」同步`)
    emit('changed')
  } catch (err) {
    // 409 surfaces here when a sync is already running for this hub.
    ElMessage.error((err as Error).message)
  }
}

function resetForm() {
  form.name = ''
  form.base_url = ''
  form.token = ''
}

function openCreate() {
  editingId.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: Hub) {
  editingId.value = row.id
  form.name = row.name
  form.base_url = row.base_url
  form.token = '' // empty keeps existing token
  dialogVisible.value = true
}

async function onSubmit() {
  if (!form.name.trim() || !form.base_url.trim()) {
    ElMessage.warning('请填写名称与 Base URL')
    return
  }
  submitting.value = true
  try {
    if (editingId.value === null) {
      await createHub({ name: form.name, base_url: form.base_url, token: form.token })
      ElMessage.success('已创建')
    } else {
      // Omit token when left blank so the backend keeps the existing one.
      const payload: { name: string; base_url: string; token?: string } = {
        name: form.name,
        base_url: form.base_url,
      }
      if (form.token) payload.token = form.token
      await updateHub(editingId.value, payload)
      ElMessage.success('已保存')
    }
    dialogVisible.value = false
    emit('changed')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}

async function onDelete(row: Hub) {
  try {
    await ElMessageBox.confirm(`确认删除 Hub「${row.name}」?`, '提示', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return // user cancelled
  }
  try {
    await deleteHub(row.id)
    ElMessage.success('已删除')
    emit('changed')
  } catch (err) {
    // Backend returns 409 with a message when the hub still has models.
    ElMessage.error((err as Error).message)
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
.token-hint {
  font-family: monospace;
  /* Masked token stays legible (W6): regular text color, never faint gray. */
  color: var(--hs-text-regular);
}
</style>
