// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LegacySessionDetectResult } from '@shared/analytix-api'
import i18n from '../i18n'
import { LegacySessionImportCard } from './settings-section-general-legacy-import'

let root: Root, host: HTMLDivElement, originalBridge: Window['analytix'] | undefined
const t = (key: string, values?: Record<string, unknown>) => String(i18n.t('settings:' + key, values))
const result: LegacySessionDetectResult = { destDir: '/fixture/current/threads', sources: [
  { id: 'fixture-source', kind: 'analytix', path: '/fixture/previous/threads', threadCount: 3, newCount: 2 }
] }
const importSessions = vi.fn(), pickDirectory = vi.fn(), restart = vi.fn(), confirm = vi.fn()
function bridge(detect?: () => Promise<LegacySessionDetectResult>) {
  window.analytix = { diagnostics: { detectLegacySessions: detect, importLegacySessions: importSessions,
    pickLegacySessionDir: pickDirectory }, runtime: { restartRuntime: restart }, app: { confirmDialog: confirm } } as unknown as Window['analytix']
}
async function mount() { await act(async () => root.render(createElement(LegacySessionImportCard, { t, tCommon: key => key }))) }
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  originalBridge = window.analytix
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove()
  if (originalBridge) window.analytix = originalBridge
  else Reflect.deleteProperty(window, 'analytix')
  vi.clearAllMocks(); vi.unstubAllGlobals()
})

describe.each(['en', 'zh'])('honest legacy-source card in %s', language => {
  beforeEach(async () => { await i18n.changeLanguage(language) })
  it('retains observed sources and counts with an unavailable import explanation and no failing actions', async () => {
    const detect = vi.fn(async () => result); bridge(detect); await mount()
    expect(detect).toHaveBeenCalledTimes(1)
    expect(host.textContent).toContain(t('legacyImportFound', { count: 2 }))
    expect(host.textContent).toContain('/fixture/previous/threads')
    expect(host.textContent).toContain(t('legacyImportUnavailable'))
    expect(host.querySelectorAll('button')).toHaveLength(0)
    expect(importSessions).not.toHaveBeenCalled(); expect(pickDirectory).not.toHaveBeenCalled()
    expect(restart).not.toHaveBeenCalled(); expect(confirm).not.toHaveBeenCalled()
  })
  it('reports a known empty detection separately from an unavailable detector', async () => {
    bridge(async () => ({ destDir: result.destDir, sources: [] })); await mount()
    expect(host.textContent).toContain(t('legacyImportNoneFound'))
    expect(host.textContent).not.toContain(t('legacyImportScanUnavailable'))
  })
  it.each(['missing', 'rejected', 'throws'] as const)('keeps %s detection honestly unavailable', async kind => {
    bridge(kind === 'missing' ? undefined : kind === 'throws' ? () => { throw new Error('fixture scan unavailable') }
      : async () => { throw new Error('fixture scan unavailable') })
    await mount()
    expect(host.textContent).toContain(t('legacyImportScanUnavailable'))
    expect(host.textContent).not.toContain(t('legacyImportNoneFound'))
    expect(host.querySelectorAll('button')).toHaveLength(0)
  })
  it('ignores a retired detector reply without replacing the next source card', async () => {
    let resolve!: (value: LegacySessionDetectResult) => void
    bridge(() => new Promise<LegacySessionDetectResult>(done => { resolve = done })); await mount()
    await act(async () => root.render(null))
    bridge(async () => ({ destDir: result.destDir, sources: [] })); await mount()
    await act(async () => resolve(result))
    expect(host.textContent).toContain(t('legacyImportNoneFound'))
    expect(host.textContent).not.toContain('/fixture/previous/threads')
  })
})
