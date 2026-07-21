// Overview (status matrix) API calls.
import { http } from './client'
import type { Overview } from './types'

export async function fetchOverview(): Promise<Overview> {
  return http.get<Overview>('/overview')
}
