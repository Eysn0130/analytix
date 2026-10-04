// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { WorkbenchResizeHandle } from './WorkbenchResizeHandle'

afterEach(() => vi.unstubAllGlobals())
describe('WorkbenchResizeHandle keyboard boundary', () => {
  it('exposes current sizing and sends one directional command to the owning fitter', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const host = document.createElement('div'); document.body.append(host)
    const root = createRoot(host), onResizeDelta = vi.fn(), onReset = vi.fn()
    try {
      for (const [edge, key] of [['right','ArrowRight'],['left','ArrowLeft'],['top','ArrowUp']] as const) {
        await act(async () => root.render(createElement(WorkbenchResizeHandle, {
          edge, onPointerDown: vi.fn(), ariaLabel: 'Resize panel', value: 320, min: 180, max: 520,
          onResizeDelta, onReset
        })))
        const separator = host.querySelector<HTMLElement>('[role=separator]')!
        separator.focus()
        expect(document.activeElement).toBe(separator)
        expect(separator.getAttribute('aria-valuenow')).toBe('320')
        expect(separator.getAttribute('aria-label')).toBe('Resize panel')
        await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true})))
      }
      expect(onResizeDelta.mock.calls).toEqual([[24],[24],[24]])
      const separator = host.querySelector<HTMLElement>('[role=separator]')!
      await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true})))
      expect(onReset).toHaveBeenCalledExactlyOnceWith()
      await act(async () => root.render(createElement(WorkbenchResizeHandle, {
        disabled: true, onPointerDown: vi.fn(), onResizeDelta, onReset
      })))
      await act(async () => host.querySelector<HTMLElement>('[role=separator]')!.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})))
      expect(onResizeDelta).toHaveBeenCalledTimes(3)
      expect(host.querySelector('[role=separator]')!.getAttribute('tabindex')).toBeNull()
    } finally { await act(async () => root.unmount()); host.remove() }
  })
})
