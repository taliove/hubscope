<template>
  <div ref="rootEl" class="settings-stack">
    <el-card shadow="never" class="admin-card">
      <template #header>
        <span>告警设置</span>
      </template>

      <el-form :model="form" label-width="120px" :disabled="saving">
        <el-form-item label="飞书 Webhook" data-item="lark_webhook_url">
          <el-input
            v-model="form.lark_webhook_url"
            class="webhook-input"
            placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..."
            clearable
          />
          <el-button class="test-lark-btn" :loading="testing" @click="onTestLark">发送测试</el-button>
        </el-form-item>
        <el-form-item label="端点告警" data-item="alert_enabled">
          <el-switch v-model="form.alert_enabled" active-text="开" inactive-text="关" />
        </el-form-item>
        <el-form-item label="分数大跌告警" data-item="score_drop_alert_enabled">
          <el-switch v-model="form.score_drop_alert_enabled" active-text="开" inactive-text="关" />
          <div class="field-hint block-hint">
            每轮评估(Campaign)完成后,与上一轮同评估集对比,任一评估集得分跌幅超过 0.2
            触发,告警附各评估集跌幅与变动题目明细;题目版本变更的对比自动跳过,并标注「题目已变更,分数不可比」
          </div>
        </el-form-item>
        <el-form-item label="静默时段" data-item="quiet_hours_enabled">
          <el-switch v-model="form.quiet_hours_enabled" active-text="开" inactive-text="关" />
          <div class="field-hint block-hint">
            启用后,静默时段内告警只记录不发送,时段结束时发送一条摘要(仅列仍故障端点/厂商组与期内分数大跌);登录爆破告警不受静默约束,保持即时
          </div>
        </el-form-item>
        <el-form-item label="静默起止" data-item="quiet_hours_start">
          <el-select v-model="form.quiet_hours_start" :disabled="!form.quiet_hours_enabled">
            <el-option v-for="h in hourOptions" :key="h" :label="hourLabel(h)" :value="h" />
          </el-select>
          <span class="field-hint">至</span>
          <el-select v-model="form.quiet_hours_end" :disabled="!form.quiet_hours_enabled">
            <el-option v-for="h in hourOptions" :key="h" :label="hourLabel(h)" :value="h" />
          </el-select>
          <div class="field-hint block-hint">
            按服务器本地时区的整点小时,支持跨日(如 23 时至次日 7 时);开始与结束相同视为未启用
          </div>
        </el-form-item>
      </el-form>

      <el-divider content-position="left">近期告警事件</el-divider>
      <el-table :data="alerts" size="small" empty-text="暂无告警事件">
        <el-table-column prop="kind" label="类型" width="110">
          <template #default="{ row }">
            <el-tag :type="kindTagType(row.kind)" size="small">{{ kindLabel(row.kind) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="message" label="内容" min-width="240" show-overflow-tooltip />
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

    <el-card shadow="never" class="admin-card">
      <template #header>
        <span>评估设置</span>
      </template>

      <el-form :model="form" label-width="120px" :disabled="saving">
        <el-form-item label="裁判模型" data-item="judge_model">
          <el-input v-model="form.judge_model" class="judge-input" placeholder="claude-opus-4-8" />
        </el-form-item>
        <el-form-item label="默认采样次数" data-item="default_sample_count">
          <el-input-number v-model="form.default_sample_count" :min="1" :max="10" />
          <span class="field-hint">每题作答次数,多次取平均;题目可单独覆盖</span>
        </el-form-item>
        <el-form-item label="评估并发数" data-item="eval_concurrency">
          <el-input-number v-model="form.eval_concurrency" :min="1" :max="16" />
          <span class="field-hint">同时执行的评估单元(评估集 × 模型)数;调大可缩短批次时长,但会增加 Hub 压力</span>
        </el-form-item>
        <el-form-item label="每周计划">
          <span class="field-static">每周日凌晨自动发起全量评估(内置计划,无需配置)</span>
        </el-form-item>
        <el-form-item label="评估集权重" data-item="suite_weights">
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
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getSettings,
  updateSettings,
  listAlerts,
  testLark,
  type AlertEvent,
  type AppSettings,
} from '@/api/settings'
import { listSuites } from '@/api/evals'
import { formatTime } from '@/utils/format'
import type { SettingsItem } from '@/utils/adminNav'
import type { Suite } from '@/api/types'

// Settings item anchor requested via /admin?tab=settings&item=... (GH #29):
// AdminView owns the query parsing; this panel only scrolls the named row
// into view and flashes it. Null = no deep link, render as before.
const props = defineProps<{
  highlightItem?: SettingsItem | null
}>()

const rootEl = ref<HTMLElement | null>(null)

// How long the flash stays at full brand-soft before fading back out
// through the same --hs-transition it arrived with.
const HIGHLIGHT_HOLD_MS = 2400
let highlightTimer: ReturnType<typeof setTimeout> | undefined

function clearHighlight() {
  if (highlightTimer !== undefined) {
    clearTimeout(highlightTimer)
    highlightTimer = undefined
  }
  rootEl.value?.querySelector('.setting-highlight')?.classList.remove('setting-highlight')
}

// Scroll the anchored row into view and flash it. Re-targeting replaces any
// in-flight flash so two quick deep links never stack timers.
async function flashItem(item: SettingsItem) {
  await nextTick()
  const el = rootEl.value?.querySelector<HTMLElement>(`[data-item="${item}"]`)
  if (!el) return
  clearHighlight()
  el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  el.classList.add('setting-highlight')
  highlightTimer = setTimeout(() => {
    el.classList.remove('setting-highlight')
    highlightTimer = undefined
  }, HIGHLIGHT_HOLD_MS)
}

watch(
  () => props.highlightItem,
  (item) => {
    if (item) void flashItem(item)
  },
)

onBeforeUnmount(clearHighlight)

const form = reactive<AppSettings>({
  lark_webhook_url: '',
  alert_enabled: true,
  score_drop_alert_enabled: true,
  judge_model: 'claude-opus-4-8',
  default_sample_count: 1,
  eval_concurrency: 4,
  suite_weights: {},
  quiet_hours_enabled: false,
  quiet_hours_start: 23,
  quiet_hours_end: 7,
})

// Quiet-hours bounds: integer hours 0–23 rendered as HH:00 (spec 0017
// ticket 4). el-select keeps its component default width (§4 admin form
// tiers: el-switch / el-select are not width-capped).
const hourOptions = Array.from({ length: 24 }, (_, h) => h)
function hourLabel(h: number): string {
  return `${String(h).padStart(2, '0')}:00`
}
const suites = ref<Suite[]>([])
const alerts = ref<AlertEvent[]>([])
const saving = ref(false)
const testing = ref(false)

const KIND_LABELS: Record<AlertEvent['kind'], string> = {
  down: '故障',
  recovered: '恢复',
  score_drop: '分数大跌',
  score_drop_skipped: '对比跳过',
  test: '测试',
}

function kindLabel(kind: AlertEvent['kind']): string {
  return KIND_LABELS[kind] ?? kind
}

// The manual channel check is not a health signal: it takes the neutral
// info tone rather than a status color.
function kindTagType(kind: AlertEvent['kind']): 'danger' | 'success' | 'warning' | 'info' {
  if (kind === 'down') return 'danger'
  if (kind === 'recovered') return 'success'
  if (kind === 'test') return 'info'
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

// Sends the test message to the address currently in the input (not the
// saved setting). Success or failure, the attempt lands in the alert history
// as kind="test", so the table refreshes after every try.
async function onTestLark() {
  const url = form.lark_webhook_url.trim()
  if (!url) {
    ElMessage.warning('请先填写飞书 Webhook 地址')
    return
  }
  testing.value = true
  try {
    const result = await testLark(url)
    if (result.sent_ok) {
      ElMessage.success('测试消息已发送,请到飞书群确认收到')
    } else {
      ElMessage.error(`测试消息发送失败:${result.error ?? '未知原因'}`)
    }
    alerts.value = await listAlerts()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    testing.value = false
  }
}

onMounted(async () => {
  // Initial deep link (/admin?tab=settings&item=...): the tab is already
  // active, so the anchored row can flash as soon as the pane is painted.
  if (props.highlightItem) void flashItem(props.highlightItem)
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
/* Two stacked section cards (alert / eval) with block-level rhythm (§4). */
.settings-stack {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-4);
  width: 100%;
}
/* Deep-link anchor rows (GH #29): the flash arrives and leaves on the
   shared transition token, so it breathes instead of blinking. */
.settings-stack [data-item] {
  border-radius: var(--hs-radius-sm);
  transition: background-color var(--hs-transition);
}
/* The flash tint is the brand soft surface — emphasis by the existing
   brand family, no new hue (ui-guidelines §2/§3). */
.settings-stack .setting-highlight {
  background-color: var(--hs-brand-soft);
}
/* Admin density tier: compact 12px card padding (ui-guidelines §2). */
.admin-card {
  --el-card-padding: 12px;
}
/* Admin form control width tiers (§4): 560px long-input tier for URLs. */
.webhook-input {
  width: 560px;
  flex: 0 1 auto;
}
/* 320px standard-input tier for short identifiers (§4). */
.judge-input {
  width: 320px;
}
/* Delivery outcome maps to the semantic status palette (§3). */
.sent-ok {
  color: var(--hs-success);
}
.sent-fail {
  color: var(--hs-danger);
}
.field-hint {
  margin-left: var(--hs-space-3);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
/* Inline action button stays on the input row; wraps only when the
   container is too narrow (§4 admin form tiers). */
.test-lark-btn {
  margin-left: var(--hs-space-2);
  flex: none;
}
/* Suite weights flow horizontally in compact label+control groups (§4). */
.weights-block {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  column-gap: var(--hs-space-4);
  row-gap: var(--hs-space-2);
}
.weight-item {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
}
.weight-label {
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.weights-block .field-hint {
  flex-basis: 100%;
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
