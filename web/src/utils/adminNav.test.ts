import { describe, expect, it } from 'vitest'
import { parseAdminTabQuery, parseSettingsItemQuery } from './adminNav'

describe('parseAdminTabQuery', () => {
  it('parses a known admin tab', () => {
    expect(parseAdminTabQuery('settings')).toBe('settings')
    expect(parseAdminTabQuery('resources')).toBe('resources')
    expect(parseAdminTabQuery('eval-ops')).toBe('eval-ops')
  })

  it('takes the first value of a repeated query param', () => {
    expect(parseAdminTabQuery(['logs', 'settings'])).toBe('logs')
  })

  it('ignores unknown tabs silently', () => {
    expect(parseAdminTabQuery('bogus')).toBeNull()
    expect(parseAdminTabQuery('SETTINGS')).toBeNull()
    expect(parseAdminTabQuery('settings ')).toBeNull()
  })

  it('ignores missing or empty values', () => {
    expect(parseAdminTabQuery(undefined)).toBeNull()
    expect(parseAdminTabQuery(null)).toBeNull()
    expect(parseAdminTabQuery('')).toBeNull()
    expect(parseAdminTabQuery([])).toBeNull()
  })
})

describe('parseSettingsItemQuery', () => {
  it('parses a known settings item anchor', () => {
    expect(parseSettingsItemQuery('judge_model')).toBe('judge_model')
    expect(parseSettingsItemQuery('lark_webhook_url')).toBe('lark_webhook_url')
    expect(parseSettingsItemQuery('eval_concurrency')).toBe('eval_concurrency')
  })

  it('takes the first value of a repeated query param', () => {
    expect(parseSettingsItemQuery(['judge_model', 'alert_enabled'])).toBe('judge_model')
  })

  it('ignores unknown items silently', () => {
    expect(parseSettingsItemQuery('bogus')).toBeNull()
    expect(parseSettingsItemQuery('judge-model')).toBeNull()
    expect(parseSettingsItemQuery('JUDGE_MODEL')).toBeNull()
  })

  it('ignores missing or empty values', () => {
    expect(parseSettingsItemQuery(undefined)).toBeNull()
    expect(parseSettingsItemQuery(null)).toBeNull()
    expect(parseSettingsItemQuery('')).toBeNull()
    expect(parseSettingsItemQuery([])).toBeNull()
  })
})
