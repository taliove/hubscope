<template>
  <el-card shadow="never" class="admin-card">
    <template #header>
      <div class="card-header">
        <span>图像探测参数</span>
        <el-button type="primary" size="small" @click="openCreate">新建规则</el-button>
      </div>
    </template>

    <div class="hint">
      模型 ID 包含关键词(不区分大小写)即向图像探测请求追加对应参数;多条命中合并,同键取优先级小者。
      参数值仅支持字符串;model / prompt / n 为保留键,不可覆盖。规则保存后立即生效,无需重启。
    </div>

    <el-table :data="rules" v-loading="loading" size="small" empty-text="暂无规则">
      <el-table-column prop="keyword" label="关键词" min-width="140" show-overflow-tooltip />
      <el-table-column label="参数" min-width="220">
        <template #default="{ row }">
          <span class="params-cell" :title="paramsText(row)">{{ paramsText(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="priority" label="优先级" width="80" sortable />
      <el-table-column label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" size="small" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editingId === null ? '新建规则' : '编辑规则'" width="520px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="关键词">
          <el-input v-model="form.keyword" placeholder="如 gpt-image / flux" />
        </el-form-item>
        <el-form-item label="参数">
          <div class="params-editor">
            <div v-for="(row, idx) in form.params" :key="idx" class="param-row">
              <el-input v-model="row.key" placeholder="键,如 quality" class="param-key" />
              <el-input v-model="row.value" placeholder="值,如 low" class="param-value" />
              <el-button link type="danger" size="small" @click="removeParamRow(idx)">移除</el-button>
            </div>
            <el-button link type="primary" size="small" @click="addParamRow">添加参数</el-button>
          </div>
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="form.priority" :min="1" :max="10000" />
        </el-form-item>
        <div v-if="formError" class="form-error">{{ formError }}</div>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import {
  listImageParamRules,
  createImageParamRule,
  updateImageParamRule,
  deleteImageParamRule,
} from '@/api/imageParams'
import type { ImageParamRule } from '@/api/types'

// The probe contract owns these keys; the backend rejects them too — the
// inline check just fails faster and with a clearer message.
const RESERVED_KEYS = new Set(['model', 'prompt', 'n'])

interface ParamRow {
  key: string
  value: string
}

const rules = ref<ImageParamRule[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const submitting = ref(false)
const editingId = ref<number | null>(null)
const formError = ref('')
const form = reactive<{ keyword: string; params: ParamRow[]; priority: number }>({
  keyword: '',
  params: [],
  priority: 100,
})

function paramsText(rule: ImageParamRule): string {
  return Object.entries(rule.params)
    .map(([k, v]) => `${k}=${v}`)
    .join(', ')
}

async function reload() {
  loading.value = true
  try {
    rules.value = await listImageParamRules()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  form.keyword = ''
  form.params = [{ key: '', value: '' }]
  form.priority = 100
  formError.value = ''
  dialogVisible.value = true
}

function openEdit(row: ImageParamRule) {
  editingId.value = row.id
  form.keyword = row.keyword
  form.params = Object.entries(row.params).map(([key, value]) => ({ key, value }))
  form.priority = row.priority
  formError.value = ''
  dialogVisible.value = true
}

function addParamRow() {
  form.params.push({ key: '', value: '' })
}

function removeParamRow(idx: number) {
  form.params.splice(idx, 1)
}

// validateForm returns the params map, or null with formError set (inline
// form feedback, per the feedback-trio guideline).
function validateForm(): Record<string, string> | null {
  formError.value = ''
  if (!form.keyword.trim()) {
    formError.value = '请填写关键词'
    return null
  }
  const params: Record<string, string> = {}
  for (const row of form.params) {
    const key = row.key.trim()
    if (!key && !row.value) continue // untouched trailing row
    if (!key) {
      formError.value = '参数键不能为空'
      return null
    }
    if (RESERVED_KEYS.has(key.toLowerCase())) {
      formError.value = `「${key}」是保留键(model / prompt / n 由探测契约固定),不可覆盖`
      return null
    }
    params[key] = row.value
  }
  if (Object.keys(params).length === 0) {
    formError.value = '至少填写一个参数'
    return null
  }
  return params
}

async function onSubmit() {
  const params = validateForm()
  if (params === null) return
  submitting.value = true
  try {
    if (editingId.value === null) {
      await createImageParamRule({
        keyword: form.keyword,
        params,
        priority: form.priority,
      })
      ElMessage.success('规则已创建,下一次探测即生效')
    } else {
      await updateImageParamRule(editingId.value, {
        keyword: form.keyword,
        params,
        priority: form.priority,
      })
      ElMessage.success('规则已保存,下一次探测即生效')
    }
    dialogVisible.value = false
    await reload()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}

async function onDelete(row: ImageParamRule) {
  try {
    await ElMessageBox.confirm(
      `确认删除规则「${row.keyword} → ${paramsText(row)}」?删除后下一次探测即恢复最小请求体。`,
      '删除规则',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return // user cancelled
  }
  try {
    await deleteImageParamRule(row.id)
    ElMessage.success('规则已删除')
    await reload()
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
}

onMounted(reload)
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
.hint {
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-xs);
  margin-bottom: 8px;
  white-space: pre-line;
}
.params-cell {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}
.params-editor {
  width: 100%;
}
.param-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}
.param-key {
  flex: 1;
}
.param-value {
  flex: 1;
}
.form-error {
  color: var(--hs-danger);
  font-size: var(--hs-text-xs);
  margin-top: 4px;
}
</style>
