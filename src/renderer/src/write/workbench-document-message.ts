import type { WriteRetrievalContext, WriteRetrievalRequest, WriteRetrievalResult } from '@shared/write-retrieval'
import { composeWritePrompt, type WriteQuotedSelection } from './quoted-selection'

export function workbenchEmptyMessageKeys(fileCount: number, quoteCount: number, imageCount: number): {
  prompt: 'composerFileAndImageOnlyPrompt' | 'composerFileOnlyPrompt' | 'composerImageOnlyPrompt'
  display: 'composerFileAndImageOnlyDisplay' | 'composerFileOnlyDisplay' | 'composerImageOnlyDisplay'
  count: number
} {
  const count = fileCount + quoteCount
  if (count > 0 && imageCount > 0) return { prompt: 'composerFileAndImageOnlyPrompt', display: 'composerFileAndImageOnlyDisplay', count }
  if (count > 0) return { prompt: 'composerFileOnlyPrompt', display: 'composerFileOnlyDisplay', count }
  return { prompt: 'composerImageOnlyPrompt', display: 'composerImageOnlyDisplay', count }
}

// Layout never participates in a turn. An explicitly selected writing preset
// applies only to a request submitted from the editor's action controls.
// A quote uses its frozen working copy; only an explicit editor request asks
// the existing local retrieval service for additional document context.
export async function prepareWorkbenchDocumentMessage(options: {
  threadId?: string
  input: string
  quotes: WriteQuotedSelection[]
  editorRequest: boolean
  workspaceRoot: string
  activeFilePath: string | null
  requestUserInputAvailable: boolean
  editorPersona?: string
  retrieveContext?: (request: WriteRetrievalRequest) => Promise<WriteRetrievalResult>
}): Promise<string> {
  if (!options.quotes.length && !options.editorRequest) return options.input
  let retrieval: WriteRetrievalContext | null = null
  if (options.editorRequest && options.retrieveContext) {
    try {
      const result = await options.retrieveContext({
        threadId: options.threadId,
        workspaceRoot: options.workspaceRoot,
        currentFilePath: options.activeFilePath ?? undefined,
        query: [...options.quotes.map((quote) => quote.text), options.input].join('\n\n'),
        maxSnippets: 4,
        includeCurrentFile: true
      })
      if (result.ok) retrieval = result.context
    } catch {
      // Optional retrieval never replaces the explicit frozen reference.
    }
  }
  return composeWritePrompt(options.input, options.quotes, {
    workspaceRoot: options.workspaceRoot,
    activeFilePath: options.editorRequest ? options.activeFilePath : null,
    requestUserInputAvailable: options.requestUserInputAvailable,
    agentPersona: options.editorRequest ? options.editorPersona : undefined,
    retrieval
  })
}
