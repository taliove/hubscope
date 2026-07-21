// Domain types mirroring api-contract.md exactly. Field names must not drift.

export type Protocol = 'anthropic' | 'openai'

export interface Hub {
  id: number
  name: string
  base_url: string
  // Response never carries the plaintext token; only the last-4 hint.
  token_hint: string
  created_at: string // RFC3339
}

export interface Endpoint {
  id: number
  model_id: number
  protocol: Protocol
  enabled: boolean
}

export interface Model {
  id: number
  hub_id: number
  model_id: string
  origin: 'manual'
  status: 'active'
  capability: 'chat'
  endpoints: Endpoint[]
}

export interface ProbeRecord {
  id: number
  endpoint_id: number
  streaming: boolean
  ok: boolean
  http_status: number
  error_summary: string | null
  latency_ms: number
  ttft_ms: number | null // always null for non-streaming
  input_tokens: number | null
  output_tokens: number | null
  created_at: string // RFC3339
}

export interface ProbeRunResult {
  endpoint_id: number
  results: ProbeRecord[] // [non-streaming, streaming]
}
