<template>
  <el-dialog
    :model-value="modelValue"
    title="触发评估"
    width="520px"
    @update:model-value="onVisibleChange"
  >
    <el-form label-width="90px">
      <el-form-item label="评估集">
        <el-select v-model="suiteId" filterable placeholder="选择评估集" class="full-width">
          <el-option v-for="s in suites" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="模型">
        <el-select
          v-model="selectedModelIds"
          multiple
          filterable
          collapse-tags
          collapse-tags-tooltip
          placeholder="选择参与评估的对话模型"
          class="full-width"
        >
          <el-option
            v-for="m in models"
            :key="m.id"
            :label="m.model_id"
            :value="m.id"
            :disabled="m.capability !== 'chat'"
          >
            <div class="model-option">
              <span class="model-option-name">{{ m.model_id }}</span>
              <span v-if="m.capability !== 'chat'" class="model-option-hint">非对话模型不能参与评估</span>
            </div>
          </el-option>
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <div class="dialog-footer">
        <!-- Disabled buttons swallow pointer events, so the reason tooltip
             needs a wrapper span. -->
        <el-tooltip
          :disabled="chatModelCount > 0"
          content="暂无可参与评估的对话模型"
          placement="top"
        >
          <span>
            <el-button
              type="warning"
              plain
              :disabled="submitting || chatModelCount === 0"
              @click="onFullSweep"
            >
              一键全量评估
            </el-button>
          </span>
        </el-tooltip>
        <div class="footer-actions">
          <el-button @click="close">取消</el-button>
          <el-button
            type="primary"
            :loading="submitting"
            :disabled="!suiteId || selectedModelIds.length === 0"
            @click="onTrigger"
          >
            触发评估
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createEvalRun, createFullSweep } from '@/api/evals'
import type { Campaign, Model, Suite } from '@/api/types'

// Trigger-eval dialog: pick one suite plus any number of chat models, or fire
// the one-click full sweep (every suite x every chat model). Non-chat models
// stay visible but disabled, matching the server-side constraint. A
// successful trigger hands the created campaign to the parent, which polls it
// until settled.
const props = defineProps<{
  modelValue: boolean
  suites: Suite[]
  models: Model[]
}>()

const emit = defineEmits<{
  'update:modelValue': [visible: boolean]
  triggered: [campaign: Campaign]
}>()

const suiteId = ref<number | null>(null)
const selectedModelIds = ref<number[]>([])
const submitting = ref(false)

const chatModelCount = computed(() => props.models.filter(m => m.capability === 'chat').length)

// Reset the form every time the dialog opens so stale picks never leak into
// the next trigger.
watch(
  () => props.modelValue,
  visible => {
    if (visible) {
      suiteId.value = null
      selectedModelIds.value = []
    }
  }
)

function onVisibleChange(visible: boolean) {
  if (!visible) emit('update:modelValue', false)
}

function close() {
  emit('update:modelValue', false)
}

async function onTrigger() {
  if (!suiteId.value) return
  submitting.value = true
  try {
    const campaign = await createEvalRun(suiteId.value, selectedModelIds.value)
    emit('triggered', campaign)
    close()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}

// The full sweep hits every suite x every chat model, so it costs a whole
// evaluation round — confirm before firing.
async function onFullSweep() {
  try {
    await ElMessageBox.confirm(
      `将对全部评估集 × ${chatModelCount.value} 个对话模型发起评估,确认继续?`,
      '一键全量评估',
      { confirmButtonText: '开始评估', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  submitting.value = true
  try {
    const campaign = await createFullSweep()
    emit('triggered', campaign)
    close()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.full-width {
  width: 100%;
}
.model-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.model-option-hint {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.dialog-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.footer-actions {
  display: flex;
  gap: 12px;
}
.footer-actions .el-button + .el-button {
  margin-left: 0;
}
</style>
