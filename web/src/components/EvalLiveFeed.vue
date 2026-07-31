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
         horizontal (ui-guidelines §4). Rows expand inline (GH #41): click
         toggles the four-block detail (prompt / expectation / answer /
         verdict) fetched on demand per result id; the expansion set is
         keyed by entry id so polling prepends never collapse an open row. -->
    <div v-if="entries.length > 0" class="feed-list">
      <div v-for="e in displayEntries" :key="e.id" class="feed-item">
        <div class="feed-row feed-row-clickable" @click="toggle(e.id)">
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
                 governs it; rule failures and scored rows render as before.
                 The link must not toggle the row expansion. -->
            <router-link
              v-if="isJudgeFailure(e)"
              class="feed-fix-link"
              :to="{ path: '/settings', query: { tab: 'settings', item: 'judge_model' } }"
              @click.stop
            >检查裁判模型设置</router-link>
            <span class="feed-expand">{{ expanded.has(e.id) ? '▾' : '▸' }}</span>
          </span>
        </div>
        <!-- Inline expansion (GH #41): own loading/error/retry trio — a
             failed detail fetch only affects this expansion, never the
             stream itself. -->
        <div v-if="expanded.has(e.id)" class="feed-detail">
          <div v-if="detailLoading.has(e.id)" class="detail-note">详情加载中…</div>
          <div v-else-if="detailErrors.has(e.id)" class="detail-note">
            详情加载失败:{{ detailErrors.get(e.id) }}
            <el-button size="small" @click.stop="loadDetail(e.id)">重试</el-button>
          </div>
          <template v-else-if="details.has(e.id)">
            <div class="detail-block">
              <span class="detail-label">题目全文</span>
              <span class="detail-content" :class="{ 'detail-placeholder': !details.get(e.id)!.case_prompt }">{{
                details.get(e.id)!.case_prompt || '-'
              }}</span>
            </div>
            <div class="detail-block">
              <!-- Expectation label forks by verdict method, same caliber as
                   the row's verdict tag: rule = 期望答案, judge = 评分要点. -->
              <span class="detail-label">{{ details.get(e.id)!.verdict_type === 'judge' ? '评分要点' : '期望答案' }}</span>
              <span class="detail-content" :class="{ 'detail-placeholder': details.get(e.id)!.expected === null }">{{
                details.get(e.id)!.expected ?? '-'
              }}</span>
            </div>
            <div class="detail-block">
              <span class="detail-label">模型作答</span>
              <span class="detail-content" :class="{ 'detail-placeholder': details.get(e.id)!.answer_text === null }">{{
                details.get(e.id)!.answer_text ?? '无作答记录'
              }}</span>
            </div>
            <div class="detail-block">
              <span class="detail-label">裁判结果</span>
              <span class="detail-content">
                <span
                  class="feed-score"
                  :class="scoreClass(details.get(e.id)!.score)"
                  :title="details.get(e.id)!.score === null ? '裁判失败,未判分' : undefined"
                  >{{ scoreText(details.get(e.id)!.score) }}</span
                ><template v-if="details.get(e.id)!.verdict_detail"> · {{ details.get(e.id)!.verdict_detail }}</template>
              </span>
            </div>
          </template>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { LiveFeedEntry, LiveFeedResultDetail } from '@/api/types'
import { getCampaignLiveFeedResult } from '@/api/campaigns'
import { formatClockTime, formatMs, formatScore } from '@/utils/format'
import { liveFeedDisplay, isJudgeFailure, toggleExpansion, verdictTypeLabel } from '@/utils/liveFeed'
import { scoreBand } from '@/utils/scoreTier'

// EvalLiveFeed is the running batch's case-level event stream (issue #17):
// one row per judged case — model, suite, case, verdict method, score,
// latency — newest first. It is a pure display unit: the parent view owns
// the cursor, the incremental fetch and the polling timer (ui-guidelines
// §6 — no timer of its own), and mounts this card only for unfinished
// batches on the console leaderboard. Console-only by construction: the
// shared/public surface never renders it, so no live feed call can ever
// leave the session boundary (spec 0004).
//
// Inline expansion (GH #41): a row click expands four detail blocks —
// 题目全文 / 期望答案(rule 题)或评分要点(judge 题)/ 模型作答 / 裁判结果 —
// fetched on demand by result id (the polling payload never grows), cached
// per id so re-expanding never refetches, and keyed by entry id so polling
// prepends never collapse an open row.
const props = defineProps<{
  campaignId: number
  entries: LiveFeedEntry[]
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'retry'): void
}>()

const displayEntries = computed(() => liveFeedDisplay(props.entries))

// Expansion state, keyed by entry id (GH #41). Details and errors are Maps
// so a failed fetch only marks its own expansion; every update replaces the
// container to stay reactive.
const expanded = ref<Set<number>>(new Set())
const details = ref<Map<number, LiveFeedResultDetail>>(new Map())
const detailErrors = ref<Map<number, string>>(new Map())
const detailLoading = ref<Set<number>>(new Set())

function toggle(id: number) {
  const wasOpen = expanded.value.has(id)
  expanded.value = toggleExpansion(expanded.value, id)
  if (wasOpen || details.value.has(id)) return
  void loadDetail(id)
}

async function loadDetail(id: number) {
  if (detailLoading.value.has(id)) return
  const loading = new Set(detailLoading.value)
  loading.add(id)
  detailLoading.value = loading
  const errors = new Map(detailErrors.value)
  errors.delete(id)
  detailErrors.value = errors
  try {
    const detail = await getCampaignLiveFeedResult(props.campaignId, id)
    const next = new Map(details.value)
    next.set(id, detail)
    details.value = next
  } catch (err) {
    const next = new Map(detailErrors.value)
    next.set(id, err instanceof Error ? err.message : String(err))
    detailErrors.value = next
  } finally {
    const loading = new Set(detailLoading.value)
    loading.delete(id)
    detailLoading.value = loading
  }
}

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
.feed-item:last-child .feed-row {
  border-bottom: none;
}
/* Row expansion (GH #41): the whole row is the toggle — pointer + hover
   feedback, the trailing ▸/▾ as the affordance. */
.feed-row-clickable {
  cursor: pointer;
  user-select: none;
}
.feed-row-clickable:hover {
  background: var(--hs-bg-hover);
}
.feed-expand {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  min-width: 12px;
  text-align: right;
}
/* Inline detail: four stacked label+content blocks, capped height with
   vertical scroll so one long answer cannot blow up the feed layout;
   horizontal scroll never (ui-guidelines §4). */
.feed-detail {
  max-height: 240px;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 8px 0 10px;
  border-bottom: 1px solid var(--hs-border-light);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.feed-item:last-child .feed-detail {
  border-bottom: none;
}
.detail-block {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.detail-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.detail-content {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  white-space: pre-wrap;
  word-break: break-word;
}
.detail-placeholder {
  color: var(--hs-text-placeholder);
}
.detail-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  display: flex;
  align-items: center;
  gap: 8px;
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
