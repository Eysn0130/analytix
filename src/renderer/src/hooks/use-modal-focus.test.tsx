// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { useModalFocus } from './use-modal-focus'

it('retains top-layer input focus when a covered sibling dialog unmounts and skips negative tabIndex', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const opener = document.createElement('button'); document.body.append(opener); opener.focus()
  const close = vi.fn()
  function Dialog({ name }: { name: string }) {
    const ref = useModalFocus(close)
    return createElement('div', { ref, role: 'dialog', tabIndex: -1, 'aria-label': name },
      createElement('input', { 'aria-label': name + ' input' }),
      createElement('button', null, name + ' last'),
      createElement('button', { tabIndex: -1 }, name + ' programmatic'))
  }
  const render = (covered: boolean) => act(async () => root.render(createElement('div', null,
    covered ? createElement(Dialog, { key: 'covered', name: 'Covered' }) : null,
    createElement(Dialog, { key: 'top', name: 'Top' }))))
  try {
    await render(true)
    const input = host.querySelector<HTMLInputElement>('[aria-label="Top input"]')!
    input.focus(); await render(false); expect(document.activeElement).toBe(input)
    const last = [...host.querySelectorAll<HTMLButtonElement>('button')].find(b => b.textContent === 'Top last')!
    last.focus()
    await act(async () => last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })))
    expect(document.activeElement).toBe(input); expect(close).not.toHaveBeenCalled()
    await act(async () => root.render(null))
    expect(document.activeElement).toBe(opener)
  } finally { await act(async () => root.unmount()); host.remove(); opener.remove() }
})

it.each([true, false])('restores the original overflow after nested scroll locks close, covered-first=%s', async coveredFirst => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const previous = document.body.style.overflow; document.body.style.overflow = 'clip'
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  function Dialog({ name }: { name: string }) {
    const ref = useModalFocus(vi.fn(), true, true)
    return createElement('div', { ref, role: 'dialog', tabIndex: -1 }, createElement('button', null, name))
  }
  const render = (covered: boolean, top: boolean) => act(async () => root.render(createElement('div', null,
    covered ? createElement(Dialog, { key: 'covered', name: 'Covered' }) : null,
    top ? createElement(Dialog, { key: 'top', name: 'Top' }) : null)))
  try {
    await render(true, true); expect(document.body.style.overflow).toBe('hidden')
    await render(!coveredFirst, coveredFirst); expect(document.body.style.overflow).toBe('hidden')
    await render(false, false); expect(document.body.style.overflow).toBe('clip')
  } finally { await act(async () => root.unmount()); host.remove(); document.body.style.overflow = previous; vi.unstubAllGlobals() }
})

it('does not raise a covered modal when its explicit opener prop changes', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const previous = document.body.style.overflow
  const host = document.createElement('div'), first = document.createElement('button'), second = document.createElement('button')
  document.body.append(host, first, second); const root = createRoot(host)
  function Dialog({ name, opener }: { name: string; opener?: HTMLButtonElement }) {
    const ref = useModalFocus(vi.fn(), true, true, opener)
    return createElement('div', { ref, role: 'dialog', tabIndex: -1 }, createElement('input', { 'aria-label': name }))
  }
  const render = (opener: HTMLButtonElement) => act(async () => root.render(createElement('div', null,
    createElement(Dialog, { key: 'covered', name: 'Covered', opener }), createElement(Dialog, { key: 'top', name: 'Top' }))))
  try {
    await render(first); const input = host.querySelector<HTMLInputElement>('[aria-label="Top"]')!; input.focus()
    await render(second); expect(document.activeElement).toBe(input); expect(document.body.style.overflow).toBe('hidden')
  } finally { await act(async () => root.unmount()); host.remove(); first.remove(); second.remove(); document.body.style.overflow = previous; vi.unstubAllGlobals() }
})
