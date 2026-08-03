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
          <el-option v-for="s in enabledSuites" :key="s.id" :label="s.name" :value="s.id" />
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
              <!-- GH #170: opted-out models stay selectable here because a
                   manual trigger is the explicit override path. -->
              <span v-else-if="!m.eval_enabled" class="model-option-hint">已关闭「参与评估」,手动触发仍将执行</span>
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
          :disabled="evalModelCount > 0"
          content="暂无参与评估的对话模型(均已在模型管理中关闭「参与评估」)"
          placement="top"
        >
          <span>
            <el-button
              type="warning"
              plain
              :disabled="submitting || evalModelCount === 0"
              @click="onFullSweep"
            >
              一键全量评估
            </el-button>
          </span>
        </el-tooltip>
        <span class="footer-count">将评估 {{ evalModelCount }} 个模型</span>
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
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
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
  preselectedModelId?: number
}>()

const emit = defineEmits<{
  'update:modelValue': [visible: boolean]
  triggered: [campaign: Campaign]
}>()

const suiteId = ref<number | null>(null)
const selectedModelIds = ref<number[]>([])
const submitting = ref(false)

// GH #170: the full sweep and the weekly batch only cover active chat
// models whose "join evaluations" switch is on; the footer surfaces that
// count so the operator sees the blast radius before firing.
const evalModelCount = computed(
  () => props.models.filter(m => m.capability === 'chat' && m.status === 'active' && m.eval_enabled).length
)

// Retired suites (question-bank v3, ADR 0010) stay in the library for
// history but are not offered for new triggers.
const enabledSuites = computed(() => props.suites.filter(s => s.enabled))

// localStorage keys for remembering last selection (ticket 60.7)
const STORAGE_KEY_SUITE = 'eval-last-suite'
const STORAGE_KEY_MODELS = 'eval-last-models'

// Read last selection from localStorage
function loadLastSelection() {
  try {
    const lastSuiteId = localStorage.getItem(STORAGE_KEY_SUITE)
    const lastModelsJson = localStorage.getItem(STORAGE_KEY_MODELS)

    // Restore suite if it's still in enabled suites
    if (lastSuiteId) {
      const parsedSuiteId = Number(lastSuiteId)
      if (enabledSuites.value.some(s => s.id === parsedSuiteId)) {
        suiteId.value = parsedSuiteId
      }
    }

    // Restore models if not preselected and they're still valid chat models
    if (!props.preselectedModelId && lastModelsJson) {
      const lastModelIds = JSON.parse(lastModelsJson) as number[]
      const validModelIds = lastModelIds.filter(id =>
        props.models.some(m => m.id === id && m.capability === 'chat')
      )
      if (validModelIds.length > 0) {
        selectedModelIds.value = validModelIds
      }
    }
  } catch (err) {
    // Ignore localStorage errors (e.g., quota exceeded, parse errors)
    console.warn('Failed to load last eval selection:', err)
  }
}

// Debounced save to localStorage
let saveTimer: number | null = null
function saveSelection() {
  if (saveTimer !== null) {
    clearTimeout(saveTimer)
  }
  saveTimer = window.setTimeout(() => {
    try {
      if (suiteId.value !== null) {
        localStorage.setItem(STORAGE_KEY_SUITE, String(suiteId.value))
      }
      if (selectedModelIds.value.length > 0) {
        localStorage.setItem(STORAGE_KEY_MODELS, JSON.stringify(selectedModelIds.value))
      }
    } catch (err) {
      // Ignore localStorage errors (e.g., quota exceeded)
      console.warn('Failed to save eval selection:', err)
    }
  }, 500)
}

// Load last selection when dialog opens. If preselectedModelId is provided,
// it overrides the localStorage model memory (ticket 04).
watch(
  () => props.modelValue,
  visible => {
    if (visible) {
      // Reset first
      suiteId.value = null
      selectedModelIds.value = []

      // Then load last selection (respecting preselectedModelId)
      loadLastSelection()

      // If model is preselected from outside (e.g., detail page), use it
      if (props.preselectedModelId) {
        selectedModelIds.value = [props.preselectedModelId]
      }
    }
  }
)

// Save selection to localStorage when user changes suite or models
watch([suiteId, selectedModelIds], saveSelection, { deep: true })

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
      `将对全部评估集 × ${evalModelCount.value} 个对话模型发起评估,确认继续?`,
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
.footer-count {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-left: 12px;
  white-space: nowrap;
}
</style>
