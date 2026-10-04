// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import '../../i18n'
import { ImagePreviewLightbox } from './ImagePreviewLightbox'

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
