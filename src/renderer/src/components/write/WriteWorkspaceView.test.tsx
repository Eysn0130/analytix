// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
import { WriteWorkspaceView } from './WriteWorkspaceView'
import type { WriteInlineAgent } from './WriteInlineAgent'
import type { WriteWorkspaceDocumentPane } from './WriteWorkspaceDocumentPane'

const mocks = vi.hoisted(() => ({
  pendingDiff: false,
  showTopNotice: vi.fn()
}))
vi.mock('react-i18next', async (importOriginal) => ({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation: () => ({ t: (key: string) => key })
}))
vi.mock('../../store/chat-store', () => ({
  useChatStore: (selector: (state: unknown) => unknown) => selector({
    runtimeConnection: 'ready', showTopNotice: mocks.showTopNotice
  })
}))
vi.mock('./WriteWorkspaceToolbar', () => ({ WriteWorkspaceToolbar: () => null }))
vi.mock('./WriteConflictReview', () => ({ WriteConflictReview: () => null }))
vi.mock('./use-write-split-scroll-sync', () => ({ useWriteSplitScrollSync: () => undefined }))
vi.mock('./WriteWorkspaceDocumentPane', () => ({
  WriteWorkspaceDocumentPane: ({ markdownHandleRef }: ComponentProps<typeof WriteWorkspaceDocumentPane>) => {
    if (markdownHandleRef) {
      markdownHandleRef.current = { isDiffReviewActive: () => mocks.pendingDiff } as NonNullable<typeof markdownHandleRef.current>
    }
    return null
  }
}))
vi.mock('./WriteInlineAgent', () => ({
  WriteInlineAgent: ({ onApplyEdit, onSubmitPrompt }: ComponentProps<typeof WriteInlineAgent>) => createElement('div', null,
    createElement('button', { 'data-testid': 'edit', onClick: () => onApplyEdit('  Revise selection  ') }),
    createElement('button', { 'data-testid': 'ask', onClick: () => onSubmitPrompt('  Explain selection  ') }))
}))

const initialState = useWriteWorkspaceStore.getState()
const flushSave = vi.fn(async (_workspaceRoot: string) => true)
const quoteCurrentSelection = vi.fn()
const onSubmitPrompt = vi.fn()
const requestWriteInlineCompletion = vi.fn()
let root: Root
let container: HTMLDivElement

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.clearAllMocks()
  mocks.pendingDiff = false
  flushSave.mockImplementation(async () => true)
  Object.defineProperty(window, 'analytix', { configurable: true, value: { write: { requestWriteInlineCompletion } } })
  useWriteWorkspaceStore.setState({
    ...initialState,
    workspaceRoot: '/synthetic', activeFilePath: '/synthetic/note.md', activeFileKind: 'text',
    fileContent: 'Selected paragraph', fileSize: 18, fileTruncated: false, fileLoading: false,
    saveStatus: 'saved', previewMode: 'source', shutdownFrozen: false, reviewActive: false,
    pendingAgentReview: null, fileError: null,
    selection: {
      text: 'Selected paragraph', charCount: 18,
      ranges: [{ from: 0, to: 18, startLine: 1, startColumn: 1, endLine: 1, endColumn: 19, text: 'Selected paragraph', charCount: 18 }],
      anchorRect: { left: 20, right: 120, top: 100, bottom: 120, width: 100, height: 20 }
    },
    loadWriteSettings: vi.fn(async () => undefined), flushSave, quoteCurrentSelection
  })
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  useWriteWorkspaceStore.setState(initialState, true)
  Reflect.deleteProperty(window, 'analytix')
  vi.unstubAllGlobals()
})
async function renderAndClick(action = 'edit'): Promise<void> {
  await act(async () => root.render(createElement(WriteWorkspaceView, {
    threadId: 'current-main', leftSidebarCollapsed: false, input: '', setInput: vi.fn(), onSubmitPrompt
  })))
  await act(async () => container.querySelector<HTMLButtonElement>(`[data-testid="${action}"]`)!.click())
}

it('saves the selected snapshot before sending an edit through the current conversation callback', async () => {
  await renderAndClick()
  expect(flushSave).toHaveBeenCalledExactlyOnceWith('/synthetic')
  expect(quoteCurrentSelection).toHaveBeenCalledExactlyOnceWith('/synthetic')
  expect(onSubmitPrompt).toHaveBeenCalledExactlyOnceWith('Revise selection')
  expect(flushSave.mock.invocationCallOrder[0]).toBeLessThan(onSubmitPrompt.mock.invocationCallOrder[0])
  expect(requestWriteInlineCompletion).not.toHaveBeenCalled()
  expect(useWriteWorkspaceStore.getState().fileContent).toBe('Selected paragraph')
})
it('submits a selection question through the required callback', async () => {
  await renderAndClick('ask')
  expect(onSubmitPrompt).toHaveBeenCalledExactlyOnceWith('Explain selection')
  expect(quoteCurrentSelection).toHaveBeenCalledExactlyOnceWith('/synthetic')
  expect(flushSave).not.toHaveBeenCalled()
})
it('refuses an edit when saving fails', async () => {
  flushSave.mockResolvedValue(false)
  await renderAndClick()
  expect(flushSave).toHaveBeenCalledExactlyOnceWith('/synthetic')
  expect(onSubmitPrompt).not.toHaveBeenCalled()
  expect(quoteCurrentSelection).not.toHaveBeenCalled()
})
it.each(['workspaceRoot', 'activeFilePath', 'fileContent', 'selection'] as const)(
  'refuses an edit if %s changes while the snapshot is saving', async field => {
    flushSave.mockImplementationOnce(async () => {
      const state = useWriteWorkspaceStore.getState()
      if (field === 'selection') useWriteWorkspaceStore.setState({ selection: { ...state.selection } })
      else useWriteWorkspaceStore.setState({ [field]: `${state[field]}-changed` })
      return true
    })
    await renderAndClick()
    expect(onSubmitPrompt).not.toHaveBeenCalled()
    expect(quoteCurrentSelection).not.toHaveBeenCalled()
    expect(useWriteWorkspaceStore.getState().fileError).toBe('workbenchReferenceChanged')
  }
)
it.each([
  ['read-only document', 'writeReadOnlySaveDisabled'],
  ['pending diff', 'writeInlineEditReviewPending'],
  ['multiple selections', 'writeInlineEditMultiSelection']
])('refuses an edit for %s before saving', async (condition, error) => {
  if (condition === 'read-only document') useWriteWorkspaceStore.setState({ shutdownFrozen: true })
  if (condition === 'pending diff') mocks.pendingDiff = true
  if (condition === 'multiple selections') {
    const selection = useWriteWorkspaceStore.getState().selection
    useWriteWorkspaceStore.setState({ selection: { ...selection, ranges: [...selection.ranges, ...selection.ranges] } })
  }
  await renderAndClick()
  expect(flushSave).not.toHaveBeenCalled()
  expect(onSubmitPrompt).not.toHaveBeenCalled()
  expect(useWriteWorkspaceStore.getState().fileError).toBe(error)
})
