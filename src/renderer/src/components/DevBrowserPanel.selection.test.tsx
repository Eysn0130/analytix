// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { DevBrowserPanel } from './DevBrowserPanel'
import { useChatStore } from '../store/chat-store'
import { useNativeReferenceStore } from '../office/native-reference-store'
vi.mock('react-i18next', async importOriginal => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: (key: string) => key }) }))
const request = vi.fn(), submit = vi.fn()
const scope = { scopeId: 'a'.repeat(48), documentId: 'b'.repeat(48), selectionId: 'c'.repeat(48), threadId: 'browser-thread', workspace: '/synthetic/browser' }
let root: Root, element: HTMLDivElement
const initial = useChatStore.getInitialState()
beforeEach(async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  vi.resetAllMocks(); localStorage.clear()
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Electron/41.10.3')
  Object.assign(window, { analytix: { app: { openExternal: vi.fn() }, browserSelection: { request } } })
  useChatStore.setState({ ...initial, activeThreadId: scope.threadId, workspaceRoot: scope.workspace }, true)
  useNativeReferenceStore.setState({ references: [], drafts: {} })
  request.mockResolvedValue({ ok: true, scope })
  element = document.createElement('div'); document.body.append(element); root = createRoot(element)
  await act(async () => root.render(createElement(DevBrowserPanel, { preferredUrl: 'http://localhost:3000/', onCollapse: vi.fn(), onSubmitPrompt: submit })))
  Object.assign(element.querySelector('webview')!, { getWebContentsId: () => 17 })
})
afterEach(async () => { await act(async () => root.unmount()); element.remove(); useChatStore.setState(initial, true); vi.restoreAllMocks(); vi.unstubAllGlobals() })
async function click(key: string) {
  const inline = element.querySelector<HTMLButtonElement>(`[data-workspace-viewer-header] button[aria-label="${key}"]`)
  if (inline) {
    await act(async () => inline.click())
    return
  }
  await act(async () => element.querySelector<HTMLButtonElement>('[aria-label="browserMore"]')!.click())
  await act(async () => document.querySelector<HTMLButtonElement>(`[role="menu"] button[aria-label="${key}"]`)!.click())
}
test('quotes directly from the shared operation row using only the opaque scope and never submits', async () => {
  const quote = element.querySelector('[data-workspace-viewer-header] button[aria-label="browserQuoteSelection"]')!
  expect(quote.querySelector('svg[aria-hidden="true"]')).not.toBeNull()
  await click('browserQuoteSelection')
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(submit).not.toHaveBeenCalled()
  expect(request).toHaveBeenCalledWith({ action: 'capture', guestId: 17, threadId: scope.threadId })
  expect(useNativeReferenceStore.getState().references).toEqual([expect.objectContaining({ browserScope: scope, text: '', path: '', editable: false })])
})
test('disables the direct quote and explanation while the same guest capture is pending', async () => {
  let resolve!: (value: unknown) => void
  request.mockReturnValueOnce(new Promise(yes => { resolve = yes }))
  await click('browserQuoteSelection')
  const quote = element.querySelector<HTMLButtonElement>('[data-workspace-viewer-header] button[aria-label="browserQuoteSelection"]')!
  expect(quote.disabled).toBe(true)
  await act(async () => quote.click())
  await act(async () => element.querySelector<HTMLButtonElement>('[aria-label="browserMore"]')!.click())
  const explain = document.querySelector<HTMLButtonElement>('[role="menu"] button[aria-label="browserExplainSelection"]')!
  expect(explain.disabled).toBe(true)
  await act(async () => explain.click())
  expect(request).toHaveBeenCalledTimes(1)
  await act(async () => resolve({ ok: true, scope }))
  expect(quote.disabled).toBe(false)
  expect(submit).not.toHaveBeenCalled()
  expect(useNativeReferenceStore.getState().references).toHaveLength(1)
})
test('keeps direct quote disabled while the retained browser is hidden', async () => {
  await act(async () => root.render(createElement(DevBrowserPanel, {
    preferredUrl: 'http://localhost:3000/', onCollapse: vi.fn(), onSubmitPrompt: submit, visible: false
  })))
  const quote = element.querySelector<HTMLButtonElement>('[data-workspace-viewer-header] button[aria-label="browserQuoteSelection"]')!
  expect(quote.disabled).toBe(true)
  await act(async () => quote.click())
  expect(request).not.toHaveBeenCalled()
  expect(submit).not.toHaveBeenCalled()
  expect(useNativeReferenceStore.getState().references).toHaveLength(0)
})
test('explicit explanation calls the ordinary conversation owner without adding a draft reference', async () => {
  await click('browserExplainSelection')
  expect(submit).toHaveBeenCalledWith('browserExplainPrompt', [expect.objectContaining({ browserScope: scope, threadId: scope.threadId })])
  expect(useNativeReferenceStore.getState().references).toHaveLength(0)
})
test('shows the unsupported child-frame result without a quote or task', async () => {
  request.mockResolvedValue({ ok: false, error: 'child-frame-unsupported' })
  await click('browserExplainSelection')
  expect(submit).not.toHaveBeenCalled(); expect(useNativeReferenceStore.getState().references).toHaveLength(0)
  expect(element.textContent).toContain('browserChildSelectionUnsupported')
})
test.each(['browserQuoteSelection', 'browserExplainSelection'])('revokes a late %s capture after a thread change without dispatching', async action => {
  let resolve!: (value: unknown) => void
  request.mockReturnValueOnce(new Promise(yes => { resolve = yes }))
  await click(action)
  await act(async () => useChatStore.setState({ activeThreadId: 'other-thread' }))
  await act(async () => resolve({ ok: true, scope }))
  expect(submit).not.toHaveBeenCalled(); expect(request).toHaveBeenLastCalledWith({ action: 'revoke', scope })
  expect(useNativeReferenceStore.getState().references).toHaveLength(0)
})
