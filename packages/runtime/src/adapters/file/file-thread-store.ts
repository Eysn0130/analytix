import { mkdir, readFile, readdir, rm, stat } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import type { ThreadStore, ThreadStoreListOptions } from '../../ports/thread-store.js'
import type { ThreadRecord, ThreadSummary } from '../../contracts/threads.js'
import { toThreadSummary } from '../../domain/thread.js'
import { atomicWriteFile } from './atomic-write.js'

type ThreadIndex = {
  order: string[]
  updatedAt: string
  summaries: Record<string, ThreadSummary>
}

/**
 * File-backed thread store. Writes small JSON state files via atomic
 * `rename` and keeps compact summaries in index.json to make `list` cheap.
 *
 * Layout:
 *   {dataDir}/threads/index.json
 *   {dataDir}/threads/{threadId}/thread.json
 *   {dataDir}/threads/{threadId}/messages.jsonl
 *   {dataDir}/threads/{threadId}/events.jsonl
 *   {dataDir}/threads/{threadId}/usage.json
 */
export class FileThreadStore implements ThreadStore {
  private readonly dataDir: string
  private readonly now: () => Date
  private indexQueue: Promise<void> = Promise.resolve()

  constructor(options: { dataDir: string; now?: () => Date }) {
    this.dataDir = resolve(options.dataDir, 'threads')
    this.now = options.now ?? (() => new Date())
  }

  async list(_options?: ThreadStoreListOptions): Promise<ThreadSummary[]> {
    await this.ensureDir(this.dataDir)
    const index = await this.readIndex()
    const summaries: ThreadSummary[] = []
    const backfillSummaries: Record<string, ThreadSummary> = {}
    for (const threadId of index.order) {
      const indexedSummary = index.summaries[threadId]
      if (indexedSummary) {
        summaries.push(indexedSummary)
        continue
      }
      const thread = await this.readThreadRecord(threadId)
      if (thread) {
        const summary = toThreadSummary(thread)
        summaries.push(summary)
        backfillSummaries[threadId] = summary
      }
    }
    if (Object.keys(backfillSummaries).length > 0) {
      await this.updateIndex((current) => ({
        order: current.order,
        summaries: {
          ...current.summaries,
          ...backfillSummaries
        },
        updatedAt: this.now().toISOString()
      }))
    }
    return summaries.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
  }

  async get(threadId: string): Promise<ThreadRecord | null> {
    return this.readThreadRecord(threadId)
  }

  async upsert(thread: ThreadRecord): Promise<ThreadRecord> {
    await this.ensureDir(this.threadDir(thread.id))
    const path = this.threadFilePath(thread.id)
    await this.atomicWrite(path, JSON.stringify(thread))
    const summary = toThreadSummary(thread)
    await this.updateIndex((current) => {
      const next = new Set(current.order)
      next.add(thread.id)
      return {
        order: [...next],
        summaries: {
          ...current.summaries,
          [thread.id]: summary
        },
        updatedAt: this.now().toISOString()
      }
    })
    return thread
  }

  async delete(threadId: string): Promise<boolean> {
    const dir = this.threadDir(threadId)
    try {
      await stat(dir)
    } catch {
      return false
    }
    await rm(dir, { recursive: true, force: true })
    await this.updateIndex((current) => {
      const order = current.order.filter((id) => id !== threadId)
      const summaries = { ...current.summaries }
      delete summaries[threadId]
      return { order, summaries, updatedAt: this.now().toISOString() }
    })
    return true
  }

  private async readIndex(): Promise<ThreadIndex> {
    try {
      const raw = await readFile(this.indexPath(), 'utf-8')
      const parsed = JSON.parse(raw) as { order?: unknown; updatedAt?: unknown; summaries?: unknown }
      const summaries = normalizeThreadIndexSummaries(parsed.summaries)
      return {
        order: Array.isArray(parsed.order)
          ? parsed.order.filter((value): value is string => typeof value === 'string' && value.trim().length > 0)
          : [],
        summaries,
        updatedAt: typeof parsed.updatedAt === 'string' ? parsed.updatedAt : this.now().toISOString()
      }
    } catch {
      return { order: [], summaries: {}, updatedAt: this.now().toISOString() }
    }
  }

  private async updateIndex(
    mutator: (current: ThreadIndex) => ThreadIndex
  ): Promise<void> {
    const run = this.indexQueue.catch(() => undefined).then(async () => {
      const current = await this.readIndex()
      const next = mutator(current)
      await this.ensureDir(this.dataDir)
      await this.atomicWrite(this.indexPath(), JSON.stringify(next))
    })
    this.indexQueue = run.then(() => undefined, () => undefined)
    await run
  }

  private threadDir(threadId: string): string {
    return join(this.dataDir, threadId)
  }

  private threadFilePath(threadId: string): string {
    return join(this.threadDir(threadId), 'thread.json')
  }

  private indexPath(): string {
    return join(this.dataDir, 'index.json')
  }

  private async readThreadRecord(threadId: string): Promise<ThreadRecord | null> {
    try {
      const raw = await readFile(this.threadFilePath(threadId), 'utf-8')
      return JSON.parse(raw) as ThreadRecord
    } catch {
      return null
    }
  }

  private async ensureDir(path: string): Promise<void> {
    await mkdir(path, { recursive: true })
  }

  private async atomicWrite(path: string, contents: string): Promise<void> {
    await atomicWriteFile(path, contents)
  }
}

/**
 * Helper used by the JSONL event store to enumerate disk content
 * during replay. Exposed for tests and the file session store.
 */
export async function readJsonl<T>(path: string): Promise<T[]> {
  try {
    const content = await readFile(path, 'utf-8')
    const out: T[] = []
    for (const line of content.split('\n')) {
      const trimmed = line.trim()
      if (!trimmed) continue
      try {
        out.push(JSON.parse(trimmed) as T)
      } catch {
        // Skip malformed lines so a single bad record does not poison
        // the whole replay.
      }
    }
    return out
  } catch {
    return []
  }
}

/** Re-export so other files in the package can import through a single path. */
export { readdir }

function normalizeThreadIndexSummaries(value: unknown): Record<string, ThreadSummary> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const summaries: Record<string, ThreadSummary> = {}
  for (const [threadId, rawSummary] of Object.entries(value)) {
    const summary = normalizeThreadIndexSummary(rawSummary, threadId)
    if (summary) summaries[threadId] = summary
  }
  return summaries
}

function normalizeThreadIndexSummary(value: unknown, fallbackId: string): ThreadSummary | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const summary = value as Partial<ThreadSummary>
  const id = fallbackId.trim()
  if (
    !id.trim() ||
    typeof summary.title !== 'string' ||
    typeof summary.workspace !== 'string' ||
    typeof summary.model !== 'string' ||
    typeof summary.mode !== 'string' ||
    typeof summary.status !== 'string' ||
    typeof summary.createdAt !== 'string' ||
    typeof summary.updatedAt !== 'string'
  ) {
    return null
  }
  return {
    ...summary,
    id
  } as ThreadSummary
}
