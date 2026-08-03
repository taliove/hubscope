<template>
  <el-dialog
    :model-value="modelValue"
    title="触发评估"
    width="520px"
    @update:model-value="onVisibleChange"
  >
    <el-form label-width="90px">
      <el-form-item label="评估集">
        <el-select
          v-model="selectedSuiteIds"
          multiple
          filterable
          collapse-tags
          collapse-tags-tooltip
          placeholder="选择参与评估的评估集"
          class="full-width"
        >
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
          <el-option v-for="m in candidateModels" :key="m.id" :label="m.model_id" :value="m.id" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <div class="dialog-footer">
        <span class="footer-count">将评估 {{ selectedSuiteIds.length }} 个评估集 × {{ selectedModelIds.length }} 个模型</span>
        <div class="footer-actions">
          <el-button @click="close">取消</el-button>
          <!-- Disabled buttons swallow pointer events, so the reason tooltip
               needs a wrapper span. -->
          <el-tooltip
            :disabled="canSubmit"
            :content="disabledReason"
            placement="top"
          >
            <span>
              <el-button
                type="primary"
                :loading="submitting"
                :disabled="!canSubmit"
                @click="onTrigger"
              >
                触发评估
              </el-button>
            </span>
          </el-tooltip>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { triggerEval } from '@/api/evals'
import { buildTriggerBody, evalCandidates, friendlyTriggerError } from '@/utils/evalTrigger'
import type { Campaign, Model, Suite } from '@/api/types'

// Trigger-eval dialog: multi-select the suites (default: the whole enabled
// rotation) and the models (default: every active chat model whose "join
// evaluations" switch is on — non-chat, retired and opted-out models are
// not listed). Both dimensions fully selected submits the empty body, the
// one-click full sweep; a narrowed dimension rides as explicit suite_ids /
// model_ids. A successful trigger hands the created campaign to the parent,
// which polls it until settled.
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

const selectedSuiteIds = ref<number[]>([])
const selectedModelIds = ref<number[]>([])
const submitting = ref(false)

// Retired suites (question-bank v3, ADR 0010) stay in the library for
// history but are not offered for new triggers.
const enabledSuites = computed(() => props.suites.filter(s => s.enabled))

// Only sweep-eligible models are listed (GH #170): active, chat-capable,
// "join evaluations" on.
const candidateModels = computed(() =>
  props.models.filter(m => m.capability === 'chat' && m.status === 'active' && m.eval_enabled)
)

const candidates = computed(() => evalCandidates(props.suites, props.models))

const canSubmit = computed(() => selectedSuiteIds.value.length > 0 && selectedModelIds.value.length > 0)

const disabledReason = computed(() => {
  if (enabledSuites.value.length === 0) return '暂无启用的评估集'
  if (candidateModels.value.length === 0) return '暂无参与评估的对话模型(均已在模型管理中关闭「参与评估」)'
  if (selectedSuiteIds.value.length === 0) return '请至少选择一个评估集'
  return '请至少选择一个模型'
})

// localStorage keys for remembering last selection (ticket 60.7)
const STORAGE_KEY_SUITES = 'eval-last-suites'
const STORAGE_KEY_MODELS = 'eval-last-models'

// Read last selection from localStorage, intersected with what's still
// selectable; a stored list that no longer matches anything falls back to
// the default (all selected).
function loadLastSelection() {
  try {
    const lastSuitesJson = localStorage.getItem(STORAGE_KEY_SUITES)
    if (lastSuitesJson) {
      const ids = (JSON.parse(lastSuitesJson) as number[]).filter(id => candidates.value.suiteIds.includes(id))
      if (ids.length > 0) selectedSuiteIds.value = ids
    }
    if (!props.preselectedModelId) {
      const lastModelsJson = localStorage.getItem(STORAGE_KEY_MODELS)
      if (lastModelsJson) {
        const ids = (JSON.parse(lastModelsJson) as number[]).filter(id => candidates.value.modelIds.includes(id))
        if (ids.length > 0) selectedModelIds.value = ids
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
      if (selectedSuiteIds.value.length > 0) {
        localStorage.setItem(STORAGE_KEY_SUITES, JSON.stringify(selectedSuiteIds.value))
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

// Load last selection when dialog opens. If preselectedModelId is provided
// and still sweep-eligible, it overrides the model selection (ticket 04).
watch(
  () => props.modelValue,
  visible => {
    if (visible) {
      // Default: everything selected (the full sweep).
      selectedSuiteIds.value = [...candidates.value.suiteIds]
      selectedModelIds.value = [...candidates.value.modelIds]

      loadLastSelection()

      if (props.preselectedModelId && candidates.value.modelIds.includes(props.preselectedModelId)) {
        selectedModelIds.value = [props.preselectedModelId]
      }
    }
  }
)

// Save selection to localStorage when the user changes suites or models
watch([selectedSuiteIds, selectedModelIds], saveSelection, { deep: true })

function onVisibleChange(visible: boolean) {
  if (!visible) emit('update:modelValue', false)
}

function close() {
  emit('update:modelValue', false)
}

async function onTrigger() {
  if (!canSubmit.value) return
  submitting.value = true
  try {
    const campaign = await triggerEval(buildTriggerBody(selectedSuiteIds.value, selectedModelIds.value, candidates.value))
    emit('triggered', campaign)
    close()
  } catch (err) {
    ElMessage.error(friendlyTriggerError(err))
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.full-width {
  width: 100%;
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
  white-space: nowrap;
}
</style>
