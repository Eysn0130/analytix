import { describe, expect, it } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { coerceRendererSettings, DEFAULT_WORKSPACE_ROOT, mergeSettings } from './settings-utils'

const settings = (uiFontScale?: unknown): AppSettingsV1 => ({ ...normalizeAppSettings({} as AppSettingsV1), uiFontScale } as AppSettingsV1)

describe('renderer workspace preference compatibility', () => {
  it.each([undefined, null, 123])('uses the visible default for an absent workspace (%j)', value => {
    const raw = { ...settings(), workspaceRoot: value } as unknown as AppSettingsV1
    expect(DEFAULT_WORKSPACE_ROOT).toBe('~/Analytix')
    expect(coerceRendererSettings(raw).workspaceRoot).toBe('~/Analytix')
  })

  it.each([
    '~/.analytix/default_workspace',
    '/synthetic-home/.analytix/default_workspace',
    '/synthetic-project',
    'relative-project'
  ])('preserves the existing workspace %s through an unrelated patch', workspaceRoot => {
    const raw = { ...settings(), workspaceRoot }
    expect(coerceRendererSettings(raw).workspaceRoot).toBe(workspaceRoot)
    expect(mergeSettings(raw, { locale: 'en' }).workspaceRoot).toBe(workspaceRoot)
  })
})

describe('renderer font preference fallback', () => {
  it.each([undefined, null, 'invalid', 123])('matches the shared small default for %j', value => {
    const raw = settings(value)
    expect(coerceRendererSettings(raw).uiFontScale).toBe('small')
    expect(coerceRendererSettings(raw).uiFontScale).toBe(normalizeAppSettings(raw).uiFontScale)
  })
  it.each(['small', 'medium', 'large'] as const)('preserves the explicit %s choice through coercion and an unrelated patch', value => {
    const raw = settings(value)
    expect(coerceRendererSettings(raw).uiFontScale).toBe(value)
    expect(mergeSettings(raw, { locale: 'en' }).uiFontScale).toBe(value)
  })
  it('uses small when an unrelated patch encounters a missing font preference', () => {
    const raw = settings(); delete (raw as Partial<AppSettingsV1>).uiFontScale
    expect(mergeSettings(raw, { locale: 'en' }).uiFontScale).toBe('small')
  })
})
