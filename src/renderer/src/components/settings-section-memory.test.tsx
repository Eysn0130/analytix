// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemorySettingsSection } from './settings-section-memory'
import type { CoreMemoryRecordJson } from '../agent/analytix-contract'

const record = (id: string): CoreMemoryRecordJson => ({
  id, content: id, scope: 'workspace', workspace: '/synthetic/a', tags: [], confidence: 1,
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z'
})
let root: Root, host: HTMLDivElement
let workspace: string
let ctx: Record<string, any>
let settle: (ok: boolean) => void
async function render() {
  await act(async () => root.render(createElement(MemorySettingsSection, { ctx: { ...ctx, form: { workspaceRoot: workspace } } })))
}
async function click(label: string, index = 0) {
  const buttons = [...host.querySelectorAll<HTMLButtonElement>('button')].filter(button =>
    button.textContent === label || button.getAttribute('aria-label') === label)
  expect(buttons[index]).toBeDefined()
  await act(async () => buttons[index].click())
}
async function edit(value: string) {
  const area = host.querySelector('textarea')!
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(area, value)
    area.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
async function complete(ok = true) { await act(async () => settle(ok)) }

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  workspace = '/synthetic/a'
  const save = vi.fn(() => new Promise<boolean>(resolve => { settle = resolve }))
  ctx = { t: (key: string) => key, memoryRecords: [record('one'), record('two')],
    createMemoryRecord: save, updateMemoryRecord: save, disableMemoryRecord: vi.fn(), deleteMemoryRecord: vi.fn() }
  await render()
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() })

describe('Memory editor save ownership', () => {
  it('keeps input made while a successful save is held', async () => {
    await click('memoryCreate'); await edit('saved'); await click('memorySave'); await edit('new draft')
    await complete(); expect(host.querySelector('textarea')?.value).toBe('new draft')
    expect(ctx.createMemoryRecord).toHaveBeenCalledWith(expect.objectContaining({ content: 'saved' }))
  })
  it('keeps a newly created identical draft after Cancel retires the old session', async () => {
    await click('memoryCreate'); await edit('same'); await click('memorySave'); await click('memoryCancel')
    await click('memoryCreate'); await edit('same'); await complete()
    expect(host.querySelector('textarea')?.value).toBe('same')
  })
  it('keeps another record editor after an older update succeeds', async () => {
    await click('memoryEdit'); await edit('changed one'); await click('memorySave'); await click('memoryEdit')
    await complete(); expect(host.querySelector('textarea')?.value).toBe('two')
    expect(ctx.updateMemoryRecord).toHaveBeenCalledWith('one', expect.objectContaining({ content: 'changed one' }))
  })
  it('closes only the current successful draft and retains a current failed draft', async () => {
    await click('memoryCreate'); await edit('current'); await click('memorySave'); await complete(false)
    expect(host.querySelector('textarea')?.value).toBe('current')
    await click('memorySave'); await complete(); expect(host.querySelector('textarea')).toBeNull()
  })
  it('does not close a new workspace editor on the old success', async () => {
    await click('memoryCreate'); await edit('workspace A'); await click('memorySave')
    workspace = '/synthetic/b'; await render(); await click('memoryCreate'); await edit('workspace B')
    await complete(); expect(host.querySelector('textarea')?.value).toBe('workspace B')
  })
  it('retains an unsaved draft in its original workspace until return or explicit replacement', async () => {
    await click('memoryCreate'); await edit('retained workspace A')
    workspace = '/synthetic/b'; await render()
    expect(host.querySelector('textarea')?.value).toBe('retained workspace A')
    expect([...host.querySelectorAll('button')].find(button => button.textContent === 'memorySave')?.disabled).toBe(true)
    expect(host.textContent).toContain('/synthetic/a'); expect(ctx.createMemoryRecord).not.toHaveBeenCalled()
    workspace = '/synthetic/a'; await render(); await click('memorySave'); await complete()
    expect(host.querySelector('textarea')).toBeNull()
  })
  it('does not affect a new mounted entry when a retired save succeeds', async () => {
    await click('memoryCreate'); await edit('retired'); await click('memorySave')
    await act(async () => root.unmount()); root = createRoot(host); await render()
    await click('memoryCreate'); await edit('new entry'); await complete()
    expect(host.querySelector('textarea')?.value).toBe('new entry')
  })
})
