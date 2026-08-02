// Domain types mirroring api-contract.md exactly. Field names must not drift.

// images_generation joined in spec 0014 (GH #31), images_edit in GH #32;
// the tag color mapping lives in utils/protocol.ts (GH #34).
export type Protocol = 'anthropic' | 'openai' | 'images_generation' | 'images_edit'

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

// Result of a manual protocol re-trial (POST /api/models/{id}/trial):
// one enabled endpoint per missing protocol that answered; failed trials
// create nothing and are explained in failures ("" when nothing failed).
export interface ModelTrialResult {
  model: Model
  created_protocols: Protocol[]
  failures: string
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

// An image-probe cost-saving parameter rule (GH #33, spec 0014): image
// probe requests for models whose ID contains keyword (case-insensitive)
// carry the rule's params in addition to the minimal {model, prompt, n:1}
// body. Every matching rule contributes; on a key collision the smaller
// priority wins. Values are strings only.
export interface ImageParamRule {
  id: number
  keyword: string
  params: Record<string, string>
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
  hub_id?: number | null // hub scope of the actor; null for super_admin / hub-less actions
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

// Structured degrade causes reported by the status machine (spec 0013).
export type DegradeCause = 'availability' | 'latency'

// One hour-aligned bucket of probe counts for the 24h stability dots.
export interface OverviewDot {
  bucket_start: string // RFC3339, hour-aligned
  total: number
  failures: number
  // P50 of the bucket's SUCCESSFUL probes only (failed-probe latency is
  // time-to-failure and never counted); null when the bucket has no success.
  p50_ms: number | null
}

export interface OverviewEntry {
  endpoint_id: number
  model_id: string // the model identifier string, not the database id
  protocol: Protocol
  enabled: boolean
  status: EndpointStatus
  status_reason: string
  // Structured degrade causes; always an array, empty unless degraded
  // (availability first when both rules hit).
  degrade_causes: DegradeCause[]
  success_rate_24h: number | null // 0~1, null when the window has no data
  p50_ms: number | null
  p95_ms: number | null
  last_probe_at: string | null // RFC3339
  family: string // vendor series classification
  capability: string // capability classification
  score: number | null // 0-100 stability score, null when no probe data
  score_reasons: string[] // Chinese deduction explanations, empty when none
  dots_24h: OverviewDot[] // always 24 elements, oldest hour first
  eval_score: number | null // 0-100 eval total score, null when no eval data
  // The status machine's own 7-day P50 baseline (same value the latency
  // degradation rule compares against); null below the sample minimum.
  baseline_p50_ms: number | null
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
  // Global aggregates (ticket 36): enabled_endpoints counts only enabled
  // endpoints; availability_24h is the probe-weighted 24h availability
  // across all enabled endpoints (same weighting as the per-group metric),
  // null when no enabled endpoint has probes in the window.
  enabled_endpoints: number
  availability_24h: number | null
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
  mode: 'exact' | 'regex' | 'contains' | 'mcq' | 'numeric' | 'output_match' | 'ifeval'
  expected: string
}

// One IFEval verifiable instruction (ticket 97): the official instruction id
// plus its kwargs, as cast into check_params by the benchmark seed. Seed-cast
// data — the admin case API never authors it, only preserves it on edits.
export interface IFEvalInstruction {
  instruction_id: string
  kwargs: Record<string, unknown>
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
  check_params: IFEvalInstruction[] | null // only for rule mode "ifeval"
  enabled: boolean
}

// Capability dimension of question-bank v3 (ADR 0010). Pre-v3 legacy suites
// no longer exist: disabled suites are hard-deleted server-side (ADR 0012).
export type Capability = 'instruction' | 'reasoning' | 'coding' | 'language' | 'knowledge'

export interface Suite {
  id: number
  key: string
  name: string
  version: number // question-bank version, bumps on every case mutation
  capability: Capability
  nadir: number // normalization constant (ADR 0009); 0 = legacy raw-mean caliber
  enabled: boolean // false = retired; retired suites are purged server-side and never returned
  cases: EvalCase[]
}

export type EvalTrigger = 'scheduled' | 'manual'
export type EvalRunStatus = 'running' | 'done' | 'failed'

export interface EvalRun {
  id: number
  campaign_id: number // the evaluation batch this run belongs to
  suite_id: number
  suite_version: number // suite version this run scored against
  nadir: number // run-level snapshot of the suite's normalization constant (ADR 0009)
  trigger: EvalTrigger
  judge_model: string
  status: EvalRunStatus
  started_at: string // RFC3339
  finished_at: string | null
  score: number | null // nadir-normalized (ADR 0009) mean of non-null result scores, 0~1 scale
}

// Eval Campaign types (ticket 29): one assessment batch grouping one run per
// suite. "pending" is reserved; campaigns are created running.
export type CampaignStatus = 'pending' | 'running' | 'done' | 'failed'

export interface CampaignProgress {
  total: number
  done: number
  failed: number
  running: number
}

export interface Campaign {
  id: number
  trigger: EvalTrigger
  status: CampaignStatus
  started_at: string | null // RFC3339, null only for the reserved pending state
  finished_at: string | null
  created_at: string // RFC3339
  progress: CampaignProgress
}

export interface CampaignDetail extends Campaign {
  runs: EvalRun[]
}

// Live-feed entry (issue #17): one judged-case event of a campaign, pulled
// incrementally by id cursor (GET /campaigns/{id}/live-feed, console-only —
// session + hub-isolated, never on the shared/public surface). score is the
// raw 0~1 per-case score (null = judge failure, never zero); the 0-100
// conversion happens at render through formatScore (ui-guidelines §7).
// verdict_type is 'rule' | 'judge', or '' when the case was purged.
export interface LiveFeedEntry {
  id: number
  model_id: string
  suite_key: string
  suite_name: string
  case_id: number
  case_prompt: string
  verdict_type: string
  score: number | null
  latency_ms: number
  created_at: string // RFC3339
}

// Live-feed result detail (GH #41): the on-demand expansion of one feed row
// (GET /campaigns/{id}/live-feed/{resultId}, console-only — the expectation
// is question-bank content and never crosses to the shared/public surface).
// expected forks by verdict_type server-side: rule cases carry the standard
// answer, judge cases the rubric scoring points; null when the case was
// purged. answer_text is null when no answer was recorded.
export interface LiveFeedResultDetail {
  id: number
  case_prompt: string
  verdict_type: string
  expected: string | null
  answer_text: string | null
  score: number | null // raw 0~1 per-case score
  verdict_detail: string | null
}

// Campaign report types (ticket 31): the leaderboard over a campaign's done
// runs. All scores are on the 0-100 scale, nadir-normalized per suite (ADR
// 0009); null means unscored.
export interface ReportSuite {
  id: number
  key: string
  name: string
  version: number // question-bank version the campaign scored against
}

// Progress cell of one model x suite inside a campaign report (ticket 52):
// the run's status from the model's perspective plus the judged-case
// coverage (one result row per case; samples are averaged server-side).
// samples is the number of judged answer attempts behind the scored cases —
// with the coverage it forms the score's confidence marker (ticket 51).
export type ReportCellStatus = 'pending' | 'running' | 'done' | 'failed'

export interface ReportCell {
  suite_key: string
  status: ReportCellStatus
  judged_cases: number
  expected_cases: number
  samples: number
  // Cost sums (GH #42): Σ latency / Σ tokens over the model's results in
  // the run, null tokens counted as 0. Console-only — the shared/public
  // payloads omit all three keys, so consumers must tolerate undefined.
  latency_ms?: number
  input_tokens?: number
  output_tokens?: number
}

export interface ReportRow {
  model_db_id: number
  model_id: string
  family: string
  total_score: number | null // weighted total, null when nothing scored
  total_delta: number | null // total vs the baseline campaign, null when not comparable
  suite_scores: Record<string, number | null> // per suite key
  cells: ReportCell[] // per-suite progress detail, one per campaign suite
  // Coverage gate (ticket 91 contract, spec 0014 decision A): settled
  // (done/failed) batches only — live rows never carry the key. false means
  // judging is incomplete: the row forfeits total/rank/delta and sinks below
  // every complete row; the per-suite scores stay as judged.
  complete?: boolean
  // Present only when complete === false: how many gating suites (covered
  // suites with enabled cases) went unjudged — the watermark's N.
  missing_suites?: number
}

// View switch of the unfinished-batch board (ticket 52): the progress grid
// is the default; "scores" is the live half-scored leaderboard.
export type EvalBoardView = 'grid' | 'scores'

// The previous done campaign a report's deltas compare against (ticket 45).
// comparable=false means the caliber broke between batches; reason is
// "suite_changed" (question-bank version bump, ADR 0007), "profile_changed"
// (verdict-profile caliber break, ADR 0008) or "suite_missing" (the baseline
// never covered a suite this batch covers).
export interface ReportBaseline {
  campaign_id: number
  comparable: boolean
  reason?: string
}

export interface CampaignReport extends Campaign {
  suites: ReportSuite[]
  weights: Record<string, number> // effective weight per suite key
  rows: ReportRow[]
  baseline: ReportBaseline | null // null when no earlier done campaign exists
  // Null-score (failed) result rows of the batch (GH #28); on a settled
  // batch, failed_results > 0 renders the「重跑失败项」entry.
  failed_results: number
  // Cost metrics (GH #42), console-only: the session report carries them on
  // running and settled batches alike; the shared/public payloads omit both
  // keys (operational data never crosses the session boundary).
  cost?: CampaignCost
  cost_rows?: CampaignCostRow[]
}

// Batch-level cost summary (GH #42): Σ latency / Σ input / Σ output tokens
// over the campaign's results, null tokens counted as 0.
export interface CampaignCost {
  latency_ms: number
  input_tokens: number
  output_tokens: number
}

// One line of the report page's cost detail table (GH #42): one model's
// cost inside one run. Token fields are null when the model recorded no
// token at all in the run (the table renders a dash).
export interface CampaignCostRow {
  model_id: string
  suite_key: string
  suite_name: string
  status: string // run status: pending/running/done/failed
  latency_ms: number
  input_tokens: number | null
  output_tokens: number | null
}

// Public eval board (ticket 81, spec 0010): GET /api/public/eval/board —
// the newest settled campaign's report (the exact shape of the session
// report) plus a flag for in-flight batches; report is null when nothing
// has settled yet. The endpoint takes no params: the client ranks and
// filters the one full payload.
export interface PublicEvalBoard {
  report: CampaignReport | null
  running: boolean
}

// Campaign trend types (ticket 32): the per-model drill-down of a report —
// cross-campaign score trend with suite-version break markers, plus the
// probe-side hourly aggregate over the same timeline.
export interface TrendModel {
  model_db_id: number
  model_id: string
  family: string
  deleted: boolean // manually deleted or hub-retired; the trend stays visible
}

export interface TrendPoint {
  campaign_id: number
  score: number | null // 0-100 nadir-normalized (ADR 0009), null when the batch judged nothing
  suite_version: number
  version_changed: boolean // question bank changed vs the previous point
  verdict_profile: string // scoring caliber of the point (ADR 0008)
  profile_changed: boolean // scoring caliber changed vs the previous point
}

export interface TrendSuite {
  suite_id: number
  key: string
  name: string
  points: TrendPoint[]
}

export interface CampaignTrends {
  model: TrendModel
  suites: TrendSuite[]
  probe: SeriesBucket[] // hourly aggregate over the model's enabled endpoints
}

// Share link types (ticket 33, ADR 0006): a token-gated read-only door onto
// one campaign report. The token is the capability; it only ever appears in
// session-gated management responses, never on the public shared endpoint.
export interface ShareLink {
  id: number
  token: string
  campaign_id: number
  created_by: string
  created_at: string // RFC3339
  revoked_at: string | null // RFC3339; non-null means the link 404s
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

// Model evaluation summary types (ticket 60.1): the latest campaign evaluation
// summary for a single model, including total score and per-suite breakdown.
// Returned by GET /api/models/{id}/eval-summary; null when the model has never
// been evaluated.
export interface ModelEvalSuiteScore {
  suite_id: number
  suite_name: string
  version: number
  score: number | null // 0-100 nadir-normalized, null when the suite wasn't judged
}

export interface ModelEvalSummary {
  model_id: number
  model_id_str: string
  campaign_id: number
  campaign_created_at: string // RFC3339
  total_score: number | null // weighted average of suite scores, 0-100 scale
  suite_scores: ModelEvalSuiteScore[]
}

// Latest aggregate score of one (suite, model) pair (GET /api/evals/latest).

// Task center types (tickets 18, 28).

export type TaskType = 'eval_run' | 'discovery_sync' | 'rollup' | 'retention_cleanup'
export type TaskSource = 'manual' | 'scheduled'
export type TaskStatus = 'pending' | 'running' | 'success' | 'failed'
export type TaskLogLevel = 'info' | 'warn' | 'error'

// One background job. duration_ms is null until the task finishes.
// campaign_id is set on eval_run tasks only (the /eval?batch= deep link,
// GH #156); progress is the run's (model, case) unit completion in 0~1,
// set on running eval_run tasks only and null everywhere else.
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
  campaign_id: number | null
  progress: number | null // 0~1, running eval_run tasks only
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
