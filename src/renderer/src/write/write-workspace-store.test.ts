import { afterEach, describe, expect, it, vi } from 'vitest'
import { useWriteWorkspaceStore } from './write-workspace-store'

type FileBridgeTestOverrides = Partial<{
  readWorkspaceFile: Window['analytix']['files']['read']
}>

function installDsGui(overrides: FileBridgeTestOverrides): void {
  vi.stubGlobal('window', {
    analytix: {
      objects: { request: async () => ({ ok: false, code: 'unsupported_platform', message: 'Synthetic legacy platform' }) },
      files: {
        read: overrides.readWorkspaceFile
      }
    } as unknown as Window['analytix']
  })
}

function activateTextFile(path = '/tmp/write/draft.md'): void {
  useWriteWorkspaceStore.setState({
    activeFilePath: path,
    activeFileKind: 'text',
    legacyObjectEditing: true,
    fileContent: 'old content',
    fileError: null,
    fileLoading: false,
    saveStatus: 'saved'
  })
}

afterEach(() => {
  useWriteWorkspaceStore.getState().resetWorkspace()
  vi.unstubAllGlobals()
})

describe('write workspace store', () => {
  it('does not mark edits made during persistence as saved or allow a document switch', async () => {
    let finish: (value: { ok: true; path: string }) => void = () => undefined
    const write = vi.fn(() => new Promise<{ ok: true; path: string }>((resolve) => { finish = resolve }))
    vi.stubGlobal('window', { analytix: { files: { write } } })
    activateTextFile()
    useWriteWorkspaceStore.setState({ workspaceRoot: '/tmp/write' })
    useWriteWorkspaceStore.getState().setFileContent('first revision')
    const saving = useWriteWorkspaceStore.getState().flushSave('/tmp/write')
    useWriteWorkspaceStore.getState().setFileContent('newer revision')
    finish({ ok: true, path: '/tmp/write/draft.md' })
    expect(await saving).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ fileContent: 'newer revision', saveStatus: 'dirty' })
    expect(write).toHaveBeenCalledExactlyOnceWith({ workspaceRoot: '/tmp/write', path: '/tmp/write/draft.md', content: 'first revision' })
    const retry = useWriteWorkspaceStore.getState().flushSave('/tmp/write')
    finish({ ok: true, path: '/tmp/write/draft.md' })
    expect(await retry).toBe(true)
    expect(useWriteWorkspaceStore.getState().saveStatus).toBe('saved')
  })

  it('refuses a wrong workspace save and retains a draft after a failed receipt', async () => {
    const write = vi.fn(async () => ({ ok: false, message: 'read-only synthetic target' }))
    vi.stubGlobal('window', { analytix: { files: { write } } })
    activateTextFile()
    useWriteWorkspaceStore.setState({ workspaceRoot: '/tmp/write' })
    useWriteWorkspaceStore.getState().setFileContent('retained draft')
    expect(await useWriteWorkspaceStore.getState().flushSave('/tmp/other')).toBe(false)
    expect(write).not.toHaveBeenCalled()
    expect(await useWriteWorkspaceStore.getState().openWorkspaceHome('/tmp/write')).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ activeFilePath: '/tmp/write/draft.md', fileContent: 'retained draft', saveStatus: 'error' })
  })

  it('keeps a failed workspace transition on the original document', async () => {
    const write = vi.fn(async () => ({ ok: false, message: 'disk full' }))
    const listDirectory = vi.fn()
    vi.stubGlobal('window', { analytix: { files: { write, listDirectory } } })
    activateTextFile()
    useWriteWorkspaceStore.setState({ workspaceRoot: '/tmp/write', rootDirectory: '/tmp/write' })
    useWriteWorkspaceStore.getState().setFileContent('unsaved original')
    await useWriteWorkspaceStore.getState().initializeWorkspace('/tmp/other')
    expect(write).toHaveBeenCalledWith({ workspaceRoot: '/tmp/write', path: '/tmp/write/draft.md', content: 'unsaved original' })
    expect(listDirectory).not.toHaveBeenCalled()
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ workspaceRoot: '/tmp/write', fileContent: 'unsaved original' })
  })

  it('reports read errors when syncing the active text file from disk', async () => {
    installDsGui({
      readWorkspaceFile: vi.fn(async () => {
        throw new Error('read failed')
      })
    })
    activateTextFile()

    const result = await useWriteWorkspaceStore.getState().syncActiveFileFromDisk('/tmp/write', { force: true })

    expect(result).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({
      fileError: 'read failed',
      saveStatus: 'error'
    })
  })

  it('does not apply late read errors after the active text file changes', async () => {
    installDsGui({
      readWorkspaceFile: vi.fn(async () => {
        useWriteWorkspaceStore.setState({ activeFilePath: '/tmp/write/next.md' })
        throw new Error('late read failed')
      })
    })
    activateTextFile()

    const result = await useWriteWorkspaceStore.getState().syncActiveFileFromDisk('/tmp/write', { force: true })

    expect(result).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({
      activeFilePath: '/tmp/write/next.md',
      fileError: null,
      saveStatus: 'saved'
    })
  })
})
