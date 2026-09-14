import type { WriteQuotedSelection } from './quoted-selection'

export type WorkbenchReferenceSnapshot = {
  threadId: string | null
  threadWorkspace: string
  documentWorkspace: string
  filePath: string | null
  content: string
  quotes: readonly WriteQuotedSelection[]
  documentContext?: boolean
}

// This is a local stale-input fence, not an authorization token. Core still
// validates and projects the actual turn at its existing public boundary.
export function workbenchReferencesCurrent(
  frozen: WorkbenchReferenceSnapshot,
  current: Omit<WorkbenchReferenceSnapshot, 'quotes'>
): boolean {
  if (frozen.threadId !== current.threadId || frozen.threadWorkspace !== current.threadWorkspace) return false
  if (!frozen.quotes.length && !frozen.documentContext) return true
  return current.threadWorkspace === current.documentWorkspace &&
    frozen.documentWorkspace === current.documentWorkspace &&
    frozen.filePath === current.filePath && frozen.content === current.content &&
    frozen.quotes.every((quote) => quote.workspaceRoot === current.documentWorkspace &&
      quote.sourceFilePath === current.filePath && quote.snapshotContent === current.content)
}
