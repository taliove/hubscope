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
      </el-form-item>
      <el-form-item label="裁判模型">
        <el-input v-model="form.judge_model" placeholder="claude-opus-4-8" />
      </el-form-item>
      <el-form-item label="默认采样次数">
        <el-input-number v-model="form.default_sample_count" :min="1" :max="10" />
        <span class="field-hint">每题作答次数,多次取平均;题目可单独覆盖</span>
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
          <span :class="row.sent_ok ? 'sent-ok' : 'sent-fail'">{{ row.sent_ok ? '成功' : '失败' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getSettings,
  updateSettings,
  listAlerts,
  type AlertEvent,
  type AppSettings,
} from '@/api/settings'
import { formatTime } from '@/utils/format'

const form = reactive<AppSettings>({
  lark_webhook_url: '',
  alert_enabled: true,
  score_drop_alert_enabled: true,
  judge_model: 'claude-opus-4-8',
  default_sample_count: 1,
})
const alerts = ref<AlertEvent[]>([])
const saving = ref(false)

const KIND_LABELS: Record<AlertEvent['kind'], string> = {
  down: '故障',
  recovered: '恢复',
  score_drop: '分数大跌',
}

function kindLabel(kind: AlertEvent['kind']): string {
  return KIND_LABELS[kind] ?? kind
}

function kindTagType(kind: AlertEvent['kind']): 'danger' | 'success' | 'warning' {
  if (kind === 'down') return 'danger'
  if (kind === 'recovered') return 'success'
  return 'warning'
}

async function onSave() {
  saving.value = true
  try {
    const saved = await updateSettings({ ...form })
    Object.assign(form, saved)
    ElMessage.success('设置已保存,即时生效')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    Object.assign(form, await getSettings())
    alerts.value = await listAlerts()
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
</style>
