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

type ImageReceipt = Awaited<ReturnType<Window['analytix']['files']['readImage']>>

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function imageReceipt(dataUrl = 'fresh', path = '/tmp/write/image.png'): ImageReceipt {
  return { ok: true, path, dataUrl, mimeType: 'image/png', size: dataUrl.length }
}

function activateImage(): void {
  useWriteWorkspaceStore.setState({
    workspaceRoot: '/tmp/write',
    rootDirectory: '/tmp/write',
    activeFilePath: '/tmp/write/image.png',
    activeFileKind: 'image',
    imageDataUrl: 'initial',
    imageMimeType: 'image/png',
    fileSize: 7,
    fileError: null,
    fileLoading: false,
    saveStatus: 'saved'
  })
}

function installImageBridge(readImage: Window['analytix']['files']['readImage']): void {
  vi.stubGlobal('window', {
    analytix: {
      files: {
        readImage,
        listDirectory: async ({ workspaceRoot }: { workspaceRoot: string }) => ({ ok: true, root: workspaceRoot, entries: [] }),
        renameEntry: async ({ path, newName }: { path: string; newName: string }) => ({ ok: true, previousPath: path, path: `/tmp/write/${newName}` }),
        deleteEntry: async ({ path }: { path: string }) => ({ ok: true, path })
      }
    }
  })
}

function finishStaleRead(pending: ReturnType<typeof deferred<ImageReceipt>>, outcome: string): void {
  if (outcome === 'throw') pending.reject(new Error('stale image failure'))
  else pending.resolve(outcome === 'error' ? { ok: false, message: 'stale image failure' } : imageReceipt('stale'))
}

describe('active image refresh ownership', () => {
  it.each(['error', 'throw'])('retains bytes and reports a current refresh %s', async (outcome) => {
    const pending = deferred<ImageReceipt>()
    installImageBridge(vi.fn(() => pending.promise))
    activateImage()
    const refreshing = useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')
    finishStaleRead(pending, outcome)
    expect(await refreshing).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ imageDataUrl: 'initial', fileError: 'stale image failure' })
  })

  it('ignores a successful image receipt for an unexpected path', async () => {
    installImageBridge(vi.fn(async () => imageReceipt('wrong', '/tmp/write/other.png')))
    activateImage()
    expect(await useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ imageDataUrl: 'initial', fileError: null })
  })

  it('refreshes the current image and rejects requests for another workspace or path', async () => {
    const readImage = vi.fn(async () => imageReceipt())
    installImageBridge(readImage)
    activateImage()
    expect(await useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/other')).toBe(false)
    expect(await useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write', '/tmp/write/other.png')).toBe(false)
    expect(readImage).not.toHaveBeenCalled()
    expect(await useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')).toBe(true)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ imageDataUrl: 'fresh', fileSize: 5, fileError: null })
  })

  it.each(['success', 'error', 'throw'])('discards an older same-file refresh %s after a newer refresh', async (outcome) => {
    const pending = deferred<ImageReceipt>()
    const readImage = vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValueOnce(imageReceipt())
    installImageBridge(readImage)
    activateImage()
    const first = useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')
    expect(await useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')).toBe(true)
    finishStaleRead(pending, outcome)
    expect(await first).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ imageDataUrl: 'fresh', fileError: null, fileSize: 5 })
  })

  it.each(['success', 'error', 'throw'])('discards a refresh %s across workspace A to B to A', async (outcome) => {
    const pending = deferred<ImageReceipt>()
    installImageBridge(vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValue(imageReceipt()))
    activateImage()
    const first = useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')
    await useWriteWorkspaceStore.getState().initializeWorkspace('/tmp/other')
    await useWriteWorkspaceStore.getState().initializeWorkspace('/tmp/write')
    await useWriteWorkspaceStore.getState().openFile('/tmp/write', '/tmp/write/image.png')
    finishStaleRead(pending, outcome)
    expect(await first).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ workspaceRoot: '/tmp/write', imageDataUrl: 'fresh', fileError: null })
  })

  it.each(['open', 'home', 'reset', 'rename', 'delete', 'shutdown'])('invalidates an outstanding refresh on %s even when the target returns', async (transition) => {
    const pending = deferred<ImageReceipt>()
    installImageBridge(vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValue(imageReceipt()))
    activateImage()
    const first = useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')
    const state = useWriteWorkspaceStore.getState()
    if (transition === 'open') await state.openFile('/tmp/write', '/tmp/write/image.png')
    if (transition === 'home') await state.openWorkspaceHome('/tmp/write')
    if (transition === 'reset') state.resetWorkspace()
    if (transition === 'rename') await state.renameEntry('/tmp/write', '/tmp/write/image.png', 'renamed.png')
    if (transition === 'delete') await state.deleteEntry('/tmp/write', '/tmp/write/image.png')
    if (transition === 'shutdown') {
      const shutdown = state.beginShutdown()
      try {
        expect(await useWriteWorkspaceStore.getState().syncActiveImageFromDisk('/tmp/write')).toBe(false)
      } finally {
        shutdown.release()
      }
    }
    // Reusing the same target cannot resurrect the earlier request's ownership.
    activateImage()
    pending.resolve(imageReceipt('stale'))
    expect(await first).toBe(false)
    expect(useWriteWorkspaceStore.getState()).toMatchObject({ imageDataUrl: 'initial', fileError: null })
  })

  it.each(['reset', 'shutdown'])('discards a pending image open after %s', async (transition) => {
    const pending = deferred<ImageReceipt>()
    const readImage = vi.fn(() => pending.promise)
    installImageBridge(readImage)
    activateImage()
    const opening = useWriteWorkspaceStore.getState().openFile('/tmp/write', '/tmp/write/next.png')
    await vi.waitFor(() => expect(readImage).toHaveBeenCalledOnce())
    if (transition === 'reset') useWriteWorkspaceStore.getState().resetWorkspace()
    else useWriteWorkspaceStore.getState().beginShutdown().release()
    const snapshot = useWriteWorkspaceStore.getState()
    pending.resolve(imageReceipt('stale', '/tmp/write/next.png'))
    await opening
    expect(useWriteWorkspaceStore.getState()).toBe(snapshot)
    expect(useWriteWorkspaceStore.getState().fileLoading).toBe(false)
  })
})
