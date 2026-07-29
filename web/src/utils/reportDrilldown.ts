// Row drill-down gating (issue #10, design ruling 2026-07-28, option A).
// The shared report page (/report/:token) never opens the trend dialog:
// the trends endpoint sits in the authenticated route group and the share
// token's grant is "this batch's report", not cross-batch history — an
// anonymous row click could only produce a 401. This mirrors /board's
// "rows not clickable" public-page caliber (ui-guidelines §5). Console
// pages (/eval, /campaigns/:id/report) keep drill-down untouched.
export function rowDrilldownEnabled(shared: boolean): boolean {
  return !shared
}
