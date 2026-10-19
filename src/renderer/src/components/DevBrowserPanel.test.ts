// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  DevBrowserPanel,
  canUseElectronWebviewEnvironment,
  resolveInitialDevBrowserUrl
} from './DevBrowserPanel'

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

vi.mock('react-i18next', async (importOriginal) => ({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation: () => ({ t: (key: string) => key })
}))

describe('DevBrowserPanel shared operation row', () => {
  let element: HTMLDivElement
  let root: Root
  const collapse = vi.fn()
  const external = vi.fn().mockResolvedValue(undefined)
  const originalBridge = window.analytix

  beforeEach(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    vi.clearAllMocks()
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
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
    vi.restoreAllMocks(); vi.unstubAllGlobals()
  })

  async function render(props: Partial<Parameters<typeof DevBrowserPanel>[0]> = {}) {
    await act(async () => root.render(createElement(DevBrowserPanel, {
      preferredUrl: 'http://localhost:3000/one', detectedUrls: [], onCollapse: collapse, ...props
    })))
  }

  async function menu() {
    if (!document.querySelector('[role="menu"]')) {
      await act(async () => element.querySelector<HTMLButtonElement>('button[aria-label="browserMore"]')!.click())
    }
    return document.querySelector<HTMLElement>('[role="menu"]')!
  }

  async function button(key: string) {
    const panel = await menu()
    const result = panel.querySelector<HTMLButtonElement>(`button[aria-label="${key}"]`)
    expect(result, key).not.toBeNull()
    return result!
  }

  async function click(key: string) {
    const target = await button(key)
    await act(async () => target.click())
  }

  it('uses exactly one operation row for address, navigation and workspace controls', async () => {
    const focus = vi.fn(), close = vi.fn()
    await render({ viewerControls: { focused: false, onToggleFocus: focus, onCollapse: close } })
    const header = element.querySelector('[data-workspace-viewer-header]')!
    expect(element.querySelectorAll('[data-workspace-viewer-header]')).toHaveLength(1)
    expect(header.closest('form')).toBe(element.querySelector('form'))
    expect(header.querySelector('input[aria-label="browserAddressPlaceholder"]')).not.toBeNull()
    for (const label of ['browserBack', 'browserForward', 'browserReload', 'browserOpen', 'browserMore', 'workbenchFocus', 'workbenchCollapse']) {
      expect(header.querySelector(`button[aria-label="${label}"] svg`), label).not.toBeNull()
    }
    await act(async () => header.querySelector<HTMLButtonElement>('[aria-label="workbenchFocus"]')!.click())
    await act(async () => header.querySelector<HTMLButtonElement>('[aria-label="workbenchCollapse"]')!.click())
    expect(focus).toHaveBeenCalledTimes(1); expect(close).toHaveBeenCalledTimes(1)
    expect((await menu()).querySelector('[aria-label="browserCloseTab"]')).toBeNull()
  })

  it('keeps address submission and iframe history attached to the existing handlers', async () => {
    await render()
    expect((await button('browserBack')).disabled).toBe(true)
    expect((await button('browserForward')).disabled).toBe(true)
    const input = element.querySelector('input')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'localhost:3000/two')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await click('browserOpen')
    expect(element.querySelector('iframe')?.getAttribute('src')).toBe('http://localhost:3000/two')
    expect((await button('browserBack')).disabled).toBe(false)
    await click('browserBack')
    expect(input.value).toBe('localhost:3000/one')
    expect((await button('browserBack')).disabled).toBe(true)
    expect((await button('browserForward')).disabled).toBe(false)
    await click('browserForward')
    expect(input.value).toBe('localhost:3000/two')
    const beforeReload = element.querySelector('iframe')
    await click('browserReload')
    expect(element.querySelector('iframe')).not.toBe(beforeReload)
    expect(element.querySelectorAll('iframe')).toHaveLength(1)
    await act(async () => element.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })))
    expect(element.querySelectorAll('iframe')).toHaveLength(1)
  })

  it('dispatches native navigation and reload once through existing guest handlers', async () => {
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Electron/41.10.3')
    await render()
    const guest = element.querySelector('webview')!
    const back = vi.fn(), forward = vi.fn(), reload = vi.fn()
    Object.assign(guest, { canGoBack: () => true, canGoForward: () => true,
      getURL: () => 'http://localhost:3000/one', goBack: back, goForward: forward,
      reloadIgnoringCache: reload })
    await act(async () => guest.dispatchEvent(new Event('did-stop-loading')))
    await click('browserBack'); await click('browserForward'); await click('browserReload')
    expect(back).toHaveBeenCalledTimes(1); expect(forward).toHaveBeenCalledTimes(1); expect(reload).toHaveBeenCalledTimes(1)
  })

  it('preserves reset, auto-follow, external open and standalone collapse actions', async () => {
    await render()
    await click('browserAutoFollow')
    expect((await button('browserAutoFollow')).getAttribute('aria-pressed')).toBe('false')
    await click('browserOpenExternal')
    expect(external).toHaveBeenCalledExactlyOnceWith('http://localhost:3000/one')
    await click('browserReset')
    expect(element.querySelector('input')?.value).toBe('')
    for (const label of ['browserBack', 'browserForward', 'browserReload', 'browserOpenExternal']) expect((await button(label)).disabled).toBe(true)
    await click('browserCloseTab')
    expect(collapse).toHaveBeenCalledTimes(1)
  })

  it('offers detected URLs only inside More and keeps navigation and auto-follow semantics', async () => {
    const latest = 'http://localhost:4000/latest', previous = 'http://localhost:5000/previous'
    localStorage.setItem('analytix.devPreview.autoFollow', 'false')
    await render({ detectedUrls: [latest, previous] })
    expect(element.querySelectorAll('[data-workspace-viewer-header]')).toHaveLength(1)
    expect(element.querySelector('[data-browser-detected-url]')).toBeNull()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    let panel = await menu()
    expect(panel.querySelectorAll('[data-browser-detected-url]')).toHaveLength(2)
    const oldUrl = panel.querySelector<HTMLButtonElement>(`[data-browser-detected-url="${previous}"]`)!
    expect(oldUrl.querySelector('svg[aria-hidden="true"]')).not.toBeNull()
    expect(oldUrl.getAttribute('aria-label')).toBe(previous)
    await act(async () => oldUrl.click())
    expect(element.querySelector('iframe')?.getAttribute('src')).toBe(previous)
    expect(localStorage.getItem('analytix.devPreview.autoFollow')).toBe('false')
    expect(document.querySelector('[role="menu"]')).toBeNull()
    panel = await menu()
    await act(async () => panel.querySelector<HTMLButtonElement>(`[data-browser-detected-url="${latest}"]`)!.click())
    expect(element.querySelector('iframe')?.getAttribute('src')).toBe(latest)
    expect(localStorage.getItem('analytix.devPreview.autoFollow')).toBe('false')
    expect(element.querySelector('[data-browser-detected-url]')).toBeNull()
  })

  it('closes the menu on Escape and when the retained browser stops being visible', async () => {
    await render()
    const reload = await button('browserReload')
    await act(async () => reload.focus())
    await act(async () => reload.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(document.activeElement).toBe(element.querySelector('[aria-label="browserMore"]'))
    await menu()
    await render({ visible: false })
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(element.querySelector<HTMLButtonElement>('[aria-label="browserMore"]')!.disabled).toBe(true)
    expect(element.querySelectorAll('iframe')).toHaveLength(1)
  })
})
