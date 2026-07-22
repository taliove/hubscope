<template>
  <el-card shadow="never" class="settings-panel admin-card">
    <template #header>
      <span>告警与评估设置</span>
    </template>

    <el-form :model="form" label-width="160px" :disabled="saving">
      <el-form-item label="飞书 Webhook">
        <el-input
          v-model="form.lark_webhook_url"
          placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..."
          clearable
        />
      </el-form-item>
      <el-form-item label="端点告警">
        <el-switch v-model="form.alert_enabled" active-text="开" inactive-text="关" />
      </el-form-item>
      <el-form-item label="分数大跌告警">
        <el-switch v-model="form.score_drop_alert_enabled" active-text="开" inactive-text="关" />
        <div class="field-hint block-hint">
          每轮评估(Campaign)完成后,与上一轮同评估集对比,任一评估集得分跌幅超过 0.2
          触发,告警附各评估集跌幅与变动题目明细;题目版本变更的对比自动跳过,并标注「题目已变更,分数不可比」
        </div>
      </el-form-item>
      <el-form-item label="裁判模型">
        <el-input v-model="form.judge_model" placeholder="claude-opus-4-8" />
      </el-form-item>
      <el-form-item label="默认采样次数">
        <el-input-number v-model="form.default_sample_count" :min="1" :max="10" />
        <span class="field-hint">每题作答次数,多次取平均;题目可单独覆盖</span>
      </el-form-item>
      <el-form-item label="每周计划">
        <span class="field-static">每周日凌晨自动发起全量评估(内置计划,无需配置)</span>
      </el-form-item>
      <el-form-item label="评估集权重">
        <div class="weights-block">
          <div v-for="suite in enabledSuites" :key="suite.key" class="weight-item">
            <span class="weight-label" :title="suite.key">{{ suite.name }}</span>
            <el-input-number
              v-model="form.suite_weights[suite.key]"
              :min="0.1"
              :max="100"
              :step="1"
              size="small"
            />
          </div>
          <div class="field-hint">权重越大,该评估集对排行榜总分影响越大;缺省等权 1</div>
        </div>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="saving" @click="onSave">保存设置</el-button>
      </el-form-item>
    </el-form>

    <el-divider content-position="left">近期告警事件</el-divider>
    <el-table :data="alerts" size="small" empty-text="暂无告警事件">
      <el-table-column prop="kind" label="类型" width="110">
        <template #default="{ row }">
          <el-tag :type="kindTagType(row.kind)" size="small">{{ kindLabel(row.kind) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="message" label="内容" show-overflow-tooltip />
      <el-table-column label="发送" width="80">
        <template #default="{ row }">
          <span v-if="row.kind === 'score_drop_skipped'" class="sent-skip">未发送</span>
          <span v-else :class="row.sent_ok ? 'sent-ok' : 'sent-fail'">{{ row.sent_ok ? '成功' : '失败' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getSettings,
  updateSettings,
  listAlerts,
  type AlertEvent,
  type AppSettings,
} from '@/api/settings'
import { listSuites } from '@/api/evals'
import { formatTime } from '@/utils/format'
import type { Suite } from '@/api/types'

const form = reactive<AppSettings>({
  lark_webhook_url: '',
  alert_enabled: true,
  score_drop_alert_enabled: true,
  judge_model: 'claude-opus-4-8',
  default_sample_count: 1,
  suite_weights: {},
})
const suites = ref<Suite[]>([])
const alerts = ref<AlertEvent[]>([])
const saving = ref(false)

const KIND_LABELS: Record<AlertEvent['kind'], string> = {
  down: '故障',
  recovered: '恢复',
  score_drop: '分数大跌',
  score_drop_skipped: '对比跳过',
}

function kindLabel(kind: AlertEvent['kind']): string {
  return KIND_LABELS[kind] ?? kind
}

function kindTagType(kind: AlertEvent['kind']): 'danger' | 'success' | 'warning' {
  if (kind === 'down') return 'danger'
  if (kind === 'recovered') return 'success'
  return 'warning'
}

// Only suites in the evaluation rotation take a weight input: retired
// suites no longer join campaigns, so weighting them would be misleading.
const enabledSuites = computed(() => suites.value.filter(s => s.enabled))

// Fill every enabled suite key into the weights map so the inputs stay
// controlled; suites without a configured weight default to 1 (equal).
function fillSuiteWeights(weights: Record<string, number>) {
  const filled: Record<string, number> = {}
  for (const suite of enabledSuites.value) {
    const w = weights[suite.key]
    filled[suite.key] = w !== undefined && w > 0 ? w : 1
  }
  form.suite_weights = filled
}

async function onSave() {
  saving.value = true
  try {
    const saved = await updateSettings({ ...form })
    Object.assign(form, saved)
    fillSuiteWeights(saved.suite_weights)
    ElMessage.success('设置已保存,即时生效')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    const [settings, suiteList, alertList] = await Promise.all([getSettings(), listSuites(), listAlerts()])
    Object.assign(form, settings)
    suites.value = suiteList
    fillSuiteWeights(settings.suite_weights)
    alerts.value = alertList
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
})
</script>

<style scoped>
.settings-panel {
  width: 100%;
}
/* Admin density tier: compact 12px card padding (ui-guidelines §2). */
.admin-card {
  --el-card-padding: 12px;
}
/* Delivery outcome maps to the semantic status palette (§3). */
.sent-ok {
  color: var(--el-color-success);
}
.sent-fail {
  color: var(--el-color-danger);
}
.field-hint {
  margin-left: 12px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.weights-block .field-hint {
  margin-left: 0;
}
/* Multi-line switch hints align under the control, not beside it. */
.block-hint {
  margin-left: 0;
  line-height: 1.5;
}
/* Informational (non-editable) settings rows reuse the secondary text tone. */
.field-static {
  font-size: var(--hs-text-md);
  color: var(--hs-text-secondary);
}
/* A recorded-but-unsent event (skipped comparison) reads neutral. */
.sent-skip {
  color: var(--hs-text-secondary);
}
</style>
