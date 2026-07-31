import { describe, expect, it } from 'vitest'
import {
  EVAL_TABS,
  MODELS_TABS,
  SETTINGS_TABS,
  legacyAdminTarget,
  parseSettingsItemQuery,
  parseTabQuery,
} from './adminNav'

describe('parseTabQuery', () => {
  it('parses a known tab of the allowed set', () => {
    expect(parseTabQuery('rules', MODELS_TABS)).toBe('rules')
    expect(parseTabQuery('logs', SETTINGS_TABS)).toBe('logs')
    expect(parseTabQuery('ops', EVAL_TABS)).toBe('ops')
  })

  it('takes the first value of a repeated query param', () => {
    expect(parseTabQuery(['logs', 'settings'], SETTINGS_TABS)).toBe('logs')
  })

  it('ignores tabs outside the allowed set silently', () => {
    expect(parseTabQuery('logs', MODELS_TABS)).toBeNull()
    expect(parseTabQuery('resources', SETTINGS_TABS)).toBeNull()
    expect(parseTabQuery('bogus', EVAL_TABS)).toBeNull()
  })

  it('ignores casing and whitespace variants silently', () => {
    expect(parseTabQuery('SETTINGS', SETTINGS_TABS)).toBeNull()
    expect(parseTabQuery('settings ', SETTINGS_TABS)).toBeNull()
  })

  it('ignores missing or empty values', () => {
    expect(parseTabQuery(undefined, SETTINGS_TABS)).toBeNull()
    expect(parseTabQuery(null, SETTINGS_TABS)).toBeNull()
    expect(parseTabQuery('', SETTINGS_TABS)).toBeNull()
    expect(parseTabQuery([], SETTINGS_TABS)).toBeNull()
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

describe('legacyAdminTarget', () => {
  it('maps the resources/rules panes to /models keeping the tab query', () => {
    expect(legacyAdminTarget('resources', null)).toEqual({ path: '/models', query: { tab: 'resources' } })
    expect(legacyAdminTarget('rules', undefined)).toEqual({ path: '/models', query: { tab: 'rules' } })
  })

  it('maps the logs/users panes to /settings keeping the tab query', () => {
    expect(legacyAdminTarget('logs', null)).toEqual({ path: '/settings', query: { tab: 'logs' } })
    expect(legacyAdminTarget('users', null)).toEqual({ path: '/settings', query: { tab: 'users' } })
  })

  it('maps the settings pane to /settings preserving a valid item anchor', () => {
    expect(legacyAdminTarget('settings', 'judge_model')).toEqual({
      path: '/settings',
      query: { tab: 'settings', item: 'judge_model' },
    })
  })

  it('drops an invalid item anchor on the settings pane', () => {
    expect(legacyAdminTarget('settings', 'bogus')).toEqual({ path: '/settings', query: { tab: 'settings' } })
  })

  it('drops the item anchor for non-settings panes (AdminView semantics)', () => {
    expect(legacyAdminTarget('logs', 'judge_model')).toEqual({ path: '/settings', query: { tab: 'logs' } })
  })

  it('maps eval-ops and case-library to the /eval secondary tabs', () => {
    expect(legacyAdminTarget('eval-ops', null)).toEqual({ path: '/eval', query: { tab: 'ops' } })
    expect(legacyAdminTarget('case-library', null)).toEqual({ path: '/eval', query: { tab: 'cases' } })
  })

  it('falls back to /settings for missing or unknown tabs', () => {
    expect(legacyAdminTarget(null, null)).toEqual({ path: '/settings', query: {} })
    expect(legacyAdminTarget(undefined, 'judge_model')).toEqual({ path: '/settings', query: {} })
    expect(legacyAdminTarget('bogus', null)).toEqual({ path: '/settings', query: {} })
  })
})
