import { open as openFile, readdir, stat } from 'node:fs/promises'
import { basename, extname, isAbsolute, join, relative, resolve } from 'node:path'
import type { WriteInlineCompletionRequest } from '../../shared/write-inline-completion'
import type {
  WriteRetrievalContext,
  WriteRetrievalRequest,
  WriteRetrievalSnippet,
  WriteRetrievalSnippetLocation
} from '../../shared/write-retrieval'
import { isWritePdfFileExtension, isWriteTextFileExtension } from '../../shared/write-text-file'
import { expandHomePath } from './workspace-service'
import { readWritePdfText, type WritePdfTextPage } from './write-pdf-text-service'

const INDEX_CACHE_TTL_MS = 30_000
const MAX_INDEX_BUILD_MS = 250
const MAX_ASSISTANT_INDEX_BUILD_MS = 2_500
const MAX_SCAN_ENTRIES = 8_000
const MAX_INDEX_FILES = 160
const MAX_FILE_BYTES = 600_000
const MAX_INDEX_CHUNKS = 720
const MAX_CHUNK_CHARS = 900
const MIN_CHUNK_CHARS = 48
const MAX_TOKENS_PER_CHUNK = 1_200
const MAX_QUERY_TERMS = 36
const DEFAULT_MAX_SNIPPETS = 3
const MAX_SNIPPET_CHARS = 520

const SKIP_DIRS = new Set([
  '.git',
  '.hg',
  '.svn',
  'node_modules',
  'dist',
  'out',
  'build',
  '.next',
  'coverage',
  '.cache',
  '.idea',
  '.pnpm-store',
  '.turbo',
  '.venv',
  '.vscode',
  '.yarn',
  '.yarn-cache',
  '.parcel-cache',
  'log',
  'logs',
  'target',
  'temp',
  'tmp',
  'vendor',
  'venv'
])

const STOP_WORDS = new Set([
  'the',
  'and',
  'for',
  'with',
  'this',
  'that',
  'from',
  'into',
  'about',
  'there',
  'their',
  'will',
  'would',
  'could',
  'should',
  'have',
  'has',
  'are',
  'was',
  'were',
  'been',
  'not',
  'but',
  'you',
  'your',
  'our',
  'can',
  'then',
  'when',
  'what',
  'how'
])

export type {
  WriteRetrievalContext,
  WriteRetrievalRequest,
  WriteRetrievalSnippet,
  WriteRetrievalSnippetLocation
} from '../../shared/write-retrieval'

type IndexedChunk = {
  path: string
  relativePath: string
  title: string
  text: string
  lowerText: string
  tokens: string[]
  termFrequency: Map<string, number>
  titleTokens: Set<string>
  pathTokens: Set<string>
  location: WriteRetrievalSnippetLocation
}

type WorkspaceIndex = {
  workspaceRoot: string
  builtAt: number
  files: number
  chunks: IndexedChunk[]
  averageLength: number
  documentFrequency: Map<string, number>
}

type QueryModel = {
  text: string
  terms: string[]
  weights: Map<string, number>
  phrases: string[]
}

const indexCache = new Map<string, WorkspaceIndex>()
const inFlightIndexCache = new Map<string, Promise<WorkspaceIndex>>()

type WorkspaceIndexOptions = {
  includePdf: boolean
  buildMs: number
}

function deadlineExceeded(deadline: number): boolean {
  return Date.now() > deadline
}

function compactText(text = ''): string {
  return String(text || '').replace(/\r\n?/g, '\n').replace(/\s+/g, ' ').trim()
}

function normalizeRelativePath(value: string): string {
  return value.replaceAll('\\', '/')
}

function clipTail(text = '', maxChars = 0): string {
  const source = String(text || '')
  if (!maxChars || source.length <= maxChars) return source
  return source.slice(source.length - maxChars)
}

function normalizeLower(text = ''): string {
  return String(text || '').normalize('NFKC').toLowerCase()
}

function tokenAllowed(token: string): boolean {
  if (!token || STOP_WORDS.has(token)) return false
  if (/^\d+$/.test(token)) return false
  return token.length >= 2
}

export function tokenizeWriteRetrievalText(text = ''): string[] {
  const source = normalizeLower(text)
  const tokens: string[] = []

  const latinTerms = source.match(/[a-z0-9][a-z0-9_-]{1,}/g) ?? []
  for (const term of latinTerms) {
    if (tokenAllowed(term)) tokens.push(term)
  }

  const hanSegments = source.match(/\p{Script=Han}+/gu) ?? []
  for (const segment of hanSegments) {
    const chars = [...segment].slice(0, 120)
    if (chars.length === 1) {
      tokens.push(chars[0])
      continue
    }
    for (let size = 2; size <= Math.min(4, chars.length); size += 1) {
      for (let index = 0; index <= chars.length - size; index += 1) {
        tokens.push(chars.slice(index, index + size).join(''))
      }
    }
  }

  return tokens
}

function termFrequency(tokens: string[]): Map<string, number> {
  const map = new Map<string, number>()
  for (const token of tokens) {
    map.set(token, (map.get(token) ?? 0) + 1)
  }
  return map
}

function isWithinWorkspace(workspaceRoot: string, targetPath: string): boolean {
  const rel = relative(workspaceRoot, targetPath)
  return rel === '' || (!rel.startsWith('..') && !isAbsolute(rel))
}

function resolveWorkspaceRoot(raw: string | undefined): string {
  const value = raw?.trim()
  if (!value) return ''
  return resolve(expandHomePath(value))
}

function resolveComparablePath(raw: string | undefined): string {
  const value = raw?.trim()
  if (!value) return ''
  return resolve(expandHomePath(value))
}

function isIndexedFile(path: string, includePdf: boolean): boolean {
  const ext = extname(path).toLowerCase()
  return isWriteTextFileExtension(ext) || (includePdf && isWritePdfFileExtension(ext))
}

async function scanWorkspaceFiles(
  workspaceRoot: string,
  deadline: number,
  includePdf: boolean
): Promise<string[]> {
  const files: string[] = []
  const stack = [workspaceRoot]
  let scanned = 0

  while (
    stack.length > 0 &&
    scanned < MAX_SCAN_ENTRIES &&
    files.length < MAX_INDEX_FILES &&
    !deadlineExceeded(deadline)
  ) {
    const current = stack.pop()!
    let entries
    try {
      entries = await readdir(current, { withFileTypes: true })
    } catch {
      continue
    }

    entries.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }))
    for (const entry of entries) {
      if (deadlineExceeded(deadline)) break
      scanned += 1
      if (scanned >= MAX_SCAN_ENTRIES || files.length >= MAX_INDEX_FILES) break
      if (entry.name === '.DS_Store') continue
      const path = join(current, entry.name)
      if (entry.isDirectory()) {
        if (!SKIP_DIRS.has(entry.name)) stack.push(path)
        continue
      }
      if (entry.isFile() && isIndexedFile(path, includePdf)) files.push(path)
    }
  }

  return files
}

function cleanHeading(text: string): string {
  return text
    .replace(/^#{1,6}\s+/, '')
    .replace(/\s+#+\s*$/, '')
    .trim()
}

function headingFromLine(text: string): string | null {
  const match = text.match(/^\s{0,3}#{1,6}\s+(.+?)\s*$/)
  return match ? cleanHeading(match[0]) : null
}

function buildChunk(
  path: string,
  relativePath: string,
  title: string,
  lines: string[],
  location: WriteRetrievalSnippetLocation
): IndexedChunk | null {
  const raw = lines.join('\n').trim()
  const text = raw.length > MAX_CHUNK_CHARS + 160 ? `${raw.slice(0, MAX_CHUNK_CHARS).trimEnd()}...` : raw
  if (compactText(text).length < MIN_CHUNK_CHARS) return null

  const tokens = tokenizeWriteRetrievalText(`${title}\n${text}`).slice(0, MAX_TOKENS_PER_CHUNK)
  if (tokens.length === 0) return null
  return {
    path,
    relativePath,
    title,
    text,
    lowerText: normalizeLower(text),
    tokens,
    termFrequency: termFrequency(tokens),
    titleTokens: new Set(tokenizeWriteRetrievalText(title)),
    pathTokens: new Set(tokenizeWriteRetrievalText(relativePath.replace(/[\\/._-]+/g, ' '))),
    location
  }
}

function chunkMarkdown(path: string, relativePath: string, content: string): IndexedChunk[] {
  const lines = content.replace(/\r\n?/g, '\n').split('\n')
  const chunks: IndexedChunk[] = []
  let currentTitle = basename(path)
  let buffer: string[] = []
  let lineStart = 1
  let charCount = 0

  const flush = (): void => {
    const chunk = buildChunk(path, relativePath, currentTitle, buffer, {
      kind: 'text',
      lineStart,
      lineEnd: lineStart + buffer.length - 1
    })
    if (chunk) chunks.push(chunk)
    buffer = []
    charCount = 0
  }

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]
    const heading = headingFromLine(line)
    if (heading) {
      if (buffer.length > 0) flush()
      currentTitle = heading
      lineStart = index + 1
    } else if (buffer.length === 0) {
      lineStart = index + 1
    }

    buffer.push(line)
    charCount += line.length + 1
    const paragraphBreak = !line.trim() && charCount >= 360
    if (paragraphBreak || charCount >= MAX_CHUNK_CHARS) flush()
  }

  if (buffer.length > 0) flush()
  return chunks
}

function chunkPdfPage(
  path: string,
  relativePath: string,
  page: WritePdfTextPage
): IndexedChunk[] {
  const paragraphs = page.text
    .replace(/\r\n?/g, '\n')
    .split(/\n{2,}|(?<=[。.!?！？])\s+/)
    .map((part) => part.trim())
    .filter(Boolean)
  const chunks: IndexedChunk[] = []
  let buffer: string[] = []
  let charCount = 0

  const flush = (): void => {
    const chunk = buildChunk(path, relativePath, `Page ${page.page}`, buffer, {
      kind: 'pdf',
      pageStart: page.page,
      pageEnd: page.page
    })
    if (chunk) chunks.push(chunk)
    buffer = []
    charCount = 0
  }

  for (const paragraph of paragraphs.length > 0 ? paragraphs : [page.text]) {
    buffer.push(paragraph)
    charCount += paragraph.length + 1
    if (charCount >= MAX_CHUNK_CHARS) flush()
  }
  if (buffer.length > 0) flush()
  return chunks
}

async function chunkPdf(
  path: string,
  workspaceRoot: string,
  relativePath: string
): Promise<IndexedChunk[]> {
  const result = await readWritePdfText({ path, workspaceRoot })
  if (!result.ok || !result.hasText) return []
  const chunks: IndexedChunk[] = []
  for (const page of result.pages) {
    if (chunks.length >= MAX_INDEX_CHUNKS) break
    chunks.push(...chunkPdfPage(path, relativePath, page))
  }
  return chunks
}

async function readIndexableFile(path: string, deadline: number): Promise<string> {
  if (deadlineExceeded(deadline)) return ''
  const info = await stat(path)
  if (!info.isFile() || info.size <= 0) return ''
  const maxBytes = Math.min(info.size, MAX_FILE_BYTES)
  const handle = await openFile(path, 'r')
  try {
      const buffer = Buffer.alloc(maxBytes)
      const { bytesRead } = await handle.read(buffer, 0, maxBytes, 0)
      const bytes = buffer.subarray(0, bytesRead)
      if (bytes.includes(0)) return ''
      if (deadlineExceeded(deadline)) return ''
      return bytes.toString('utf8')
  } finally {
    await handle.close()
  }
}

function workspaceIndexCacheKey(workspaceRoot: string, includePdf: boolean): string {
  return `${workspaceRoot}::${includePdf ? 'pdf' : 'text'}`
}

async function buildWorkspaceIndex(
  workspaceRoot: string,
  options: WorkspaceIndexOptions
): Promise<WorkspaceIndex> {
  const deadline = Date.now() + options.buildMs
  const files = await scanWorkspaceFiles(workspaceRoot, deadline, options.includePdf)
  const chunks: IndexedChunk[] = []
  let indexedFiles = 0

  for (const path of files) {
    if (chunks.length >= MAX_INDEX_CHUNKS || deadlineExceeded(deadline)) break
    try {
      const relativePath = normalizeRelativePath(relative(workspaceRoot, path) || basename(path))
      const ext = extname(path).toLowerCase()
      const fileChunks = isWritePdfFileExtension(ext)
        ? await chunkPdf(path, workspaceRoot, relativePath)
        : chunkMarkdown(path, relativePath, await readIndexableFile(path, deadline))
      if (fileChunks.length > 0) indexedFiles += 1
      chunks.push(...fileChunks.slice(0, Math.max(0, MAX_INDEX_CHUNKS - chunks.length)))
    } catch {
      /* Ignore unreadable files and keep completion responsive. */
    }
  }

  const documentFrequency = new Map<string, number>()
  let tokenCount = 0
  for (const chunk of chunks) {
    tokenCount += chunk.tokens.length
    for (const token of new Set(chunk.tokens)) {
      documentFrequency.set(token, (documentFrequency.get(token) ?? 0) + 1)
    }
  }

  return {
    workspaceRoot,
    builtAt: Date.now(),
    files: indexedFiles,
    chunks,
    averageLength: chunks.length > 0 ? tokenCount / chunks.length : 1,
    documentFrequency
  }
}

async function loadWorkspaceIndex(
  workspaceRoot: string,
  options: WorkspaceIndexOptions
): Promise<WorkspaceIndex> {
  const cacheKey = workspaceIndexCacheKey(workspaceRoot, options.includePdf)
  const cached = indexCache.get(cacheKey)
  if (cached && Date.now() - cached.builtAt <= INDEX_CACHE_TTL_MS) return cached
  const existing = inFlightIndexCache.get(cacheKey)
  if (existing) return existing

  const build = buildWorkspaceIndex(workspaceRoot, options)
    .then((index) => {
      indexCache.set(cacheKey, index)
      return index
    })
    .finally(() => {
      inFlightIndexCache.delete(cacheKey)
    })

  inFlightIndexCache.set(cacheKey, build)
  return build
}

function addWeightedTerms(weights: Map<string, number>, text: string, weight: number): void {
  for (const token of tokenizeWriteRetrievalText(text)) {
    weights.set(token, (weights.get(token) ?? 0) + weight)
  }
}

function extractPhrases(request: WriteInlineCompletionRequest): string[] {
  const candidates = [
    request.context.currentLinePrefix,
    request.context.previousNonEmptyLine,
    request.context.previousLine,
    request.editCandidate?.original,
    ...(request.recentEdits ?? []).flatMap((edit) => [edit.deletedText, edit.insertedText])
  ]
  const phrases: string[] = []
  for (const candidate of candidates) {
    const compact = compactText(candidate)
    if (compact.length >= 8) phrases.push(normalizeLower(clipTail(compact, 80)))
  }
  return [...new Set(phrases)].slice(0, 4)
}

function buildQueryModel(request: WriteInlineCompletionRequest): QueryModel {
  const weights = new Map<string, number>()
  addWeightedTerms(weights, request.context.currentLinePrefix, 3)
  addWeightedTerms(weights, request.context.previousNonEmptyLine, 2)
  addWeightedTerms(weights, request.context.previousLine, 1.4)
  addWeightedTerms(weights, request.editCandidate?.original ?? '', 1.8)
  addWeightedTerms(weights, request.preview.documentTail, 1)
  for (const edit of request.recentEdits ?? []) {
    addWeightedTerms(weights, edit.deletedText, 1.6)
    addWeightedTerms(weights, edit.insertedText, 1.8)
  }
  addWeightedTerms(weights, clipTail(request.prefix, 700), 0.7)

  const ranked = [...weights.entries()]
    .sort((a, b) => b[1] - a[1] || b[0].length - a[0].length || a[0].localeCompare(b[0]))
    .slice(0, MAX_QUERY_TERMS)
  const terms = ranked.map(([term]) => term)
  const queryText = compactText([
    request.context.currentLinePrefix,
    request.context.previousNonEmptyLine,
    request.editCandidate?.original ?? '',
    ...(request.recentEdits ?? []).flatMap((edit) => [edit.deletedText, edit.insertedText]),
    request.preview.documentTail
  ].join(' ')).slice(0, 240)
  return {
    text: queryText,
    terms,
    weights: new Map(ranked),
    phrases: extractPhrases(request)
  }
}

function buildQueryModelFromText(text: string): QueryModel {
  const weights = new Map<string, number>()
  addWeightedTerms(weights, text, 2)
  const compact = compactText(text)
  const ranked = [...weights.entries()]
    .sort((a, b) => b[1] - a[1] || b[0].length - a[0].length || a[0].localeCompare(b[0]))
    .slice(0, MAX_QUERY_TERMS)
  return {
    text: compact.slice(0, 240),
    terms: ranked.map(([term]) => term),
    weights: new Map(ranked),
    phrases: compact.length >= 8 ? [normalizeLower(clipTail(compact, 80))] : []
  }
}

function bm25Score(chunk: IndexedChunk, index: WorkspaceIndex, query: QueryModel): number {
  const totalDocs = Math.max(1, index.chunks.length)
  const averageLength = Math.max(1, index.averageLength)
  const k1 = 1.2
  const b = 0.72
  let score = 0

  for (const term of query.terms) {
    const tf = chunk.termFrequency.get(term) ?? 0
    if (!tf) continue
    const df = index.documentFrequency.get(term) ?? 0
    const idf = Math.log(1 + (totalDocs - df + 0.5) / (df + 0.5))
    const normalized = (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * (chunk.tokens.length / averageLength)))
    const weight = query.weights.get(term) ?? 1
    score += weight * idf * normalized
  }

  return score
}

function keywordScore(chunk: IndexedChunk, query: QueryModel): { score: number; keywords: string[] } {
  const keywords: string[] = []
  let score = 0
  for (const term of query.terms) {
    if (!chunk.termFrequency.has(term)) continue
    keywords.push(term)
    const weight = query.weights.get(term) ?? 1
    if (chunk.titleTokens.has(term)) score += 0.35 * weight
    if (chunk.pathTokens.has(term)) score += 0.18 * weight
  }
  if (keywords.length > 0) score += Math.sqrt(keywords.length) * 0.18

  for (const phrase of query.phrases) {
    if (phrase.length >= 8 && chunk.lowerText.includes(phrase)) score += 0.9
  }

  return { score, keywords: keywords.slice(0, 8) }
}

function bestSnippetText(chunk: IndexedChunk, keywords: string[]): string {
  const compact = chunk.text.replace(/\r\n?/g, '\n').trim()
  if (compact.length <= MAX_SNIPPET_CHARS) return compact

  const lower = normalizeLower(compact)
  let bestIndex = -1
  for (const keyword of keywords) {
    const index = lower.indexOf(keyword)
    if (index >= 0 && (bestIndex < 0 || index < bestIndex)) bestIndex = index
  }
  const center = bestIndex >= 0 ? bestIndex : Math.floor(compact.length / 2)
  const start = Math.max(0, center - Math.floor(MAX_SNIPPET_CHARS / 2))
  const end = Math.min(compact.length, start + MAX_SNIPPET_CHARS)
  const prefix = start > 0 ? '...' : ''
  const suffix = end < compact.length ? '...' : ''
  return `${prefix}${compact.slice(start, end).trim()}${suffix}`
}

function rankChunks(
  index: WorkspaceIndex,
  query: QueryModel,
  currentFilePath: string,
  maxSnippets: number,
  includeCurrentFile: boolean
): WriteRetrievalSnippet[] {
  const ranked = index.chunks
    .filter((chunk) => includeCurrentFile || !currentFilePath || resolveComparablePath(chunk.path) !== currentFilePath)
    .map((chunk) => {
      const keyword = keywordScore(chunk, query)
      const score = bm25Score(chunk, index, query) + keyword.score
      return {
        chunk,
        score,
        keywords: keyword.keywords
      }
    })
    .filter((item) => item.score >= 0.25 && item.keywords.length > 0)
    .sort((a, b) => b.score - a.score)

  const snippets: WriteRetrievalSnippet[] = []
  const perFile = new Map<string, number>()
  const seenText = new Set<string>()
  for (const item of ranked) {
    if (snippets.length >= maxSnippets) break
    const used = perFile.get(item.chunk.path) ?? 0
    if (used >= 2) continue
    const snippetText = bestSnippetText(item.chunk, item.keywords)
    const signature = compactText(snippetText).slice(0, 120)
    if (!signature || seenText.has(signature)) continue
    seenText.add(signature)
    perFile.set(item.chunk.path, used + 1)
    snippets.push({
      path: item.chunk.relativePath,
      title: item.chunk.title,
      text: snippetText,
      score: Number(item.score.toFixed(3)),
      keywords: item.keywords,
      location: item.chunk.location,
      ...(item.chunk.location.kind === 'text'
        ? { lineStart: item.chunk.location.lineStart, lineEnd: item.chunk.location.lineEnd }
        : { pageStart: item.chunk.location.pageStart, pageEnd: item.chunk.location.pageEnd })
    })
  }
  return snippets
}

export async function retrieveWriteInlineCompletionContext(
  request: WriteInlineCompletionRequest,
  options: { maxSnippets?: number } = {}
): Promise<WriteRetrievalContext | null> {
  const workspaceRoot = resolveWorkspaceRoot(request.workspaceRoot)
  if (!workspaceRoot) return null
  const currentFilePath = resolveComparablePath(request.currentFilePath)
  if (currentFilePath && !isWithinWorkspace(workspaceRoot, currentFilePath)) return null

  const index = await loadWorkspaceIndex(workspaceRoot, {
    includePdf: false,
    buildMs: MAX_INDEX_BUILD_MS
  })
  if (index.chunks.length === 0) return null

  const query = buildQueryModel(request)
  if (query.terms.length === 0) return null

  const snippets = rankChunks(
    index,
    query,
    currentFilePath,
    Math.max(1, Math.min(6, Math.round(options.maxSnippets ?? DEFAULT_MAX_SNIPPETS))),
    false
  )
  if (snippets.length === 0) return null

  return {
    source: 'bm25-keyword',
    query: query.text,
    keywords: query.terms.slice(0, 12),
    snippets,
    indexedFiles: index.files,
    indexedChunks: index.chunks.length
  }
}

export async function retrieveWriteContext(
  request: WriteRetrievalRequest
): Promise<WriteRetrievalContext | null> {
  const workspaceRoot = resolveWorkspaceRoot(request.workspaceRoot)
  if (!workspaceRoot) return null
  const currentFilePath = resolveComparablePath(request.currentFilePath)
  if (currentFilePath && !isWithinWorkspace(workspaceRoot, currentFilePath)) return null

  const index = await loadWorkspaceIndex(workspaceRoot, {
    includePdf: true,
    buildMs: MAX_ASSISTANT_INDEX_BUILD_MS
  })
  if (index.chunks.length === 0) return null

  const query = buildQueryModelFromText(request.query)
  if (query.terms.length === 0) return null

  const snippets = rankChunks(
    index,
    query,
    currentFilePath,
    Math.max(1, Math.min(8, Math.round(request.maxSnippets ?? DEFAULT_MAX_SNIPPETS))),
    request.includeCurrentFile !== false
  )
  if (snippets.length === 0) return null

  return {
    source: 'bm25-keyword',
    query: query.text,
    keywords: query.terms.slice(0, 12),
    snippets,
    indexedFiles: index.files,
    indexedChunks: index.chunks.length
  }
}

export function clearWriteRetrievalCache(): void {
  indexCache.clear()
  inFlightIndexCache.clear()
}
