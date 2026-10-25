// @vitest-environment jsdom
import { act, createElement, StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Dialog } from './Dialog'

let host: HTMLDivElement
let root: Root
let opener: HTMLButtonElement
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  host = document.createElement('div')
  opener = document.createElement('button')
  opener.textContent = 'Import log'
  document.body.append(opener, host)
  root = createRoot(host)
  opener.focus()
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove()
  opener.remove()
  vi.unstubAllGlobals()
})
const key = async (target: HTMLElement, value: string, shiftKey = false) => {
  const event = new KeyboardEvent('keydown', { key: value, shiftKey, bubbles: true, cancelable: true })
  await act(async () => target.dispatchEvent(event))
  return event
}
const render = (open = true) => act(async () => root.render(createElement(Dialog, {
  open, title: 'Import log', onClose: vi.fn(),
  children: createElement('div', null,
    createElement('input', { 'data-modal-autofocus': true, 'aria-label': 'Find log' }),
    createElement('button', null, 'Last action'))
})))

describe('Data import shared Dialog focus lifecycle', () => {
  it('enters the actual dialog and wraps Tab in both directions', async () => {
    await render()
    const first = host.querySelector<HTMLButtonElement>('button')!
    const input = host.querySelector<HTMLInputElement>('input')!
    const last = [...host.querySelectorAll<HTMLButtonElement>('button')].at(-1)!
    expect(document.activeElement).toBe(input)
    last.focus()
    expect((await key(last, 'Tab')).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(first)
    expect((await key(first, 'Tab', true)).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)
    opener.focus()
    expect(host.querySelector('[role="dialog"]')!.contains(document.activeElement)).toBe(true)
  })
  it('restores the trigger when open becomes false, including a repeated opening', async () => {
    await render()
    await render(false)
    expect(document.activeElement).toBe(opener)
    await render()
    expect(host.querySelector('[role="dialog"]')!.contains(document.activeElement)).toBe(true)
    await render(false)
    expect(document.activeElement).toBe(opener)
  })
  it('focuses the dialog fallback when loading leaves no enabled field and tolerates a removed trigger', async () => {
    await act(async () => root.render(createElement(Dialog, {
      open: true, title: 'Busy import', showCloseButton: false, onClose: vi.fn(),
      children: createElement('button', { disabled: true }, 'Importing')
    })))
    const dialog = host.querySelector<HTMLElement>('[role="dialog"]')!
    expect(document.activeElement).toBe(dialog)
    expect((await key(dialog, 'Tab')).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(dialog)
    opener.remove()
    await act(async () => root.render(null))
    expect(document.activeElement?.isConnected).toBe(true)
  })
  it('gives only the top dialog Escape and returns focus to the parent before the original trigger', async () => {
    const outerClose = vi.fn(), innerClose = vi.fn()
    const nested = (inner: boolean) => act(async () => root.render(createElement(Dialog, {
      open: true, title: 'Import ledger', onClose: outerClose,
      children: createElement('div', null,
        createElement('button', null, 'Open export'),
        inner ? createElement(Dialog, { open: true, title: 'Export log', onClose: innerClose,
          children: createElement('input', { 'aria-label': 'Export field' }) }) : null)
    })))
    await nested(false)
    const trigger = [...host.querySelectorAll<HTMLButtonElement>('button')].find(b => b.textContent === 'Open export')!
    trigger.focus()
    await nested(true)
    const top = host.querySelector<HTMLInputElement>('input')!
    top.focus()
    await key(top, 'Escape')
    expect(innerClose).toHaveBeenCalledTimes(1)
    expect(outerClose).not.toHaveBeenCalled()
    await nested(false)
    expect(document.activeElement).toBe(trigger)
    await act(async () => root.render(null))
    expect(document.activeElement).toBe(opener)
  })
})


it('restores the latest opener and original scroll lock across StrictMode reopen', async () => {
  const previousOverflow = document.body.style.overflow
  document.body.style.overflow = 'scroll'
  const another = document.createElement('button'); document.body.append(another)
  const show = (open: boolean) => act(async () => root.render(createElement(StrictMode, null,
    createElement(Dialog, { open, title: 'Import', onClose: vi.fn(), children: createElement('input') }))))
  try {
    await show(true); expect(document.body.style.overflow).toBe('hidden')
    await show(false); expect(document.activeElement).toBe(opener)
    expect(document.body.style.overflow).toBe('scroll')
    another.focus(); await show(true); await show(false)
    expect(document.activeElement).toBe(another); expect(document.body.style.overflow).toBe('scroll')
  } finally { another.remove(); document.body.style.overflow = previousOverflow }
})

it('does not close the Data dialog on legacy IME keyCode 229', async () => {
  const close = vi.fn()
  await act(async () => root.render(createElement(Dialog, {
    open: true, title: 'Import', onClose: close, children: createElement('input')
  })))
  const event = new KeyboardEvent('keydown', { key: 'Escape', keyCode: 229, bubbles: true, cancelable: true })
  await act(async () => document.activeElement!.dispatchEvent(event))
  expect(event.defaultPrevented).toBe(false); expect(close).not.toHaveBeenCalled()
})
