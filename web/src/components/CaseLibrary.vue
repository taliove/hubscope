<template>
  <el-card shadow="never" class="library-card">
    <div class="card-title">
      题库
      <span v-if="!authed" class="auth-hint">登录后可新增 / 修改 Case</span>
    </div>

    <el-collapse>
      <el-collapse-item v-for="suite in suites" :key="suite.id" :name="suite.id">
        <template #title>
          <span class="suite-title">{{ suite.name }}({{ suite.key }},{{ suite.cases.length }} 题,v{{ suite.version }})</span>
        </template>
        <div v-if="authed" class="suite-actions">
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
          <el-table-column v-if="authed" label="操作" width="80" align="center">
            <template #default="{ row }">
              <el-button size="small" text type="primary" @click="openEdit(suite.id, row)">编辑</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-collapse-item>
    </el-collapse>

    <!-- Create/edit dialog: fields follow the verdict type. -->
    <el-dialog v-model="dialogVisible" :title="editingId === null ? '新增 Case' : `编辑 Case #${editingId}`" width="560px">
      <el-form label-width="90px">
        <el-form-item label="题目">
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
            </el-select>
          </el-form-item>
          <el-form-item label="期望值">
            <el-input v-model="form.ruleExpected" placeholder="命中得 1 分,否则 0 分" />
          </el-form-item>
        </template>
        <el-form-item v-else label="评分标准">
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
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createCase, patchCase } from '@/api/evals'
import type { Difficulty, EvalCase, Suite, VerdictType } from '@/api/types'

// Case library: read-only browsing for everyone; create/edit forms appear
// only for authenticated admins (the server still enforces the session).
// Cases are immutable server-side: a content edit returns a new case id and
// retires the old row, which stays visible as disabled after the refresh.
defineProps<{
  suites: Suite[]
  authed: boolean
}>()

const emit = defineEmits<{ refresh: [] }>()

const dialogVisible = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const editingSuiteId = ref<number>(0)

interface CaseForm {
  prompt: string
  verdict_type: VerdictType
  ruleMode: 'exact' | 'regex' | 'contains'
  ruleExpected: string
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
  rubric: '',
  difficulty: 'basic',
  sampleCount: 1,
  sampleCountCustom: false,
  enabled: true,
})

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
    rubric: c.rubric ?? '',
    difficulty: c.difficulty,
    sampleCount: c.sample_count ?? 1,
    sampleCountCustom: c.sample_count !== null,
    enabled: c.enabled,
  })
  dialogVisible.value = true
}

// Persist the form; the server validates the whole merged case. A content
// edit comes back with a new id (the old case is retired) — the refresh
// makes both rows visible.
async function onSave() {
  saving.value = true
  try {
    const payload = {
      prompt: form.prompt,
      verdict_type: form.verdict_type,
      rule_config:
        form.verdict_type === 'rule'
          ? { mode: form.ruleMode, expected: form.ruleExpected }
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
    emit('refresh')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.library-card {
  margin-bottom: 16px;
}
.card-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
  display: flex;
  align-items: baseline;
  gap: 12px;
}
.auth-hint {
  font-size: 12px;
  font-weight: 400;
  color: #909399;
}
.suite-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}
.suite-actions {
  margin-bottom: 8px;
}
</style>
