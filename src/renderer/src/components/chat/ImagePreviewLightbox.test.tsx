// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { ImagePreviewLightbox } from './ImagePreviewLightbox'

it.each([[400, 300, 400, 300], [1600, 1200, 800, 600], [300, 1200, 150, 600]])(
  'uses natural dimensions and viewport fit for %sx%s, with numeric zoom, resize and src/reopen reset',
  async (naturalWidth, naturalHeight, expectedWidth, expectedHeight) => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    let width = 800, height = 600
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => width)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(() => height)
    const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
    const download = vi.fn(), close = vi.fn()
    const render = (src = 'data:image/png;base64,first', open = true) => act(async () => root.render(createElement(ImagePreviewLightbox, {
      open, src, alt: 'Geometry', onClose: close, onDownload: download
    })))
    const load = async () => {
      const image = document.querySelector<HTMLImageElement>('[role="dialog"] img')!
      Object.defineProperties(image, { naturalWidth: { configurable: true, value: naturalWidth }, naturalHeight: { configurable: true, value: naturalHeight } })
      await act(async () => image.dispatchEvent(new Event('load')))
      return image
    }
    const click = async (label: string) => act(async () => document.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!.click())
    try {
      await i18n.changeLanguage('en'); await render(); let image = await load()
      expect(image.style.width).toBe(expectedWidth + 'px'); expect(image.style.height).toBe(expectedHeight + 'px')
      await click('Zoom in'); expect(image.style.width).toBe(expectedWidth * 1.25 + 'px')
      await click('Zoom out'); expect(image.style.width).toBe(expectedWidth + 'px')
      await click('Zoom in'); width = 400; height = 300
      await act(async () => window.dispatchEvent(new Event('resize')))
      const fit = Math.min(1, width / naturalWidth, height / naturalHeight)
      expect(image.style.width).toBe(naturalWidth * fit * 1.25 + 'px')
      const viewport = image.closest<HTMLDivElement>('[tabindex="0"]')!; viewport.scrollLeft = 55; viewport.scrollTop = 72
      await render('data:image/png;base64,second'); image = await load()
      expect(image.style.width).toBe(naturalWidth * fit + 'px'); expect(viewport.scrollLeft).toBe(0); expect(viewport.scrollTop).toBe(0)
      await click('Download image'); expect(download).toHaveBeenCalledTimes(1)
      await click('Zoom in'); await render('data:image/png;base64,second', false); await render('data:image/png;base64,second'); image = await load()
      expect(image.style.width).toBe(naturalWidth * fit + 'px')
    } finally { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() }
  }
)

it('restores the current opener for close/reopen while the consumer keeps its wrapper mounted', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host)
  const root = createRoot(host)
  const opener1 = document.createElement('button'), opener2 = document.createElement('button'); document.body.append(opener1, opener2)
  const close = vi.fn()
  const render = async (open: boolean) => act(async () => root.render(createElement(ImagePreviewLightbox, { open, src: 'data:image/png;base64,synthetic', alt: 'Synthetic image', onClose: close, downloadHref:'data:image/png;base64,synthetic' })))
  try {
    await render(false); opener1.focus(); await render(true)
    const dialog = document.querySelector('[role="dialog"]')!
    expect(dialog.contains(document.activeElement)).toBe(true)
    const last = [...dialog.querySelectorAll<HTMLButtonElement>('button')].at(-1)!
    last.focus(); await act(async () => last.dispatchEvent(new KeyboardEvent('keydown', {key:'Tab',bubbles:true,cancelable:true})))
    expect(dialog.contains(document.activeElement)).toBe(true); expect(document.activeElement).not.toBe(last)
    await act(async () => dialog.dispatchEvent(new KeyboardEvent('keydown', {key:'Escape',isComposing:true,bubbles:true,cancelable:true})))
    expect(close).not.toHaveBeenCalled()
    await render(false); expect(document.activeElement).toBe(opener1)
    opener2.focus(); await render(true); await render(false); expect(document.activeElement).toBe(opener2)
  } finally { await act(async () => root.unmount()); host.remove(); opener1.remove(); opener2.remove(); vi.unstubAllGlobals() }
})


it('generates the intended neutral backdrop, dark surface and border through the installed Tailwind pipeline', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  try {
    await act(async () => root.render(createElement(ImagePreviewLightbox, { open: true,
      src: 'data:image/png;base64,synthetic', alt: 'Synthetic image', onClose: vi.fn() })))
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    // Keep Node-only compiler helpers out of the renderer's ambient timer types.
    const nodeModule = 'node:module', nodeProcessModule = 'node:process'
    const { createRequire } = await import(nodeModule)
    const taskProcess = (await import(nodeProcessModule)).default
    const require = createRequire(taskProcess.cwd() + '/package.json')
    const configPath = taskProcess.cwd() + '/tailwind.config.js'
    const config = (await import(configPath)).default
    const result = await require('postcss')([require('tailwindcss')({
      ...config, content: [{ raw: dialog.outerHTML, extension: 'html' }]
    })]).process('@tailwind utilities;', { from: undefined })
    const declarations: string[] = []
    result.root.walkDecls((declaration: { prop: string; value: string }) => declarations.push(declaration.prop + ':' + declaration.value))
    expect(declarations).toContain('background-color:rgb(9 9 11 / 0.82)')
    expect(declarations).toContain('background-color:rgb(9 9 11 / 0.88)')
    expect(declarations).toContain('border-color:rgb(255 255 255 / 0.16)')
  } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
}, 15000)
