<template>
  <div class="admin">
    <header class="admin-header">
      <h1>HubScope</h1>
      <span class="subtitle">管理视图</span>
      <router-link to="/" class="nav-link">状态总览</router-link>
      <el-button class="logout-button" size="small" @click="onLogout">退出登录</el-button>
    </header>

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
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAdminData } from '@/composables/useAdminData'
import { logout } from '@/api/auth'
import HubManager from '@/components/HubManager.vue'
import ModelAdder from '@/components/ModelAdder.vue'
import EndpointTable from '@/components/EndpointTable.vue'
import ClassificationRules from '@/components/ClassificationRules.vue'
import AuditLogs from '@/components/AuditLogs.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'

const router = useRouter()
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

// Clear the server session, then land on the login page regardless of outcome.
async function onLogout() {
  try {
    await logout()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    router.push('/login')
  }
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
  padding: 24px 16px 48px;
}
.admin-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 20px;
}
.admin-header h1 {
  margin: 0;
  font-size: 22px;
  color: #303133;
}
.subtitle {
  color: #909399;
  font-size: 14px;
}
.nav-link {
  margin-left: auto;
  font-size: 14px;
  color: #409eff;
  text-decoration: none;
}
.logout-button {
  margin-left: 12px;
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
