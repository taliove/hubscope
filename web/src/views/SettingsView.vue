<template>
  <!-- 系统设置 (GH #112 shell, GH #119 migration, spec 0018 IA mapping):
       AdminView's 设置 / 操作日志 / 用户管理 panes plus the folded-in task
       center (decision 63). The panels are unchanged (EP complex widgets +
       admin 12px density + 560/320 form-width tiers carry over); the task
       center renders embedded-only now, so its page chrome is gone and the
       T2 double-padding is resolved by construction. -->
  <div class="settings-page">
    <h1 class="page-title">系统设置</h1>
    <el-tabs v-model="activeTab" class="settings-tabs">
      <el-tab-pane label="设置" name="settings">
        <div class="tab-stack">
          <SettingsPanel :highlight-item="settingsItem" />
          <ShareLinksPanel />
        </div>
      </el-tab-pane>
      <el-tab-pane label="任务中心" name="tasks">
        <TaskCenterView />
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
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchAuthStatus } from '@/api/auth'
import { SETTINGS_TABS, parseSettingsItemQuery, parseTabQuery, type SettingsTab } from '@/utils/adminNav'
import SettingsPanel from '@/components/SettingsPanel.vue'
import ShareLinksPanel from '@/components/ShareLinksPanel.vue'
import TaskCenterView from '@/views/TaskCenterView.vue'
import AuditLogs from '@/components/AuditLogs.vue'
import UserManager from '@/components/UserManager.vue'

const router = useRouter()
const route = useRoute()

// A valid ?tab= query overrides the default landing tab (AdminView GH #29
// precedent); an invalid value falls back silently. Panes stay mounted (no
// lazy) like AdminView.
const activeTab = ref<SettingsTab>(parseTabQuery(route.query.tab, SETTINGS_TABS) ?? 'settings')

// Settings item anchor (?item=, GH #29): only meaningful on the settings tab.
const settingsItem = computed(() =>
  activeTab.value === 'settings' ? parseSettingsItemQuery(route.query.item) : null,
)

// A manual tab switch syncs the URL so a stale deep-link query cannot drag
// the user back (AdminView precedent).
watch(activeTab, (tab) => {
  if (route.query.tab === tab) return
  void router.replace({ query: { tab } })
})

// Late navigation to /settings?tab=... while already mounted re-targets the
// tab; invalid values are ignored silently.
watch(
  () => route.query.tab,
  (raw) => {
    const tab = parseTabQuery(raw, SETTINGS_TABS)
    if (tab !== null && tab !== activeTab.value) activeTab.value = tab
  },
)

// User-management pane is visible only to admin+super_admin (AdminView
// precedent); the role is read once on mount.
const canManageUsers = ref(false)

onMounted(async () => {
  try {
    const status = await fetchAuthStatus()
    if (status.user) {
      canManageUsers.value = status.user.role === 'super_admin' || status.user.role === 'admin'
    }
    // A ?tab=users deep link from a lesser role lands on the default tab.
    if (activeTab.value === 'users' && !canManageUsers.value) {
      activeTab.value = 'settings'
    }
  } catch {
    // Router guard handles redirect; default hides the user pane.
  }
})
</script>

<style scoped>
.settings-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px;
}
.page-title {
  margin: 0 0 var(--hs-space-4);
  font-size: var(--hs-text-3xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.settings-tabs {
  width: 100%;
}
.tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
