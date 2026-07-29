// Unit tests for the middle-truncation split (GH #54): splitMiddle cuts a
// long model name into a head (CSS-ellipsis zone) and a fixed-width tail so
// the suffix — usually the distinguishing version/variant part — survives
// truncation. The split boundary (tail keep count, the no-split floor) is
// the load-bearing caliber pinned here.
import { describe, it, expect } from 'vitest'
import { splitMiddle, MIDDLE_TAIL_KEEP } from '@/utils/truncate'

describe('splitMiddle', () => {
  it('returns short names unsplit with an empty tail', () => {
    expect(splitMiddle('gpt-4o')).toEqual({ head: 'gpt-4o', tail: '' })
  })

  it('preserves the tail character-for-character on long names', () => {
    const name = 'claude-3-5-sonnet-20241022'
    const { head, tail } = splitMiddle(name)
    expect(tail).toBe('net-20241022')
    expect(head).toBe('claude-3-5-son')
    expect(head + tail).toBe(name)
    expect(tail.length).toBe(MIDDLE_TAIL_KEEP)
  })

  it('does not split at the boundary length (tailKeep + 1) — a 1-char head is noise', () => {
    const name = 'a'.repeat(MIDDLE_TAIL_KEEP + 1)
    expect(splitMiddle(name)).toEqual({ head: name, tail: '' })
  })

  it('splits at tailKeep + 2 and head + tail reassemble the original without overlap', () => {
    const name = 'x'.repeat(MIDDLE_TAIL_KEEP + 2)
    const { head, tail } = splitMiddle(name)
    expect(head).toBe('xx')
    expect(tail).toBe('x'.repeat(MIDDLE_TAIL_KEEP))
    expect(head + tail).toBe(name)
  })

  it('splits long names without hyphens by raw character count', () => {
    const name = 'qwen2point5vlinstruct72b'
    const { head, tail } = splitMiddle(name)
    expect(head + tail).toBe(name)
    expect(head).toBe('qwen2point5v')
    expect(tail).toBe('linstruct72b')
  })

  it('handles an empty string', () => {
    expect(splitMiddle('')).toEqual({ head: '', tail: '' })
  })

  it('honors a custom tailKeep', () => {
    expect(splitMiddle('abcdefghij', 4)).toEqual({ head: 'abcdef', tail: 'ghij' })
  })
})
