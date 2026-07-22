<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>分类规则</span>
        <el-button type="primary" size="small" @click="openCreate">新建规则</el-button>
      </div>
    </template>

    <div class="hint">
      模型 ID 包含关键词(不区分大小写)即归入对应分类;每个维度按优先级从小到大取首个命中。每次保存都会立即重算全部模型分类。
    </div>

    <el-table :data="rules" v-loading="loading" size="small" empty-text="暂无规则">
      <el-table-column label="维度" width="110">
        <template #default="{ row }">
          <el-tag :type="row.dimension === 'family' ? 'primary' : 'success'" size="small">
            {{ row.dimension === 'family' ? '厂商' : '能力' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="keyword" label="关键词" min-width="140" show-overflow-tooltip />
      <el-table-column prop="category" label="分类" min-width="120" show-overflow-tooltip />
      <el-table-column prop="priority" label="优先级" width="80" sortable />
      <el-table-column label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" size="small" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editingId === null ? '新建规则' : '编辑规则'" width="440px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="维度">
          <el-select v-model="form.dimension" :disabled="editingId !== null" style="width: 100%">
            <el-option label="厂商(family)" value="family" />
            <el-option label="能力(capability)" value="capability" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="form.keyword" placeholder="如 qwen / embedding" />
        </el-form-item>
        <el-form-item label="分类">
          <el-input v-model="form.category" placeholder="如 qwen / embedding" />
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="form.priority" :min="1" :max="10000" />
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
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listClassificationRules,
  createClassificationRule,
  updateClassificationRule,
  deleteClassificationRule,
} from '@/api/classification'
import type { ClassificationRule } from '@/api/types'

const emit = defineEmits<{ (e: 'changed'): void }>()

const rules = ref<ClassificationRule[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const submitting = ref(false)
const editingId = ref<number | null>(null)
const form = reactive<{ dimension: 'capability' | 'family'; keyword: string; category: string; priority: number }>({
  dimension: 'family',
  keyword: '',
  category: '',
  priority: 100,
})

async function reload() {
  loading.value = true
  try {
    rules.value = await listClassificationRules()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  form.dimension = 'family'
  form.keyword = ''
  form.category = ''
  form.priority = 100
  dialogVisible.value = true
}

function openEdit(row: ClassificationRule) {
  editingId.value = row.id
  form.dimension = row.dimension
  form.keyword = row.keyword
  form.category = row.category
  form.priority = row.priority
  dialogVisible.value = true
}

async function onSubmit() {
  if (!form.keyword.trim() || !form.category.trim()) {
    ElMessage.warning('请填写关键词与分类')
    return
  }
  submitting.value = true
  try {
    if (editingId.value === null) {
      await createClassificationRule({
        dimension: form.dimension,
        keyword: form.keyword,
        category: form.category,
        priority: form.priority,
      })
      ElMessage.success('规则已创建,已重算全部模型分类')
    } else {
      await updateClassificationRule(editingId.value, {
        keyword: form.keyword,
        category: form.category,
        priority: form.priority,
      })
      ElMessage.success('规则已保存,已重算全部模型分类')
    }
    dialogVisible.value = false
    await reload()
    emit('changed') // models were reclassified; parent reloads them
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}

async function onDelete(row: ClassificationRule) {
  try {
    await ElMessageBox.confirm(
      `确认删除规则「${row.dimension} / ${row.keyword} → ${row.category}」?删除后将立即重算全部模型分类。`,
      '删除规则',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return // user cancelled
  }
  try {
    await deleteClassificationRule(row.id)
    ElMessage.success('规则已删除,已重算全部模型分类')
    await reload()
    emit('changed')
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
}

onMounted(reload)
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.hint {
  color: #909399;
  font-size: 12px;
  margin-bottom: 10px;
}
</style>
