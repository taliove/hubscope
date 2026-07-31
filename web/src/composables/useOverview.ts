// Composable backing the Dashboard: polls GET /api/overview on a fixed
// interval and exposes the entries, group aggregates, and per-status counts.
import { ref, computed, onBeforeUnmount, type Ref } from 'vue'
import { fetchOverview } from '@/api/overview'
import type { OverviewEntry, OverviewGroup, EndpointStatus } from '@/api/types'
import { createVisibilityPoll, type VisibilityPollHandle } from '@/utils/visibilityPoll'

// How often the dashboard refreshes, in milliseconds. Exported so the health
// banner can state the interval in its subtext without duplicating the value.
export const POLL_INTERVAL_MS = 10_000
// Hidden-tab cadence (spec 0015 decision 5, ui-guidelines §6): a dashboard
// left on a wall screen keeps refreshing, just six times slower; returning
// to the tab triggers an immediate refresh.
export const HIDDEN_POLL_INTERVAL_MS = 60_000

export function useOverview() {
  const entries: Ref<OverviewEntry[]> = ref([])
  const byFamily: Ref<OverviewGroup[]> = ref([])
  const byCapability: Ref<OverviewGroup[]> = ref([])
  const byProtocol: Ref<OverviewGroup[]> = ref([])
  const generatedAt = ref<string | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Global aggregates (GH #115): the backend is the single source of truth
  // for the health index, its day-over-day delta, the probe count and the
  // enabled-endpoint population — the dashboard displays, never derives.
  const enabledEndpoints = ref(0)
  const availability24h: Ref<number | null> = ref(null)
  const healthScore24h: Ref<number | null> = ref(null)
  const healthScoreDelta: Ref<number | null> = ref(null)
  const probes24h = ref(0)

  let poll: VisibilityPollHandle | null = null

  async function reload() {
    loading.value = true
    try {
      const overview = await fetchOverview()
      entries.value = overview.endpoints
      byFamily.value = overview.by_family ?? []
      byCapability.value = overview.by_capability ?? []
      byProtocol.value = overview.by_protocol ?? []
      generatedAt.value = overview.generated_at
      enabledEndpoints.value = overview.enabled_endpoints
      availability24h.value = overview.availability_24h
      healthScore24h.value = overview.health_score_24h
      healthScoreDelta.value = overview.health_score_delta
      probes24h.value = overview.probes_24h
      error.value = null
    } catch (err) {
      // Keep the last good data on screen; just surface the failure.
      error.value = (err as Error).message
    } finally {
      loading.value = false
    }
  }

  // Start polling; safe to call once per view mount.
  function start() {
    void reload()
    poll = createVisibilityPoll(() => void reload(), {
      intervalMs: POLL_INTERVAL_MS,
      hiddenIntervalMs: HIDDEN_POLL_INTERVAL_MS,
    })
  }

  function stop() {
    poll?.clear()
    poll = null
  }

  onBeforeUnmount(stop)

  // Count entries per status for the summary row.
  const statusCounts = computed<Record<EndpointStatus, number>>(() => {
    const counts: Record<EndpointStatus, number> = {
      healthy: 0,
      degraded: 0,
      down: 0,
      failing: 0,
    }
    for (const entry of entries.value) {
      counts[entry.status] += 1
    }
    return counts
  })

  return {
    entries,
    byFamily,
    byCapability,
    byProtocol,
    generatedAt,
    loading,
    error,
    statusCounts,
    enabledEndpoints,
    availability24h,
    healthScore24h,
    healthScoreDelta,
    probes24h,
    reload,
    start,
    stop,
  }
}
