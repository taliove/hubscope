<template>
  <!-- 模型管理 (GH #112, spec 0018 IA mapping): AdminView's 资源 / 分类规则
       tabs move out to a first-class sidebar entry. Transitional shell: the
       existing panels unchanged inside the new shell; the v2 rebuild lands
       in T8. AdminView stays alive at /admin until T11 (eval-ops and the
       case library still live there), so this page and AdminView share the
       same panels during the mixed period. -->
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
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { useAdminData } from '@/composables/useAdminData'
import HubManager from '@/components/HubManager.vue'
import ModelAdder from '@/components/ModelAdder.vue'
import EndpointTable from '@/components/EndpointTable.vue'
import ClassificationRules from '@/components/ClassificationRules.vue'
import ImageParamRules from '@/components/ImageParamRules.vue'

const { hubs, endpointRows, endpointlessRows, loading, reloadModels, reloadAll, reloadHubs } = useAdminData()

// el-tabs keeps every pane mounted (no lazy rendering), so HubManager's
// internal sync polling keeps running on the inactive tab (AdminView
// precedent).
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
.models-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px;
}
.page-title {
  margin: 0 0 var(--hs-space-4);
  font-size: var(--hs-text-xl);
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
