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
