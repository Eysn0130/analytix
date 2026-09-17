import { useCallback, type SetStateAction } from 'react'
import { useChatStore } from '../../store/chat-store'
import {
  clearSubmittedComposerDraft,
  composerDraftKey,
  emptyComposerDraft,
  type ComposerDraft
} from '../../store/composer-drafts'

export function useThreadComposerDraft(workspace: string, threadId: string | null) {
  const key = composerDraftKey(workspace, threadId)
  const draft = useChatStore((state) => state.composerDrafts[key] ?? emptyComposerDraft)
  const update = useChatStore((state) => state.updateComposerDraft)
  const setInput = useCallback((action: SetStateAction<string>) => {
    update(key, (current) => {
      const input = typeof action === 'function' ? action(current.input) : action
      return input === current.input ? current : {
        ...current, input, inputRevision: current.inputRevision + 1
      }
    })
  }, [key, update])
  const setAttachments = useCallback((action: SetStateAction<ComposerDraft['attachments']>) => {
    update(key, (current) => ({
      ...current, attachments: typeof action === 'function' ? action(current.attachments) : action
    }))
  }, [key, update])
  const setFileReferences = useCallback((action: SetStateAction<ComposerDraft['fileReferences']>) => {
    update(key, (current) => ({
      ...current, fileReferences: typeof action === 'function' ? action(current.fileReferences) : action
    }))
  }, [key, update])
  // This callback retains the submitting identity and revision across navigation
  // and async receipts; it cannot clear a different conversation's new draft.
  const clearSubmitted = useCallback((submitted: {
    includeInput: boolean
    attachments: ComposerDraft['attachments']
    fileReferences: ComposerDraft['fileReferences']
  }) => {
    update(key, (current) => clearSubmittedComposerDraft(current, {
      ...draft, attachments: submitted.attachments, fileReferences: submitted.fileReferences
    }, submitted.includeInput))
  }, [key, draft, update])
  return { draft, setInput, setAttachments, setFileReferences, clearSubmitted }
}
