export type WriteRetrievalSnippetLocation =
  | {
      kind: 'text'
      lineStart: number
      lineEnd: number
    }
  | {
      kind: 'pdf'
      pageStart: number
      pageEnd: number
    }

export type WriteRetrievalSnippet = {
  path: string
  title: string
  text: string
  score: number
  keywords: string[]
  location: WriteRetrievalSnippetLocation
  lineStart?: number
  lineEnd?: number
  pageStart?: number
  pageEnd?: number
}

export type WriteRetrievalContext = {
  source: 'bm25-keyword'
  query: string
  keywords: string[]
  snippets: WriteRetrievalSnippet[]
  indexedFiles: number
  indexedChunks: number
}

export type WriteRetrievalRequest = {
  workspaceRoot?: string
  currentFilePath?: string
  query: string
  maxSnippets?: number
  includeCurrentFile?: boolean
}

export type WriteRetrievalResult =
  | {
      ok: true
      context: WriteRetrievalContext | null
    }
  | {
      ok: false
      message: string
    }
