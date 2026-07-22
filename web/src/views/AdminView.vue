<template>
  <div class="admin">
    <main class="admin-body">
      <el-tabs v-model="activeTab" class="admin-tabs">
        <el-tab-pane label="资源" name="resources">
          <div class="tab-stack">
            <HubManager :hubs="hubs" :loading="loading" @changed="onHubsChanged" @sync-settled="onSyncSettled" />
            <ModelAdder :hubs="hubs" @added="reloadModels" />
            <EndpointTable :rows="endpointRows" :loading="loading" @changed="reloadModels" />
          </div>
        </el-tab-pane>
        <el-tab-pane label="分类规则" name="rules">
          <div class="tab-stack">
            <ClassificationRules @changed="reloadModels" />
          </div>
        </el-tab-pane>
        <el-tab-pane label="评估运营" name="eval-ops">
          <div class="tab-stack">
            <EvalOpsPanel />
          </div>
        </el-tab-pane>
        <el-tab-pane label="题库" name="case-library">
          <div class="tab-stack">
            <CaseLibrary />
          </div>
        </el-tab-pane>
        <el-tab-pane label="操作日志" name="logs">
          <div class="tab-stack">
            <AuditLogs />
          </div>
        </el-tab-pane>
        <el-tab-pane label="设置" name="settings">
          <div class="tab-stack">
            <SettingsPanel />
          </div>
        </el-tab-pane>
      </el-tabs>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useAdminData } from '@/composables/useAdminData'
import HubManager from '@/components/HubManager.vue'
import ModelAdder from '@/components/ModelAdder.vue'
import EndpointTable from '@/components/EndpointTable.vue'
import ClassificationRules from '@/components/ClassificationRules.vue'
import EvalOpsPanel from '@/components/EvalOpsPanel.vue'
import CaseLibrary from '@/components/CaseLibrary.vue'
import AuditLogs from '@/components/AuditLogs.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'

const { hubs, endpointRows, loading, reloadModels, reloadAll, reloadHubs } = useAdminData()

// Default landing tab; el-tabs keeps every pane mounted (no lazy rendering),
// so HubManager's internal sync polling keeps running on inactive tabs.
const activeTab = ref('resources')

// Deleting a hub can cascade nothing but changing hubs may affect model hub names.
async function onHubsChanged() {
  await reloadHubs()
}

// A hub sync just settled: freshly discovered models and endpoints only show
// up after a full reload.
async function onSyncSettled() {
  await reloadAll()
}

onMounted(async () => {
  try {
    await reloadAll()
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
})
</script>

<style scoped>
.admin {
  max-width: 1200px;
  margin: 0 auto;
  /* Page vertical rhythm per ui-guidelines §2 (24px top/bottom). */
  padding: 24px 16px;
}
.admin-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.admin-tabs {
  width: 100%;
}
.tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
