import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterAll, afterEach, describe, expect, it, vi } from 'vitest'
import AppShell, { prewarmAppShellSurfaces } from './AppShell'
import appShellSource from './AppShell.tsx?raw'
import i18n from './i18n'

function stubWindow(platform: string): void {
  vi.stubGlobal('window', {
    analytix: { app: { platform } },
    location: { href: 'http://localhost/' }
  })
}

describe('AppShell', () => {
  afterAll(async () => {
    await prewarmAppShellSurfaces()
  })

  afterEach(() => {
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
})
