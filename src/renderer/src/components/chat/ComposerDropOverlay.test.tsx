// @vitest-environment jsdom
import { act, createElement, createRef } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ComposerDropOverlay, type ComposerDropOverlayProps } from './ComposerDropOverlay'

let host: HTMLDivElement, outside: HTMLDivElement, root: Root
let rootRef: ReturnType<typeof createRef<HTMLDivElement>>
const forwardedDrop = vi.fn(), cardDrop = vi.fn()
const canAccept = (transfer: DataTransfer): boolean => transfer.files[0]?.type === 'image/png'
const overlay = () => document.querySelector<HTMLElement>('[data-composer-drop-overlay]')
const card = () => rootRef.current!
const transfer = (mimeType = 'image/png', types = ['Files']): DataTransfer => ({
  types, files: [new File(['synthetic'], 'fixture', { type: mimeType })], dropEffect: 'none'
}) as unknown as DataTransfer

async function draw(patch: Partial<ComposerDropOverlayProps> = {}) {
  await act(async () => root.render(createElement('div', { ref: rootRef,
    onDrop: (event: { preventDefault: () => void }) => { event.preventDefault(); cardDrop() }
  }, createElement('span', { 'data-card-child': '' }, 'Composer'), createElement(ComposerDropOverlay, {
    enabled: true, ownerKey: 'workspace-a/thread-a', rootRef, canAccept, onDrop: forwardedDrop,
    label: 'Drop files here', ...patch
  }))))
}

async function drag(type: 'dragenter' | 'dragover' | 'dragleave' | 'drop', data: DataTransfer,
  target: EventTarget = outside, coordinates = { clientX: 50, clientY: 50 }) {
  const event = new Event(type, { bubbles: true, cancelable: true }) as DragEvent
  Object.defineProperties(event, {
    dataTransfer: { value: data }, clientX: { value: coordinates.clientX }, clientY: { value: coordinates.clientY }
  })
  await act(async () => { target.dispatchEvent(event) })
  return event
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  forwardedDrop.mockReset(); cardDrop.mockReset()
  host = document.createElement('div'); outside = document.createElement('div')
  document.body.append(host, outside); root = createRoot(host); rootRef = createRef<HTMLDivElement>()
  await draw()
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); outside.remove(); vi.unstubAllGlobals()
})

describe('composer document drop invitation', () => {
  it('advertises only files accepted by the existing composer gate', async () => {
    const text = transfer('text/plain', ['text/plain']), unsupported = transfer('application/octet-stream')
    expect((await drag('dragenter', text)).defaultPrevented).toBe(false)
    expect((await drag('dragenter', unsupported)).defaultPrevented).toBe(false)
    expect((await drag('dragover', unsupported)).defaultPrevented).toBe(false)
    expect(unsupported.dropEffect).toBe('none'); expect(overlay()).toBeNull()
    expect((await drag('drop', unsupported)).defaultPrevented).toBe(false)
    expect(forwardedDrop).not.toHaveBeenCalled()

    const accepted = transfer()
    expect((await drag('dragenter', accepted)).defaultPrevented).toBe(true)
    expect(overlay()?.textContent).toBe('Drop files here')
    expect(overlay()?.parentElement).toBe(document.body)
    expect(overlay()?.querySelector('button,a,input,[tabindex]')).toBeNull()
    expect((await drag('dragover', accepted)).defaultPrevented).toBe(true)
    expect(accepted.dropEffect).toBe('copy')
  })

  it('balances nested enters and leaves, and clears immediately on viewport exit', async () => {
    const accepted = transfer(), child = card().querySelector('[data-card-child]')!
    await drag('dragenter', accepted, document.body)
    await drag('dragenter', accepted, child)
    await drag('dragleave', accepted, child)
    expect(overlay()).not.toBeNull()
    await drag('dragleave', accepted, document.body)
    expect(overlay()).toBeNull()

    await drag('dragenter', accepted, document.body)
    await drag('dragenter', accepted, child)
    await drag('dragleave', accepted, document.body, { clientX: 0, clientY: 50 })
    expect(overlay()).toBeNull()
  })

  it('forwards an accepted outside drop once and leaves the resident card intake alone', async () => {
    const accepted = transfer()
    await drag('dragenter', accepted)
    const event = await drag('drop', accepted)
    expect(event.defaultPrevented).toBe(true)
    expect(forwardedDrop).toHaveBeenCalledExactlyOnceWith(event)
    expect(cardDrop).not.toHaveBeenCalled(); expect(overlay()).toBeNull()

    await drag('dragenter', accepted, card())
    await drag('drop', accepted, card().querySelector('[data-card-child]')!)
    expect(cardDrop).toHaveBeenCalledOnce()
    expect(forwardedDrop).toHaveBeenCalledOnce(); expect(overlay()).toBeNull()
  })

  it('preserves another outside drop owner that already consumed the event', async () => {
    const accepted = transfer()
    outside.addEventListener('drop', event => event.preventDefault(), { once: true })
    await drag('dragenter', accepted)
    await drag('drop', accepted)
    expect(forwardedDrop).not.toHaveBeenCalled(); expect(overlay()).toBeNull()
  })

  it('retires the old drag on owner changes, including returning to the same owner', async () => {
    const accepted = transfer()
    await drag('dragenter', accepted)
    await draw({ ownerKey: 'workspace-b/thread-b' })
    expect(overlay()).toBeNull()
    await draw()
    expect(overlay()).toBeNull()
    expect((await drag('drop', accepted)).defaultPrevented).toBe(false)
    expect(forwardedDrop).not.toHaveBeenCalled()
    await drag('dragenter', accepted)
    await drag('drop', accepted)
    expect(forwardedDrop).toHaveBeenCalledOnce()
  })

  it.each(['disabled', 'gate', 'Escape', 'dragend'] as const)('retires activity on %s without inventing acceptance', async reason => {
    const accepted = transfer()
    await drag('dragenter', accepted)
    if (reason === 'disabled') await draw({ enabled: false })
    else if (reason === 'gate') {
      await draw({ canAccept: () => false })
      expect((await drag('dragover', accepted)).defaultPrevented).toBe(false)
      await draw()
    } else if (reason === 'Escape') {
      const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
      await act(async () => { document.dispatchEvent(escape) })
      expect(escape.defaultPrevented).toBe(false)
    } else await act(async () => { window.dispatchEvent(new Event('dragend')) })
    expect(overlay()).toBeNull()
    expect((await drag('drop', accepted)).defaultPrevented).toBe(false)
    expect(forwardedDrop).not.toHaveBeenCalled()
  })

  it('cleans up all document handling when unmounted', async () => {
    const accepted = transfer()
    await drag('dragenter', accepted)
    await act(async () => root.render(null))
    expect(overlay()).toBeNull()
    expect((await drag('dragenter', accepted)).defaultPrevented).toBe(false)
    expect((await drag('dragover', accepted)).defaultPrevented).toBe(false)
    expect((await drag('drop', accepted)).defaultPrevented).toBe(false)
    expect(forwardedDrop).not.toHaveBeenCalled()
  })
})
