// Unit tests for the display-layer status mapping (spec 0018, GH #113;
// unverified tier GH #160): five domain values in, 3+1 display states out —
// every combination pinned, plus the degrade-cause passthrough, the
// unknown-input defense (「未知」 stays the OUT-OF-DOMAIN fallback;
// unverified is an in-domain formal identity), and the count merge
// (down + failing → incident, unverified passthrough).
import { describe, it, expect } from 'vitest'
import type { EndpointStatus } from '@/api/types'
import {
  DISPLAY_SEVERITY_ORDER,
  displayStatusCounts,
  statusDisplay,
  statusLabel,
  statusTone,
  toDisplayStatus,
} from '@/utils/statusDisplay'

describe('statusDisplay: domain values in, 3+1 display states out', () => {
  it('maps every domain status to its display state, word and tone slot', () => {
    const cases: Array<[EndpointStatus, string, string, string]> = [
      ['healthy', 'stable', '稳定', 'success'],
      ['degraded', 'degraded', '降级', 'warning'],
      ['down', 'incident', '异常', 'danger'],
      ['failing', 'incident', '异常', 'danger'],
      ['unverified', 'unverified', '未验证', 'neutral'],
    ]
    for (const [domain, display, label, tone] of cases) {
      const result = statusDisplay(domain)
      expect(result.status).toBe(display)
      expect(result.label).toBe(label)
      expect(result.tone).toBe(tone)
      expect(result.causeSuffix).toBe('')
    }
  })

  it('collapses down and failing onto the identical display identity', () => {
    expect(statusDisplay('down')).toEqual(statusDisplay('failing'))
  })

  it('accepts an already-collapsed display status (aggregate scenes)', () => {
    expect(statusDisplay('stable').label).toBe('稳定')
    expect(statusDisplay('degraded').label).toBe('降级')
    expect(statusDisplay('incident').label).toBe('异常')
    expect(statusDisplay('unverified').label).toBe('未验证')
  })

  // GH #160: unverified is an IN-DOMAIN formal identity (未验证, neutral
  // slot) — never warning yellow (yellow = degraded only), never the
  // out-of-domain 未知 fallback.
  it('gives unverified its formal neutral identity, never warning yellow', () => {
    const result = statusDisplay('unverified')
    expect(result.label).toBe('未验证')
    expect(result.tone).toBe('neutral')
    expect(result.tone).not.toBe('warning')
  })
})

describe('statusDisplay: degrade-cause passthrough', () => {
  it('passes the cause suffix through for the degraded state only', () => {
    expect(statusDisplay('degraded', ['availability']).causeSuffix).toBe('· 可用性')
    expect(statusDisplay('degraded', ['latency']).causeSuffix).toBe('· 延迟')
    expect(statusDisplay('degraded', ['availability', 'latency']).causeSuffix).toBe('· 可用性 + 延迟')
  })

  it('honors causes passed to the collapsed display status as well', () => {
    expect(statusDisplay('degraded' as EndpointStatus | 'degraded', ['latency']).causeSuffix).toBe('· 延迟')
  })

  it('renders no suffix for other states even when causes are passed (defense)', () => {
    expect(statusDisplay('healthy', ['availability']).causeSuffix).toBe('')
    expect(statusDisplay('down', ['latency']).causeSuffix).toBe('')
    expect(statusDisplay('failing', ['availability', 'latency']).causeSuffix).toBe('')
  })

  it('renders no suffix for an empty cause list', () => {
    expect(statusDisplay('degraded', []).causeSuffix).toBe('')
    expect(statusDisplay('degraded').causeSuffix).toBe('')
  })
})

describe('statusDisplay: unknown-input defense', () => {
  it('falls back to a neutral placeholder word, never a raw technical string', () => {
    for (const junk of ['', 'UNKNOWN', ' Healthy ', 'null', 'undefined']) {
      const result = statusDisplay(junk)
      expect(result.label).toBe('未知')
      expect(result.causeSuffix).toBe('')
    }
  })

  it('picks the middle tone slot — never false comfort, never false alarm', () => {
    expect(statusDisplay('bogus').tone).toBe('warning')
  })

  it('toDisplayStatus returns null outside both unions', () => {
    expect(toDisplayStatus('bogus')).toBeNull()
    expect(toDisplayStatus('')).toBeNull()
    expect(toDisplayStatus('healthy')).toBe('stable')
    expect(toDisplayStatus('failing')).toBe('incident')
    expect(toDisplayStatus('incident')).toBe('incident')
    expect(toDisplayStatus('unverified')).toBe('unverified')
  })
})

describe('statusLabel / statusTone accessors', () => {
  it('expose the same mapping as statusDisplay', () => {
    expect(statusLabel('healthy')).toBe('稳定')
    expect(statusLabel('degraded')).toBe('降级')
    expect(statusLabel('down')).toBe('异常')
    expect(statusLabel('failing')).toBe('异常')
    expect(statusLabel('unverified')).toBe('未验证')
    expect(statusTone('healthy')).toBe('success')
    expect(statusTone('degraded')).toBe('warning')
    expect(statusTone('down')).toBe('danger')
    expect(statusTone('failing')).toBe('danger')
    expect(statusTone('unverified')).toBe('neutral')
  })
})

describe('displayStatusCounts', () => {
  it('merges down and failing into the incident count', () => {
    expect(displayStatusCounts({ healthy: 4, degraded: 2, down: 1, failing: 3 })).toEqual({
      stable: 4,
      degraded: 2,
      unverified: 0,
      incident: 4,
    })
  })

  it('passes unverified through unmerged (GH #160)', () => {
    expect(displayStatusCounts({ healthy: 1, unverified: 2 })).toEqual({
      stable: 1,
      degraded: 0,
      unverified: 2,
      incident: 0,
    })
  })

  it('treats missing keys as zero (partial count records)', () => {
    expect(displayStatusCounts({})).toEqual({ stable: 0, degraded: 0, unverified: 0, incident: 0 })
    expect(displayStatusCounts({ failing: 1 })).toEqual({ stable: 0, degraded: 0, unverified: 0, incident: 1 })
  })
})

describe('DISPLAY_SEVERITY_ORDER', () => {
  it('lists display states heavy → light, unverified between degraded and stable', () => {
    expect(DISPLAY_SEVERITY_ORDER).toEqual(['incident', 'degraded', 'unverified', 'stable'])
  })

  // GH #160 ruling ⑤: the status filter's four options derive from the
  // severity order reversed (single source, no second ordering) —
  // 稳定/未验证/降级/异常, light → heavy.
  it('derives the filter options light → heavy when reversed', () => {
    expect([...DISPLAY_SEVERITY_ORDER].reverse()).toEqual(['stable', 'unverified', 'degraded', 'incident'])
  })
})
