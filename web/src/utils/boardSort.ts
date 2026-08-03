// Client-side ranking/filtering for the public Benchmark page (/benchmark,
// ticket 81, spec 0010 — renamed from /board in the v2 IA, GH #112/#118):
// the public endpoint serves the newest settled campaign's full report
// once; the page ranks and filters it locally. sortRows mirrors the server
// caliber exactly (internal/server/report_scoring.go sortReportRows):
// judged-incomplete rows last regardless of the column (spec 0014 coverage
// gate), unscored models last within the complete group, scores descending,
// ties by model_id lexicographic — never a second ranking caliber.
import type { ReportRow } from '@/api/types'

// scoreOf reads the ranking column: the total for "total", the suite score
// for a suite key.
function scoreOf(row: ReportRow, key: string): number | null {
  if (key === 'total') return row.total_score
  return row.suite_scores[key] ?? null
}

// sortRows returns a fresh array ranked by the given column descending.
// Null (unscored in that column) sinks to the bottom; equal scores break by
// model_id ascending so the board is deterministic. The coverage gate (spec
// 0014, ticket 91/92) dominates every column: rows marked judged-incomplete
// (complete === false) sink below all complete ones regardless of their
// scores and order among themselves by model_id — an explicit tiebreak,
// never a reliance on sort stability. Rows without the key (live board or a
// pre-gate backend) count as complete, same caliber as the server's
// rankable().
export function sortRows(rows: ReportRow[], key: string): ReportRow[] {
  return [...rows].sort((a, b) => {
    const ra = a.complete !== false
    const rb = b.complete !== false
    if (ra !== rb) return ra ? -1 : 1
    if (!ra) return a.model_id < b.model_id ? -1 : a.model_id > b.model_id ? 1 : 0
    const sa = scoreOf(a, key)
    const sb = scoreOf(b, key)
    if (sa === null || sb === null) return sa === null ? (sb === null ? 0 : 1) : -1
    if (sa !== sb) return sb - sa
    return a.model_id < b.model_id ? -1 : a.model_id > b.model_id ? 1 : 0
  })
}

// filterRowsByFamily keeps the rows of one model family (exact match); an
// empty family keeps every row.
export function filterRowsByFamily(rows: ReportRow[], family: string): ReportRow[] {
  if (!family) return rows
  return rows.filter((row) => row.family === family)
}

// familyOptionsOf lists the distinct families of the UNFILTERED board,
// sorted — the filter's option list never collapses with the selection.
export function familyOptionsOf(rows: ReportRow[]): string[] {
  const seen = new Set<string>()
  for (const row of rows) seen.add(row.family)
  return [...seen].sort()
}
