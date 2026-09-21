import type { ImageRegionActions } from './image-region-session'
import type { WriteShutdownResult } from '@shared/write-shutdown'
import type { WriteAgentPresetV1, WriteInlineCompletionSettingsV1, WriteSelectionAssistSettingsV1 } from '@shared/app-settings'
import type { WorkspaceEntry } from '@shared/workspace-file'
import type { WriteEditorSelectionState } from '../components/write/WriteMarkdownEditor'
import type { WriteQuotedSelection } from './quoted-selection'
import type { WriteRecentEdit } from './recent-edits'

export type WritePreviewMode = 'rich' | 'source' | 'live' | 'split' | 'preview'
export type WriteSaveStatus = 'saved' | 'dirty' | 'saving' | 'error' | 'conflict' | 'unknown'
export type WriteObjectSession = { sessionId: string; objectId: string; revision: string }
export type WritePendingSave = { operationId: string; baseRevision: string; content: string; objectId: string }
export type WriteConflictComparison = WriteObjectSession & { workspaceRoot: string; path: string; diskContent: string; localContent: string }
export type WriteActiveFileKind = 'text' | 'image' | 'pdf'

export type WriteDiffReviewRecovery = {
  workspaceRoot: string
  filePath: string
  /** The unchanged working copy while the editor owns the pending review. */
  baseline: string
  /** Accepted chunks have already advanced this original document. */
  original: string
  /** Rejected chunks have already reverted this proposed document. */
  nextDoc: string
}

export type WriteWorkspaceState = ImageRegionActions & {
  defaultWorkspaceRoot: string
  workspaceRoots: string[]
  inlineCompletion: WriteInlineCompletionSettingsV1
  inlineCompletionApiReady: boolean
  /** Selection toolbar AI assists: quick action prompts + infographic prompt. */
  selectionAssist: WriteSelectionAssistSettingsV1
  /** Named writing-assistant personas for quick switching. */
  agentPresets: WriteAgentPresetV1[]
  /** True when the image generation provider is fully configured (enables 生成信息图). */
  imageGenReady: boolean
  /** True when the primary chat provider is configured (enables 生成交互原型). */
  prototypeReady: boolean
  settingsLoading: boolean
  settingsError: string | null
  workspaceRoot: string
  rootDirectory: string
  entriesByDir: Record<string, WorkspaceEntry[]>
  expandedDirs: Set<string>
  loadingDirs: Record<string, boolean>
  treeError: string | null
  activeFilePath: string | null
  activeFileKind: WriteActiveFileKind | null
  fileContent: string
  objectSession: WriteObjectSession | null
  legacyObjectEditing: boolean
  pendingSave: WritePendingSave | null
  imageDataUrl: string
  imageMimeType: string
  pdfDataBase64: string
  pdfMimeType: string
  pdfMtimeMs: number
  fileSize: number
  fileTruncated: boolean
  fileError: string | null
  fileLoading: boolean
  saveStatus: WriteSaveStatus
  /** Set when an agent edited the active file and the change awaits red/green review. */
  pendingAgentReview: { nextContent: string } | null
  /** True while an inline diff review (agent edit or AI rewrite) is in progress. */
  reviewActive: boolean
  reviewRecovery: WriteDiffReviewRecovery | null
  suspendReview: (recovery: WriteDiffReviewRecovery) => void
  previewMode: WritePreviewMode
  assistantOpen: boolean
  assistantModel: string
  assistantProviderId: string
  /** Active writing-agent persona preset id ('' = none); applied to assistant sends. */
  assistantAgentPresetId: string
  selection: WriteEditorSelectionState
  quotedSelections: WriteQuotedSelection[]
  recentEdits: WriteRecentEdit[]
  loadWriteSettings: () => Promise<void>
  selectWriteWorkspace: (workspaceRoot: string) => Promise<void>
  addWriteWorkspace: (workspaceRoot: string) => Promise<void>
  removeWriteWorkspace: (workspaceRoot: string) => Promise<void>
  initializeWorkspace: (workspaceRoot: string) => Promise<void>
  loadDirectory: (workspaceRoot: string, path?: string) => Promise<string | null>
  toggleDirectory: (workspaceRoot: string, path: string) => Promise<void>
  refreshWorkspace: (workspaceRoot: string) => Promise<void>
  openWorkspaceHome: (workspaceRoot?: string) => Promise<boolean>
  openFile: (workspaceRoot: string, path: string) => Promise<void>
  setFileContent: (content: string) => void
  syncActiveFileFromDisk: (
    workspaceRoot: string,
    options?: {
      path?: string
      content?: string
      size?: number
      truncated?: boolean
      message?: string
      animate?: boolean
      force?: boolean
      /** When true, surface the change as a red/green diff review instead of applying it. */
      reviewAsDiff?: boolean
    }
  ) => Promise<boolean>
  syncActiveImageFromDisk: (workspaceRoot: string, path?: string) => Promise<boolean>
  shutdownFrozen: boolean
  beginShutdown: () => { save: () => Promise<WriteShutdownResult>; release: () => void }
  exportInProgress: boolean
  beginExport: () => { settled: Promise<void>; release: () => void }
  flushSave: (workspaceRoot: string) => Promise<boolean>
  resolveFileConflict: (comparison: WriteConflictComparison, choice: 'keep-draft' | 'use-disk') => Promise<boolean>
  createFile: (workspaceRoot: string, path: string, content?: string) => Promise<string | null>
  createDirectory: (workspaceRoot: string, path: string) => Promise<string | null>
  renameEntry: (workspaceRoot: string, path: string, newName: string) => Promise<string | null>
  deleteEntry: (workspaceRoot: string, path: string) => Promise<boolean>
  setFileError: (message: string | null) => void
  setPreviewMode: (mode: WritePreviewMode) => void
  setAssistantOpen: (open: boolean) => void
  setAssistantModel: (model: string, providerId?: string) => void
  setAssistantAgentPresetId: (id: string) => void
  setSelection: (selection: WriteEditorSelectionState) => void
  setReviewActive: (active: boolean) => void
  clearPendingAgentReview: () => void
  recordRecentEdits: (edits: WriteRecentEdit[]) => void
  quoteCurrentSelection: (workspaceRoot: string) => void
  removeQuotedSelection: (id: string) => void
  clearQuotedSelections: () => void
  resetWorkspace: () => void
}

export type WriteWorkspaceSet = (
  partial: Partial<WriteWorkspaceState> | ((state: WriteWorkspaceState) => Partial<WriteWorkspaceState>)
) => void

export type WriteWorkspaceGet = () => WriteWorkspaceState
