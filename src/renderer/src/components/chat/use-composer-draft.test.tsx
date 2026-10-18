// @vitest-environment jsdom
import { act, createElement, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useInputComposition } from './use-composer-draft'

let root: Root, host: HTMLDivElement, blocked: boolean[]
function CompositionEditor() {
  const composition = useInputComposition()
  return createElement('textarea', {
    defaultValue: 'Draft remains',
    onCompositionStart: composition.onCompositionStart,
    onCompositionEnd: composition.onCompositionEnd,
    onKeyDown: (event: ReactKeyboardEvent<HTMLTextAreaElement>) => blocked.push(composition.isComposingEvent(event))
  })
}
const editor = () => host.querySelector('textarea')!
async function composition(type: 'compositionstart' | 'compositionend', data = '') {
  await act(async () => { editor().dispatchEvent(new CompositionEvent(type, { bubbles: true, data })) })
}
async function enter(options: KeyboardEventInit = {}) {
  await act(async () => { editor().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true, ...options })) })
  return blocked.at(-1)
}
beforeEach(async () => {
  vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-17T00:00:00Z'))
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  blocked = []; host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  await act(async () => root.render(createElement(CompositionEditor)))
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove()
  vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('useInputComposition composition grace', () => {
  it('leaves ordinary Enter available before composition starts', async () => {
    expect(await enter()).toBe(false)
  })
  it('blocks Enter throughout active composition without native flags', async () => {
    await composition('compositionstart')
    vi.advanceTimersByTime(100)
    expect(await enter()).toBe(true)
  })
  it('blocks trailing Enter at 0 and 9ms, then releases it at 10ms', async () => {
    await composition('compositionstart'); await composition('compositionend', 'Selected text')
    expect(await enter()).toBe(true)
    vi.advanceTimersByTime(9); expect(await enter()).toBe(true)
    vi.advanceTimersByTime(1); expect(await enter()).toBe(false)
  })
  it('keeps a restarted composition active beyond the previous deadline', async () => {
    await composition('compositionstart'); await composition('compositionend')
    vi.advanceTimersByTime(9); await composition('compositionstart')
    vi.advanceTimersByTime(1); expect(await enter()).toBe(true)
    await composition('compositionend')
    vi.advanceTimersByTime(9); expect(await enter()).toBe(true)
    vi.advanceTimersByTime(1); expect(await enter()).toBe(false)
  })
  it('retains grace after an empty cancelled composition without changing the draft', async () => {
    await composition('compositionstart'); await composition('compositionend', '')
    expect(editor().value).toBe('Draft remains')
    expect(await enter()).toBe(true)
    vi.advanceTimersByTime(10); expect(await enter()).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })
  it('leaves no composition timer or grace behind after unmount and remount', async () => {
    await composition('compositionstart'); await composition('compositionend')
    vi.advanceTimersByTime(9)
    expect(vi.getTimerCount()).toBe(0)
    await act(async () => root.render(null))
    expect(vi.getTimerCount()).toBe(0)
    await act(async () => root.render(createElement(CompositionEditor)))
    expect(await enter()).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })
  it.each([{ isComposing: true }, { keyCode: 229 }])('retains native composition signals: %j', async options => {
    expect(await enter(options)).toBe(true)
  })
})
