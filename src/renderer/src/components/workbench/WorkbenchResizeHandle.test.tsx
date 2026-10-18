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
      for (const [edge, key, opposite] of [
        ['right', 'ArrowRight', 'ArrowLeft'], ['left', 'ArrowLeft', 'ArrowRight'],
        ['top', 'ArrowUp', 'ArrowDown'], ['bottom', 'ArrowDown', 'ArrowUp']
      ] as const) {
        await act(async () => root.render(createElement(WorkbenchResizeHandle, {
          edge, onPointerDown: vi.fn(), ariaLabel: 'Resize panel', value: 320, min: 180, max: 520,
          onResizeDelta, onReset
        })))
        const separator = host.querySelector<HTMLElement>('[role=separator]')!
        separator.focus()
        expect(document.activeElement).toBe(separator)
        expect(separator.getAttribute('aria-valuenow')).toBe('320')
        expect(separator.getAttribute('aria-label')).toBe('Resize panel')
        expect(separator.getAttribute('aria-orientation')).toBe(edge === 'left' || edge === 'right' ? 'vertical' : 'horizontal')
        await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true})))
        await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown',{key:opposite,bubbles:true,cancelable:true})))
        await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown',{key,shiftKey:true,bubbles:true,cancelable:true})))
      }
      expect(onResizeDelta.mock.calls).toEqual(Array.from({ length: 4 }, () => [[10], [-10], [50]]).flat())
      const separator = host.querySelector<HTMLElement>('[role=separator]')!
      await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true})))
      expect(onReset).toHaveBeenCalledExactlyOnceWith()
      await act(async () => root.render(createElement(WorkbenchResizeHandle, {
        disabled: true, onPointerDown: vi.fn(), onResizeDelta, onReset
      })))
      await act(async () => host.querySelector<HTMLElement>('[role=separator]')!.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})))
      expect(onResizeDelta).toHaveBeenCalledTimes(12)
      expect(host.querySelector('[role=separator]')!.getAttribute('tabindex')).toBeNull()
      expect(host.querySelector('[role=separator]')!.getAttribute('aria-disabled')).toBe('true')
    } finally { await act(async () => root.unmount()); host.remove() }
  })

  it('sends Home and End deltas using the current dynamic bounds', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const host = document.createElement('div'); document.body.append(host)
    const root = createRoot(host), onResizeDelta = vi.fn()
    try {
      for (const sizing of [{ value: 640, min: 320, max: 776 }, { value: 400, min: 240, max: 480 }]) {
        await act(async () => root.render(createElement(WorkbenchResizeHandle, {
          ...sizing, onPointerDown: vi.fn(), onResizeDelta
        })))
        const separator = host.querySelector<HTMLElement>('[role=separator]')!
        for (const key of ['Home', 'End']) {
          const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
          await act(async () => separator.dispatchEvent(event))
          expect(event.defaultPrevented).toBe(true)
        }
      }
      expect(onResizeDelta.mock.calls).toEqual([[-320], [136], [-160], [80]])
    } finally { await act(async () => root.unmount()); host.remove() }
  })

  it('ignores Home and End without finite ordered bounds and a finite delta', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const host = document.createElement('div'); document.body.append(host)
    const root = createRoot(host), onResizeDelta = vi.fn()
    try {
      for (const sizing of [
        {}, { min: 180, max: 520 }, { value: 320, max: 520 }, { value: 320, min: 180 },
        { value: NaN, min: 180, max: 520 }, { value: 320, min: -Infinity, max: 520 },
        { value: 320, min: 180, max: Infinity }, { value: 320, min: 520, max: 180 },
        { value: -Number.MAX_VALUE, min: Number.MAX_VALUE, max: Number.MAX_VALUE }
      ]) {
        await act(async () => root.render(createElement(WorkbenchResizeHandle, {
          ...sizing, onPointerDown: vi.fn(), onResizeDelta
        })))
        const separator = host.querySelector<HTMLElement>('[role=separator]')!
        for (const key of ['Home', 'End']) {
          const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
          await act(async () => separator.dispatchEvent(event))
          expect(event.defaultPrevented).toBe(false)
        }
      }
      expect(onResizeDelta).not.toHaveBeenCalled()
    } finally { await act(async () => root.unmount()); host.remove() }
  })

  it('keeps modified and disabled endpoint keys outside the resize boundary', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const host = document.createElement('div'); document.body.append(host)
    const root = createRoot(host), onResizeDelta = vi.fn(), onReset = vi.fn()
    try {
      await act(async () => root.render(createElement(WorkbenchResizeHandle, {
        value: 320, min: 180, max: 520, onPointerDown: vi.fn(), onResizeDelta, onReset
      })))
      const separator = host.querySelector<HTMLElement>('[role=separator]')!
      for (const modifier of [{ altKey: true }, { ctrlKey: true }, { metaKey: true }]) {
        for (const key of ['Home', 'End', 'ArrowRight', 'Enter']) {
          await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown', {
            key, ...modifier, bubbles: true, cancelable: true
          })))
        }
      }
      await act(async () => root.render(createElement(WorkbenchResizeHandle, {
        disabled: true, value: 320, min: 180, max: 520, onPointerDown: vi.fn(), onResizeDelta, onReset
      })))
      for (const key of ['Home', 'End', 'Enter']) {
        await act(async () => host.querySelector<HTMLElement>('[role=separator]')!.dispatchEvent(new KeyboardEvent('keydown', {
          key, bubbles: true, cancelable: true
        })))
      }
      expect(onResizeDelta).not.toHaveBeenCalled()
      expect(onReset).not.toHaveBeenCalled()
    } finally { await act(async () => root.unmount()); host.remove() }
  })
})
