// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { analytixToolPermissionModeSettings } from '@shared/app-settings'
import { FloatingComposerExecutionPicker } from './FloatingComposerExecutionPicker'

let root: Root
let host: HTMLDivElement
let outside: HTMLButtonElement
let frames: Map<number, FrameRequestCallback>
let nextFrame: number
const onChange = vi.fn()
const onOpenPermissionSettings = vi.fn()
const value = analytixToolPermissionModeSettings('request-approval')
const trigger = () => host.querySelector<HTMLButtonElement>('[aria-haspopup="menu"]')!
const items = () => [...document.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')]
async function render(props: Partial<Parameters<typeof FloatingComposerExecutionPicker>[0]> = {}, scope = 'thread-a') {
  await act(async () => root.render(createElement(FloatingComposerExecutionPicker, {
    key: scope, value, onChange, onOpenPermissionSettings, ...props
  })))
}
async function key(target: HTMLElement, name: string, shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key: name, shiftKey, bubbles: true, cancelable: true })
  await act(async () => { target.dispatchEvent(event) })
  return event
}
async function flushFrames() {
  await act(async () => {
    const pending = [...frames.values()]; frames.clear()
    for (const callback of pending) callback(0)
  })
}
async function open() {
  await act(async () => { trigger().focus(); trigger().click() })
  await flushFrames()
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('zh')
  onChange.mockClear(); onOpenPermissionSettings.mockClear()
  frames = new Map(); nextFrame = 0
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frames.set(++nextFrame, callback); return nextFrame })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { frames.delete(id) })
  host = document.createElement('div'); outside = document.createElement('button')
  document.body.append(host, outside); root = createRoot(host)
  await render()
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); outside.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('execution permission menu keyboard lifecycle', () => {
  it('focuses the current item on opening without changing authority', async () => {
    await render({ value: analytixToolPermissionModeSettings('auto-approval') })
    await open()
    expect(document.activeElement).toBe(items()[1])
    expect(items()[1].getAttribute('aria-checked')).toBe('true')
    expect(onChange).not.toHaveBeenCalled()
    expect(onOpenPermissionSettings).not.toHaveBeenCalled()
  })
  it('closes on Escape and restores focus to the current trigger', async () => {
    await open(); await key(items()[0], 'Escape')
    expect(items()).toHaveLength(0)
    expect(trigger().getAttribute('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger())
    expect(onChange).not.toHaveBeenCalled()
  })
  it('moves menu focus with arrows and boundaries without activating a permission', async () => {
    await open(); await key(items()[0], 'ArrowUp')
    expect(document.activeElement).toBe(items()[3])
    await key(items()[3], 'Home'); expect(document.activeElement).toBe(items()[0])
    await key(items()[0], 'End'); expect(document.activeElement).toBe(items()[3])
    await key(items()[3], 'ArrowUp'); expect(document.activeElement).toBe(items()[2])
    expect(onChange).not.toHaveBeenCalled()
    expect(items()[0].getAttribute('aria-checked')).toBe('true')
  })
  it.each(['ArrowDown', 'ArrowUp'])('opens from the trigger using %s', async name => {
    await key(trigger(), name); await flushFrames()
    expect(document.activeElement).toBe(name === 'ArrowDown' ? items()[0] : items()[3])
    expect(onChange).not.toHaveBeenCalled()
  })
  it('preserves explicit activation and returns focus after a preset selection', async () => {
    await open()
    await act(async () => items()[1].click())
    expect(onChange).toHaveBeenCalledExactlyOnceWith(analytixToolPermissionModeSettings('auto-approval'))
    expect(items()).toHaveLength(0); expect(document.activeElement).toBe(trigger())
  })
  it('keeps custom settings as its existing callback without a policy patch', async () => {
    await open(); await act(async () => items()[3].click())
    expect(onOpenPermissionSettings).toHaveBeenCalledOnce()
    expect(onChange).not.toHaveBeenCalled(); expect(items()).toHaveLength(0)
  })
  it.each([false, true])('dismisses for Tab with shift=%s and leaves native focus navigation enabled', async shift => {
    await open(); const event = await key(items()[0], 'Tab', shift)
    expect(items()).toHaveLength(0); expect(document.activeElement).toBe(trigger())
    expect(event.defaultPrevented).toBe(false); expect(onChange).not.toHaveBeenCalled()
  })
  it('preserves menu focus through a same-thread settings update', async () => {
    await open(); await key(items()[0], 'ArrowDown'); await key(items()[1], 'ArrowDown')
    await render({ value: analytixToolPermissionModeSettings('auto-approval') }); await flushFrames()
    expect(document.activeElement).toBe(items()[2])
    expect(items()[1].getAttribute('aria-checked')).toBe('true')
    expect(onChange).not.toHaveBeenCalled()
  })
  it('dismisses on outside focus without returning to the trigger', async () => {
    await open(); await act(async () => outside.focus())
    expect(items()).toHaveLength(0); expect(document.activeElement).toBe(outside)
    expect(onChange).not.toHaveBeenCalled()
  })
  it('keeps outside pointer dismissal from stealing focus back', async () => {
    await open()
    await act(async () => { outside.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); outside.focus() })
    expect(items()).toHaveLength(0); expect(document.activeElement).toBe(outside)
    expect(onChange).not.toHaveBeenCalled()
  })
  it.each(['disabled', 'applying'] as const)('cleans up an open menu when %s becomes true and does not reopen', async prop => {
    await open(); await render({ [prop]: true })
    expect(items()).toHaveLength(0); expect(trigger().disabled).toBe(true)
    await render({ [prop]: false }); await flushFrames()
    expect(items()).toHaveLength(0); expect(trigger().getAttribute('aria-expanded')).toBe('false')
    expect(onChange).not.toHaveBeenCalled()
  })
  it('cancels pending entry focus when closed before its animation frame', async () => {
    await act(async () => { trigger().focus(); trigger().click() })
    await key(trigger(), 'Escape'); await flushFrames()
    expect(items()).toHaveLength(0); expect(document.activeElement).toBe(trigger())
    expect(frames.size).toBe(0); expect(onChange).not.toHaveBeenCalled()
  })
  it('drops the old portal and pending focus when its thread scope unmounts', async () => {
    await act(async () => trigger().click())
    await render({}, 'thread-b'); outside.focus(); await flushFrames()
    expect(items()).toHaveLength(0); expect(document.activeElement).toBe(outside)
    expect(frames.size).toBe(0); expect(onChange).not.toHaveBeenCalled()
  })
})
