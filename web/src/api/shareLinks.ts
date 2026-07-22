// Share link API calls (ticket 33, ADR 0006).
import { http } from './client'
import type { ReportQuery } from './campaigns'
import type { CampaignReport, ShareLink } from './types'

export async function listShareLinks(): Promise<ShareLink[]> {
  return http.get<ShareLink[]>('/share-links')
}

export async function createShareLink(campaignId: number): Promise<ShareLink> {
  return http.post<ShareLink>(`/campaigns/${campaignId}/share-links`)
}

export async function revokeShareLink(id: number): Promise<void> {
  return http.del<void>(`/share-links/${id}`)
}

// The absolute URL a recipient opens; token-gated, no login required.
export function shareLinkUrl(token: string): string {
  return `${window.location.origin}/report/${token}`
}

// Public read: the shared report of the token's campaign. Unknown or revoked
// tokens surface as a 404 error with the server message.
export async function getSharedReport(token: string, query: ReportQuery = {}): Promise<CampaignReport> {
  const params = new URLSearchParams()
  if (query.family) params.set('family', query.family)
  if (query.sort) params.set('sort', query.sort)
  const suffix = params.size > 0 ? `?${params.toString()}` : ''
  return http.get<CampaignReport>(`/shared-reports/${token}${suffix}`)
}
