// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  DevBrowserPanel,
  canUseElectronWebviewEnvironment,
  resolveInitialDevBrowserUrl
} from './DevBrowserPanel'
import devBrowserPanelSource from './DevBrowserPanel.tsx?raw'

describe('DevBrowserPanel webview environment detection', () => {
  it('requires the Electron user agent in addition to the shell bridge', () => {
    expect(
      canUseElectronWebviewEnvironment({
        openExternalAvailable: true,
        userAgent:
          'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126 Safari/537.36'
      })
    ).toBe(false)
  })

  it('allows Electron renderer environments with the shell bridge', () => {
    expect(
      canUseElectronWebviewEnvironment({
        openExternalAvailable: true,
        userAgent:
          'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/132 Safari/537.36 Electron/34.2.0'
      })
    ).toBe(true)
  })

  it('rejects Electron-like pages when the shell bridge is absent', () => {
    expect(
      canUseElectronWebviewEnvironment({
        openExternalAvailable: false,
        userAgent:
          'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/132 Safari/537.36 Electron/34.2.0'
      })
    ).toBe(false)
  })
})

describe('DevBrowserPanel initial URL resolution', () => {
  it('stays blank when no preview URL source exists', () => {
    expect(
      resolveInitialDevBrowserUrl({
        normalizedPreferredUrl: null,
        storedUrl: null,
        latestDetectedUrl: null
      })
    ).toBeNull()
  })

  it('prefers explicit preview URL sources in order', () => {
    expect(
      resolveInitialDevBrowserUrl({
        normalizedPreferredUrl: 'http://localhost:3000/',
        storedUrl: 'http://localhost:4000/',
        latestDetectedUrl: 'http://localhost:5000/'
      })
    ).toBe('http://localhost:3000/')

    expect(
      resolveInitialDevBrowserUrl({
        normalizedPreferredUrl: null,
        storedUrl: 'http://localhost:4000/',
        latestDetectedUrl: 'http://localhost:5000/'
      })
    ).toBe('http://localhost:4000/')

    expect(
      resolveInitialDevBrowserUrl({
        normalizedPreferredUrl: null,
        storedUrl: null,
        latestDetectedUrl: 'http://localhost:5000/'
      })
    ).toBe('http://localhost:5000/')
  })
})

describe('DevBrowserPanel titlebar controls', () => {
  it('keeps browser actions in the address row beneath the shared workspace tabs', () => {
    const addressbarStart = devBrowserPanelSource.indexOf('<form onSubmit={submitUrl}')
    const addressbarEnd = devBrowserPanelSource.indexOf('</form>', addressbarStart)
    const collapseStart = devBrowserPanelSource.indexOf('onClick={onCollapse}')

    expect(devBrowserPanelSource).not.toContain('flex h-[var(--ax-chat-topbar-shell-height)] min-w-0 items-center')
    expect(addressbarStart).toBeGreaterThan(-1)
    expect(collapseStart).toBeGreaterThan(addressbarStart)
    expect(collapseStart).toBeLessThan(addressbarEnd)
    expect(devBrowserPanelSource.slice(addressbarStart, addressbarEnd)).toContain('onClick={resetPreview}')
    expect(devBrowserPanelSource.slice(addressbarStart, addressbarEnd)).toContain('onClick={reload}')
    expect(devBrowserPanelSource.slice(addressbarStart, addressbarEnd)).toContain('captureSelection()')
    expect(devBrowserPanelSource.slice(addressbarStart, addressbarEnd)).toContain("captureSelection(t('browserExplainPrompt'))")
  })
})

vi.mock('react-i18next', async (importOriginal) => ({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation: () => ({ t: (key: string) => key })
}))

describe('DevBrowserPanel compact toolbar actions', () => {
  let element: HTMLDivElement
  let root: Root
  const collapse = vi.fn()
  const external = vi.fn().mockResolvedValue(undefined)
  const originalBridge = window.analytix

  beforeEach(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    vi.clearAllMocks()
    localStorage.clear()
    Object.assign(window, { analytix: { app: { openExternal: external } } })
    element = document.createElement('div')
    document.body.append(element)
    root = createRoot(element)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    element.remove()
    localStorage.clear()
    if (originalBridge === undefined) Reflect.deleteProperty(window, 'analytix')
    else Object.assign(window, { analytix: originalBridge })
    vi.restoreAllMocks()
  })

  async function render() {
    await act(async () => root.render(createElement(DevBrowserPanel, {
      preferredUrl: 'http://localhost:3000/one', detectedUrls: [], onCollapse: collapse
    })))
  }

  function button(key: string, compact = true): HTMLButtonElement {
    const target = compact ? element.querySelector('.ds-browser-compact-actions') : element.querySelector('details')
    const result = target?.querySelector<HTMLButtonElement>(`button[aria-label="${key}"]`)
    expect(result, key).not.toBeNull()
    return result!
  }

  async function click(key: string, compact = true) {
    await act(async () => button(key, compact).click())
  }

  it('uses the existing submit and iframe history owners from the compact menu', async () => {
    await render()
    expect(button('browserBack').disabled).toBe(true)
    expect(button('browserForward').disabled).toBe(true)
    const input = element.querySelector('input')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'localhost:3000/two')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await click('browserOpen')
    expect(element.querySelector('iframe')?.getAttribute('src')).toBe('http://localhost:3000/two')
    expect(button('browserBack').disabled).toBe(false)
    await click('browserBack')
    expect(input.value).toBe('localhost:3000/one')
    expect(button('browserBack').disabled).toBe(true)
    expect(button('browserForward').disabled).toBe(false)
    await click('browserForward')
    expect(input.value).toBe('localhost:3000/two')
    expect(button('browserForward').disabled).toBe(true)
    const beforeReload = element.querySelector('iframe')
    await click('browserReload')
    expect(element.querySelector('iframe')).not.toBe(beforeReload)
    expect(element.querySelectorAll('iframe')).toHaveLength(1)
  })

  it('dispatches native navigation and reload exactly once through existing guest handlers', async () => {
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Electron/41.10.3')
    await render()
    const guest = element.querySelector('webview')!
    const back = vi.fn(), forward = vi.fn(), reload = vi.fn()
    Object.assign(guest, { canGoBack: () => true, canGoForward: () => true,
      getURL: () => 'http://localhost:3000/one', goBack: back, goForward: forward,
      reloadIgnoringCache: reload })
    await act(async () => guest.dispatchEvent(new Event('did-stop-loading')))
    await click('browserBack')
    await click('browserForward')
    await click('browserReload')
    expect(back).toHaveBeenCalledTimes(1)
    expect(forward).toHaveBeenCalledTimes(1)
    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('keeps reset, auto-follow, external open and close in the existing menu', async () => {
    await render()
    await click('browserAutoFollow', false)
    expect(button('browserAutoFollow', false).getAttribute('aria-pressed')).toBe('false')
    await click('browserOpenExternal', false)
    expect(external).toHaveBeenCalledExactlyOnceWith('http://localhost:3000/one')
    await click('browserReset', false)
    expect(element.querySelector('input')?.value).toBe('')
    expect(button('browserBack').disabled).toBe(true)
    expect(button('browserForward').disabled).toBe(true)
    expect(button('browserReload').disabled).toBe(true)
    expect(button('browserOpenExternal', false).disabled).toBe(true)
    await click('browserCloseTab', false)
    expect(collapse).toHaveBeenCalledTimes(1)
  })

  it('closes on Escape and returns keyboard focus to More', async () => {
    await render()
    const details = element.querySelector('details')!
    details.open = true
    const reload = button('browserReload')
    reload.focus()
    await act(async () => reload.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(details.open).toBe(false)
    expect(document.activeElement).toBe(details.querySelector('summary'))
  })
})
