import { describe, expect, it } from 'vitest'
import {
  availabilityBarWidth,
  entryLatencySeries,
  familyInitials,
  familyOptions,
  groupListSections,
  LIST_GROUPING_DEFAULT,
  LIST_GROUPING_LABELS,
  LIST_GROUPINGS,
  LIST_SORT_DEFAULT,
  LIST_SORT_STORAGE_KEY,
  listSectionMeta,
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

describe('LIST_GROUPINGS / LIST_GROUPING_LABELS (GH #140)', () => {
  it('covers all four grouping values from one Record source', () => {
    expect(LIST_GROUPINGS).toEqual(['none', 'family', 'capability', 'protocol'])
    expect(LIST_GROUPING_LABELS.none).toBe('不分组')
    expect(LIST_GROUPING_LABELS.family).toBe('按厂商')
    expect(LIST_GROUPING_LABELS.capability).toBe('按能力')
    expect(LIST_GROUPING_LABELS.protocol).toBe('按协议')
    expect(LIST_GROUPING_DEFAULT).toBe('none')
  })
})

describe('groupListSections (GH #140)', () => {
  const rows = (): OverviewEntry[] => [
    entry({ endpoint_id: 1, model_id: 'a-1', family: 'gpt', capability: 'chat', protocol: 'openai', status: 'healthy', success_rate_24h: 0.99 }),
    entry({ endpoint_id: 2, model_id: 'b-1', family: 'claude', capability: 'chat', protocol: 'anthropic', status: 'failing', success_rate_24h: 0.5 }),
    entry({ endpoint_id: 3, model_id: 'a-1', family: 'gpt', capability: 'chat', protocol: 'anthropic', status: 'healthy', success_rate_24h: 0.96 }),
    entry({ endpoint_id: 4, model_id: 'c-1', family: 'glm', capability: 'image', protocol: 'images_generation', status: 'degraded', success_rate_24h: 0.8 }),
  ]

  it('buckets by the grouping dimension, one section per distinct key', () => {
    expect(groupListSections(rows(), 'family', LIST_SORT_DEFAULT).map(g => g.key).sort()).toEqual(['claude', 'glm', 'gpt'])
    expect(groupListSections(rows(), 'capability', LIST_SORT_DEFAULT).map(g => g.key).sort()).toEqual(['chat', 'image'])
    expect(groupListSections(rows(), 'protocol', LIST_SORT_DEFAULT).map(g => g.key).sort()).toEqual(['anthropic', 'images_generation', 'openai'])
    expect(groupListSections([], 'family', LIST_SORT_DEFAULT)).toEqual([])
  })

  it('ranks groups by the most severe ENABLED entry, ties by key lex', () => {
    // claude group carries the failing entry → leads; glm degraded → second;
    // gpt all-healthy → last, even under the default rate-desc column sort.
    const sections = groupListSections(rows(), 'family', LIST_SORT_DEFAULT)
    expect(sections.map(g => g.key)).toEqual(['claude', 'glm', 'gpt'])
  })

  it('sinks disabled-only groups below every enabled group', () => {
    const disabled = rows().map(e => ({ ...e, enabled: false }))
    const mixed = [...rows().filter(e => e.family !== 'glm'), entry({ endpoint_id: 9, model_id: 'z-1', family: 'zzz', status: 'down', enabled: false })]
    const sections = groupListSections(mixed, 'family', LIST_SORT_DEFAULT)
    expect(sections[sections.length - 1]!.key).toBe('zzz')
    expect(groupListSections(disabled, 'family', LIST_SORT_DEFAULT).map(g => g.key)).toEqual(['claude', 'glm', 'gpt'])
  })

  it('sorts entries INSIDE each group by the active column sort (buckets kept)', () => {
    // The gpt group has a rated 0.99 row, a rated 0.96 row: rate desc keeps
    // 0.99 first; rate asc flips them; a disabled row sinks in every state.
    const gptRows = [
      entry({ endpoint_id: 1, model_id: 'a-1', family: 'gpt', success_rate_24h: 0.96 }),
      entry({ endpoint_id: 2, model_id: 'a-2', family: 'gpt', success_rate_24h: 0.99 }),
      entry({ endpoint_id: 3, model_id: 'a-3', family: 'gpt', success_rate_24h: 0.5, enabled: false }),
    ]
    const desc = groupListSections(gptRows, 'family', { key: 'rate', dir: 'desc' })[0]!
    expect(desc.entries.map(e => e.endpoint_id)).toEqual([2, 1, 3])
    const asc = groupListSections(gptRows, 'family', { key: 'rate', dir: 'asc' })[0]!
    expect(asc.entries.map(e => e.endpoint_id)).toEqual([1, 2, 3])
    const byName = groupListSections(gptRows, 'family', { key: 'name', dir: 'asc' })[0]!
    expect(byName.entries.map(e => e.model_id)).toEqual(['a-1', 'a-2', 'a-3'])
  })

  it('never mutates the input array', () => {
    const input = rows()
    const snapshot = [...input]
    groupListSections(input, 'family', LIST_SORT_DEFAULT)
    expect(input).toEqual(snapshot)
  })
})

describe('listSectionMeta (GH #140)', () => {
  it('reads N 个端点 when nothing is abnormal', () => {
    const healthy = [
      entry({ endpoint_id: 1, model_id: 'a' }),
      entry({ endpoint_id: 2, model_id: 'b' }),
    ]
    expect(listSectionMeta(healthy)).toBe('2 个端点')
  })

  it('appends display-state counts with words from the single mapping', () => {
    const mixed = [
      entry({ endpoint_id: 1, model_id: 'a', status: 'failing' }),
      entry({ endpoint_id: 2, model_id: 'b', status: 'down' }),
      entry({ endpoint_id: 3, model_id: 'c', status: 'degraded' }),
      entry({ endpoint_id: 4, model_id: 'd', status: 'healthy' }),
    ]
    // down + failing read together as 异常 (display-layer mapping); the
    // counts dedupe by model (abnormalModelCounts caliber).
    expect(listSectionMeta(mixed)).toBe('4 个端点 · 异常 2 · 降级 1')
  })

  it('dedupes abnormal counts by model, never by endpoint row', () => {
    const twoProtocols = [
      entry({ endpoint_id: 1, model_id: 'a', protocol: 'openai', status: 'down' }),
      entry({ endpoint_id: 2, model_id: 'a', protocol: 'anthropic', status: 'degraded' }),
    ]
    expect(listSectionMeta(twoProtocols)).toBe('2 个端点 · 异常 1 · 降级 0')
  })

  it('ignores disabled endpoints in the abnormal counts', () => {
    const disabledDown = [
      entry({ endpoint_id: 1, model_id: 'a', status: 'down', enabled: false }),
      entry({ endpoint_id: 2, model_id: 'b', status: 'healthy' }),
    ]
    expect(listSectionMeta(disabledDown)).toBe('2 个端点')
  })
})
