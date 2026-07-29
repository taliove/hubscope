import { describe, expect, it } from 'vitest'
import { rowDrilldownEnabled } from './reportDrilldown'

// Issue #10 (design ruling 2026-07-28, option A): the shared report page
// (/report/:token) never drills down — the trends endpoint is in the
// authenticated route group and the share token's grant is "this batch's
// report", so a row click by an anonymous visitor would only produce a 401.
describe('rowDrilldownEnabled', () => {
  it('disables row drill-down on the shared report page', () => {
    expect(rowDrilldownEnabled(true)).toBe(false)
  })

  it('keeps row drill-down on console pages (/eval, /campaigns/:id/report)', () => {
    expect(rowDrilldownEnabled(false)).toBe(true)
  })
})
