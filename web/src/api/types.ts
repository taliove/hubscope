// Domain types mirroring api-contract.md exactly. Field names must not drift.

export type Protocol = 'anthropic' | 'openai'

export interface Hub {
  id: number
  name: string
  base_url: string
  // Response never carries the plaintext token; only the last-4 hint.
  token_hint: string
  // Model-discovery sync state: idle | syncing | succeeded | failed.
  sync_status: 'idle' | 'syncing' | 'succeeded' | 'failed'
  last_synced_at: string | null // RFC3339, null when never synced
  last_sync_error: string | null
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
  origin: string // "manual" | "discovered"
  status: string // "active" | "retired"
  capability: string // "chat" | "embedding" | "image" | "audio" | ...
  family: string // vendor series: "gpt" | "claude" | "qwen" | ... | "other"
  endpoints: Endpoint[]
}

// A model-classification rule: models whose ID contains keyword
// (case-insensitive) get category on the rule's dimension. Lower priority
// values match first.
export interface ClassificationRule {
  id: number
  dimension: 'capability' | 'family'
  keyword: string
  category: string
  priority: number
}

// One administrative action record.
export interface AuditLog {
  id: number
  at: string // RFC3339
  actor: string
  ip: string
  action: string // e.g. "hub.create"
  object_type: string
  object_id: string
  detail: string
  result: string // "success" | "failed: ..." | "accepted"
}

export interface AuditLogPage {
  items: AuditLog[]
  total: number
  page: number
  page_size: number
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

// Endpoint health states produced by the status machine.
export type EndpointStatus = 'healthy' | 'degraded' | 'down' | 'failing'

// One hour-aligned bucket of probe counts for the 24h stability dots.
export interface OverviewDot {
  bucket_start: string // RFC3339, hour-aligned
  total: number
  failures: number
}

export interface OverviewEntry {
  endpoint_id: number
  model_id: string // the model identifier string, not the database id
  protocol: Protocol
  enabled: boolean
  status: EndpointStatus
  status_reason: string
  success_rate_24h: number | null // 0~1, null when the window has no data
  p50_ms: number | null
  p95_ms: number | null
  last_probe_at: string | null // RFC3339
  family: string // vendor series classification
  capability: string // capability classification
  score: number | null // 0-100 stability score, null when no probe data
  score_reasons: string[] // Chinese deduction explanations, empty when none
  dots_24h: OverviewDot[] // always 24 elements, oldest hour first
}

// Health aggregate of one classification group: status distribution
// ('disabled' counted separately) plus probe-weighted 24h availability and
// mean latency (null when the group has no probes in 24h).
export interface OverviewGroup {
  key: string
  endpoint_count: number
  status_counts: Record<string, number>
  availability_24h: number | null
  avg_latency_ms: number | null
}

export interface Overview {
  generated_at: string // RFC3339
  endpoints: OverviewEntry[]
  by_family: OverviewGroup[]
  by_capability: OverviewGroup[]
  by_protocol: OverviewGroup[]
}

// Endpoint detail page types (ticket 04).

export interface EndpointDetail extends Endpoint {
  model_id_str: string // the model identifier string
  hub_name: string
  status: EndpointStatus
  status_reason: string
}

// Streaming selector of the series API; "all" merges both probe kinds.
export type SeriesStreaming = 'all' | 'streaming' | 'non_streaming'

export interface SeriesBucket {
  bucket_start: string // RFC3339, hour-aligned
  total: number
  failures: number
  p50_ms: number | null
  p95_ms: number | null
  avg_ttft_ms: number | null
}

// Evaluation center types (tickets 08/09).

export type VerdictType = 'rule' | 'judge'

export type Difficulty = 'basic' | 'intermediate' | 'hard'

export interface RuleConfig {
  mode: 'exact' | 'regex' | 'contains'
  expected: string
}

// Named EvalCase to avoid clashing with the JS reserved-word flavor of "Case".
// Cases are immutable server-side: a content edit returns a new id and the
// old row stays in the listing as disabled.
export interface EvalCase {
  id: number
  suite_id: number
  prompt: string
  verdict_type: VerdictType
  rule_config: RuleConfig | null // only for verdict_type "rule"
  rubric: string | null // only for verdict_type "judge"
  difficulty: Difficulty
  sample_count: number | null // null = inherit the global default
  enabled: boolean
}

export interface Suite {
  id: number
  key: string
  name: string
  version: number // question-bank version, bumps on every case mutation
  cases: EvalCase[]
}

export type EvalTrigger = 'scheduled' | 'manual'
export type EvalRunStatus = 'running' | 'done' | 'failed'

export interface EvalRun {
  id: number
  suite_id: number
  suite_version: number // suite version this run scored against
  trigger: EvalTrigger
  judge_model: string
  status: EvalRunStatus
  started_at: string // RFC3339
  finished_at: string | null
  score: number | null // aggregate over non-null result scores
}

export interface EvalResult {
  id: number
  model_id: string // the model identifier string
  case_id: number
  answer_text: string | null
  score: number | null // null when the case could not be judged
  verdict_detail: string | null
  latency_ms: number
  input_tokens: number | null
  output_tokens: number | null
  model_deleted: boolean // model no longer exists; history views badge it
}

export interface EvalRunDetail extends EvalRun {
  results: EvalResult[]
}

// Latest aggregate score of one (suite, model) pair (GET /api/evals/latest).
export interface LatestScore {
  suite_id: number
  suite_key: string
  model_id: string
  model_db_id: number
  score: number | null
  eval_run_id: number
  finished_at: string
}

// Task center types (ticket 18).

export type TaskType = 'eval_run'
export type TaskSource = 'manual' | 'scheduled'
export type TaskStatus = 'pending' | 'running' | 'success' | 'failed'
export type TaskLogLevel = 'info' | 'warn' | 'error'

// One background job. duration_ms is null until the task finishes.
export interface TaskItem {
  id: number
  type: TaskType
  source: TaskSource
  status: TaskStatus
  entity_type: string
  entity_id: number
  started_at: string | null // RFC3339
  finished_at: string | null // RFC3339
  duration_ms: number | null
  created_at: string // RFC3339
}

export interface TaskLogEntry {
  id: number
  at: string // RFC3339
  level: TaskLogLevel
  message: string
}

export interface TaskPage {
  items: TaskItem[]
  total: number
  page: number
  page_size: number
}

export interface TaskDetail extends TaskItem {
  logs: TaskLogEntry[]
}
