<template>
  <el-card shadow="never" class="library-card">
    <div class="card-header">
      <div class="card-title">题库</div>
      <el-select v-model="capabilityFilter" class="capability-filter" size="small" placeholder="按能力点筛选">
        <el-option
          v-for="opt in capabilityOptions"
          :key="opt.value"
          :label="opt.label"
          :value="opt.value"
        />
      </el-select>
    </div>

    <el-alert
      v-if="error"
      class="load-alert"
      type="error"
      :closable="false"
      :title="`加载失败:${error}`"
    >
      <el-button size="small" @click="loadSuites">重试</el-button>
    </el-alert>

    <div v-loading="loading">
      <el-collapse v-if="filteredSuites.length > 0">
        <el-collapse-item v-for="suite in filteredSuites" :key="suite.id" :name="suite.id">
          <template #title>
            <span class="suite-title" :title="`${suite.name}(${suite.key},${suite.cases.length} 题,v${suite.version})`">
              {{ suite.name }}({{ suite.key }},{{ suite.cases.length }} 题,v{{ suite.version }})
            </span>
            <el-tag size="small" effect="plain" class="suite-tag">{{ capabilityLabel(suite.capability) }}</el-tag>
            <el-tag v-if="suite.nadir > 0" size="small" effect="plain" class="suite-tag">
              nadir {{ suite.nadir }}
            </el-tag>
          </template>
          <div class="suite-actions">
            <el-button size="small" type="primary" plain @click="openCreate(suite)">新增 Case</el-button>
          </div>
          <el-table :data="suite.cases">
            <el-table-column label="ID" prop="id" width="60" />
            <el-table-column label="题目" min-width="260" show-overflow-tooltip>
              <template #default="{ row }">{{ row.prompt }}</template>
            </el-table-column>
            <el-table-column label="难度" width="80" align="center">
              <template #default="{ row }">
                <el-tag size="small" :type="difficultyTagType(row.difficulty)">
                  {{ difficultyLabel(row.difficulty) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="采样" width="70" align="center">
              <template #default="{ row }">{{ row.sample_count ?? '默认' }}</template>
            </el-table-column>
            <el-table-column label="判定方式" width="90" align="center">
              <template #default="{ row }">
                <el-tag size="small" :type="row.verdict_type === 'rule' ? 'success' : 'warning'">
                  {{ row.verdict_type === 'rule' ? '规则' : '裁判' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="判定配置" min-width="220" show-overflow-tooltip>
              <template #default="{ row }">{{ verdictConfig(row) }}</template>
            </el-table-column>
            <el-table-column label="启用" width="70" align="center">
              <template #default="{ row }">
                <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
                  {{ row.enabled ? '启用' : '停用' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="80" align="center">
              <template #default="{ row }">
                <el-button size="small" text type="primary" @click="openEdit(suite.id, row)">编辑</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-collapse-item>
      </el-collapse>
      <el-empty
        v-else-if="!loading && !error"
        :description="capabilityFilter === 'all' ? '暂无评估集' : '该能力点下暂无评估集'"
      />
    </div>

    <!-- Create/edit dialog: fields follow the verdict type. -->
    <el-dialog v-model="dialogVisible" :title="editingId === null ? '新增 Case' : `编辑 Case #${editingId}`" width="560px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="题目" prop="prompt">
          <el-input v-model="form.prompt" type="textarea" :rows="3" placeholder="给模型的完整提问" />
        </el-form-item>
        <el-form-item label="判定方式">
          <el-radio-group v-model="form.verdict_type">
            <el-radio-button value="rule">规则</el-radio-button>
            <el-radio-button value="judge">裁判</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <template v-if="form.verdict_type === 'rule'">
          <el-form-item label="匹配模式">
            <el-select v-model="form.ruleMode">
              <el-option label="完全相等 (exact)" value="exact" />
              <el-option label="正则匹配 (regex)" value="regex" />
              <el-option label="包含子串 (contains)" value="contains" />
              <el-option label="选项字母 (mcq)" value="mcq" />
              <el-option label="数值提取 (numeric)" value="numeric" />
              <el-option label="输出匹配 (output_match)" value="output_match" />
              <!-- ifeval 仅能由权威题库 seed 铸入,新增不提供该选项;编辑 ifeval case 时显示以保住当前值。 -->
              <el-option v-if="form.ruleMode === 'ifeval'" label="指令校验 (ifeval)" value="ifeval" />
            </el-select>
          </el-form-item>
          <template v-if="form.ruleMode === 'ifeval'">
            <el-form-item label="校验参数">
              <div class="ifeval-params">
                <el-input
                  :model-value="form.ifevalParams"
                  type="textarea"
                  :rows="6"
                  readonly
                  placeholder="无校验参数"
                />
                <div class="ifeval-note">校验参数由权威题库种子铸入,编辑此 Case 时原样保留,不可手工修改。</div>
              </div>
            </el-form-item>
          </template>
          <el-form-item v-else label="期望值" prop="ruleExpected">
            <el-input v-model="form.ruleExpected" placeholder="命中得 1 分,否则 0 分" />
          </el-form-item>
        </template>
        <el-form-item v-else label="评分标准" prop="rubric">
          <el-input v-model="form.rubric" type="textarea" :rows="4" placeholder="裁判模型据以打 0~1 分的 rubric" />
        </el-form-item>
        <el-form-item label="难度">
          <el-select v-model="form.difficulty">
            <el-option label="基础" value="basic" />
            <el-option label="中等" value="intermediate" />
            <el-option label="困难" value="hard" />
          </el-select>
        </el-form-item>
        <el-form-item label="采样次数">
          <el-checkbox v-model="form.sampleCountCustom" class="sample-custom">覆盖全局默认</el-checkbox>
          <el-input-number v-model="form.sampleCount" :min="1" :max="10" :disabled="!form.sampleCountCustom" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { createCase, listSuites, patchCase } from '@/api/evals'
import type { Capability, Difficulty, EvalCase, Suite, VerdictType } from '@/api/types'

// Case library (admin console): browses suites by capability (question-bank
// v3, ADR 0010) and creates/edits cases. The /admin route already gates the
// session, so write forms are always shown; the server still re-validates.
// Cases are immutable server-side: a content edit returns a new case id and
// retires the old row, which stays visible as disabled after the refresh.
// Retired suites never reach this panel: disabled suites are hard-deleted
// server-side at Open (ADR 0012). The panel loads its own suite data.
const suites = ref<Suite[]>([])
const loading = ref(false)
const error = ref('')

// Capability filter: 'all' lists every suite; the other options narrow to
// one capability dimension. Pre-v3 legacy suites no longer exist (hard-deleted
// server-side at Open, ADR 0012), so there is no legacy option.
const capabilityFilter = ref('all')

const CAPABILITY_LABELS: Record<Capability, string> = {
  instruction: '指令遵循',
  reasoning: '推理',
  coding: '代码',
  language: '语言理解与生成',
  knowledge: '知识问答',
}

const capabilityOptions = computed(() => [
  { value: 'all', label: '全部能力点' },
  { value: 'instruction', label: CAPABILITY_LABELS.instruction },
  { value: 'reasoning', label: CAPABILITY_LABELS.reasoning },
  { value: 'coding', label: CAPABILITY_LABELS.coding },
  { value: 'language', label: CAPABILITY_LABELS.language },
  { value: 'knowledge', label: CAPABILITY_LABELS.knowledge },
])

const filteredSuites = computed(() => {
  if (capabilityFilter.value === 'all') return suites.value
  return suites.value.filter(s => s.capability === capabilityFilter.value)
})

function capabilityLabel(c: Capability): string {
  return CAPABILITY_LABELS[c] ?? c
}

const dialogVisible = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const editingSuiteId = ref<number>(0)
const formRef = ref<FormInstance>()

async function loadSuites() {
  loading.value = true
  error.value = ''
  try {
    suites.value = await listSuites()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(loadSuites)

interface CaseForm {
  prompt: string
  verdict_type: VerdictType
  ruleMode: 'exact' | 'regex' | 'contains' | 'mcq' | 'numeric' | 'output_match' | 'ifeval'
  ruleExpected: string
  ifevalParams: string // read-only pretty JSON of check_params (ifeval only)
  rubric: string
  difficulty: Difficulty
  sampleCount: number
  sampleCountCustom: boolean
  enabled: boolean
}

const form = reactive<CaseForm>({
  prompt: '',
  verdict_type: 'rule',
  ruleMode: 'contains',
  ruleExpected: '',
  ifevalParams: '',
  rubric: '',
  difficulty: 'basic',
  sampleCount: 1,
  sampleCountCustom: false,
  enabled: true,
})

// Inline validation (ui-guidelines §5): required fields follow the verdict
// type — rule cases need an expected value, judge cases need a rubric. An
// ifeval case's expectation lives in its seed-cast check params, so the
// expected-value input is neither shown nor required for that mode.
const rules = computed<FormRules>(() => ({
  prompt: [{ required: true, message: '题目不能为空', trigger: 'blur' }],
  ruleExpected:
    form.verdict_type === 'rule' && form.ruleMode !== 'ifeval'
      ? [{ required: true, message: '期望值不能为空', trigger: 'blur' }]
      : [],
  rubric:
    form.verdict_type === 'judge'
      ? [{ required: true, message: '评分标准不能为空', trigger: 'blur' }]
      : [],
}))

const DIFFICULTY_LABELS: Record<Difficulty, string> = {
  basic: '基础',
  intermediate: '中等',
  hard: '困难',
}

function difficultyLabel(d: Difficulty): string {
  return DIFFICULTY_LABELS[d] ?? d
}

function difficultyTagType(d: Difficulty): 'success' | 'warning' | 'danger' {
  if (d === 'basic') return 'success'
  if (d === 'intermediate') return 'warning'
  return 'danger'
}

// Summarize the verdict configuration for the read-only table.
function verdictConfig(c: EvalCase): string {
  if (c.verdict_type === 'rule' && c.rule_config) {
    if (c.rule_config.mode === 'ifeval') {
      return `ifeval: ${c.check_params?.length ?? 0} 条校验指令`
    }
    return `${c.rule_config.mode}: ${c.rule_config.expected}`
  }
  return c.rubric ?? '-'
}

function openCreate(suite: Suite) {
  editingId.value = null
  editingSuiteId.value = suite.id
  Object.assign(form, {
    prompt: '',
    verdict_type: 'rule' as VerdictType,
    ruleMode: 'contains' as const,
    ruleExpected: '',
    ifevalParams: '',
    rubric: '',
    difficulty: 'basic' as Difficulty,
    sampleCount: 1,
    sampleCountCustom: false,
    enabled: true,
  })
  dialogVisible.value = true
}

function openEdit(suiteId: number, c: EvalCase) {
  editingId.value = c.id
  editingSuiteId.value = suiteId
  Object.assign(form, {
    prompt: c.prompt,
    verdict_type: c.verdict_type,
    ruleMode: c.rule_config?.mode ?? 'contains',
    ruleExpected: c.rule_config?.expected ?? '',
    ifevalParams: c.check_params ? JSON.stringify(c.check_params, null, 2) : '',
    rubric: c.rubric ?? '',
    difficulty: c.difficulty,
    sampleCount: c.sample_count ?? 1,
    sampleCountCustom: c.sample_count !== null,
    enabled: c.enabled,
  })
  dialogVisible.value = true
}

// Persist the form after inline validation; the server validates the whole
// merged case. A content edit comes back with a new id (the old case is
// retired) — the refresh makes both rows visible. An ifeval case's
// rule_config is never submitted: the check params are seed-cast, and the
// server preserves them across the retire-and-mint replace (ticket 97).
async function onSave() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload = {
      prompt: form.prompt,
      verdict_type: form.verdict_type,
      rule_config:
        form.verdict_type === 'rule' && form.ruleMode !== 'ifeval'
          ? { mode: form.ruleMode, expected: form.ruleExpected }
          : form.verdict_type === 'rule'
            ? undefined
            : null,
      rubric: form.verdict_type === 'judge' ? form.rubric : null,
      difficulty: form.difficulty,
      sample_count: form.sampleCountCustom ? form.sampleCount : null,
      enabled: form.enabled,
    }
    if (editingId.value === null) {
      await createCase({ suite_id: editingSuiteId.value, ...payload })
      ElMessage.success('Case 已创建')
    } else {
      await patchCase(editingId.value, payload)
      ElMessage.success('Case 已更新')
    }
    dialogVisible.value = false
    await loadSuites()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
/* Admin console compact density: 12px card padding (ui-guidelines §2). */
.library-card {
  margin-bottom: 16px;
  --el-card-padding: 12px;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.card-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.capability-filter {
  width: 160px;
}
.load-alert {
  margin-bottom: 12px;
}
.suite-title {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-primary);
  max-width: 560px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.suite-tag {
  margin-left: 8px;
  flex-shrink: 0;
}
.suite-actions {
  margin-bottom: 8px;
}
.ifeval-params {
  width: 100%;
}
.ifeval-note {
  margin-top: var(--hs-space-1);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  line-height: 1.5;
}
</style>
