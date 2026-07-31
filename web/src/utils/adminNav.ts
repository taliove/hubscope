// Admin-console deep-link helpers (GH #29, restructured GH #119): tab queries
// (?tab=) are the settings-page counterpart of /eval?batch=<id> (issue #16) —
// an anomaly surface (the live feed's judge-failure rows) can link straight to
// the console tab or setting that governs it. Same discipline as
// batchSelect.ts: parsing lives in a pure, vitest-covered util; unknown values
// are ignored silently (never an error, never blocking the page); manual
// navigation rewrites the query via router.replace so a stale link cannot
// drag the user back.
//
// GH #119 (spec 0018 IA): AdminView retired. Its seven panes landed on three
// consoles — /models (resources/rules), /settings (settings/tasks/logs/users)
// and /eval (eval-ops/case-library as secondary tabs, finalized GH #120) —
// and /admin redirects through legacyAdminTarget below.

// Every el-tab-pane name of ModelsView. Kept in one place so the query
// parser and the view cannot drift apart.
export const MODELS_TABS = ['resources', 'rules'] as const
export type ModelsTab = (typeof MODELS_TABS)[number]

// Every el-tab-pane name of SettingsView (the task center folds in as the
// tasks pane, spec 0018 IA).
export const SETTINGS_TABS = ['settings', 'tasks', 'logs', 'users'] as const
export type SettingsTab = (typeof SETTINGS_TABS)[number]

// Eval-center secondary tabs (finalized GH #120, spec 0018 IA): the
// leaderboard is the default pane; AdminView's eval-ops and case-library
// panes live here as the 评估运营 / 题库 tabs. The query values are stable
// deep-link anchors (legacy /admin?tab=eval-ops|case-library redirects land
// on them via legacyAdminTarget below).
export const EVAL_TABS = ['board', 'ops', 'cases'] as const
export type EvalTab = (typeof EVAL_TABS)[number]

// Parse a ?tab= route query against an allowed pane set, rejecting anything
// that does not name an existing pane.
export function parseTabQuery<T extends string>(
  raw: string | (string | null)[] | null | undefined,
  allowed: readonly T[],
): T | null {
  const first = Array.isArray(raw) ? raw[0] : raw
  if (!first) return null
  return (allowed as readonly string[]).includes(first) ? (first as T) : null
}

// Settings item anchors: the keys are the AppSettings field names, so a
// deep link stays stable for as long as the setting itself exists. Each key
// marks one form row in SettingsPanel via a data-item attribute.
export const SETTINGS_ITEMS = [
  'lark_webhook_url',
  'alert_enabled',
  'score_drop_alert_enabled',
  'quiet_hours_enabled',
  'quiet_hours_start',
  'judge_model',
  'default_sample_count',
  'eval_concurrency',
  'suite_weights',
] as const
export type SettingsItem = (typeof SETTINGS_ITEMS)[number]

// Parse the ?item= route query into a settings item anchor, rejecting
// anything that does not name an anchored row.
export function parseSettingsItemQuery(raw: string | (string | null)[] | null | undefined): SettingsItem | null {
  const first = Array.isArray(raw) ? raw[0] : raw
  if (!first) return null
  return (SETTINGS_ITEMS as readonly string[]).includes(first) ? (first as SettingsItem) : null
}

export interface LegacyAdminTarget {
  path: string
  query: Record<string, string>
}

// Legacy /admin deep links (GH #119): AdminView is gone, but shared links
// like /admin?tab=settings&item=judge_model must keep landing on the pane
// they named. Unknown or absent tabs fall back to /settings (spec 0018:
// /admin redirects to /settings). The item anchor only ever applied to the
// settings tab (AdminView semantics), so it is dropped for every other
// target.
export function legacyAdminTarget(
  tab: string | null | undefined,
  item: string | null | undefined,
): LegacyAdminTarget {
  switch (tab) {
    case 'resources':
    case 'rules':
      return { path: '/models', query: { tab } }
    case 'logs':
    case 'users':
      return { path: '/settings', query: { tab } }
    case 'settings': {
      const query: Record<string, string> = { tab }
      if (item && parseSettingsItemQuery(item)) query.item = item
      return { path: '/settings', query }
    }
    case 'eval-ops':
      return { path: '/eval', query: { tab: 'ops' } }
    case 'case-library':
      return { path: '/eval', query: { tab: 'cases' } }
    default:
      return { path: '/settings', query: {} }
  }
}
