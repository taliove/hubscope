<template>
  <el-card shadow="never" class="share-links-panel admin-card">
    <template #header>
      <div class="panel-header">
        <span>分享链接</span>
        <el-button size="small" :loading="loading" @click="reload">刷新</el-button>
      </div>
    </template>

    <!-- Error state with a retry entry (ui-guidelines §6). -->
    <el-alert v-if="error" type="error" :closable="false" class="state-block">
      <template #title>加载失败:{{ error }}</template>
      <el-button size="small" @click="reload">重试</el-button>
    </el-alert>

    <el-table v-else v-loading="loading" :data="links" size="small" empty-text="暂无分享链接,到考核批次报告页点击「分享」创建">
      <el-table-column label="批次" width="90">
        <template #default="{ row }">
          <router-link class="campaign-link" :to="`/campaigns/${row.campaign_id}/report`">
            #{{ row.campaign_id }}
          </router-link>
        </template>
      </el-table-column>
      <el-table-column label="链接" min-width="220">
        <template #default="{ row }">
          <span class="token" :title="shareLinkUrl(row.token)">{{ shortToken(row.token) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="created_by" label="创建人" width="90" />
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="120">
        <template #default="{ row }">
          <el-tag v-if="row.revoked_at" size="small" type="info" :title="formatTime(row.revoked_at)">
            已撤销
          </el-tag>
          <el-tag v-else size="small" type="success">有效</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="130">
        <template #default="{ row }">
          <el-button size="small" link type="primary" :disabled="!!row.revoked_at" @click="onCopy(row)">复制链接</el-button>
          <el-button
            v-if="!row.revoked_at"
            size="small"
            link
            type="danger"
            :loading="revokingId === row.id"
            @click="onRevoke(row)"
          >
            撤销
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listShareLinks, revokeShareLink, shareLinkUrl } from '@/api/shareLinks'
import { formatTime } from '@/utils/format'
import { copyText } from '@/utils/clipboard'
import type { ShareLink } from '@/api/types'

// Share-link management (ticket 33, ADR 0006): every minted link, live and
// revoked alike. Revocation is a destructive action, so it confirms first;
// the row stays visible afterwards (revoked links are kept for audit).
const links = ref<ShareLink[]>([])
const loading = ref(false)
const error = ref('')
const revokingId = ref<number | null>(null)

// Only the token's head renders in the table; the full URL is on hover and
// in the clipboard, never fully spelled out in a wide column.
function shortToken(token: string): string {
  return `/report/${token.slice(0, 8)}…`
}

async function onCopy(row: ShareLink) {
  if (await copyText(shareLinkUrl(row.token))) {
    ElMessage.success('链接已复制')
  } else {
    await ElMessageBox.alert(shareLinkUrl(row.token), '分享链接(请手动复制)', { confirmButtonText: '关闭' })
  }
}

async function onRevoke(row: ShareLink) {
  try {
    await ElMessageBox.confirm(
      `撤销后批次 #${row.campaign_id} 的该分享链接将立即失效(打开显示 404),且不可恢复。`,
      '撤销分享链接',
      { confirmButtonText: '撤销', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return // Cancelled: nothing to do.
  }
  revokingId.value = row.id
  try {
    await revokeShareLink(row.id)
    ElMessage.success('链接已撤销')
    await reload()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    revokingId.value = null
  }
}

async function reload() {
  loading.value = true
  error.value = ''
  try {
    links.value = await listShareLinks()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(reload)
</script>

<style scoped>
.share-links-panel {
  width: 100%;
}
/* Admin density tier: compact 12px card padding (ui-guidelines §2). */
.admin-card {
  --el-card-padding: 12px;
}
.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.state-block {
  margin-bottom: 12px;
}
.campaign-link {
  color: var(--hs-brand);
  text-decoration: none;
}
.campaign-link:hover {
  color: var(--hs-brand-hover);
}
/* Token preview stays single-line and truncation-safe (§6). */
.token {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
