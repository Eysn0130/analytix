// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { Download, Save } from '../../design/AnalytixUiIcons'
import i18n from '../../i18n'
import { WorkspaceViewerHeader, WorkspaceViewerIconButton, WorkspaceViewerMenu } from './WorkspaceViewerHeader'

let root: Root, host: HTMLDivElement
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  await i18n.changeLanguage('en')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() })

it('keeps actions and optional workspace controls in the existing single header and forwards the current callbacks', async () => {
  const download = vi.fn(), focus = vi.fn(), collapse = vi.fn()
  await act(async () => root.render(createElement(WorkspaceViewerHeader, {
    title: 'A very long retained file name.xlsx', status: 'Result pending', statusRole: 'alert',
    viewerControls: { focused: false, onToggleFocus: focus, onCollapse: collapse },
    actions: createElement(WorkspaceViewerIconButton, { label: 'Download', onClick: download, children: createElement(Download) })
  })))
  expect(host.querySelectorAll('[data-workspace-viewer-header]')).toHaveLength(1)
  expect(host.querySelector('header')?.textContent).toContain('A very long retained file name.xlsx')
  expect(host.querySelector('[role="alert"]')?.textContent).toBe('Result pending')
  const buttons = [...host.querySelectorAll<HTMLButtonElement>('button')]
  expect(buttons).toHaveLength(3)
  expect(buttons.every(button => button.querySelector('svg') && button.textContent === '')).toBe(true)
  await act(async () => { for (const button of buttons) button.click() })
  expect(download).toHaveBeenCalledTimes(1); expect(focus).toHaveBeenCalledTimes(1); expect(collapse).toHaveBeenCalledTimes(1)
  expect(host.querySelector('#workbench-workspace-collapse')?.getAttribute('aria-controls')).toBe('workbench-right-workspace')
})

it('keeps disabled commands inert and exposes the icon label on keyboard focus', async () => {
  const save = vi.fn()
  const draw = async (disabled: boolean) => act(async () => root.render(createElement(WorkspaceViewerHeader, { title: 'Draft.docx',
    actions: createElement(WorkspaceViewerIconButton, { label: 'Save changes', tone: 'commit', disabled, onClick: save, children: createElement(Save) }) })))
  await draw(true)
  await act(async () => host.querySelector('button')!.click())
  expect(save).not.toHaveBeenCalled()
  await draw(false)
  await act(async () => host.querySelector<HTMLButtonElement>('button')!.focus())
  expect(document.querySelector('[role="tooltip"]')?.textContent).toBe('Save changes')
  await act(async () => host.querySelector('button')!.click())
  expect(save).toHaveBeenCalledTimes(1)
  expect(host.querySelector('#workbench-workspace-collapse')).toBeNull()
})

it('provides keyboard menu navigation and Escape focus without clearing an enclosing editor selection', async () => {
  const outerKey = vi.fn(), run = vi.fn()
  await act(async () => root.render(createElement('div', { onKeyDown: outerKey }, createElement(WorkspaceViewerMenu, {
    label: 'File actions', title: 'Report.docx', ownerKey: 'document-a',
    children: [createElement('button', { key: 'disabled', disabled: true, role: 'menuitem' }, 'Disabled'),
      createElement('button', { key: 'open', role: 'menuitem', onClick: run }, 'Open'),
      createElement('button', { key: 'download', role: 'menuitem' }, 'Download')]
  }))))
  const trigger = host.querySelector<HTMLButtonElement>('button')!
  await act(async () => { trigger.focus(); trigger.click() })
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!
  expect(document.activeElement?.textContent).toBe('Open')
  await act(async () => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true })))
  expect(document.activeElement?.textContent).toBe('Download')
  outerKey.mockClear()
  await act(async () => menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(document.activeElement).toBe(trigger)
  expect(outerKey).not.toHaveBeenCalled()
  expect(run).not.toHaveBeenCalled()
})

it('retires an open menu on document identity or visibility change', async () => {
  const draw = async (ownerKey: string, enabled = true) => act(async () => root.render(createElement(WorkspaceViewerMenu, {
    ownerKey, enabled, label: 'More', children: createElement('button', { role: 'menuitem' }, ownerKey)
  })))
  await draw('a')
  await act(async () => host.querySelector('button')!.click())
  expect(document.querySelector('[role="menu"]')?.textContent).toBe('a')
  await draw('b')
  expect(document.querySelector('[role="menu"]')).toBeNull()
  await act(async () => host.querySelector('button')!.click())
  await draw('b', false)
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(host.querySelector('button')!.disabled).toBe(true)
})

it.each(['select', 'input', 'textarea'] as const)('focuses a native %s and preserves its own navigation keys', async tag => {
  const control = tag === 'select' ? createElement('select', { key: 'native', 'aria-label': 'Value type', defaultValue: 'text' },
    createElement('option', { value: 'text' }, 'Text'), createElement('option', { value: 'number' }, 'Number'))
    : createElement(tag, { key: 'native', 'aria-label': 'Value type', defaultValue: 'Retained value' })
  await act(async () => root.render(createElement(WorkspaceViewerMenu, { label: 'Editing options', ownerKey: 'same-cell',
    children: [control, createElement('button', { key: 'disabled', disabled: true, role: 'menuitem' }, 'Discard')] })))
  const trigger = host.querySelector<HTMLButtonElement>('button')!
  await act(async () => { trigger.focus(); trigger.click() })
  const native = document.querySelector<HTMLElement>(`${tag}[aria-label="Value type"]`)!
  expect(document.activeElement).toBe(native)
  for (const key of ['ArrowDown', 'ArrowUp', 'Home', 'End']) {
    const event = new KeyboardEvent('keydown', { key, altKey: key === 'ArrowDown', bubbles: true, cancelable: true })
    await act(async () => native.dispatchEvent(event))
    expect(event.defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(native)
  }
  await act(async () => native.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(document.activeElement).toBe(trigger)
})
