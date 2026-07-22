// Composable backing the Dashboard: polls GET /api/overview on a fixed
// interval and exposes the entries, group aggregates, and per-status counts.
import { ref, computed, onBeforeUnmount, type Ref } from 'vue'
import { fetchOverview } from '@/api/overview'
import type { OverviewEntry, OverviewGroup, EndpointStatus } from '@/api/types'

// How often the dashboard refreshes, in milliseconds. Exported so the health
// banner can state the interval in its subtext without duplicating the value.
export const POLL_INTERVAL_MS = 10_000

export function useOverview() {
  const entries: Ref<OverviewEntry[]> = ref([])
  const byFamily: Ref<OverviewGroup[]> = ref([])
  const byCapability: Ref<OverviewGroup[]> = ref([])
  const byProtocol: Ref<OverviewGroup[]> = ref([])
  const generatedAt = ref<string | null>(null)
  // Global aggregates for the health banner (ticket 36/37): enabled-only
  // endpoint count and probe-weighted 24h availability; null until the first
  // successful load or when the window has no data.
  const enabledEndpoints = ref<number | null>(null)
  const availability24h = ref<number | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  let timer: ReturnType<typeof setInterval> | null = null

  async function reload() {
    loading.value = true
    try {
      const overview = await fetchOverview()
      entries.value = overview.endpoints
      byFamily.value = overview.by_family ?? []
      byCapability.value = overview.by_capability ?? []
      byProtocol.value = overview.by_protocol ?? []
      generatedAt.value = overview.generated_at
      enabledEndpoints.value = overview.enabled_endpoints ?? null
      availability24h.value = overview.availability_24h ?? null
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
    timer = setInterval(() => void reload(), POLL_INTERVAL_MS)
  }

  function stop() {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
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

  return { entries, byFamily, byCapability, byProtocol, generatedAt, enabledEndpoints, availability24h, loading, error, statusCounts, reload, start, stop }
}
