// Eval Campaign API calls (ticket 29).
import { http } from './client'
import type { Campaign, CampaignDetail, CampaignReport, CampaignTrends, LiveFeedEntry, PublicEvalBoard } from './types'

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

// Retry-failed (GH #28): re-evaluate a settled batch's failed (null-score)
// results. The batch returns to running; scored results are never touched.
export async function retryCampaignFailed(id: number): Promise<CampaignDetail> {
  return http.post<CampaignDetail>(`/campaigns/${id}/retry-failed`)
}
