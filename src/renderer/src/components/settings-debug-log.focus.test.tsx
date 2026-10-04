// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { WriteDebugLogModal } from './settings-debug-log'

it('focuses Close, traps the empty/loading modal and restores opener without firing log actions', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  const opener = document.createElement('button'); opener.textContent = 'View logs'; document.body.append(opener); opener.focus()
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const refresh = vi.fn(), clear = vi.fn(), close = vi.fn()
  const props = { completionEntries: [], completionSelectedId: null, loading: true, error: null, onSelectCompletion: vi.fn(), onRefresh: refresh, onClear: clear, onClose: close, t: (key: string) => key }
  try {
    await act(async () => root.render(createElement(WriteDebugLogModal, props)))
    const modal = host.querySelector('[role="dialog"]')!; expect(modal.getAttribute('aria-modal')).toBe('true')
    const button = [...host.querySelectorAll('button')].find(b => b.textContent === 'close')!
    expect(document.activeElement).toBe(button)
    await act(async () => button.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })))
    expect(document.activeElement).toBe(button)
    await act(async () => root.render(createElement(WriteDebugLogModal, { ...props, loading: false })))
    expect(document.activeElement).toBe(button)
    await act(async () => button.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    expect(close).toHaveBeenCalledTimes(1); expect(refresh).not.toHaveBeenCalled(); expect(clear).not.toHaveBeenCalled()
    await act(async () => root.unmount()); expect(document.activeElement).toBe(opener)
  } finally { if (host.childNodes.length) await act(async () => root.unmount()); host.remove(); opener.remove() }
})
