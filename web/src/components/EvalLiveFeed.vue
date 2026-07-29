<template>
  <el-card shadow="never" class="live-feed-card">
    <div class="feed-head">
      <span class="feed-title">实时动态</span>
      <span class="feed-note">题目级判分事件,随轮询增量刷新</span>
    </div>

    <!-- Error state: reason plus a retry entry (ui-guidelines §6). The
         accumulated entries stay visible below the alert. -->
    <el-alert v-if="error" type="error" :closable="false" class="feed-alert">
      <template #title>动态加载失败:{{ error }}</template>
      <el-button size="small" @click="emit('retry')">重试</el-button>
    </el-alert>

    <!-- First-load state: only before the first response (entries still
         empty); later ticks keep showing the accumulated list. -->
    <el-skeleton v-if="loading && entries.length === 0" :rows="3" animated />

    <!-- Empty state: the batch has not judged a single case yet. -->
    <el-empty
      v-else-if="entries.length === 0 && !error"
      description="暂无判分动态:等待第一题判分完成"
    />

    <!-- Newest-first event stream; vertical scroll inside the card, never
         horizontal (ui-guidelines §4). -->
    <div v-if="entries.length > 0" class="feed-list">
      <div v-for="e in displayEntries" :key="e.id" class="feed-row">
        <span class="feed-time">{{ formatClockTime(e.created_at) }}</span>
        <span class="feed-model" :title="e.model_id">{{ e.model_id }}</span>
        <span class="feed-suite" :title="e.suite_name">{{ e.suite_name }}</span>
        <span class="feed-case" :title="e.case_prompt">#{{ e.case_id }} {{ e.case_prompt }}</span>
        <span class="feed-meta">
          <el-tag size="small" type="info">{{ verdictTypeLabel(e.verdict_type) }}</el-tag>
          <span
            class="feed-score"
            :class="scoreClass(e.score)"
            :title="e.score === null ? '裁判失败,未判分' : undefined"
          >
            {{ scoreText(e.score) }}
          </span>
          <span class="feed-latency">{{ formatMs(e.latency_ms) }}</span>
          <!-- GH #29: only a judge failure deep-links to the setting that
               governs it; rule failures and scored rows render as before. -->
          <router-link
            v-if="isJudgeFailure(e)"
            class="feed-fix-link"
            :to="{ path: '/admin', query: { tab: 'settings', item: 'judge_model' } }"
          >检查裁判模型设置</router-link>
        </span>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { LiveFeedEntry } from '@/api/types'
import { formatClockTime, formatMs, formatScore } from '@/utils/format'
import { liveFeedDisplay, isJudgeFailure, verdictTypeLabel } from '@/utils/liveFeed'
import { scoreBand } from '@/utils/scoreTier'

// EvalLiveFeed is the running batch's case-level event stream (issue #17):
// one row per judged case — model, suite, case, verdict method, score,
// latency — newest first. It is a pure display unit: the parent view owns
// the cursor, the incremental fetch and the polling timer (ui-guidelines
// §6 — no timer of its own), and mounts this card only for unfinished
// batches on the console leaderboard. Console-only by construction: the
// shared/public surface never renders it, so no live feed call can ever
// leave the session boundary (spec 0004).
const props = defineProps<{
  entries: LiveFeedEntry[]
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'retry'): void
}>()

const displayEntries = computed(() => liveFeedDisplay(props.entries))

// Raw 0~1 per-case score → the 0-100 display scale via the centralized
// formatter (ui-guidelines §7: raw scores never leave the API layer).
function scoreText(score: number | null): string {
  return formatScore(score === null ? null : score * 100)
}

// Score band coloring (ui-guidelines §3: the shared >=80/>=50 thresholds);
// a null score (judge failure) renders as a placeholder, never as zero.
function scoreClass(score: number | null): string {
  if (score === null) return 'score-none'
  return `score-${scoreBand(score * 100)}`
}
</script>

<style scoped>
/* Consumption-page density: 16px card padding via the variable
   (ui-guidelines §2). */
.live-feed-card {
  --el-card-padding: 16px;
  margin-bottom: 16px;
}
.feed-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 12px;
}
.feed-title {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.feed-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.feed-alert {
  margin-bottom: 12px;
}
.feed-list {
  max-height: 320px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}
.feed-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding: 6px 0;
  border-bottom: 1px solid var(--hs-border-light);
}
.feed-row:last-child {
  border-bottom: none;
}
.feed-time {
  flex-shrink: 0;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  font-variant-numeric: tabular-nums;
}
.feed-model {
  flex-shrink: 0;
  max-width: 200px;
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.feed-suite {
  flex-shrink: 0;
  max-width: 140px;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.feed-case {
  flex: 1;
  min-width: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.feed-meta {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 12px;
}
.feed-score {
  font-size: var(--hs-text-md);
  font-weight: 600;
  min-width: 40px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.score-success {
  color: var(--hs-success);
}
.score-warning {
  color: var(--hs-warning);
}
.score-danger {
  color: var(--hs-danger);
}
.score-none {
  color: var(--hs-text-placeholder);
  font-weight: 400;
}
.feed-latency {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  min-width: 56px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
/* Judge-failure deep link (GH #29): brand-colored inline action, secondary
   weight so it never competes with the row's score (ui-guidelines §2/§7). */
.feed-fix-link {
  font-size: var(--hs-text-xs);
  color: var(--hs-brand);
  text-decoration: none;
  white-space: nowrap;
}
.feed-fix-link:hover {
  color: var(--hs-brand-hover);
  text-decoration: underline;
}
</style>
