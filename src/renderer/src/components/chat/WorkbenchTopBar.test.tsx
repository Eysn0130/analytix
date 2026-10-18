// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { WorkbenchTopBar } from './WorkbenchTopBar'

let root: Root
let container: HTMLDivElement

describe('WorkbenchTopBar workspace entry ownership', () => {
  beforeEach(async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    await i18n.changeLanguage('en')
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: false })
  })
  it('reopens a Schedule workspace without offering an unmounted terminal', async () => {
    const reopen = vi.fn()
    await act(async () => root.render(createElement(WorkbenchTopBar, { workspaceOpen: false, terminalOpen: false,
      terminalAvailable: false, onToggleWorkspace: reopen, onToggleTerminal: vi.fn() })))
    expect(container.querySelector('#workbench-terminal-toggle')).toBeNull()
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-controls="workbench-right-workspace"]')?.click())
    expect(reopen).toHaveBeenCalledTimes(1)
  })
  it('offers one reopen callback when closed and leaves collapse to the dock when open', async () => {
    const onToggleWorkspace = vi.fn()
    const onToggleTerminal = vi.fn()
    const props = { workspaceOpen: false, terminalOpen: false, onToggleWorkspace, onToggleTerminal }
    await act(async () => root.render(createElement(WorkbenchTopBar, props)))
    const reopen = container.querySelector<HTMLButtonElement>('[aria-controls="workbench-right-workspace"]')
    expect(reopen?.getAttribute('aria-expanded')).toBe('false')
    expect(reopen?.getAttribute('aria-label')).toBe(i18n.t('common:workbenchOpen'))
    expect([...container.querySelectorAll('button')].at(-1)).toBe(reopen)
    expect(reopen?.querySelector('g')?.getAttribute('transform')).toBe('translate(16 0) scale(-1 1)')
    expect(reopen?.getAttribute('aria-keyshortcuts')).toBe('Control+Shift+B Meta+Shift+B')
    await act(async () => reopen?.click())
    expect(onToggleWorkspace).toHaveBeenCalledTimes(1)
    await act(async () => root.render(createElement(WorkbenchTopBar, { ...props, workspaceOpen: true })))
    expect(container.querySelector('[aria-controls="workbench-right-workspace"]')).toBeNull()
    const terminal = container.querySelector<HTMLButtonElement>('button')
    expect(terminal?.getAttribute('aria-label')).toBe(i18n.t('common:workbenchOpenTerminal'))
    await act(async () => terminal?.click())
    expect(onToggleTerminal).toHaveBeenCalledTimes(1)
    expect(onToggleWorkspace).toHaveBeenCalledTimes(1)
  })
  it('offers a chat summary entry independently of an open workspace', async () => {
    const summary = vi.fn(), workspace = vi.fn()
    const props = { workspaceOpen: true, terminalOpen: false, terminalAvailable: false,
      onToggleWorkspace: workspace, onToggleTerminal: vi.fn(), summaryAvailable: true, summaryOpen: false, onToggleSummary: summary }
    await act(async () => root.render(createElement(WorkbenchTopBar, props)))
    const trigger = container.querySelector<HTMLButtonElement>('#workbench-summary-toggle')!
    expect(trigger.getAttribute('aria-controls')).toBe('workbench-thread-summary')
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    await act(async () => trigger.click())
    expect(summary).toHaveBeenCalledTimes(1); expect(workspace).not.toHaveBeenCalled()
    await act(async () => root.render(createElement(WorkbenchTopBar, { ...props, summaryOpen: true })))
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
  })
})
