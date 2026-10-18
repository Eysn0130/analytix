import type { AttachmentReference } from '../agent/types'
import type { ComposerFileReference } from '../lib/composer-file-references'

/** Files stay in private renderer memory until upload succeeds or the user removes them. */
export type ComposerAttachmentUpload = {
  id: string
  attemptId: string
  file: File
  localFilePath?: string
  status: 'uploading' | 'failed'
  error?: string
}

/** Private, renderer-memory-only drafts. Never project these into history or storage. */
export type ComposerDraft = {
  input: string
  inputRevision: number
  attachments: AttachmentReference[]
  fileReferences: ComposerFileReference[]
  pendingAttachmentUploads?: number
  attachmentUploadError?: string | null
  attachmentUploads?: ComposerAttachmentUpload[]
}

export const emptyComposerDraft: ComposerDraft = {
  input: '', inputRevision: 0, attachments: [], fileReferences: [], pendingAttachmentUploads: 0, attachmentUploadError: null
}

export function composerDraftKey(workspace: string, threadId: string | null): string {
  return JSON.stringify([workspace, threadId])
}

export function clearSubmittedComposerDraft(
  current: ComposerDraft,
  submitted: ComposerDraft,
  includeInput: boolean
): ComposerDraft {
  const clearInput = includeInput && current.inputRevision === submitted.inputRevision
  return {
    ...current,
    input: clearInput ? '' : current.input,
    inputRevision: current.inputRevision + (clearInput ? 1 : 0),
    attachments: current.attachments.filter((item) => !submitted.attachments.includes(item)),
    fileReferences: current.fileReferences.filter((item) => !submitted.fileReferences.includes(item))
  }
}
