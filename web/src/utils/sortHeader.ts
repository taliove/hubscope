// Column-header sort state machine for the matrix Leaderboard (ticket 78,
// spec 0009, descending-only ruling): clicking a column ranks the board by
// that column descending; clicking the already-active column falls back to
// the total. The sort key feeds the server-side `query.sort` unchanged —
// the backend ranks descending only (internal/server/report_scoring.go
// sortReportRows), so ascending order is deliberately out of scope here and
// left to a separate backend ticket. No client-side row reversal: a local
// reverse would flip the server's unscored-models-last tie-break into
// unscored-first, a second ranking caliber.
export function nextSortKey(current: string, clicked: string): string {
  return clicked === current ? 'total' : clicked
}
