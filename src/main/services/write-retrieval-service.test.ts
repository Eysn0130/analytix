import { mkdir, mkdtemp, open, readdir, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { WriteInlineCompletionRequest } from '../../shared/write-inline-completion'

vi.mock('node:fs/promises', async (importOriginal) => {
  const actual = await importOriginal<typeof import('node:fs/promises')>()
  return { ...actual, open: vi.fn(actual.open), readdir: vi.fn(actual.readdir) }
})

vi.mock('./write-pdf-text-service', () => ({
  readWritePdfText: async (payload: { path: string }) => {
    const text = [
      'PDF BM25 关键词检索 with literature context improves retrieval quality.',
      'The assistant can cite the relevant page when explaining research evidence.'
    ].join(' ')
    return {
      ok: true,
      path: payload.path,
      size: 123,
      mtimeMs: 1,
      pageCount: 1,
      pages: [{
        page: 1,
        text,
        charStart: 0,
        charEnd: text.length
      }],
      hasText: true,
      truncated: false
    }
  },
  clearWritePdfTextCache: () => undefined
}))

import {
  clearWriteRetrievalCache,
  retrieveWriteContext,
  retrieveWriteInlineCompletionContext,
  tokenizeWriteRetrievalText
} from './write-retrieval-service'

function createRequest(workspaceRoot: string): WriteInlineCompletionRequest {
  return {
    workspaceRoot,
    currentFilePath: join(workspaceRoot, 'draft.md'),
    prefix: '# Draft\n\nBM25 关键词',
    suffix: '',
    cursor: {
      line: 3,
      column: 9
    },
    context: {
      language: 'markdown',
      currentLinePrefix: 'BM25 关键词',
      currentLineSuffix: '',
      previousLine: '',
      previousNonEmptyLine: '# Draft',
      nextLine: '',
      indentation: '',
      signals: {
        list: false,
        quote: false,
        heading: false,
        table: false,
        atLineEnd: true,
        endsWithSentencePunctuation: false,
        previousLineEndsWithSentencePunctuation: false,
        prefersNewLineCompletion: false,
        paragraphBreakOpportunity: false
      }
    },
    policy: {
      name: 'precision-inline-v2',
      instruction: 'Return only inserted text.',
      acceptanceCriteria: ['Keep it short.'],
      rejectionCriteria: ['Do not ramble.']
    },
    preview: {
      local: 'BM25 关键词',
      documentTail: '# Draft BM25 关键词'
    },
    model: 'deepseek-v4-flash'
  }
}

afterEach(() => {
  clearWriteRetrievalCache()
  vi.clearAllMocks()
  vi.useRealTimers()
})

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((done) => { resolve = done })
  return { promise, resolve }
}

async function retrievalWorkspace(marker: string) {
  const root = await mkdtemp(join(tmpdir(), 'analytix-write-cache-'))
  await writeFile(join(root, 'notes.md'), `# BM25 关键词检索\n\nBM25 关键词检索 ${marker} provides sufficiently long synthetic reference context.`, 'utf8')
  return root
}

async function holdNextIndexRead(empty = false) {
  const actual = await vi.importActual<typeof import('node:fs/promises')>('node:fs/promises')
  const started = deferred(), release = deferred()
  vi.mocked(open).mockImplementationOnce(async (...args) => {
    const handle = await actual.open(...args)
    const read = await handle.readFile()
    const bytes = empty ? Buffer.alloc(0) : read
    await handle.close()
    started.resolve()
    await release.promise
    return {
      read: async (buffer: Buffer, offset: number, length: number) => ({
        bytesRead: bytes.copy(buffer, offset, 0, length), buffer
      }),
      close: async () => undefined
    } as Awaited<ReturnType<typeof open>>
  })
  return { started: started.promise, release: release.resolve }
}

describe('write retrieval invalidation', () => {
  it('does not deliver or recache an index completed after clear', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    const root = await retrievalWorkspace('OLD_MARKER')
    const held = await holdNextIndexRead()
    const old = retrieveWriteInlineCompletionContext(createRequest(root))
    await held.started
    clearWriteRetrievalCache()
    held.release()
    expect(await old).toBeNull()
    await writeFile(join(root, 'notes.md'), '# BM25 关键词检索\n\nBM25 关键词检索 NEW_MARKER remains valid after the old request was cleared.', 'utf8')
    const current = await retrieveWriteInlineCompletionContext(createRequest(root))
    expect(current?.snippets[0].text).toContain('NEW_MARKER')
    expect(current?.snippets[0].text).not.toContain('OLD_MARKER')
    expect(readdir).toHaveBeenCalledTimes(2)
  })

  it('old completion cannot delete a newer in-flight index for the same key', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    const root = await retrievalWorkspace('VALID_MARKER')
    const first = await holdNextIndexRead(true)
    const old = retrieveWriteInlineCompletionContext(createRequest(root))
    await first.started
    clearWriteRetrievalCache()
    const second = await holdNextIndexRead()
    const current = retrieveWriteInlineCompletionContext(createRequest(root))
    await second.started
    first.release()
    await old
    const joined = retrieveWriteInlineCompletionContext(createRequest(root))
    second.release()
    const [a, b] = await Promise.all([current, joined])
    expect(a?.snippets[0].text).toContain('VALID_MARKER')
    expect(b).toEqual(a)
    expect(readdir).toHaveBeenCalledTimes(2)
  })

  it('an older index cannot replace the newer completed cache', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    const root = await retrievalWorkspace('OLD_MARKER')
    const held = await holdNextIndexRead()
    const old = retrieveWriteInlineCompletionContext(createRequest(root))
    await held.started
    clearWriteRetrievalCache()
    await writeFile(join(root, 'notes.md'), '# BM25 关键词检索\n\nBM25 关键词检索 NEW_MARKER is the current synthetic reference after invalidation.', 'utf8')
    const current = await retrieveWriteInlineCompletionContext(createRequest(root))
    expect(current?.snippets[0].text).toContain('NEW_MARKER')
    held.release()
    await old
    expect(await retrieveWriteInlineCompletionContext(createRequest(root))).toEqual(current)
    expect(readdir).toHaveBeenCalledTimes(2)
  })

  it('does not scan a workspace for an empty inline or assistant query', async () => {
    const root = await retrievalWorkspace('UNNEEDED')
    const request = createRequest(root)
    request.prefix = ''
    request.preview = { local: '', documentTail: '' }
    request.context = { ...request.context, currentLinePrefix: '', previousNonEmptyLine: '', previousLine: '' }
    expect(await retrieveWriteInlineCompletionContext(request)).toBeNull()
    expect(await retrieveWriteContext({ workspaceRoot: root, query: '' })).toBeNull()
    expect(readdir).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
  })
})

describe('write retrieval service', () => {
  it('tokenizes latin terms and CJK keyword ngrams', () => {
    const tokens = tokenizeWriteRetrievalText('BM25 关键词检索 RAG')

    expect(tokens).toContain('bm25')
    expect(tokens).toContain('rag')
    expect(tokens).toContain('关键词')
    expect(tokens).toContain('检索')
  })

  it('retrieves relevant cross-document snippets and excludes the active file', async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-write-rag-'))
    await mkdir(join(workspaceRoot, 'research'), { recursive: true })
    await writeFile(
      join(workspaceRoot, 'draft.md'),
      '# Draft\n\nBM25 关键词',
      'utf8'
    )
    await writeFile(
      join(workspaceRoot, 'research', 'rag.md'),
      [
        '# 检索方案',
        '',
        'BM25 关键词检索用于在写作空间中找到相关片段。',
        '这些片段会作为 RAG 上下文帮助补全保持术语一致。'
      ].join('\n'),
      'utf8'
    )
    await writeFile(
      join(workspaceRoot, 'unrelated.md'),
      '# Shopping',
      'utf8'
    )

    const result = await retrieveWriteInlineCompletionContext(createRequest(workspaceRoot))

    expect(result?.source).toBe('bm25-keyword')
    expect(result?.snippets[0].path).toBe('research/rag.md')
    expect(result?.snippets[0].text).toContain('BM25 关键词检索')
    expect(result?.snippets.some((snippet) => snippet.path === 'draft.md')).toBe(false)
  })

  it('ignores unsupported large data files while scanning the workspace', async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-write-rag-'))
    await writeFile(join(workspaceRoot, 'draft.md'), '# Draft\n\nembedding cache', 'utf8')
    await writeFile(
      join(workspaceRoot, 'notes.md'),
      '# Notes\n\nEmbedding cache notes help the inline completion stay consistent.',
      'utf8'
    )
    await writeFile(join(workspaceRoot, 'output.jsonl'), `${'x'.repeat(10_000)}\n`, 'utf8')

    const result = await retrieveWriteInlineCompletionContext({
      ...createRequest(workspaceRoot),
      prefix: '# Draft\n\nembedding cache',
      context: {
        ...createRequest(workspaceRoot).context,
        currentLinePrefix: 'embedding cache',
        previousNonEmptyLine: '# Draft'
      },
      preview: {
        local: 'embedding cache',
        documentTail: '# Draft embedding cache'
      }
    })

    expect(result?.snippets.some((snippet) => snippet.path === 'output.jsonl')).toBe(false)
    expect(result?.snippets.some((snippet) => snippet.path === 'notes.md')).toBe(true)
  })

  it('retrieves PDF chunks for assistant context with page locations', async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-write-pdf-rag-'))
    const pdfPath = join(workspaceRoot, 'papers', 'study.pdf')
    await mkdir(join(workspaceRoot, 'papers'), { recursive: true })
    await writeFile(join(workspaceRoot, 'draft.md'), '# Draft\n\nExplain literature context.', 'utf8')
    await writeFile(pdfPath, Buffer.from('%PDF-1.4\n%%EOF'))

    const result = await retrieveWriteContext({
      workspaceRoot,
      currentFilePath: pdfPath,
      query: 'PDF BM25 关键词检索 literature retrieval quality',
      maxSnippets: 3,
      includeCurrentFile: true
    })

    expect(result?.source).toBe('bm25-keyword')
    expect(result?.snippets[0]).toMatchObject({
      path: 'papers/study.pdf',
      pageStart: 1,
      pageEnd: 1,
      location: {
        kind: 'pdf',
        pageStart: 1,
        pageEnd: 1
      }
    })
    expect(result?.snippets[0].text).toContain('PDF BM25 关键词检索')
  })
})
