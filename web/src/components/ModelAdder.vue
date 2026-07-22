<template>
  <el-card shadow="never" class="admin-card">
    <template #header>
      <span>添加模型</span>
    </template>

    <el-form :inline="true" :model="form" @submit.prevent="onSubmit">
      <el-form-item label="选择 Hub">
        <el-select v-model="form.hubId" placeholder="请选择 Hub" style="width: 200px">
          <el-option
            v-for="hub in hubs"
            :key="hub.id"
            :label="hub.name"
            :value="hub.id"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="模型 ID">
        <el-input
          v-model="form.modelId"
          placeholder="如 claude-opus-4-8"
          style="width: 220px"
        />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="submitting" @click="onSubmit">添加</el-button>
      </el-form-item>
    </el-form>
    <p class="hint">添加后系统自动为该模型建立 anthropic 与 openai 两条 Endpoint。</p>
  </el-card>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createModel } from '@/api/models'
import type { Hub } from '@/api/types'

// Props: available hubs for the select dropdown.
defineProps<{ hubs: Hub[] }>()
const emit = defineEmits<{ (e: 'added'): void }>()

const submitting = ref(false)
const form = reactive<{ hubId: number | null; modelId: string }>({
  hubId: null,
  modelId: '',
})

async function onSubmit() {
  if (form.hubId === null) {
    ElMessage.warning('请选择 Hub')
    return
  }
  if (!form.modelId.trim()) {
    ElMessage.warning('请输入模型 ID')
    return
  }
  submitting.value = true
  try {
    await createModel({ hub_id: form.hubId, model_id: form.modelId.trim() })
    ElMessage.success('已添加')
    form.modelId = ''
    emit('added')
  } catch (err) {
    // Duplicate model_id under the same hub returns 409 with a message.
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
/* Admin density tier: compact 12px card padding (ui-guidelines §2). */
.admin-card {
  --el-card-padding: 12px;
}
.hint {
  margin: 0;
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-sm);
}
</style>
