<template>
  <el-card shadow="never" class="admin-card">
    <template #header>
      <div class="card-header">
        <span>用户管理</span>
        <el-button type="primary" size="small" @click="openCreate">新建用户</el-button>
      </div>
    </template>

    <el-table :data="users" v-loading="loading" empty-text="暂无用户" size="small">
      <el-table-column prop="username" label="用户名" min-width="140" show-overflow-tooltip />
      <el-table-column label="角色" width="120">
        <template #default="{ row }">
          <el-tag :type="roleTagType(row.role)" size="small" disable-transitions>
            {{ roleLabel(row.role) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="所属 Hub" min-width="160">
        <template #default="{ row }">
          <span v-if="row.hub_name" class="hub-name">{{ row.hub_name }}</span>
          <span v-else class="placeholder">全局</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'" size="small" disable-transitions>
            {{ row.enabled ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="165">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
          <el-button link type="primary" size="small" @click="openReset(row)">重置密码</el-button>
          <el-button
            link
            type="danger"
            size="small"
            :disabled="row.id === currentUserId"
            @click="onDelete(row)"
          >删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="460px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="用户名">
          <el-input
            v-model="form.username"
            :placeholder="editingId === null ? '登录用户名' : ''"
            :disabled="editingId !== null"
          />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-if="editingId === null"
            v-model="form.password"
            type="password"
            show-password
            placeholder="至少 8 位"
          />
          <span v-else class="field-hint">编辑时不修改密码,留空;需修改请用「重置密码」</span>
        </el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role" :disabled="roleSelectDisabled" placeholder="选择角色">
            <el-option
              v-for="opt in roleOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="showHubSelect" label="所属 Hub">
          <el-select v-model="form.hub_id" placeholder="选择 Hub" :disabled="hubSelectDisabled">
            <el-option
              v-if="canCreateSuperAdmin"
              :label="'全局(超级管理员)'"
              :value="0"
            />
            <el-option
              v-for="hub in hubs"
              :key="hub.id"
              :label="hub.name"
              :value="hub.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="editingId !== null" label="启用状态">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="resetDialogVisible" title="重置密码" width="440px">
      <el-form :model="resetForm" label-width="90px">
        <el-form-item label="用户名">
          <span class="field-hint">{{ resetForm.username }}</span>
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="resetForm.password" type="password" show-password placeholder="至少 8 位" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="resetting" @click="onSubmitReset">确认重置</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listUsers,
  createUser,
  updateUser,
  resetPassword,
  deleteUser,
  type User,
} from '@/api/users'
import { listHubs } from '@/api/hubs'
import { fetchAuthStatus, type Role } from '@/api/auth'
import { roleLabel, roleTagType } from '@/utils/role'
import { formatTime } from '@/utils/format'

const users = ref<User[]>([])
const hubs = ref<{ id: number; name: string }[]>([])
const loading = ref(false)
const currentUserId = ref<number>(0)
const currentRole = ref<Role | null>(null)

const canCreateSuperAdmin = computed(() => currentRole.value === 'super_admin')

// Role options: super_admin lists all four roles; admin lists only
// operator/viewer (admin reshuffling roles is a super_admin concern).
const roleOptions = computed<{ value: Role; label: string }[]>(() => {
  if (currentRole.value === 'super_admin') {
    return (['super_admin', 'admin', 'operator', 'viewer'] as Role[]).map(r => ({
      value: r,
      label: roleLabel(r),
    }))
  }
  return (['operator', 'viewer'] as Role[]).map(r => ({ value: r, label: roleLabel(r) }))
})

// The hub select shows when creating (always, since hub_id may differ) or
// when a super_admin is editing a hub-scoped user. For admin editors the
// hub is fixed to the session hub and the select is hidden (not editable).
const showHubSelect = computed(() => {
  if (editingId.value === null) return true // create
  return currentRole.value === 'super_admin'
})
const roleSelectDisabled = computed(() => {
  // admin may only flip enabled (role is pinned to nil server-side); show
  // the current role but disable the select so the intent is clear.
  return currentRole.value !== 'super_admin'
})
const hubSelectDisabled = computed(() => false)

const dialogVisible = ref(false)
const submitting = ref(false)
const editingId = ref<number | null>(null)
const form = reactive<{
  username: string
  password: string
  role: Role
  hub_id: number | null
  enabled: boolean
}>({
  username: '',
  password: '',
  role: 'viewer',
  hub_id: null,
  enabled: true,
})

const dialogTitle = computed(() => (editingId.value === null ? '新建用户' : '编辑用户'))

const resetDialogVisible = ref(false)
const resetting = ref(false)
const resetForm = reactive<{ id: number | null; username: string; password: string }>({
  id: null,
  username: '',
  password: '',
})

async function reload() {
  loading.value = true
  try {
    const [list, hubList] = await Promise.all([listUsers(), listHubs()])
    users.value = list
    hubs.value = hubList
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    loading.value = false
  }
}

function resetFormState() {
  form.username = ''
  form.password = ''
  form.role = currentRole.value === 'super_admin' ? 'super_admin' : 'operator'
  form.hub_id = null
  form.enabled = true
}

function openCreate() {
  editingId.value = null
  resetFormState()
  // Default hub to the first hub when not super_admin, so a quick submit
  // works (the backend ignores it for admin and pins the session hub).
  if (currentRole.value !== 'super_admin' && hubs.value.length > 0) {
    form.hub_id = hubs.value[0].id
  }
  dialogVisible.value = true
}

function openEdit(row: User) {
  editingId.value = row.id
  form.username = row.username
  form.password = ''
  form.role = row.role
  form.hub_id = row.hub_id ?? 0
  form.enabled = row.enabled
  dialogVisible.value = true
}

function openReset(row: User) {
  resetForm.id = row.id
  resetForm.username = row.username
  resetForm.password = ''
  resetDialogVisible.value = true
}

async function onSubmit() {
  if (editingId.value === null) {
    if (!form.username.trim() || !form.password) {
      ElMessage.warning('请填写用户名与密码')
      return
    }
    if (form.password.length < 8) {
      ElMessage.warning('密码至少 8 位')
      return
    }
    const payload: Parameters<typeof createUser>[0] = {
      username: form.username.trim(),
      password: form.password,
      role: form.role,
    }
    // super_admin creating super_admin → omit hub_id (global). The sentinel
    // value 0 in the select maps to "全局(超级管理员)"; send nothing.
    if (form.role !== 'super_admin' && form.hub_id && form.hub_id !== 0) {
      payload.hub_id = form.hub_id
    }
    submitting.value = true
    try {
      await createUser(payload)
      ElMessage.success('已创建')
      dialogVisible.value = false
      await reload()
    } catch (err) {
      ElMessage.error((err as Error).message)
    } finally {
      submitting.value = false
    }
    return
  }

  // Edit: PATCH covers role + enabled only (hub_id is not patchable per
  // ticket 67; the backend clears hub_id automatically when a user is
  // promoted to super_admin). admin can only flip enabled; super_admin can
  // edit role.
  const payload: Parameters<typeof updateUser>[1] = { enabled: form.enabled }
  if (currentRole.value === 'super_admin') {
    if (form.role) payload.role = form.role
  }
  submitting.value = true
  try {
    await updateUser(editingId.value, payload)
    ElMessage.success('已保存')
    dialogVisible.value = false
    await reload()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}

async function onSubmitReset() {
  if (!resetForm.id) return
  if (resetForm.password.length < 8) {
    ElMessage.warning('密码至少 8 位')
    return
  }
  resetting.value = true
  try {
    await resetPassword(resetForm.id, resetForm.password)
    ElMessage.success('密码已重置')
    resetDialogVisible.value = false
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    resetting.value = false
  }
}

async function onDelete(row: User) {
  if (row.id === currentUserId.value) {
    ElMessage.warning('无法删除当前登录用户')
    return
  }
  try {
    await ElMessageBox.confirm(`确认删除用户「${row.username}」?`, '提示', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await deleteUser(row.id)
    ElMessage.success('已删除')
    await reload()
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
}

onMounted(async () => {
  try {
    const status = await fetchAuthStatus()
    if (status.user) {
      currentUserId.value = status.user.id
      currentRole.value = status.user.role
    }
  } catch {
    // Router guard handles redirect; here just skip role-aware UI.
  }
  await reload()
})
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
.hub-name {
  color: var(--hs-text-regular);
}
.placeholder {
  color: var(--hs-text-placeholder);
}
.field-hint {
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-sm);
}
</style>
