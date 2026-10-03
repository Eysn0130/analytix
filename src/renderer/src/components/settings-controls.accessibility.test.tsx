// @vitest-environment jsdom
import { act, createElement, createRef } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { SettingRow, Toggle } from './settings-controls'

describe('setting field names and descriptions', () => {
  it('associates native selects and shared switches without changing their events', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host)
    const toggle = vi.fn(), select = vi.fn()
    try {
      await act(async () => root.render(createElement('div', null,
        createElement(SettingRow, { title: 'Motion', description: 'Control interface animation', control: createElement(Toggle, { checked: false, onChange: toggle }) }),
        createElement(SettingRow, { title: 'Language', description: 'Choose interface language', control: createElement('select', { value: 'en', onChange: select }, createElement('option', { value: 'en' }, 'English'), createElement('option', { value: 'zh' }, 'Chinese')) })
      )))
      const fields = [...host.querySelectorAll<HTMLElement>('button[role="switch"],select')]
      expect(fields).toHaveLength(2)
      for (const [index, field] of fields.entries()) {
        const label = document.getElementById(field.getAttribute('aria-labelledby') ?? '')
        const description = document.getElementById(field.getAttribute('aria-describedby') ?? '')
        expect(label?.textContent).toBe(['Motion', 'Language'][index])
        expect(description?.textContent).toBe(['Control interface animation', 'Choose interface language'][index])
      }
      await act(async () => fields[0].click())
      expect(toggle).toHaveBeenCalledExactlyOnceWith(true)
      await act(async () => fields[1].dispatchEvent(new Event('change', { bubbles: true })))
      expect(select).toHaveBeenCalledTimes(1)
    } finally {
      await act(async () => root.unmount())
      host.remove()
    }
  })
  it('keeps explicit field names and disabled controls inside native wrappers', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host), change = vi.fn()
    try {
      await act(async () => root.render(createElement(SettingRow, {
        title: 'Provider options', description: 'Existing explanation',
        control: createElement('div', null, createElement(Toggle, { checked: true, disabled: true, onChange: change, 'aria-label': 'Existing explicit switch name' }))
      })))
      const field = host.querySelector<HTMLButtonElement>('button')!
      expect(field.getAttribute('aria-label')).toBe('Existing explicit switch name')
      expect(field.hasAttribute('aria-labelledby')).toBe(false)
      expect(document.getElementById(field.getAttribute('aria-describedby')!)?.textContent).toBe('Existing explanation')
      expect(field.getAttribute('aria-checked')).toBe('true')
      await act(async () => field.click())
      expect(change).not.toHaveBeenCalled()
    } finally {
      await act(async () => root.unmount())
      host.remove()
    }
  })
  it('preserves implicit native labels, refs and change handlers within a multi-field row', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host), change = vi.fn(), ref = createRef<HTMLSelectElement>()
    try {
      await act(async () => root.render(createElement(SettingRow, {
        title: 'Search limits', description: 'Existing row explanation',
        control: createElement('div', null,
          createElement('label', null, 'Default limit', createElement('select', { ref, value: 'one', onChange: change }, createElement('option', { value: 'one' }, 'One'))),
          createElement('label', null, 'Enable proxy', createElement(Toggle, { checked: false, onChange: change }))
        )
      })))
      const fields = [...host.querySelectorAll<HTMLElement>('select,button[role="switch"]')]
      expect(fields).toHaveLength(2)
      for (const [index, field] of fields.entries()) {
        expect(field.hasAttribute('aria-labelledby')).toBe(false)
        expect(field.closest('label')?.textContent).toContain(['Default limit', 'Enable proxy'][index])
        expect(document.getElementById(field.getAttribute('aria-describedby')!)?.textContent).toBe('Existing row explanation')
      }
      expect(ref.current).toBe(fields[0])
      await act(async () => fields[0].dispatchEvent(new Event('change', { bubbles: true })))
      expect(change).toHaveBeenCalledTimes(1)
      await act(async () => fields[1].click())
      expect(change).toHaveBeenCalledTimes(2)
    } finally {
      await act(async () => root.unmount())
      host.remove()
    }
  })

})
