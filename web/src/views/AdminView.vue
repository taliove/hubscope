<template>
  <div class="admin">
    <main class="admin-body">
      <el-tabs v-model="activeTab" class="admin-tabs">
        <el-tab-pane label="资源" name="resources">
          <div class="tab-stack">
            <HubManager :hubs="hubs" :loading="loading" @changed="onHubsChanged" @sync-settled="onSyncSettled" />
            <ModelAdder :hubs="hubs" @added="reloadModels" />
            <EndpointTable :rows="endpointRows" :endpointless-rows="endpointlessRows" :loading="loading" @changed="reloadModels" />
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
        <el-tab-pane v-if="canManageUsers" label="用户管理" name="users">
          <div class="tab-stack">
            <UserManager />
          </div>
        </el-tab-pane>
        <el-tab-pane label="设置" name="settings">
          <div class="tab-stack">
            <SettingsPanel :highlight-item="settingsItem" />
            <ShareLinksPanel />
          </div>
        </el-tab-pane>
      </el-tabs>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAdminData } from '@/composables/useAdminData'
import { fetchAuthStatus } from '@/api/auth'
import { parseAdminTabQuery, parseSettingsItemQuery } from '@/utils/adminNav'
import HubManager from '@/components/HubManager.vue'
import ModelAdder from '@/components/ModelAdder.vue'
import EndpointTable from '@/components/EndpointTable.vue'
import ClassificationRules from '@/components/ClassificationRules.vue'
import EvalOpsPanel from '@/components/EvalOpsPanel.vue'
import CaseLibrary from '@/components/CaseLibrary.vue'
import AuditLogs from '@/components/AuditLogs.vue'
import UserManager from '@/components/UserManager.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import ShareLinksPanel from '@/components/ShareLinksPanel.vue'

const { hubs, endpointRows, endpointlessRows, loading, reloadModels, reloadAll, reloadHubs } = useAdminData()

const router = useRouter()
const route = useRoute()

// Default landing tab; el-tabs keeps every pane mounted (no lazy rendering),
// so HubManager's internal sync polling keeps running on inactive tabs.
// A valid ?tab= query (GH #29) overrides the default so a deep link lands
// on the tab it names; an invalid value falls back silently.
const activeTab = ref(parseAdminTabQuery(route.query.tab) ?? 'resources')

// Settings item anchor requested via ?item= (GH #29): only meaningful on
// the settings tab — an item query naming another tab is ignored, so no
// hidden pane is ever scrolled or highlighted.
const settingsItem = computed(() =>
  activeTab.value === 'settings' ? parseSettingsItemQuery(route.query.item) : null,
)

// A manual tab switch syncs the URL (same discipline as issue #16's batch
// query): the stale deep-link query can never drag the user back, and the
// item anchor is dropped so an old link cannot re-highlight a row the user
// navigated away from. A query-driven change (route.query.tab already
// matches) is left untouched — the URL is already the source of truth.
watch(activeTab, (tab) => {
  if (route.query.tab === tab) return
  void router.replace({ query: { tab } })
})

// Late navigation to /admin?tab=... while the page is already mounted
// (e.g. following a settings deep link from an open admin session):
// re-target the tab; invalid values are ignored silently.
watch(
  () => route.query.tab,
  (raw) => {
    const tab = parseAdminTabQuery(raw)
    if (tab !== null && tab !== activeTab.value) activeTab.value = tab
  },
)

// User-management tab is visible only to admin+super_admin (operator/viewer
// would be routed away by the guard, and the backend 403s them anyway; hiding
// the tab avoids a dead control). The role is read once on mount; it does not
// need to poll (role does not change mid-session).
const canManageUsers = ref(false)

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
    const status = await fetchAuthStatus()
    if (status.user) {
      canManageUsers.value = status.user.role === 'super_admin' || status.user.role === 'admin'
    }
    // A ?tab=users deep link from a role without the users pane lands on
    // the default tab instead of a pane-less empty page.
    if (activeTab.value === 'users' && !canManageUsers.value) {
      activeTab.value = 'resources'
    }
  } catch {
    // Router guard handles redirect; default hides the user tab.
  }
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
