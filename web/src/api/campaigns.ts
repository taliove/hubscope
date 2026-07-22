// Eval Campaign API calls (ticket 29).
import { http } from './client'
import type { Campaign, CampaignDetail, CampaignReport } from './types'

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
