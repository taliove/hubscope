import { describe, expect, it } from 'vitest'

import { shortVersion } from './version'

describe('shortVersion', () => {
  it('keeps a plain release tag as-is', () => {
    expect(shortVersion('v0.3.0')).toBe('v0.3.0')
  })

  it('trims a release tag with describe suffix to the tag prefix', () => {
    expect(shortVersion('v0.3.0-2-gfe91a9d')).toBe('v0.3.0')
  })

  it('extracts the g<hash> segment from a standard dev stamp', () => {
    expect(shortVersion('dev-v0.3.0-2-gfe91a9d-20260730-153045')).toBe('dev-gfe91a9d')
  })

  it('extracts the g<hash> segment from a dirty dev stamp', () => {
    expect(shortVersion('dev-v0.3.0-2-gfe91a9d-dirty-20260730-153045')).toBe('dev-gfe91a9d')
  })

  it('keeps the hash of a dev stamp without a tag base', () => {
    expect(shortVersion('dev-gfe91a9d-20260730-153045')).toBe('dev-gfe91a9d')
  })

  it('falls back to the describe body when no hash exists (nogit)', () => {
    expect(shortVersion('dev-nogit-20260730-153045')).toBe('dev-nogit')
  })

  it('handles a plain-hash dev stamp (git describe --always without tags)', () => {
    expect(shortVersion('dev-fe91a9d-20260730-153045')).toBe('dev-fe91a9d')
  })

  it('returns empty string for empty input', () => {
    expect(shortVersion('')).toBe('')
  })

  it('does not throw on garbage input and returns it unchanged', () => {
    expect(shortVersion('hello world')).toBe('hello world')
    expect(shortVersion('dev-')).toBe('dev-')
  })
})
