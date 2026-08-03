<template>
  <!-- 模型管理 (GH #112 shell, GH #119 migration, spec 0018 IA mapping):
       AdminView's 资源 / 分类规则 panes are this first-class sidebar entry.
       The panels are unchanged (EP complex widgets + admin 12px density +
       560/320 form-width tiers carry over); the page adds the same ?tab=
       deep-link discipline AdminView had so legacy /admin?tab=resources|rules
       redirects land on the pane they name. -->
  <div class="models-page">
    <h1 class="page-title">模型管理</h1>
    <el-tabs v-model="activeTab" class="models-tabs">
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
          <ImageParamRules />
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import { useAdminData } from '@/composables/useAdminData'
import { MODELS_TABS, parseTabQuery, type ModelsTab } from '@/utils/adminNav'
import HubManager from '@/components/HubManager.vue'
import ModelAdder from '@/components/ModelAdder.vue'
import EndpointTable from '@/components/EndpointTable.vue'
import ClassificationRules from '@/components/ClassificationRules.vue'
import ImageParamRules from '@/components/ImageParamRules.vue'

const { hubs, endpointRows, endpointlessRows, loading, reloadModels, reloadAll, reloadHubs } = useAdminData()

const router = useRouter()
const route = useRoute()

// A valid ?tab= query overrides the default landing tab (AdminView GH #29
// precedent); an invalid value falls back silently. el-tabs keeps every pane
// mounted (no lazy rendering), so HubManager's internal sync polling keeps
// running on the inactive tab (AdminView precedent).
const activeTab = ref<ModelsTab>(parseTabQuery(route.query.tab, MODELS_TABS) ?? 'resources')

// A manual tab switch syncs the URL so a stale deep-link query cannot drag
// the user back (AdminView precedent).
watch(activeTab, (tab) => {
  if (route.query.tab === tab) return
  void router.replace({ query: { tab } })
})

// Late navigation to /models?tab=... while already mounted (e.g. a legacy
// /admin redirect followed from an open session) re-targets the tab;
// invalid values are ignored silently.
watch(
  () => route.query.tab,
  (raw) => {
    const tab = parseTabQuery(raw, MODELS_TABS)
    if (tab !== null && tab !== activeTab.value) activeTab.value = tab
  },
)

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
.models-page {
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
.models-tabs {
  width: 100%;
}
.tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
