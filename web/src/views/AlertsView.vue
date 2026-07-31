<template>
  <!-- 故障记录 (GH #112, spec 0018 IA mapping): the alert history moves out
       of the settings area to a first-class sidebar entry. Transitional
       shell: the shared AlertHistory table inside the new shell; the event
       timeline rebuild lands in T10. -->
  <div class="alerts-page">
    <h1 class="page-title">故障记录</h1>
    <el-card shadow="never" class="admin-card">
      <AlertHistory />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import AlertHistory from '@/components/AlertHistory.vue'
</script>

<style scoped>
.alerts-page {
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
/* Admin console compact density: 12px card padding. */
.admin-card {
  --el-card-padding: 12px;
}
</style>
