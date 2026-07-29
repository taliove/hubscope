// Unit tests for the protocol tag type mapping (GH #34, spec 0014;
// ui-guidelines §5 protocol tag entry). The mapping is the single source of
// truth for the three el-tag protocol consumers — the fallback branch is the
// load-bearing caliber pinned here (unknown protocols must never claim a
// chat-family color).
import { describe, it, expect } from 'vitest'
import { protocolTagType } from '@/utils/protocol'

describe('protocolTagType', () => {
  it('maps anthropic to success (chat contract family A, existing color kept)', () => {
    expect(protocolTagType('anthropic')).toBe('success')
  })

  it('maps openai to warning (chat contract family B, existing color kept)', () => {
    expect(protocolTagType('openai')).toBe('warning')
  })

  it('maps images_generation to info (image contract, neutral per spec 0014)', () => {
    expect(protocolTagType('images_generation')).toBe('info')
  })

  it('maps images_edit to info (image contract, neutral per spec 0014)', () => {
    expect(protocolTagType('images_edit')).toBe('info')
  })

  it('falls back to info for unknown protocols (never claims a chat-family color)', () => {
    expect(protocolTagType('some_future_protocol')).toBe('info')
  })

  it('falls back to info for null/undefined', () => {
    expect(protocolTagType(null)).toBe('info')
    expect(protocolTagType(undefined)).toBe('info')
  })
})
