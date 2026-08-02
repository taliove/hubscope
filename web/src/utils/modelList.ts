// Model-status-list row derivations (GH #131, UI v2 optimization O4; GH #136
// seven fixes; GH #140 grouping regression): vendor-tile initials,
// availability progress-bar width, per-row latency sparkline series and its
// semantic tone, the vendor-filter option list, the column-sort state
// machine + sorter + persistence, and the grouping bucketer + section meta
// wording — centralized as pure functions so the list component never
// carries math (role.ts / overviewMetrics.ts precedent).
import type { OverviewDot, OverviewEntry } from '@/api/types'
import { statusLabel, toDisplayStatus } from '@/utils/statusDisplay'
import { abnormalModelCounts } from '@/utils/overviewMetrics'
import { entryRank, groupRank } from '@/utils/severitySort'

// --- Vendor tile initials ---------------------------------------------------

// Initials for the vendor tile fallback (≤3 chars): rendered only when the
// family does NOT map to a known vendor (vendorIcon.ts single source owns
// the mapping; known vendors render the uniform brand tile instead, GH
// #136). A single Latin word yields its first TWO letters uppercased
// (deepseek → DE); a short word (≤3 chars) yields the whole word (glm →
// GLM); a multi-word name yields the first character of each of the first
// three words (Ali Cloud → AC); CJK names yield the name itself when ≤3
// chars (阿里 → 阿里), else the first character. The fallback tile stays a
// neutral soft block — no invented brand colors.
export function familyInitials(family: string): string {
  const words = family
    .trim()
    .split(/[^a-zA-Z0-9一-鿿]+/)
    .filter(Boolean)
  if (words.length === 0) return '—'
  if (words.length > 1) {
    return words
      .slice(0, 3)
      .map(w => [...w][0])
      .join('')
      .toUpperCase()
  }
  const chars = [...words[0]]
  if (chars.length <= 3) return words[0].toUpperCase()
  return /^[a-zA-Z0-9]/.test(chars[0]) ? (chars[0] + chars[1]).toUpperCase() : chars[0]
}

// --- Row trend sparkline ----------------------------------------------------

// Per-row 24h latency series for the trend column: the entry's own hourly
// P50 buckets (successful probes only — failed-probe latency is
// time-to-failure and never feeds the curve, the LatencySparkline
// discipline). Null buckets break the line.
export function entryLatencySeries(dots: OverviewDot[]): (number | null)[] {
  return dots.map(d => d.p50_ms)
}

// Semantic tone of the row sparkline, by the entry's DISPLAY state
// (statusDisplay single mapping): stable → success, degraded → warning,
// incident (down + failing) → danger. Disabled endpoints render neutral —
// out of service by admin choice, the curve carries no health signal.
// The row subset of TrendSparkline's SparklineTone (GH #130): the brand
// lane belongs to the request widget, never to a health row.
export type SparklineTone = 'neutral' | 'success' | 'warning' | 'danger'

export function rowSparklineTone(entry: OverviewEntry): SparklineTone {
  if (!entry.enabled) return 'neutral'
  const display = toDisplayStatus(entry.status)
  if (display === 'stable') return 'success'
  if (display === 'degraded') return 'warning'
  if (display === 'incident') return 'danger'
  return 'neutral'
}

// --- Vendor filter options --------------------------------------------------

// Options of the vendor (供应商) filter: the distinct family values of the
// UNFILTERED entry set, lexicographic by code unit (board determinism —
// never localeCompare). Derived from the full set so an active filter never
// collapses its own option list.
export function familyOptions(entries: OverviewEntry[]): string[] {
  return [...new Set(entries.map(e => e.family))].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
}

// --- Column sort (GH #136) ----------------------------------------------------

// Sortable columns of the model status list: 模型名称 / 24h 可用率 / P95 延迟.
// Vendor / status / trend / action stay unsortable (no meaningful total
// order — vendor is a category, status ranks by the severity table, trend is
// a shape, action is a button).
export type ListSortKey = 'name' | 'rate' | 'p95'
export type SortDir = 'asc' | 'desc'

export interface ListSort {
  key: ListSortKey
  dir: SortDir
}

// Default sort: availability DESC — the strongest models lead the first
// viewport (GH #136 user ruling, OVERTURNING the GH #131「weakest first」
// default: the user reads the board as「is everything fine」first, not
//「what is broken」first). Rate ASC remains one click away and reproduces
// the retired caliber.
export const LIST_SORT_DEFAULT: ListSort = { key: 'rate', dir: 'desc' }

// Direction a NEWLY clicked column starts in — best-first throughout: name
// A→Z, rate high→low, p95 fast→slow. Clicking the active column flips.
const COLUMN_DEFAULT_DIR: Record<ListSortKey, SortDir> = {
  name: 'asc',
  rate: 'desc',
  p95: 'asc',
}

// Column-header click state machine: clicking another column switches to it
// at that column's best-first direction; clicking the active column flips
// its direction.
export function nextListSort(current: ListSort, clicked: ListSortKey): ListSort {
  if (clicked === current.key) {
    return { key: clicked, dir: current.dir === 'desc' ? 'asc' : 'desc' }
  }
  return { key: clicked, dir: COLUMN_DEFAULT_DIR[clicked] }
}

// lex compares two strings by code unit, never localeCompare — the board's
// tie-break must be deterministic across locales (severitySort precedent).
function lex(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

// sortListEntries ranks the list by the active column and direction.
// Buckets survive every key/direction: rated enabled rows (the direction
// applies INSIDE this bucket only) → enabled rows without data (null rate /
// null p95 — no-data must never read as 0% false alarm nor 100% false
// comfort, GH #131 caliber kept) → disabled rows last (out of service by
// admin choice, the DISABLED_RANK spirit). Rate ties fall back to the
// severity rank (direction-independent: a failing 0% still leads a down
// 0%); final ties break by model_id → protocol → endpoint_id and never flip
// with the direction (one tie-break caliber, board determinism). Never
// mutates the input.
export function sortListEntries(entries: OverviewEntry[], sort: ListSort): OverviewEntry[] {
  const value = (e: OverviewEntry): number | null =>
    sort.key === 'rate' ? e.success_rate_24h : sort.key === 'p95' ? e.p95_ms : null
  const bucket = (e: OverviewEntry): number => {
    if (!e.enabled) return 2
    if (sort.key === 'name') return 0
    return value(e) === null ? 1 : 0
  }
  const primary = (a: OverviewEntry, b: OverviewEntry): number => {
    let cmp = 0
    if (sort.key === 'name') {
      cmp = lex(a.model_id, b.model_id)
    } else {
      const va = value(a) as number
      const vb = value(b) as number
      cmp = va - vb
    }
    // The direction applies to the column value only — the severity
    // fallback below and the final tie-break never flip.
    return sort.dir === 'desc' ? -cmp : cmp
  }
  const tie = (a: OverviewEntry, b: OverviewEntry): number =>
    lex(a.model_id, b.model_id) || lex(a.protocol, b.protocol) || a.endpoint_id - b.endpoint_id
  return [...entries].sort((a, b) => {
    const ba = bucket(a)
    const bb = bucket(b)
    if (ba !== bb) return ba - bb
    if (ba === 0) {
      const cmp = primary(a, b)
      if (cmp !== 0) return cmp
      // Rate ties fall back to the severity rank (direction-independent:
      // a failing 0% still leads a down 0%).
      if (sort.key === 'rate') {
        const sev = entryRank(a) - entryRank(b)
        if (sev !== 0) return sev
      }
    }
    return tie(a, b)
  })
}

// --- Sort persistence (GH #136) -----------------------------------------------

// localStorage key of the list sort, same family as the theme key
// (`hs:dark`, §2a): one `hs:` namespace per persisted board preference.
export const LIST_SORT_STORAGE_KEY = 'hs:list-sort'

const SORT_KEYS: readonly string[] = ['name', 'rate', 'p95']
const SORT_DIRS: readonly string[] = ['asc', 'desc']

// Storage seam: the component passes nothing (browser localStorage); tests
// inject a Map-backed stand-in (vitest runs in node). A missing/throwing
// storage yields the default — a broken preference must never break the
// board.
type SortStorage = Pick<Storage, 'getItem' | 'setItem'> | null

function defaultStorage(): SortStorage {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

// Load the persisted sort, validating BOTH fields against the known
// key/dir unions; bad JSON, unknown keys/dirs and non-object payloads all
// fall back to LIST_SORT_DEFAULT.
export function loadListSort(storage: SortStorage = defaultStorage()): ListSort {
  if (!storage) return LIST_SORT_DEFAULT
  try {
    const raw = storage.getItem(LIST_SORT_STORAGE_KEY)
    if (!raw) return LIST_SORT_DEFAULT
    const parsed = JSON.parse(raw) as { key?: unknown; dir?: unknown }
    if (
      typeof parsed?.key === 'string' &&
      typeof parsed?.dir === 'string' &&
      SORT_KEYS.includes(parsed.key) &&
      SORT_DIRS.includes(parsed.dir)
    ) {
      return { key: parsed.key as ListSortKey, dir: parsed.dir as SortDir }
    }
    return LIST_SORT_DEFAULT
  } catch {
    return LIST_SORT_DEFAULT
  }
}

// Persist the sort; a throwing storage (private mode, quota) is swallowed —
// the sort still applies for the session.
export function saveListSort(sort: ListSort, storage: SortStorage = defaultStorage()): void {
  if (!storage) return
  try {
    storage.setItem(LIST_SORT_STORAGE_KEY, JSON.stringify(sort))
  } catch {
    // best effort — see above
  }
}

// --- URL query codec (2026-08-02, filter/sort deep-link) ---------------------

// Query form of the sort state: `sort=rate:desc`. The URL is the shareable
// form of the whole list view (keyword + vendor + status + grouping + sort)
// — a pasted link must reproduce the exact view, so on open the URL WINS
// over the localStorage persistence (which stays the no-param fallback).
export function listSortToQuery(sort: ListSort): string {
  return `${sort.key}:${sort.dir}`
}

// Reads the query value back; anything unrecognized (missing, wrong key,
// wrong dir) yields null so the caller falls back to localStorage.
export function parseListSortQuery(raw: unknown): ListSort | null {
  if (typeof raw !== 'string') return null
  const [key, dir] = raw.split(':')
  if (!SORT_KEYS.includes(key) || !SORT_DIRS.includes(dir)) return null
  return { key: key as ListSortKey, dir: dir as SortDir }
}

// The toolbar note restates the CURRENT ordering (GH #136): the GH #131
// static「(按可用率排序)」note would lie the moment the user re-sorts — a
// label the data does not honor is an anti-fake violation. The note is
// dynamic so it stays literally true in every state. In grouped mode (GH
// #140) the note describes the IN-GROUP ordering — the group ranking is
// the severity rank, a separate caliber the note never claimed.
export function listSortNote(sort: ListSort): string {
  const dirWord = sort.dir === 'desc' ? '降序' : '升序'
  switch (sort.key) {
    case 'name':
      return `（按名称${dirWord}）`
    case 'p95':
      return `（按 P95 延迟${dirWord}）`
    default:
      return `（按可用率${dirWord}）`
  }
}

// --- Grouping (GH #140 regression) -------------------------------------------

// Grouping dimension of the list toolbar select. Default none (flat list) —
// the GH #131 reference-toolbar default was family grouping; GH #140
// restores the selector with the flat list as the default.
export type ListGrouping = 'none' | 'family' | 'capability' | 'protocol'

// Option labels from ONE Record source (the enum single-source discipline,
// protocol.ts precedent): the select renders Object.keys of this record, so
// a new grouping value can never ship without its label.
export const LIST_GROUPING_LABELS: Record<ListGrouping, string> = {
  none: '不分组',
  family: '按厂商',
  capability: '按能力',
  protocol: '按协议',
}
export const LIST_GROUPINGS = Object.keys(LIST_GROUPING_LABELS) as ListGrouping[]
export const LIST_GROUPING_DEFAULT: ListGrouping = 'none'

// One bucket of the grouped list: the group key plus its entries sorted by
// the ACTIVE column sort.
export interface ListGroup {
  key: string
  entries: OverviewEntry[]
}

// groupListSections buckets the (already filtered) entries by the grouping
// dimension and returns the sections ranked by the group's most severe
// ENABLED entry — groupRank, the severitySort single rank table (GH #140:
// group ranking is the severity rank whatever the column sort says; the
// severitySort.sortGroupSections helper itself is NOT reused because it
// re-sorts entries by severity, while the in-group ordering here must
// follow the active column sort — sortListEntries keeps the bucket rules:
// rated enabled → no-data enabled → disabled, inside every group). Group
// ties break by key lex (board determinism). Never mutates the input.
export function groupListSections(
  entries: OverviewEntry[],
  grouping: Exclude<ListGrouping, 'none'>,
  sort: ListSort,
): ListGroup[] {
  const keyOf =
    grouping === 'family'
      ? (e: OverviewEntry) => e.family
      : grouping === 'capability'
        ? (e: OverviewEntry) => e.capability
        : (e: OverviewEntry) => e.protocol
  const buckets = new Map<string, OverviewEntry[]>()
  for (const e of entries) {
    const key = keyOf(e)
    const list = buckets.get(key)
    if (list) list.push(e)
    else buckets.set(key, [e])
  }
  return [...buckets.entries()]
    .map(([key, list]) => ({ key, entries: sortListEntries(list, sort) }))
    .sort((a, b) => groupRank(a.entries) - groupRank(b.entries) || lex(a.key, b.key))
}

// Section meta line of a group header (GH #140; unverified suffix GH #160
// ruling ⑦):「N 个端点」 when nothing is abnormal and nothing unverified,
// else「N 个端点 · 异常 N · 降级 N」 plus「· 未验证 N」when the group has
// unverified endpoints — the no-evidence dimension is never silently
// omitted (same disclosure philosophy as the material's four-segment
// distribution string). The count words come from the display-layer single
// mapping (statusLabel — never a literal); the counts dedupe BY MODEL at
// each model's worst display state (abnormalModelCounts, the GH #115
// caliber) so a model with two abnormal protocols counts once; disabled
// endpoints never inflate the counts.
export function listSectionMeta(entries: OverviewEntry[]): string {
  const counts = abnormalModelCounts(entries)
  const unverifiedSuffix = counts.unverified > 0 ? ` · ${statusLabel('unverified')} ${counts.unverified}` : ''
  if (counts.total === 0) {
    return counts.unverified > 0
      ? `${entries.length} 个端点${unverifiedSuffix}`
      : `${entries.length} 个端点`
  }
  return `${entries.length} 个端点 · ${statusLabel('incident')} ${counts.incident} · ${statusLabel('degraded')} ${counts.degraded}${unverifiedSuffix}`
}
