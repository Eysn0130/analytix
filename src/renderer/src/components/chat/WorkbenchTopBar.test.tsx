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
  it('offers one reopen callback when closed and leaves collapse to the dock when open', async () => {
    const onToggleWorkspace = vi.fn()
    const onToggleTerminal = vi.fn()
    const props = { workspaceOpen: false, terminalOpen: false, onToggleWorkspace, onToggleTerminal }
    await act(async () => root.render(createElement(WorkbenchTopBar, props)))
    const reopen = container.querySelector<HTMLButtonElement>('[aria-controls="workbench-right-workspace"]')
    expect(reopen?.getAttribute('aria-expanded')).toBe('false')
    expect(reopen?.getAttribute('aria-label')).toBe(i18n.t('common:workbenchOpen'))
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
})
