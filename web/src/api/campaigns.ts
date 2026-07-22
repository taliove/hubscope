// Eval Campaign API calls (ticket 29).
import { http } from './client'
import type { Campaign, CampaignDetail } from './types'

export async function listCampaigns(): Promise<Campaign[]> {
  return http.get<Campaign[]>('/campaigns')
}

export async function getCampaign(id: number): Promise<CampaignDetail> {
  return http.get<CampaignDetail>(`/campaigns/${id}`)
}
