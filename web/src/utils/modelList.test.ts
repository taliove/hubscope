import { describe, expect, it } from 'vitest'
import {
  availabilityBarWidth,
  entryLatencySeries,
  familyInitials,
  familyOptions,
  LIST_SORT_DEFAULT,
  LIST_SORT_STORAGE_KEY,
  listSortNote,
  loadListSort,
  nextListSort,
  rowSparklineTone,
  saveListSort,
  sortListEntries,
} from '@/utils/modelList'
import type { OverviewDot, OverviewEntry } from '@/api/types'

const entry = (over: Partial<OverviewEntry>): OverviewEntry =>
  ({
    endpoint_id: 1,
    model_id: 'm',
    protocol: 'openai',
    family: 'gpt',
    capability: 'chat',
    status: 'healthy',
    enabled: true,
    success_rate_24h: 0.99,
    p50_ms: 100,
    p95_ms: 200,
    dots_24h: [],
    baseline_p50_ms: null,
    ...over,
  }) as OverviewEntry

describe('familyInitials', () => {
  it('takes the first two letters of a single long Latin word', () => {
    expect(familyInitials('anthropic')).toBe('AN')
    expect(familyInitials('google')).toBe('GO')
    expect(familyInitials('qwen')).toBe('QW')
  })
  it('keeps a short word whole (≤3 chars), uppercased', () => {
    expect(familyInitials('gpt')).toBe('GPT')
    expect(familyInitials('x')).toBe('X')
  })
  it('takes the first character of each of the first three words', () => {
    expect(familyInitials('Ali Cloud')).toBe('AC')
    expect(familyInitials('one two three four')).toBe('OTT')
  })
  it('keeps a short CJK name whole, else the first character', () => {
    expect(familyInitials('阿里')).toBe('阿里')
    expect(familyInitials('深度求索')).toBe('深')
  })
  it('falls back to an em dash for empty input', () => {
    expect(familyInitials('')).toBe('—')
    expect(familyInitials('   ')).toBe('—')
    expect(familyInitials('---')).toBe('—')
  })
})

describe('availabilityBarWidth', () => {
  it('maps the rate onto the constant 0–100 scale', () => {
    expect(availabilityBarWidth(1)).toBe(100)
    expect(availabilityBarWidth(0.955)).toBeCloseTo(95.5)
    expect(availabilityBarWidth(0)).toBe(0)
  })
  it('renders null (no data) as an empty track', () => {
    expect(availabilityBarWidth(null)).toBe(0)
  })
  it('clamps out-of-range input defensively', () => {
    expect(availabilityBarWidth(1.2)).toBe(100)
    expect(availabilityBarWidth(-0.5)).toBe(0)
  })
})

describe('entryLatencySeries', () => {
  it('maps hourly P50 buckets, preserving null breaks', () => {
    const dots = [
      { bucket_start: 'a', total: 4, failures: 0, p50_ms: 120 },
      { bucket_start: 'b', total: 2, failures: 2, p50_ms: null },
      { bucket_start: 'c', total: 1, failures: 0, p50_ms: 90 },
    ] as OverviewDot[]
    expect(entryLatencySeries(dots)).toEqual([120, null, 90])
  })
})

describe('rowSparklineTone', () => {
  it('colors by display state: stable green / degraded yellow / incident red', () => {
    expect(rowSparklineTone(entry({ status: 'healthy' }))).toBe('success')
    expect(rowSparklineTone(entry({ status: 'degraded' }))).toBe('warning')
    expect(rowSparklineTone(entry({ status: 'down' }))).toBe('danger')
    expect(rowSparklineTone(entry({ status: 'failing' }))).toBe('danger')
  })
  it('renders disabled endpoints neutral whatever the status says', () => {
    expect(rowSparklineTone(entry({ enabled: false, status: 'down' }))).toBe('neutral')
  })
  it('falls back to neutral for payloads outside the status union', () => {
    expect(rowSparklineTone(entry({ status: 'bogus' as never }))).toBe('neutral')
  })
})

describe('familyOptions', () => {
  it('dedupes and sorts family values by code unit', () => {
    const entries = [entry({ family: 'qwen' }), entry({ family: 'gpt' }), entry({ family: 'qwen' })]
    expect(familyOptions(entries)).toEqual(['gpt', 'qwen'])
  })
  it('returns an empty list for an empty board', () => {
    expect(familyOptions([])).toEqual([])
  })
})

// --- Column sort (GH #136) --------------------------------------------------

describe('nextListSort', () => {
  it('switches to a new column with that column’s best-first default direction', () => {
    expect(nextListSort({ key: 'rate', dir: 'desc' }, 'name')).toEqual({ key: 'name', dir: 'asc' })
    expect(nextListSort({ key: 'name', dir: 'asc' }, 'rate')).toEqual({ key: 'rate', dir: 'desc' })
    expect(nextListSort({ key: 'name', dir: 'asc' }, 'p95')).toEqual({ key: 'p95', dir: 'asc' })
  })
  it('flips the direction when the active column is clicked again', () => {
    expect(nextListSort({ key: 'rate', dir: 'desc' }, 'rate')).toEqual({ key: 'rate', dir: 'asc' })
    expect(nextListSort({ key: 'rate', dir: 'asc' }, 'rate')).toEqual({ key: 'rate', dir: 'desc' })
    expect(nextListSort({ key: 'name', dir: 'asc' }, 'name')).toEqual({ key: 'name', dir: 'desc' })
  })
})

describe('sortListEntries', () => {
  it('ranks the HIGHEST 24h availability first by default (rate desc, GH #136)', () => {
    const sorted = sortListEntries(
      [
        entry({ model_id: 'bad', status: 'down', success_rate_24h: 0.4 }),
        entry({ model_id: 'good', success_rate_24h: 0.999 }),
        entry({ model_id: 'mid', status: 'degraded', success_rate_24h: 0.93 }),
      ],
      LIST_SORT_DEFAULT,
    )
    expect(sorted.map((e) => e.model_id)).toEqual(['good', 'mid', 'bad'])
  })

  it('rate asc reproduces the retired weakest-first caliber', () => {
    const sorted = sortListEntries(
      [
        entry({ model_id: 'good', success_rate_24h: 0.999 }),
        entry({ model_id: 'bad', status: 'down', success_rate_24h: 0.4 }),
        entry({ model_id: 'mid', status: 'degraded', success_rate_24h: 0.93 }),
      ],
      { key: 'rate', dir: 'asc' },
    )
    expect(sorted.map((e) => e.model_id)).toEqual(['bad', 'mid', 'good'])
  })

  it('sinks null-rate rows below every rated row and disabled rows last, in both directions', () => {
    const rows = [
      entry({ model_id: 'disabled', status: 'down', enabled: false, success_rate_24h: 0 }),
      entry({ model_id: 'nodata', success_rate_24h: null }),
      entry({ model_id: 'perfect', success_rate_24h: 1 }),
      entry({ model_id: 'zero', status: 'down', success_rate_24h: 0 }),
    ]
    expect(sortListEntries(rows, { key: 'rate', dir: 'desc' }).map((e) => e.model_id)).toEqual([
      'perfect',
      'zero',
      'nodata',
      'disabled',
    ])
    expect(sortListEntries(rows, { key: 'rate', dir: 'asc' }).map((e) => e.model_id)).toEqual([
      'zero',
      'perfect',
      'nodata',
      'disabled',
    ])
  })

  it('breaks rate ties by the severity rank (direction-independent), then model_id', () => {
    const sorted = sortListEntries(
      [
        entry({ model_id: 'b-down', status: 'down', success_rate_24h: 0 }),
        entry({ model_id: 'a-failing', status: 'failing', success_rate_24h: 0 }),
        entry({ model_id: 'a-down', status: 'down', success_rate_24h: 0 }),
      ],
      { key: 'rate', dir: 'desc' },
    )
    expect(sorted.map((e) => e.model_id)).toEqual(['a-failing', 'a-down', 'b-down'])
  })

  it('sorts by model name in both directions, disabled last', () => {
    const rows = [
      entry({ model_id: 'gamma' }),
      entry({ model_id: 'alpha' }),
      entry({ model_id: 'zeta', enabled: false }),
      entry({ model_id: 'beta' }),
    ]
    expect(sortListEntries(rows, { key: 'name', dir: 'asc' }).map((e) => e.model_id)).toEqual([
      'alpha',
      'beta',
      'gamma',
      'zeta',
    ])
    expect(sortListEntries(rows, { key: 'name', dir: 'desc' }).map((e) => e.model_id)).toEqual([
      'gamma',
      'beta',
      'alpha',
      'zeta',
    ])
  })

  it('sorts by p95 in both directions; null p95 sinks below rated rows', () => {
    const rows = [
      entry({ model_id: 'slow', p95_ms: 900 }),
      entry({ model_id: 'nodata', p95_ms: null }),
      entry({ model_id: 'fast', p95_ms: 120 }),
    ]
    expect(sortListEntries(rows, { key: 'p95', dir: 'asc' }).map((e) => e.model_id)).toEqual([
      'fast',
      'slow',
      'nodata',
    ])
    expect(sortListEntries(rows, { key: 'p95', dir: 'desc' }).map((e) => e.model_id)).toEqual([
      'slow',
      'fast',
      'nodata',
    ])
  })

  it('never mutates the input array', () => {
    const input = [
      entry({ model_id: 'good', success_rate_24h: 1 }),
      entry({ model_id: 'bad', status: 'down', success_rate_24h: 0.5 }),
    ]
    sortListEntries(input, LIST_SORT_DEFAULT)
    expect(input.map((e) => e.model_id)).toEqual(['good', 'bad'])
  })
})

describe('list sort persistence', () => {
  // Minimal Storage stand-in (vitest runs in node — no localStorage).
  const memStorage = () => {
    const map = new Map<string, string>()
    return {
      getItem: (k: string) => map.get(k) ?? null,
      setItem: (k: string, v: string) => void map.set(k, v),
      map,
    }
  }

  it('defaults to availability desc when nothing is stored', () => {
    expect(loadListSort(memStorage())).toEqual({ key: 'rate', dir: 'desc' })
    expect(loadListSort(null)).toEqual(LIST_SORT_DEFAULT)
  })

  it('round-trips a saved sort', () => {
    const s = memStorage()
    saveListSort({ key: 'p95', dir: 'desc' }, s)
    expect(s.map.get(LIST_SORT_STORAGE_KEY)).toBe('{"key":"p95","dir":"desc"}')
    expect(loadListSort(s)).toEqual({ key: 'p95', dir: 'desc' })
  })

  it('falls back to the default on bad JSON, unknown keys and unknown dirs', () => {
    const s = memStorage()
    s.map.set(LIST_SORT_STORAGE_KEY, 'not json{')
    expect(loadListSort(s)).toEqual(LIST_SORT_DEFAULT)
    s.map.set(LIST_SORT_STORAGE_KEY, '{"key":"vendor","dir":"asc"}')
    expect(loadListSort(s)).toEqual(LIST_SORT_DEFAULT)
    s.map.set(LIST_SORT_STORAGE_KEY, '{"key":"rate","dir":"sideways"}')
    expect(loadListSort(s)).toEqual(LIST_SORT_DEFAULT)
    s.map.set(LIST_SORT_STORAGE_KEY, '42')
    expect(loadListSort(s)).toEqual(LIST_SORT_DEFAULT)
  })

  it('survives a throwing storage (private mode / quota)', () => {
    const throwing = {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('quota')
      },
    }
    expect(loadListSort(throwing)).toEqual(LIST_SORT_DEFAULT)
    expect(() => saveListSort(LIST_SORT_DEFAULT, throwing)).not.toThrow()
  })
})

describe('listSortNote', () => {
  it('describes the current ordering literally', () => {
    expect(listSortNote({ key: 'rate', dir: 'desc' })).toBe('（按可用率降序）')
    expect(listSortNote({ key: 'rate', dir: 'asc' })).toBe('（按可用率升序）')
    expect(listSortNote({ key: 'name', dir: 'asc' })).toBe('（按名称升序）')
    expect(listSortNote({ key: 'name', dir: 'desc' })).toBe('（按名称降序）')
    expect(listSortNote({ key: 'p95', dir: 'asc' })).toBe('（按 P95 延迟升序）')
    expect(listSortNote({ key: 'p95', dir: 'desc' })).toBe('（按 P95 延迟降序）')
  })
})
