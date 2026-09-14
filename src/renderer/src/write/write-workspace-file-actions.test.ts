import { afterEach, describe, expect, it, vi } from 'vitest'
import { defaultWriteSettings } from '@shared/app-settings'
import { createWriteFileActions } from './write-workspace-file-actions'
import { initialState } from './write-workspace-store-helpers'
import type { WriteWorkspaceGet, WriteWorkspaceSet, WriteWorkspaceState } from './write-workspace-store-types'

function makeBaseState(): WriteWorkspaceState {
  return {
    defaultWorkspaceRoot: '',
    workspaceRoots: [],
    inlineCompletion: defaultWriteSettings().inlineCompletion,
    inlineCompletionApiReady: false,
    selectionAssist: defaultWriteSettings().selectionAssist,
    agentPresets: defaultWriteSettings().agentPresets,
    imageGenReady: false,
    prototypeReady: false,
    settingsLoading: false,
    settingsError: null,
    ...initialState(),
    previewMode: 'live',
    assistantOpen: true,
    assistantModel: 'auto',
    assistantProviderId: '',
    assistantAgentPresetId: '',
    loadWriteSettings: async () => undefined,
    selectWriteWorkspace: async () => undefined,
    addWriteWorkspace: async () => undefined,
    removeWriteWorkspace: async () => undefined,
    initializeWorkspace: async () => undefined,
    loadDirectory: async () => null,
    toggleDirectory: async () => undefined,
    refreshWorkspace: async () => undefined,
    openWorkspaceHome: async () => true,
    openFile: async () => undefined,
    setFileContent: () => undefined,
    syncActiveFileFromDisk: async () => false,
    syncActiveImageFromDisk: async () => false,
    flushSave: async () => true,
    createFile: async () => null,
    createDirectory: async () => null,
    renameEntry: async () => null,
    deleteEntry: async () => false,
    setFileError: () => undefined,
    setPreviewMode: () => undefined,
    setAssistantOpen: () => undefined,
    setAssistantModel: () => undefined,
    setAssistantAgentPresetId: () => undefined,
    setReviewActive: () => undefined,
    reviewRecovery: null,
    suspendReview: () => undefined,
    clearPendingAgentReview: () => undefined,
    setSelection: () => undefined,
    recordRecentEdits: () => undefined,
    quoteCurrentSelection: () => undefined,
    removeQuotedSelection: () => undefined,
    clearQuotedSelections: () => undefined,
    resetWorkspace: () => undefined
  }
}

function createHarness(): {
  actions: ReturnType<typeof createWriteFileActions>
  set: WriteWorkspaceSet
  get: WriteWorkspaceGet
} {
  let state = makeBaseState()
  const set: WriteWorkspaceSet = (partial) => {
    const patch = typeof partial === 'function' ? partial(state) : partial
    state = { ...state, ...patch }
  }
  const get: WriteWorkspaceGet = () => state
  const actions = createWriteFileActions({
    set,
    get,
    cancelExternalSyncAnimation: vi.fn(),
    setLastSavedContent: vi.fn()
  })
  state = { ...state, ...actions }
  return { actions, set, get }
}

type FileBridgeTestOverrides = Partial<{
  listWorkspaceDirectory: Window['analytix']['files']['listDirectory']
  createWorkspaceFile: Window['analytix']['files']['createFile']
  renameWorkspaceEntry: Window['analytix']['files']['renameEntry']
  deleteWorkspaceEntry: Window['analytix']['files']['deleteEntry']
  readWorkspacePdf: Window['analytix']['files']['readPdf']
  readWorkspaceFile: Window['analytix']['files']['read']
}>

function installDsGui(overrides: FileBridgeTestOverrides): void {
  vi.stubGlobal('window', {
    analytix: {
      files: {
        listDirectory: overrides.listWorkspaceDirectory,
        createFile: overrides.createWorkspaceFile,
        renameEntry: overrides.renameWorkspaceEntry,
        deleteEntry: overrides.deleteWorkspaceEntry,
        readPdf: overrides.readWorkspacePdf,
        read: overrides.readWorkspaceFile
      }
    } as unknown as Window['analytix']
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('write workspace file actions', () => {
  it('discards an older open receipt after another document was opened', async () => {
    let finishFirst!: (value: unknown) => void
    const read = vi.fn()
      .mockImplementationOnce(() => new Promise((resolve) => { finishFirst = resolve }))
      .mockResolvedValueOnce({ ok: true, path: '/tmp/write/new.md', content: 'new', size: 3, truncated: false })
    installDsGui({ readWorkspaceFile: read })
    const { actions, get, set } = createHarness()
    set({ workspaceRoot: '/tmp/write' })
    const first = actions.openFile('/tmp/write', '/tmp/write/old.md')
    await vi.waitFor(() => expect(read).toHaveBeenCalledTimes(1))
    await actions.openFile('/tmp/write', '/tmp/write/new.md')
    finishFirst({ ok: true, path: '/tmp/write/old.md', content: 'old', size: 3, truncated: false })
    await first
    expect(get().activeFilePath).toBe('/tmp/write/new.md')
    expect(get().fileContent).toBe('new')
  })

  it('does not reopen a document after the workspace home was selected', async () => {
    let finishRead!: (value: unknown) => void
    const read = vi.fn(() => new Promise((resolve) => { finishRead = resolve }))
    installDsGui({ readWorkspaceFile: read as Window['analytix']['files']['read'] })
    const { actions, get, set } = createHarness()
    set({ workspaceRoot: '/tmp/write' })
    const opening = actions.openFile('/tmp/write', '/tmp/write/old.md')
    await vi.waitFor(() => expect(read).toHaveBeenCalledOnce())
    expect(await actions.openWorkspaceHome('/tmp/write')).toBe(true)
    finishRead({ ok: true, path: '/tmp/write/old.md', content: 'old', size: 3, truncated: false })
    await opening
    expect(get().activeFilePath).toBeNull()
    expect(get().fileContent).toBe('')
  })

  it('clears loading state and records list errors when directory IPC throws', async () => {
    installDsGui({
      listWorkspaceDirectory: vi.fn(async () => {
        throw new Error('bridge down')
      })
    })
    const { actions, get } = createHarness()

    const result = await actions.loadDirectory('/tmp/write')

    expect(result).toBeNull()
    expect(get().loadingDirs).toEqual({})
    expect(get().treeError).toBe('bridge down')
  })

  it('returns null and reports file errors when create file IPC throws', async () => {
    installDsGui({
      createWorkspaceFile: vi.fn(async () => {
        throw new Error('create failed')
      })
    })
    const { actions, get } = createHarness()

    const result = await actions.createFile('/tmp/write', 'draft.md')

    expect(result).toBeNull()
    expect(get().fileError).toBe('create failed')
  })

  it('returns null and reports file errors when rename IPC throws', async () => {
    installDsGui({
      renameWorkspaceEntry: vi.fn(async () => {
        throw new Error('rename failed')
      })
    })
    const { actions, get } = createHarness()

    const result = await actions.renameEntry('/tmp/write', '/tmp/write/draft.md', 'final.md')

    expect(result).toBeNull()
    expect(get().fileError).toBe('rename failed')
  })

  it('returns false and reports file errors when delete IPC throws', async () => {
    installDsGui({
      deleteWorkspaceEntry: vi.fn(async () => {
        throw new Error('delete failed')
      })
    })
    const { actions, get } = createHarness()

    const result = await actions.deleteEntry('/tmp/write', '/tmp/write/draft.md')

    expect(result).toBe(false)
    expect(get().fileError).toBe('delete failed')
  })

  it('opens the workspace home after saving the active file', async () => {
    const { actions, set, get } = createHarness()
    const flushSave = vi.fn(async () => true)
    set({
      workspaceRoot: '/tmp/write',
      activeFilePath: '/tmp/write/draft.md',
      activeFileKind: 'text',
      fileContent: 'draft',
      fileSize: 5,
      fileError: 'stale error',
      saveStatus: 'dirty',
      flushSave
    })

    const result = await actions.openWorkspaceHome()

    expect(result).toBe(true)
    expect(flushSave).toHaveBeenCalledWith('/tmp/write')
    expect(get()).toMatchObject({
      activeFilePath: null,
      activeFileKind: null,
      fileContent: '',
      fileSize: 0,
      fileError: null,
      saveStatus: 'saved'
    })
  })

  it('keeps the active file open when returning home cannot save', async () => {
    const { actions, set, get } = createHarness()
    const flushSave = vi.fn(async () => false)
    set({
      workspaceRoot: '/tmp/write',
      activeFilePath: '/tmp/write/draft.md',
      activeFileKind: 'text',
      fileContent: 'draft',
      saveStatus: 'dirty',
      flushSave
    })

    const result = await actions.openWorkspaceHome()

    expect(result).toBe(false)
    expect(flushSave).toHaveBeenCalledWith('/tmp/write')
    expect(get()).toMatchObject({
      activeFilePath: '/tmp/write/draft.md',
      activeFileKind: 'text',
      fileContent: 'draft',
      saveStatus: 'dirty'
    })
  })

  it('opens PDF files through the read-only PDF preview state', async () => {
    const readWorkspacePdf = vi.fn(async () => ({
      ok: true as const,
      path: '/tmp/write/papers/study.pdf',
      dataBase64: 'JVBERi0xLjQKJSVFT0Y=',
      mimeType: 'application/pdf' as const,
      size: 14,
      mtimeMs: 1234
    }))
    installDsGui({
      readWorkspacePdf
    })
    const { actions, get } = createHarness()

    await actions.openFile('/tmp/write', '/tmp/write/papers/study.pdf')

    expect(readWorkspacePdf).toHaveBeenCalledWith({
      workspaceRoot: '/tmp/write',
      path: '/tmp/write/papers/study.pdf'
    })
    expect(get().activeFileKind).toBe('pdf')
    expect(get().activeFilePath).toBe('/tmp/write/papers/study.pdf')
    expect(get().pdfDataBase64).toBe('JVBERi0xLjQKJSVFT0Y=')
    expect(get().pdfMimeType).toBe('application/pdf')
    expect(get().fileSize).toBe(14)
    expect(get().pdfMtimeMs).toBe(1234)
    expect(get().fileContent).toBe('')
    expect(get().imageDataUrl).toBe('')
  })
})
