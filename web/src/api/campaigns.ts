// Eval Campaign API calls (ticket 29).
import { http } from './client'
import type {
  Campaign,
  CampaignDetail,
  CampaignReport,
  CampaignTrends,
  LiveFeedEntry,
  LiveFeedResultDetail,
  PublicEvalBoard,
  RetryUnitItem,
  RetryUnitsAck,
} from './types'

export async function listCampaigns(): Promise<Campaign[]> {
  return http.get<Campaign[]>('/campaigns')
}

export async function getCampaign(id: number): Promise<CampaignDetail> {
  return http.get<CampaignDetail>(`/campaigns/${id}`)
}

// Report query (ticket 31): family filters the board to one model family;
// sort picks the ranking column ('total' or a suite key), always descending.
export interface ReportQuery {
  family?: string
  sort?: string
}

export async function getCampaignReport(id: number, query: ReportQuery = {}): Promise<CampaignReport> {
  const params = new URLSearchParams()
  if (query.family) params.set('family', query.family)
  if (query.sort) params.set('sort', query.sort)
  const suffix = params.size > 0 ? `?${params.toString()}` : ''
  return http.get<CampaignReport>(`/campaigns/${id}/report${suffix}`)
}

// Trend drill-down (ticket 32): one model's cross-campaign score trend plus
// the probe-side hourly aggregate, fetched on demand when a row is clicked.
export async function getCampaignTrends(id: number, modelDbId: number): Promise<CampaignTrends> {
  return http.get<CampaignTrends>(`/campaigns/${id}/trends?model=${modelDbId}`)
}

// Public eval board (ticket 81, spec 0010): anonymous read, no params — the
// newest settled campaign's full report; the client ranks and filters it.
export async function getPublicEvalBoard(): Promise<PublicEvalBoard> {
  return http.get<PublicEvalBoard>('/public/eval/board')
}

// Live feed (issue #17): one campaign's judged-case events, ascending by id,
// pulled by exclusive cursor (since_id) from the console's polling timer.
// Console-only — session + hub-isolated; the shared/public surface never
// calls this. limit caps the page (server default 50, max 200).
export async function getCampaignLiveFeed(id: number, sinceId = 0, limit = 200): Promise<LiveFeedEntry[]> {
  return http.get<LiveFeedEntry[]>(`/campaigns/${id}/live-feed?since_id=${sinceId}&limit=${limit}`)
}

// Live-feed result detail (GH #41): one feed row's expansion — full prompt,
// expectation (rule answer / judge rubric), model answer, verdict detail.
// Fetched on demand when a row expands, never as part of the polling
// payload. Console-only, same hub isolation as the feed list.
export async function getCampaignLiveFeedResult(campaignId: number, resultId: number): Promise<LiveFeedResultDetail> {
  return http.get<LiveFeedResultDetail>(`/campaigns/${campaignId}/live-feed/${resultId}`)
}

// Retry-failed (GH #28): re-evaluate a settled batch's failed (null-score)
// results. The batch returns to running; scored results are never touched.
export async function retryCampaignFailed(id: number): Promise<CampaignDetail> {
  return http.post<CampaignDetail>(`/campaigns/${id}/retry-failed`)
}

// Retry-units (targeted retry): re-evaluate explicit (model, case) units of
// a settled batch. Only null-score units are accepted (the batch returns to
// running); judged units are skipped and counted, never re-asked (W7).
export async function retryCampaignUnits(id: number, items: RetryUnitItem[]): Promise<RetryUnitsAck> {
  return http.post<RetryUnitsAck>(`/campaigns/${id}/retry-units`, { items })
}

// Cancel (GH #152): stop a running batch — in-flight cells run to
// completion, unstarted cells are dropped, and the batch settles failed.
// Already-judged results are kept.
export async function cancelCampaign(id: number): Promise<void> {
  return http.post<void>(`/campaigns/${id}/cancel`)
}

// Confirm the jury plan of a manual batch parked at the confirmation gate
// (2026-08-04 ruling); unconfirmed batches auto-start at the deadline.
export async function confirmJury(id: number): Promise<void> {
  return http.post<void>(`/campaigns/${id}/confirm-jury`)
}

// Resume a settled batch where it stopped (2026-08-05 ruling): answered
// units keep their results and scores; only missing or null-score units
// re-run. Returns the same campaign (reopened to running).
export async function restartCampaign(id: number): Promise<Campaign> {
  return http.post<Campaign>(`/campaigns/${id}/restart`)
}
