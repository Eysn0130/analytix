// @vitest-environment jsdom
import { act, createElement, createRef } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  FloatingComposerCommandMenu,
  composerCommandMenuMaxHeight,
  composerCommandOptionId,
  type FloatingComposerCommandMenuProps
} from './FloatingComposerCommandMenu'
import type { ComposerCommandRow, SlashCommandId } from './floating-composer-commands'

const groupLabels = { add: 'Add', commands: 'Commands', extensions: 'Extensions', skills: 'Skills', plugins: 'Plugins', worktree: 'Worktree' }
function row(id: SlashCommandId, overrides: Partial<ComposerCommandRow> = {}): ComposerCommandRow {
  return { id, title: id, description: '', keywords: [], icon: createElement('svg', { viewBox: '0 0 16 16' }), ...overrides }
}
let root: Root
let host: HTMLDivElement
const onSelect = vi.fn()
const onHover = vi.fn()
async function render(props: Partial<FloatingComposerCommandMenuProps> = {}) {
  await act(async () => root.render(createElement(FloatingComposerCommandMenu<ComposerCommandRow>, {
    id: 'commands', items: [], groupLabels, ariaLabel: 'Composer commands', emptyLabel: 'No commands', onSelect, onHover, ...props
  })))
}
const item = (id: SlashCommandId) => host.querySelector<HTMLButtonElement>(`[data-command-id="${id}"]`)!
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  onSelect.mockClear(); onHover.mockClear()
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('composer command menu available height', () => {
  it.each([
    [-20, 0], [0, 0], [16, 0], [17, 1], [220, 204], [416, 400], [800, 400], [Number.NaN, 0]
  ])('clamps the space above anchor top %s to %s', (top, expected) => {
    expect(composerCommandMenuMaxHeight(top)).toBe(expected)
  })

  it('measures its actual composer parent and follows anchor resize, viewport resize and captured scroll', async () => {
    let top = 220
    let notifyResize!: () => void
    const observe = vi.fn(), disconnect = vi.fn()
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { notifyResize = callback }
      observe = observe
      disconnect = disconnect
    })
    vi.spyOn(host, 'getBoundingClientRect').mockImplementation(() => new DOMRect(0, top, 500, 100))
    await render({ items: [row('plan')] })
    const menu = host.querySelector<HTMLElement>('[data-composer-command-menu]')!
    expect(menu.style.maxHeight).toBe('204px')
    expect(observe).toHaveBeenCalledExactlyOnceWith(host)
    top = 90
    await act(async () => notifyResize())
    expect(menu.style.maxHeight).toBe('74px')
    top = 430
    await act(async () => window.dispatchEvent(new Event('resize')))
    expect(menu.style.maxHeight).toBe('400px')
    top = 10
    await act(async () => host.dispatchEvent(new Event('scroll')))
    expect(menu.style.maxHeight).toBe('0px')
  })

  it('retires the observer and viewport listeners so detached callbacks cannot remeasure a former owner', async () => {
    let notifyResize!: () => void
    const disconnect = vi.fn()
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { notifyResize = callback }
      observe() {}
      disconnect = disconnect
    })
    const measure = vi.spyOn(host, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 220, 500, 100))
    const added = vi.spyOn(window, 'addEventListener')
    const removed = vi.spyOn(window, 'removeEventListener')
    await render()
    const scroll = added.mock.calls.find(([type, , capture]) => type === 'scroll' && capture === true)!
    const resize = added.mock.calls.find(([type]) => type === 'resize')!
    expect(scroll).toBeDefined(); expect(resize).toBeDefined()
    await act(async () => root.render(null))
    expect(disconnect).toHaveBeenCalledOnce()
    expect(removed).toHaveBeenCalledWith('scroll', scroll[1], true)
    expect(removed).toHaveBeenCalledWith('resize', resize[1])
    const priorMeasurements = measure.mock.calls.length
    await act(async () => {
      notifyResize()
      window.dispatchEvent(new Event('resize'))
      host.dispatchEvent(new Event('scroll'))
    })
    expect(measure).toHaveBeenCalledTimes(priorMeasurements)
  })
})

describe('shared composer command menu', () => {
  it('renders section headings, canonical aliases, descriptions and 14px glyphs from the supplied catalog', async () => {
    await render({ items: [
      row('plan', { title: 'Plan', section: 'add', description: 'Plan before acting.' }),
      row('compact', { title: 'Compact this thread', section: 'commands', description: 'Summarize context.' })
    ] })
    expect([...host.querySelectorAll('[data-command-section]')].map(node => node.textContent)).toEqual(['Add', 'Commands'])
    expect(item('plan').querySelector('.dsh-composer-command-alias')).toBeNull()
    expect(item('compact').querySelector('.dsh-composer-command-alias')?.textContent).toBe('compact')
    expect(item('compact').querySelector('.dsh-composer-command-description')?.textContent).toBe('Summarize context.')
    expect(item('compact').querySelector('svg')?.getAttribute('width')).toBe('14')
    expect(item('compact').querySelector('svg')?.getAttribute('height')).toBe('14')
    expect(host.querySelector('[role="menu"]')?.classList.contains('dsh-input-menu')).toBe(true)
  })

  it('preserves row identity through reordering and highlights only the supplied index', async () => {
    const plan = row('plan', { title: '计划' })
    const goal = row('goal', { title: '目标' })
    await render({ items: [plan, goal], selectedIndex: 1, role: 'listbox' })
    const originalGoal = item('goal')
    expect(originalGoal.getAttribute('aria-selected')).toBe('true')
    expect(host.querySelector('[role="listbox"]')?.getAttribute('aria-activedescendant')).toBe(composerCommandOptionId('commands', 'goal'))
    await render({ items: [goal, plan], selectedIndex: 0, role: 'listbox' })
    expect(item('goal')).toBe(originalGoal)
    expect(item('goal').getAttribute('aria-selected')).toBe('true')
    expect(item('plan').getAttribute('aria-selected')).toBe('false')
  })

  it('keeps the selected option visible when keyboard selection or catalog order changes', async () => {
    const scroll = vi.fn()
    const previous = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: scroll })
    try {
      const plan = row('plan'), goal = row('goal')
      await render({ items: [plan, goal], selectedIndex: 0, role: 'listbox' })
      expect(scroll).toHaveBeenLastCalledWith({ block: 'nearest' })
      expect(scroll.mock.contexts.at(-1)).toBe(item('plan'))
      await render({ items: [plan, goal], selectedIndex: 1, role: 'listbox' })
      expect(scroll.mock.contexts.at(-1)).toBe(item('goal'))
      await render({ items: [goal, plan], selectedIndex: 1, role: 'listbox' })
      expect(scroll.mock.contexts.at(-1)).toBe(item('plan'))
      expect(scroll).toHaveBeenCalledTimes(3)
    } finally {
      if (previous) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', previous)
      else Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
    }
  })

  it('accepts a file row without coercing its identity or original reference into a command', async () => {
    const reference = { path: '/workspace/notes/Meeting notes.md', relativePath: 'notes/Meeting notes.md', type: 'file' }
    const file = {
      id: 'file:file:notes/Meeting notes.md', title: 'Meeting notes.md', canonicalName: 'Meeting notes.md',
      description: reference.relativePath, icon: createElement('svg'), section: 'files', sectionLabel: 'Files', reference
    }
    const select = vi.fn<(item: typeof file, index: number) => void>()
    await act(async () => root.render(createElement(FloatingComposerCommandMenu<typeof file>, {
      id: 'mentions', items: [file], selectedIndex: 0, role: 'listbox', ariaLabel: 'Mentions', emptyLabel: 'Empty', onSelect: select
    })))
    const option = host.querySelector<HTMLButtonElement>('[role="option"]')!
    expect(option.dataset.commandId).toBe(file.id)
    expect(option.id).toBe(composerCommandOptionId('mentions', file.id))
    expect(option.querySelector('.dsh-composer-command-alias')).toBeNull()
    expect(host.querySelector('[data-command-section]')?.textContent).toBe('Files')
    await act(async () => option.click())
    expect(select).toHaveBeenCalledExactlyOnceWith(file, 0)
    expect(select.mock.calls[0]?.[0].reference).toBe(reference)
  })

  it('keeps focus with the editor while selecting a real item and leaves dispatch to its owner', async () => {
    const command = row('file', { title: 'File' })
    await render({ items: [command] })
    const editor = document.createElement('textarea'); document.body.append(editor); editor.focus()
    const mouseDown = new MouseEvent('mousedown', { bubbles: true, cancelable: true })
    await act(async () => { item('file').dispatchEvent(mouseDown); item('file').click() })
    expect(mouseDown.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(editor)
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(command, 0)
    editor.remove()
  })

  it('shows disabled reasons and never dispatches unavailable feedback or export actions', async () => {
    await render({ items: [
      row('feedback', { disabled: true, disabledReason: 'Feedback is unavailable.', description: 'Send feedback.' }),
      row('export', { disabled: true, disabledReason: 'Export is unavailable.' })
    ] })
    expect(item('feedback').disabled).toBe(true)
    expect(item('feedback').querySelector('.dsh-composer-command-description')?.textContent).toBe('Feedback is unavailable.')
    expect(item('feedback').title).toBe('Feedback is unavailable.')
    await act(async () => { item('feedback').click(); item('export').click() })
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('reports pointer movement without moving keyboard selection or selecting a command', async () => {
    await render({ items: [row('plan'), row('goal')], selectedIndex: 0 })
    await act(async () => {
      item('plan').dispatchEvent(new MouseEvent('mousemove', { bubbles: true }))
      item('goal').dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }))
    })
    expect(onHover).not.toHaveBeenCalled()
    await act(async () => item('goal').dispatchEvent(new MouseEvent('mousemove', { bubbles: true })))
    expect(onHover).toHaveBeenCalledExactlyOnceWith(1)
    expect(onSelect).not.toHaveBeenCalled()
    expect(item('plan').getAttribute('data-active')).toBe('true')
  })

  it('exposes checked menu semantics and forwards the root ref and keyboard lifecycle', async () => {
    const ref = createRef<HTMLDivElement>()
    const onKeyDown = vi.fn()
    await act(async () => root.render(createElement(FloatingComposerCommandMenu, {
      ref, items: [row('worktree', { checked: true })], groupLabels, ariaLabel: 'Commands', emptyLabel: 'Empty', onSelect, onKeyDown
    })))
    expect(ref.current).toBe(host.firstElementChild)
    expect(item('worktree').getAttribute('role')).toBe('menuitemcheckbox')
    expect(item('worktree').getAttribute('aria-checked')).toBe('true')
    await act(async () => item('worktree').dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(onKeyDown).toHaveBeenCalledOnce()
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('renders an empty status without inventing a selectable item', async () => {
    await render()
    expect(host.querySelector('[role="status"]')?.textContent).toBe('No commands')
    expect(host.querySelector('button')).toBeNull()
  })

  it('exposes loading as a busy status without inventing a selectable item', async () => {
    await render({ role: 'listbox', loading: true, emptyLabel: 'Loading mentions…' })
    expect(host.querySelector('[role="listbox"]')?.getAttribute('aria-busy')).toBe('true')
    expect(host.querySelector('[role="status"]')?.textContent).toBe('Loading mentions…')
    expect(host.querySelector('[role="option"]')).toBeNull()
  })
})
