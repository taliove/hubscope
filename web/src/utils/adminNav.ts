// Admin deep-link helpers (GH #29): /admin?tab=<tab>&item=<settings-item>
// is the settings-page counterpart of /eval?batch=<id> (issue #16) — an
// anomaly surface (the live feed's judge-failure rows) can link straight to
// the setting that governs it. Same discipline as batchSelect.ts: parsing
// lives in a pure, vitest-covered util; unknown values are ignored silently
// (never an error, never blocking the page); manual navigation rewrites the
// query via router.replace so a stale link cannot drag the user back.

// Every el-tab-pane name of AdminView. Kept in one place so the query
// parser and the view cannot drift apart.
export const ADMIN_TABS = [
  'resources',
  'rules',
  'eval-ops',
  'case-library',
  'logs',
  'users',
  'settings',
] as const
export type AdminTab = (typeof ADMIN_TABS)[number]

// Parse the ?tab= route query into an admin tab name, rejecting anything
// that does not name an existing tab.
export function parseAdminTabQuery(raw: string | (string | null)[] | null | undefined): AdminTab | null {
  const first = Array.isArray(raw) ? raw[0] : raw
  if (!first) return null
  return (ADMIN_TABS as readonly string[]).includes(first) ? (first as AdminTab) : null
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
