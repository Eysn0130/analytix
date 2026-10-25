// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { FloatingComposerModelPicker, calculateFloatingMenuPlacement } from './FloatingComposerModelPicker'

type Props = Parameters<typeof FloatingComposerModelPicker>[0]
let root: Root
let host: HTMLDivElement
let outside: HTMLButtonElement
const onModelChange = vi.fn()
const onReasoningChange = vi.fn()
const groups: NonNullable<Props['composerModelGroups']> = [
  { providerId: 'alpha', label: 'Alpha Provider', modelIds: ['shared-chat', 'aster-1', 'aster-2'],
    modelLabels: { 'shared-chat': 'Alpha Shared', 'aster-1': 'Aster One', 'aster-2': 'Aster Two' } },
  { providerId: 'beta', label: 'Beta Provider', modelIds: ['shared-chat', 'beta-fast', 'beta-long'],
    modelLabels: { 'shared-chat': 'Beta Shared', 'beta-fast': 'Beta Fast', 'beta-long': 'Beta Long' } }
]
const trigger = () => host.querySelector<HTMLButtonElement>('[aria-haspopup="menu"]')!
const menu = () => document.querySelector<HTMLElement>('[data-model-picker-pane]')!
const options = () => [...document.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')]
const search = () => document.querySelector<HTMLInputElement>('[role="searchbox"]')!
async function render(props: Partial<Props> = {}, scope = 'thread-a') {
  await act(async () => root.render(createElement(FloatingComposerModelPicker, {
    key: scope, compact: false, mode: 'select', composerModel: 'shared-chat', composerProviderId: 'beta',
    composerPickList: groups.flatMap(group => group.modelIds), composerModelGroups: groups,
    canChangeModel: true, composerReasoningEffort: 'high', onComposerModelChange: onModelChange,
    onComposerReasoningEffortChange: onReasoningChange, ...props
  })))
}
async function click(target: HTMLElement) { await act(async () => target.click()) }
async function key(target: HTMLElement, key: string, extra: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...extra })
  await act(async () => { target.dispatchEvent(event) })
  return event
}
async function query(value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search(), value)
    search().dispatchEvent(new Event('input', { bubbles: true }))
  })
}
async function openModels() {
  await click(trigger())
  await click(menu().querySelector<HTMLButtonElement>('[data-model-pane="model"]')!)
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  onModelChange.mockClear(); onReasoningChange.mockClear()
  host = document.createElement('div'); outside = document.createElement('button')
  document.body.append(host, outside); root = createRoot(host)
  await render()
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); outside.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('composer model picker panes', () => {
  it('uses one root card for Model and Reasoning, then displays grouped models with the exact provider check', async () => {
    await click(trigger())
    expect(menu().getAttribute('data-model-picker-pane')).toBe('root')
    expect(menu().querySelectorAll('[role="menuitem"]')).toHaveLength(2)
    expect(menu().textContent).toContain('Beta Shared')
    await click(menu().querySelector<HTMLButtonElement>('[data-model-pane="model"]')!)
    expect(document.querySelectorAll('[data-model-picker-pane]')).toHaveLength(1)
    expect(document.activeElement).toBe(search())
    expect(menu().querySelectorAll('[role="group"]')).toHaveLength(2)
    expect(options().filter(item => item.getAttribute('aria-checked') === 'true').map(item => item.textContent)).toEqual(['Beta Shared'])
    expect(onModelChange).not.toHaveBeenCalled()
  })
  it('filters friendly labels and accepts the highlighted result with its provider identity on Tab', async () => {
    await openModels(); await query('Aster')
    expect(options().map(item => item.textContent)).toEqual(['Aster One', 'Aster Two'])
    await key(search(), 'ArrowDown')
    const event = await key(search(), 'Tab')
    expect(event.defaultPrevented).toBe(true)
    expect(onModelChange).toHaveBeenCalledExactlyOnceWith('aster-2', 'alpha')
    expect(document.querySelector('[data-model-picker-pane]')).toBeNull()
    expect(document.activeElement).toBe(trigger())
  })
  it('matches an ordered name subsequence without mixing provider selection identities', async () => {
    await openModels(); await query('AsOn')
    expect(options().map(item => item.textContent)).toEqual(['Aster One'])
    await key(search(), 'Enter')
    expect(onModelChange).toHaveBeenCalledExactlyOnceWith('aster-1', 'alpha')
  })
  it('focuses a checked row directly for small catalogs without a search field', async () => {
    await render({ composerModelGroups: [groups[1]], composerPickList: groups[1].modelIds })
    await openModels()
    expect(document.querySelector('[role="searchbox"]')).toBeNull()
    expect(document.activeElement).toBe(options()[0])
    expect(options()[0].getAttribute('aria-checked')).toBe('true')
  })
  it('clears a query in place and preserves the selected provider', async () => {
    await openModels(); await query('Aster')
    await click(menu().querySelector<HTMLButtonElement>('[aria-label="Clear"]')!)
    expect(search().value).toBe('')
    expect(options()).toHaveLength(6)
    expect(document.activeElement).toBe(search())
    expect(onModelChange).not.toHaveBeenCalled()
  })
  it('shows an empty search and does not select anything on Enter', async () => {
    await openModels(); await query('does-not-exist')
    expect(menu().textContent).toContain('No matching models')
    await key(search(), 'Enter')
    expect(onModelChange).not.toHaveBeenCalled()
    expect(menu()).not.toBeNull()
  })
  it.each(['Escape', 'Tab'])('backs out of a model pane using %s without selecting on Shift+Tab', async name => {
    await openModels(); await query('Aster')
    await key(search(), name, { shiftKey: name === 'Tab' })
    expect(menu().getAttribute('data-model-picker-pane')).toBe('root')
    expect(document.activeElement).toBe(menu().querySelector('[data-model-pane="model"]'))
    expect(onModelChange).not.toHaveBeenCalled()
    await key(document.activeElement as HTMLElement, 'Escape')
    expect(document.querySelector('[data-model-picker-pane]')).toBeNull()
    expect(document.activeElement).toBe(trigger())
  })
  it('moves through effort values and selects through the existing reasoning handler', async () => {
    await click(trigger()); await click(menu().querySelector<HTMLButtonElement>('[data-model-pane="effort"]')!)
    const selected = options().find(item => item.getAttribute('aria-checked') === 'true')!
    expect(document.activeElement).toBe(selected)
    await key(selected, 'Home'); await key(document.activeElement as HTMLElement, 'Tab')
    expect(onReasoningChange).toHaveBeenCalledOnce()
    expect(onModelChange).not.toHaveBeenCalled()
    expect(document.querySelector('[data-model-picker-pane]')).toBeNull()
  })
  it('leaves outside modal focus alone when dismissing', async () => {
    await openModels(); await act(async () => outside.focus())
    expect(document.querySelector('[data-model-picker-pane]')).toBeNull()
    expect(document.activeElement).toBe(outside)
    expect(onModelChange).not.toHaveBeenCalled()
  })
  it('retires its portal on thread replacement and does not restore focus from an old refresh', async () => {
    let complete!: () => void
    const refresh = vi.fn(() => new Promise<void>(resolve => { complete = resolve }))
    await render({ onReloadComposerModels: refresh }); await click(trigger())
    expect(refresh).toHaveBeenCalledOnce()
    await render({}, 'thread-b'); await act(async () => outside.focus())
    await act(async () => complete())
    expect(document.querySelector('[data-model-picker-pane]')).toBeNull()
    expect(document.activeElement).toBe(outside)
  })
  it('retires the menu when its existing model owner becomes locked', async () => {
    await openModels(); await render({ canChangeModel: false })
    expect(document.querySelector('[data-model-picker-pane]')).toBeNull()
    expect(trigger().disabled).toBe(true)
    expect(onModelChange).not.toHaveBeenCalled()
  })
  it('keeps shared controlled open state and requests dismissal through the same owner', async () => {
    const onOpenChange = vi.fn()
    await render({ open: true, onOpenChange })
    expect(menu()).not.toBeNull()
    await key(trigger(), 'Escape')
    expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
    await render({ open: false, onOpenChange })
    expect(document.querySelector('[data-model-picker-pane]')).toBeNull()
  })
  it('does not interpret composition keystrokes as selection or dismissal', async () => {
    await openModels(); await query('Aster')
    await key(search(), 'Enter', { isComposing: true })
    await key(search(), 'Escape', { keyCode: 229 })
    expect(menu().getAttribute('data-model-picker-pane')).toBe('model')
    expect(onModelChange).not.toHaveBeenCalled()
  })
})

describe('real model catalog refresh', () => {
  it('shows pending refresh state and a bounded retry after a rejected load', async () => {
    let fail!: (reason?: unknown) => void
    const refresh = vi.fn(() => new Promise<void>((_, reject) => { fail = reject }))
    await render({ onReloadComposerModels: refresh }); await openModels()
    expect(menu().getAttribute('aria-busy')).toBe('true')
    expect(menu().textContent).toContain('Loading')
    expect(options().every(item => item.disabled)).toBe(true)
    await act(async () => fail(new Error('private provider body should not render')))
    expect(menu().getAttribute('aria-busy')).toBe('false')
    expect(menu().textContent).toContain('Provider unavailable')
    expect(menu().textContent).not.toContain('private provider body')
    const retry = menu().querySelector<HTMLButtonElement>('.ds-model-load-error button')!
    await click(retry)
    expect(refresh).toHaveBeenCalledTimes(2)
  })
  it('does not turn an unconfigured fallback pick list into hidden keyboard-selectable models', async () => {
    await render({ composerModelGroups: [], composerPickList: ['fallback-one', 'fallback-two', 'fallback-three', 'fallback-four', 'fallback-five'],
      composerModel: 'fallback-one', composerProviderId: '', onConfigureProviders: vi.fn() })
    await click(trigger())
    expect(document.querySelector('[role="searchbox"]')).toBeNull()
    expect(options()).toHaveLength(0)
    await key(trigger(), 'Enter')
    expect(onModelChange).not.toHaveBeenCalled()
  })
  it('keeps a real empty-catalog reload reachable without claiming a provider-settings handler', async () => {
    const refresh = vi.fn(async () => undefined)
    await render({ composerModelGroups: [], composerPickList: [], composerModel: '', composerProviderId: '', canChangeModel: false,
      onReloadComposerModels: refresh })
    expect(trigger().disabled).toBe(false)
    await click(trigger())
    expect(menu().textContent).toContain('No chat model provider')
    expect(menu().textContent).not.toContain('Open provider settings')
    const retry = [...menu().querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Retry')!
    await click(retry)
    expect(refresh).toHaveBeenCalledTimes(2)
  })
  it('shows an honest empty catalog with a real reload and existing provider settings entry', async () => {
    const refresh = vi.fn(async () => undefined), configure = vi.fn()
    await render({ composerModelGroups: [], composerPickList: [], composerModel: '', composerProviderId: '', canChangeModel: false,
      onReloadComposerModels: refresh, onConfigureProviders: configure })
    await click(trigger())
    expect(menu().getAttribute('data-model-picker-pane')).toBe('model')
    expect(menu().textContent).toContain('No chat model provider')
    expect(menu().textContent).not.toContain('Provider unavailable')
    const settings = [...menu().querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Open provider settings')!
    await click(settings)
    expect(configure).toHaveBeenCalledOnce()
    expect(onModelChange).not.toHaveBeenCalled()
  })
})

it('fits the model menu inside a narrow viewport without a fixed width or height floor', () => {
  const placement = calculateFloatingMenuPlacement({ anchorRect: { top: 65, bottom: 93, right: 150 },
    menuHeight: 300, viewportWidth: 160, viewportHeight: 120 })
  expect(placement.left).toBe(12)
  expect(placement.width).toBe(136)
  expect(placement.top).toBeGreaterThanOrEqual(12)
  expect(placement.top + placement.maxHeight).toBeLessThanOrEqual(108)
})
