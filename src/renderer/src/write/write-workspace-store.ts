import i18n from '../i18n'
import { create } from 'zustand'
import { objectEditingFailure, openTextObject } from './object-editing-client'
import {
  DEFAULT_WRITE_INLINE_COMPLETION_DEBOUNCE_MS,
  DEFAULT_WRITE_INLINE_COMPLETION_MAX_TOKENS,
  DEFAULT_WRITE_INLINE_COMPLETION_MIN_ACCEPT_SCORE,
  DEFAULT_WRITE_INLINE_COMPLETION_MODEL,
  DEFAULT_WRITE_INLINE_LONG_COMPLETION_DEBOUNCE_MS,
  DEFAULT_WRITE_INLINE_LONG_COMPLETION_MAX_TOKENS,
  DEFAULT_WRITE_INLINE_LONG_COMPLETION_MIN_ACCEPT_SCORE,
  defaultWriteSelectionAssistSettings
} from '@shared/app-settings'
import { quotedSelectionFromEditor } from './quoted-selection'
import { writeSelectionStatesEqual } from './write-selection'
import { trimWriteRecentEdits } from './recent-edits'
import type { WriteWorkspaceState } from './write-workspace-store-types'
import { createWriteSettingsActions } from './write-workspace-settings-actions'
import { createWriteFileActions } from './write-workspace-file-actions'
import { writeBrowserStorageItem } from '../lib/browser-storage'
import {
  WRITE_ASSISTANT_MODEL_KEY,
  WRITE_ASSISTANT_PROVIDER_KEY,
  WRITE_ASSISTANT_OPEN_KEY,
  WRITE_PREVIEW_MODE_KEY,
  commonPrefixLength,
  emptySelection,
  formatWriteImageLoadError,
  initialState,
  isMissingImageIpc,
  normalizeWriteAssistantModel,
  pathsEqual,
  readStoredAssistantModel,
  readStoredAssistantOpen,
  readStoredAssistantProviderId,
  readStoredPreviewMode,
  writeBasenameFromPath,
  writeJoinPath,
  writeRelativeToWorkspace
} from './write-workspace-store-helpers'
export type { WriteActiveFileKind, WritePreviewMode, WriteSaveStatus, WriteWorkspaceState } from './write-workspace-store-types'
export { writeBasenameFromPath, writeDirnameFromPath, writeJoinPath, writeRelativeToWorkspace } from './write-workspace-store-helpers'

const MAX_ANIMATED_EXTERNAL_SYNC_CHARS = 120_000

let fileActionsInFlight = 0
let shutdownGeneration = 0
const navigation = { revision: 0 }
let imageRefreshSequence = 0
let saveInFlight: Promise<boolean> | null = null
let lastSavedContent = ''
let externalSyncTimer: number | null = null
let externalSyncAnimationToken = 0

function cancelExternalSyncAnimation(): void {
  externalSyncAnimationToken += 1
  if (externalSyncTimer !== null) {
    window.clearTimeout(externalSyncTimer)
    externalSyncTimer = null
  }
}


function shutdownAwareFileActions(actions: ReturnType<typeof createWriteFileActions>, get: () => WriteWorkspaceState): typeof actions {
  return Object.fromEntries(Object.entries(actions).map(([name, action]) => [name, async (...args: unknown[]) => {
    if (get().shutdownFrozen) {
      if (name === 'openWorkspaceHome' || name === 'deleteEntry') return false
      if (['loadDirectory', 'createFile', 'createDirectory', 'renameEntry'].includes(name)) return null
      return undefined
    }
    fileActionsInFlight += 1
    try { return await (action as (...args: unknown[]) => Promise<unknown>)(...args) }
    finally { fileActionsInFlight -= 1 }
  }])) as typeof actions
}

export const useWriteWorkspaceStore = create<WriteWorkspaceState>((set, get) => ({
  defaultWorkspaceRoot: '',
  workspaceRoots: [],
  inlineCompletion: {
    enabled: true,
    retrievalEnabled: true,
    longCompletionEnabled: true,
    inheritProvider: true,
    providerId: '',
    baseUrl: '',
    inheritModel: true,
    model: DEFAULT_WRITE_INLINE_COMPLETION_MODEL,
    debounceMs: DEFAULT_WRITE_INLINE_COMPLETION_DEBOUNCE_MS,
    longDebounceMs: DEFAULT_WRITE_INLINE_LONG_COMPLETION_DEBOUNCE_MS,
    minAcceptScore: DEFAULT_WRITE_INLINE_COMPLETION_MIN_ACCEPT_SCORE,
    longMinAcceptScore: DEFAULT_WRITE_INLINE_LONG_COMPLETION_MIN_ACCEPT_SCORE,
    maxTokens: DEFAULT_WRITE_INLINE_COMPLETION_MAX_TOKENS,
    longMaxTokens: DEFAULT_WRITE_INLINE_LONG_COMPLETION_MAX_TOKENS
  },
  inlineCompletionApiReady: false,
  selectionAssist: defaultWriteSelectionAssistSettings(),
  agentPresets: [],
  imageGenReady: false,
  prototypeReady: false,
  settingsLoading: false,
  settingsError: null,
  ...initialState(),
  reviewRecovery: null,
  exportInProgress: false,
  shutdownFrozen: false,
  previewMode: readStoredPreviewMode(),
  assistantOpen: readStoredAssistantOpen(),
  assistantModel: readStoredAssistantModel(),
  assistantProviderId: readStoredAssistantProviderId(),
  assistantAgentPresetId: '',

  ...createWriteSettingsActions({ set, get }),
  ...shutdownAwareFileActions(createWriteFileActions({
    set,
    get,
    cancelExternalSyncAnimation,
    navigation,
    setLastSavedContent: (content) => {
      lastSavedContent = content
    }
  }), get),

  setFileContent: (content) => {
    if (get().shutdownFrozen) return
    cancelExternalSyncAnimation()
    set((state) => ({
      fileContent: content,
      saveStatus: state.saveStatus === 'conflict' ? 'conflict' : state.pendingSave ? 'unknown'
        : state.activeFileKind === 'text' && state.activeFilePath && content !== lastSavedContent ? 'dirty' : 'saved'
    }))
  },

  setReviewActive: (active) => set({
    reviewActive: active === true,
    ...(!active ? { reviewRecovery: null } : {})
  }),

  suspendReview: (recovery) => {
    const state = get()
    if (!state.reviewActive || state.workspaceRoot !== recovery.workspaceRoot ||
        state.activeFilePath !== recovery.filePath || state.fileContent !== recovery.baseline) return
    set({ reviewRecovery: recovery })
  },

  clearPendingAgentReview: () => set({ pendingAgentReview: null }),

  syncActiveFileFromDisk: async (workspaceRoot, options = {}) => {
    const snapshot = get()
    const force = options.force === true
    if (snapshot.shutdownFrozen || !snapshot.activeFilePath) return false
    if (snapshot.activeFileKind !== 'text') return false
    if (snapshot.pendingSave || (!force && snapshot.fileContent !== lastSavedContent)) return false
    if (options.path && !pathsEqual(options.path, snapshot.activeFilePath)) return false

    if (options.message) {
      set({ fileError: options.message, saveStatus: 'error' })
      return false
    }

    let document: Awaited<ReturnType<typeof openTextObject>>
    try {
      document = await openTextObject(workspaceRoot, snapshot.activeFilePath)
    } catch (error) {
      if (pathsEqual(get().activeFilePath ?? '', snapshot.activeFilePath)) {
        set({ fileError: error instanceof Error ? error.message : String(error), saveStatus: 'error' })
      }
      return false
    }
    const content = document.content
    const resolvedPath = document.path
    const size = new TextEncoder().encode(content).length
    const truncated = document.truncated
    const nextSize = typeof size === 'number' && Number.isFinite(size)
      ? Math.max(0, Math.floor(size))
      : content.length
    const nextTruncated = truncated === true

    const latest = get()
    if (latest.shutdownFrozen || !latest.activeFilePath || !pathsEqual(latest.activeFilePath, resolvedPath)) return false
    if (latest.pendingSave || (!force && latest.fileContent !== lastSavedContent)) return false
    set({ objectSession: document.legacy ? null : { sessionId: document.sessionId, objectId: document.objectId, revision: document.revision }, legacyObjectEditing: document.legacy })
    if (
      latest.fileContent === content &&
      lastSavedContent === content &&
      latest.fileSize === nextSize &&
      latest.fileTruncated === nextTruncated
    ) {
      set({
        saveStatus: 'saved',
        fileError: null,
        fileLoading: false,
        fileSize: nextSize,
        fileTruncated: nextTruncated
      })
      return true
    }

    cancelExternalSyncAnimation()

    // Agent edits surface as a red/green diff review instead of silently
    // overwriting the editor. The disk already holds `content`, so we record it
    // as the saved baseline and stash it for review; the review's commit later
    // reconciles disk to whatever the user accepts or rejects.
    if (
      options.reviewAsDiff === true &&
      !nextTruncated &&
      content.length <= MAX_ANIMATED_EXTERNAL_SYNC_CHARS &&
      latest.fileContent !== content
    ) {
      lastSavedContent = content
      set({
        pendingAgentReview: { nextContent: content },
        reviewActive: true,
        fileSize: nextSize,
        fileTruncated: nextTruncated,
        fileError: null,
        fileLoading: false
      })
      return true
    }

    lastSavedContent = content

    if (
      options.animate !== false &&
      !nextTruncated &&
      content.length <= MAX_ANIMATED_EXTERNAL_SYNC_CHARS &&
      content.length > latest.fileContent.length
    ) {
      const token = externalSyncAnimationToken
      const prefix = commonPrefixLength(latest.fileContent, content)
      let cursor = prefix
      set({
        fileContent: content.slice(0, prefix),
        fileSize: nextSize,
        fileTruncated: nextTruncated,
        saveStatus: 'saved',
        fileError: null,
        fileLoading: false
      })
      const step = (): void => {
        if (token !== externalSyncAnimationToken) return
        const remaining = content.length - cursor
        const chunk = Math.max(24, Math.ceil(remaining * 0.1))
        cursor = Math.min(content.length, cursor + chunk)
        set({
          fileContent: content.slice(0, cursor),
          fileSize: nextSize,
          fileTruncated: nextTruncated,
          saveStatus: 'saved',
          fileError: null,
          fileLoading: false
        })
        if (cursor < content.length) {
          externalSyncTimer = window.setTimeout(step, 16)
        } else {
          externalSyncTimer = null
        }
      }
      externalSyncTimer = window.setTimeout(step, 16)
      return true
    }

    set({
      fileContent: content,
      fileSize: nextSize,
      fileTruncated: nextTruncated,
      saveStatus: 'saved',
      fileError: null,
      fileLoading: false
    })
    return true
  },

  syncActiveImageFromDisk: async (workspaceRoot, path) => {
    const snapshot = get()
    if (snapshot.shutdownFrozen || !snapshot.activeFilePath || snapshot.activeFileKind !== 'image') return false
    if (!pathsEqual(workspaceRoot, snapshot.workspaceRoot)) return false
    if (path && !pathsEqual(path, snapshot.activeFilePath)) return false
    const activePath = snapshot.activeFilePath
    const revision = navigation.revision
    const sequence = ++imageRefreshSequence
    const current = (): boolean => {
      const latest = get()
      return revision === navigation.revision && sequence === imageRefreshSequence &&
        !latest.shutdownFrozen && latest.activeFileKind === 'image' &&
        pathsEqual(latest.workspaceRoot, workspaceRoot) &&
        pathsEqual(latest.activeFilePath ?? '', activePath)
    }

    try {
      const result = await window.analytix.files.readImage({
        path: snapshot.activeFilePath,
        workspaceRoot
      })
      if (!current()) return false
      if (!result.ok) {
        set({ fileError: result.message })
        return false
      }
      if (!pathsEqual(snapshot.activeFilePath, result.path)) return false

      set({
        imageDataUrl: result.dataUrl,
        imageMimeType: result.mimeType,
        fileSize: result.size,
        fileError: null,
        fileLoading: false,
        saveStatus: 'saved'
      })
      return true
    } catch (error) {
      if (isMissingImageIpc(error)) return false
      if (current()) {
        set({ fileError: formatWriteImageLoadError(error) })
      }
      return false
    }
  },

  beginShutdown: () => {
    if (get().shutdownFrozen) throw new Error('Document shutdown is already pending.')
    const generation = ++shutdownGeneration
    navigation.revision += 1
    set({ shutdownFrozen: true, fileLoading: false })
    let released = false
    const current = () => !released && generation === shutdownGeneration && get().shutdownFrozen
    return {
      release: () => {
        if (released) return
        released = true
        if (generation === shutdownGeneration) set({ shutdownFrozen: false })
      },
      save: async () => {
        const snapshot = get()
        if (snapshot.exportInProgress) return { result: 'blocked', reason: 'exporting' }
        if (fileActionsInFlight || snapshot.fileLoading || externalSyncTimer !== null) return { result: 'blocked', reason: 'unavailable' }
        if (snapshot.reviewActive || snapshot.pendingAgentReview || snapshot.saveStatus === 'conflict') return { result: 'blocked', reason: 'conflict' }
        // An earlier receipt can be successful while returning false because
        // the user typed a newer draft. Save that frozen draft after settlement.
        await (saveInFlight ?? Promise.resolve(true)).catch(() => false)
        if (!current()) return { result: 'blocked', reason: 'unavailable' }
        const sameDocument = () => get().workspaceRoot === snapshot.workspaceRoot &&
          get().activeFilePath === snapshot.activeFilePath && get().activeFileKind === snapshot.activeFileKind &&
          get().fileContent === snapshot.fileContent && !get().fileLoading && !get().reviewActive &&
          !get().pendingAgentReview && !get().exportInProgress && !fileActionsInFlight
        if (!sameDocument()) return { result: 'blocked', reason: 'unavailable' }
        const saved = await get().flushSave(snapshot.workspaceRoot).catch(() => false)
        if (!current() || !sameDocument()) return { result: 'blocked', reason: 'unavailable' }
        if (!saved || get().pendingSave || (snapshot.activeFileKind === 'text' && get().saveStatus !== 'saved')) {
          return { result: 'blocked', reason: get().saveStatus === 'conflict' ? 'conflict' : 'save_unconfirmed' }
        }
        return { result: 'ready' }
      }
    }
  },

  beginExport: () => {
    if (get().shutdownFrozen) throw new Error('Document shutdown is pending.')
    if (get().exportInProgress) throw new Error('A document export is already pending.')
    set({ exportInProgress: true })
    let released = false
    return {
      // Await only the write already underway; exporting never initiates a save.
      settled: (saveInFlight ?? Promise.resolve(true)).then(() => undefined),
      release: () => { if (!released) { released = true; set({ exportInProgress: false }) } }
    }
  },

  flushSave: async (workspaceRoot) => {
    if (get().exportInProgress) return false
    // One write at a time. A caller switching documents must also persist edits
    // made while an earlier save was awaiting its receipt.
    if (saveInFlight) {
      if (!(await saveInFlight)) return false
      return get().flushSave(workspaceRoot)
    }
    const state = get()
    if (!state.activeFilePath || state.activeFileKind !== 'text') return true
    if (state.fileTruncated || state.reviewActive) return false
    if (state.workspaceRoot && state.workspaceRoot !== workspaceRoot) return false
    if (externalSyncTimer !== null) return false
    if (!state.pendingSave && state.fileContent === lastSavedContent) {
      set({ saveStatus: 'saved' })
      return true
    }
    cancelExternalSyncAnimation()
    set({ saveStatus: 'saving' })
    const stillActive = (): boolean => get().activeFilePath === state.activeFilePath &&
      get().workspaceRoot === state.workspaceRoot
    const operation = (async (): Promise<boolean> => {
      try {
        if (state.legacyObjectEditing && !state.pendingSave) {
          const result = await window.analytix.files.write({ path: state.activeFilePath!, workspaceRoot, content: state.fileContent })
          if (!stillActive()) return false
          if (!result.ok) { set({ saveStatus: 'error', fileError: result.message }); return false }
          lastSavedContent = state.fileContent
          const unchanged = get().fileContent === state.fileContent
          set({ saveStatus: unchanged ? 'saved' : 'dirty', fileError: null })
          return unchanged
        }
        let session = state.objectSession
        if (!session) {
          set({ saveStatus: 'error', fileError: i18n.t('writeObjectSessionInvalid') })
          return false
        }
        const pending = state.pendingSave ?? {
          operationId: crypto.randomUUID(), baseRevision: session.revision,
          content: state.fileContent, objectId: session.objectId
        }
        set({ pendingSave: pending })
        let result = await window.analytix.objects.request(state.pendingSave
          ? { action: 'status', sessionId: session.sessionId, operationId: pending.operationId }
          : { action: 'commit', sessionId: session.sessionId, operationId: pending.operationId,
              baseRevision: pending.baseRevision, content: pending.content })
        if (!stillActive()) return false
        if (!result.ok && result.code === 'session_invalid') {
          // Runtime restart loses ephemeral sessions, never the frozen write
          // intent. Rebind this same object and inspect its durable operation.
          const reopened = await openTextObject(workspaceRoot, state.activeFilePath!)
          if (!stillActive()) return false
          if (reopened.objectId !== pending.objectId) {
            set({ saveStatus: 'conflict', fileError: i18n.t('writeObjectConflict') })
            return false
          }
          session = { sessionId: reopened.sessionId, objectId: reopened.objectId, revision: session.revision }
          set({ objectSession: session })
          result = await window.analytix.objects.request({ action: 'status', sessionId: session.sessionId, operationId: pending.operationId })
        }
        if (!result.ok && result.code === 'operation_not_found') {
          // The exact operation ID and payload make even a racing retry safe.
          result = await window.analytix.objects.request({ action: 'commit', sessionId: session.sessionId,
            operationId: pending.operationId, baseRevision: pending.baseRevision, content: pending.content })
        }
        if (!stillActive()) return false
        if (!result.ok) {
          set({ saveStatus: result.code === 'conflict' ? 'conflict' : result.receipt ? 'unknown' : 'error', fileError: objectEditingFailure(result) })
          return false
        }
        if (!('receipt' in result) || result.receipt.status !== 'committed') {
          set({ saveStatus: 'receipt' in result && result.receipt.status === 'conflict' ? 'conflict' : 'unknown',
            fileError: i18n.t('receipt' in result && result.receipt.status === 'conflict' ? 'writeObjectConflict' : 'writeObjectUnknown') })
          return false
        }
        lastSavedContent = pending.content
        const unchanged = get().fileContent === pending.content
        set({ objectSession: { ...session, revision: result.receipt.revision }, pendingSave: null,
          saveStatus: unchanged ? 'saved' : 'dirty', fileError: null })
        return unchanged
      } catch (error) {
        if (stillActive()) set({ saveStatus: get().pendingSave ? 'unknown' : 'error', fileError: error instanceof Error ? error.message : String(error) })
        return false
      }
    })()
    saveInFlight = operation
    try { return await operation } finally { if (saveInFlight === operation) saveInFlight = null }
  },

  resolveFileConflict: async (comparison, choice) => {
    const current = get()
    if (saveInFlight || current.saveStatus !== 'conflict' || current.workspaceRoot !== comparison.workspaceRoot ||
        current.activeFilePath !== comparison.path || current.fileContent !== comparison.localContent ||
        current.objectSession?.objectId !== comparison.objectId) return false
    const latest = await openTextObject(comparison.workspaceRoot, comparison.path).catch(() => null)
    if (!latest || latest.objectId !== comparison.objectId || latest.revision !== comparison.revision ||
        get().fileContent !== comparison.localContent || get().activeFilePath !== comparison.path ||
        get().workspaceRoot !== comparison.workspaceRoot || get().saveStatus !== 'conflict') {
      set({ fileError: i18n.t('writeObjectConflict') })
      return false
    }
    lastSavedContent = latest.content
    set({ objectSession: { sessionId: latest.sessionId, objectId: latest.objectId, revision: latest.revision },
      pendingSave: null, fileContent: choice === 'use-disk' ? latest.content : comparison.localContent,
      saveStatus: choice === 'use-disk' ? 'saved' : 'dirty', fileError: null })
    return choice === 'use-disk' || get().flushSave(comparison.workspaceRoot)
  },

  setFileError: (message) => {
    set({ fileError: message })
  },

  setPreviewMode: (mode) => {
    writeBrowserStorageItem(WRITE_PREVIEW_MODE_KEY, mode)
    set({ previewMode: mode })
  },

  setAssistantOpen: (open) => {
    writeBrowserStorageItem(WRITE_ASSISTANT_OPEN_KEY, open ? '1' : '0')
    set({ assistantOpen: open })
  },

  setAssistantModel: (model, providerId) => {
    const normalized = normalizeWriteAssistantModel(model)
    writeBrowserStorageItem(WRITE_ASSISTANT_MODEL_KEY, normalized)
    const normalizedProviderId = providerId?.trim() ?? ''
    writeBrowserStorageItem(WRITE_ASSISTANT_PROVIDER_KEY, normalizedProviderId)
    set({ assistantModel: normalized, assistantProviderId: normalizedProviderId })
  },

  setAssistantAgentPresetId: (id) => {
    set({ assistantAgentPresetId: typeof id === 'string' ? id : '' })
  },

  setSelection: (selection) => {
    if (writeSelectionStatesEqual(get().selection, selection)) return
    set({ selection })
  },

  recordRecentEdits: (edits) => {
    if (edits.length === 0) return
    set((state) => ({
      recentEdits: trimWriteRecentEdits([...state.recentEdits, ...edits])
    }))
  },

  quoteCurrentSelection: (workspaceRoot) => {
    const state = get()
    if (!state.activeFilePath) return
    const quote = quotedSelectionFromEditor(state.selection, state.activeFilePath, workspaceRoot)
    if (!quote) return
    quote.workspaceRoot = workspaceRoot
    quote.snapshotContent = state.activeFileKind === 'pdf' ? String(state.pdfMtimeMs) : state.fileContent
    set((current) => ({
      assistantOpen: true,
      quotedSelections: [...current.quotedSelections, quote],
      selection: emptySelection()
    }))
  },

  removeQuotedSelection: (id) =>
    set((state) => ({
      quotedSelections: state.quotedSelections.filter((selection) => selection.id !== id)
    })),

  clearQuotedSelections: () => set({ quotedSelections: [] }),

  resetWorkspace: () => {
    if (get().shutdownFrozen) return
    navigation.revision += 1
    cancelExternalSyncAnimation()
    lastSavedContent = ''
    set({ ...initialState(), reviewRecovery: null })
  }
}))
