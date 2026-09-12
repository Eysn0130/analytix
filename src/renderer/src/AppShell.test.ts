import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AppShell, { prewarmAppShellSurfaces } from './AppShell'
import appShellSource from './AppShell.tsx?raw'
import i18n from './i18n'

// Route loading is an explicit fixture, not a race against the machine's
// module cache. These tests own the shell/Suspense boundary, not view internals.
const routeFixture = vi.hoisted(() => ({ pending: true, wait: new Promise<never>(() => {}) }))
vi.mock('./components/Workbench', () => ({
  Workbench: () => {
    if (routeFixture.pending) throw routeFixture.wait
    return 'fixture-workbench-ready'
  }
}))
vi.mock('./components/SettingsView', () => ({
  SettingsView: () => {
    if (routeFixture.pending) throw routeFixture.wait
    return 'fixture-settings-ready'
  }
}))

// Importing AppShell starts its workbench/settings preloads. afterAll does not
// run when a tag filter excludes every test, so drain collection-owned imports
// before that environment can be torn down. Do not suppress their failures.
await prewarmAppShellSurfaces()

function stubWindow(platform: string): void {
  vi.stubGlobal('window', {
    analytix: { app: { platform } },
    location: { href: 'http://localhost/' }
  })
}

describe('AppShell', () => {
  afterEach(() => {
    routeFixture.pending = true
    vi.unstubAllGlobals()
  })

  it('keeps the macOS app shell on the same full-height flex chain as desktop titlebar platforms', () => {
    stubWindow('darwin')

    const html = renderToStaticMarkup(createElement(AppShell))

    expect(html).toContain('flex h-full min-h-0 flex-col bg-transparent')
    expect(html).toContain('relative flex h-full min-h-0 flex-col bg-transparent')
    expect(html).toContain('ds-native-window-controls-hitbox')
    expect(html).toContain('flex min-h-0 flex-1 flex-col')
    expect(html).not.toContain('ds-windows-titlebar')
  })

  it('renders a visible route fallback instead of a blank shell while lazy views load', () => {
    stubWindow('win32')

    const html = renderToStaticMarkup(createElement(AppShell))

    expect(html).toContain('ds-windows-titlebar')
    expect(html).toContain('ds-window-controls ds-no-drag')
    expect(html).not.toContain('ds-win-traffic-controls')
    expect(html).toContain('role="status"')
    expect(html).toContain('ax-loading-page')
    expect(html).toContain('ax-loading-logo')
    expect(html).toContain(i18n.t('loading'))
  })

  it('leaves local Provider boot ownership exclusively with the startup gate', () => {
    expect(appShellSource).not.toContain('const boot = useChatStore')
    expect(appShellSource).not.toContain('void boot()')
    expect(appShellSource).not.toContain('bootRequestedRef')
  })

  it('renders the ready route instead of the loading fallback', () => {
    stubWindow('darwin')
    routeFixture.pending = false
    const html = renderToStaticMarkup(createElement(AppShell))
    expect(html).toContain('fixture-workbench-ready')
    expect(html).not.toContain('role="status"')
  })
})
