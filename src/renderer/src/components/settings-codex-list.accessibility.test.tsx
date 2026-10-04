// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { CodexSelect, CodexSettingsRow, CodexToggle } from './settings-codex-list'

it('names selects and connects existing row help while preserving events and disabled switches', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  const host = document.createElement('div'); document.body.append(host)
  const root = createRoot(host), change = vi.fn(), toggle = vi.fn()
  try {
    await act(async () => root.render(createElement('div', null,
      createElement(CodexSettingsRow, { title: 'URL destination', description: 'Choose where local URLs open', control:
        createElement(CodexSelect, { label: 'URL destination', value: 'app', options: [{ value: 'app', label: 'App' }, { value: 'system', label: 'System' }], onChange: change }) }),
      createElement(CodexSettingsRow, { title: 'Browser control', description: 'Existing browser explanation', control:
        createElement(CodexToggle, { label: 'Browser control', checked: false, disabled: true, onChange: toggle }) })
    )))
    const select = host.querySelector('select')!, button = host.querySelector('button')!
    expect(select.getAttribute('aria-label')).toBe('URL destination')
    expect(document.getElementById(select.getAttribute('aria-describedby')!)?.textContent).toBe('Choose where local URLs open')
    expect(button.getAttribute('aria-label')).toBe('Browser control')
    expect(document.getElementById(button.getAttribute('aria-describedby')!)?.textContent).toBe('Existing browser explanation')
    await act(async () => { select.value = 'system'; select.dispatchEvent(new Event('change', { bubbles: true })); button.click() })
    expect(change).toHaveBeenCalledExactlyOnceWith('system')
    expect(toggle).not.toHaveBeenCalled()
  } finally { await act(async () => root.unmount()); host.remove() }
})
