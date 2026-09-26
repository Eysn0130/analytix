import { createHash } from 'node:crypto'
import { mkdir, readdir, rename, rm, stat, writeFile } from 'node:fs/promises'
import { basename, join } from 'node:path'
import type {
  ThreadTraceDataValue,
  ThreadTraceEventName,
  ThreadTraceEventPayload
} from '../../shared/thread-trace'

export type ThreadTraceWriteResult =
  | { ok: true; path: string }
  | { ok: false; message: string }

const TRACE_FLUSH_INTERVAL_MS = 500
const THREAD_TRACE_FILE_MAX_BYTES = 20 * 1024 * 1024
const THREAD_TRACE_FILE_MAX_COUNT = 5
const GLOBAL_TRACE_MAX_BYTES = 512 * 1024 * 1024

const THREAD_TRACE_DATA_KEYS = {
  'thread.event.batch_received': ['seq'],
  'thread.delta.buffered': ['deltas', 'renderer_delta_buffered_at'],
  'thread.delta.flushed': [
    'assistantChars',
    'lastSeq',
    'turnId',
    'renderer_delta_flushed_at',
    'renderer_live_painted_at'
  ],
  'thread.active_stream.updated': ['reason', 'version', 'lastSeq', 'assistantChars'],
  'thread.projection.reduced': ['blocks', 'turns', 'rows', 'live'],
  'thread.rows.updated': ['rows', 'renderedRows', 'virtualized', 'totalHeight'],
  'thread.virtualizer.measured': ['measurements'],
  'thread.scroll.anchor_corrected': [
    'previousTop',
    'nextTop',
    'delta',
    'bottomDistance',
    'streaming',
    'prepend',
    'reset'
  ],
  'thread.react.commit_sample': [
    'actualDuration',
    'baseDuration',
    'commitLatency',
    'mount',
    'rows',
    'renderedRows',
    'virtualized'
  ],
  'thread.markdown.finalized': [
    'chars',
    'codeFences',
    'estimatedLines',
    'codeBlockChars',
    'deferred'
  ],
  'thread.terminal.verified': ['lastSeq', 'verificationMs', 'acceptedFinal', 'monotonicMs', 'timeOrigin'],
  'thread.terminal.ipc_sent': ['lastSeq', 'acceptedFinal', 'monotonicMs', 'timeOrigin'],
  'thread.terminal.renderer_committed': ['lastSeq', 'acceptedFinal', 'monotonicMs', 'timeOrigin'],
  'thread.terminal.dom_committed': ['lastSeq', 'acceptedFinal', 'monotonicMs', 'timeOrigin', 'storeToDomMs', 'documentVisible'],
  'thread.terminal.next_frame': ['lastSeq', 'acceptedFinal']
} as const satisfies Record<ThreadTraceEventName, readonly string[]>

type PendingTraceFile = {
  timer: NodeJS.Timeout | null
  chunks: string[]
}

const pendingTraceFiles = new Map<string, PendingTraceFile>()

function traceEnabled(): boolean {
  const value = process.env.ANALYTIX_THREAD_TRACE?.trim().toLowerCase()
  return value === '1' || value === 'true' || value === 'yes'
}

function traceDir(userDataDir: string): string {
  return join(userDataDir, 'traces')
}

function threadTraceRef(threadId: string | undefined): string | undefined {
  const normalized = threadId?.trim()
  if (!normalized) return undefined
  return `ref-${createHash('sha256').update(normalized).digest('hex')}`
}

function projectThreadTraceEvent(event: ThreadTraceEventPayload): ThreadTraceEventPayload {
  const data: Record<string, ThreadTraceDataValue> = {}
  for (const key of THREAD_TRACE_DATA_KEYS[event.name]) {
    const value = event.data?.[key]
    if (typeof value === 'number' && Number.isFinite(value)) {
      data[key] = value
    } else if (typeof value === 'boolean' || value === null) {
      data[key] = value
    }
  }
  const threadId = threadTraceRef(event.threadId)
  const turnId = threadTraceRef(event.turnId)
  return {
    name: event.name,
    timestamp: event.timestamp,
    ...(threadId ? { threadId } : {}),
    ...(turnId ? { turnId } : {}),
    ...(Object.keys(data).length > 0 ? { data } : {})
  }
}

function traceFileStem(threadId: string | undefined): string {
  const safe = (threadId?.trim() || 'unknown')
    .replace(/[^a-zA-Z0-9_.-]/g, '_')
    .slice(0, 128)
  return `thread-${safe || 'unknown'}`
}

function traceFileName(threadId: string | undefined): string {
  return `${traceFileStem(threadId)}.jsonl`
}

function rotatedTraceFileName(stem: string, index: number): string {
  return `${stem}.${index}.jsonl`
}

async function fileSize(path: string): Promise<number> {
  try {
    return (await stat(path)).size
  } catch {
    return 0
  }
}

async function rotateThreadTraceFiles(dir: string, path: string): Promise<void> {
  const size = await fileSize(path)
  if (size < THREAD_TRACE_FILE_MAX_BYTES) return

  const stem = basename(path, '.jsonl')
  await rm(join(dir, rotatedTraceFileName(stem, THREAD_TRACE_FILE_MAX_COUNT - 1)), { force: true })
  for (let index = THREAD_TRACE_FILE_MAX_COUNT - 2; index >= 1; index -= 1) {
    const from = join(dir, rotatedTraceFileName(stem, index))
    const to = join(dir, rotatedTraceFileName(stem, index + 1))
    if (await fileSize(from) > 0) {
      await rename(from, to).catch(() => undefined)
    }
  }
  await rename(path, join(dir, rotatedTraceFileName(stem, 1))).catch(() => undefined)
}

export async function pruneThreadTraces(userDataDir: string): Promise<void> {
  const dir = traceDir(userDataDir)
  let entries: Array<{ path: string; size: number; mtimeMs: number }> = []
  try {
    entries = await Promise.all(
      (await readdir(dir, { withFileTypes: true }))
        .filter((entry) => entry.isFile() && entry.name.endsWith('.jsonl'))
        .map(async (entry) => {
          const path = join(dir, entry.name)
          const info = await stat(path)
          return { path, size: info.size, mtimeMs: info.mtimeMs }
        })
    )
  } catch {
    return
  }

  let total = entries.reduce((sum, entry) => sum + entry.size, 0)
  if (total <= GLOBAL_TRACE_MAX_BYTES) return
  entries.sort((a, b) => a.mtimeMs - b.mtimeMs)
  for (const entry of entries) {
    if (total <= GLOBAL_TRACE_MAX_BYTES) break
    await rm(entry.path, { force: true }).catch(() => undefined)
    total -= entry.size
  }
}

async function flushTraceFile(userDataDir: string, path: string): Promise<void> {
  const pending = pendingTraceFiles.get(path)
  if (!pending) return
  pendingTraceFiles.delete(path)
  if (pending.timer) clearTimeout(pending.timer)
  if (pending.chunks.length === 0) return

  const dir = traceDir(userDataDir)
  try {
    await mkdir(dir, { recursive: true })
    await rotateThreadTraceFiles(dir, path)
    await writeFile(path, pending.chunks.join(''), { encoding: 'utf8', flag: 'a' })
    await rotateThreadTraceFiles(dir, path)
    void pruneThreadTraces(userDataDir)
  } catch {
    // Diagnostics must never affect the app hot path.
  }
}

export async function flushThreadTraceEventsForTests(userDataDir: string): Promise<void> {
  await Promise.all([...pendingTraceFiles.keys()].map((path) => flushTraceFile(userDataDir, path)))
}

export function resetThreadTraceWriterForTests(): void {
  for (const pending of pendingTraceFiles.values()) {
    if (pending.timer) clearTimeout(pending.timer)
  }
  pendingTraceFiles.clear()
}

export async function appendThreadTraceEvent(
  userDataDir: string,
  event: ThreadTraceEventPayload
): Promise<ThreadTraceWriteResult> {
  if (!traceEnabled()) {
    return { ok: false, message: 'thread trace disabled' }
  }

  const projectedEvent = projectThreadTraceEvent(event)
  const fileName = traceFileName(projectedEvent.threadId)
  const dir = traceDir(userDataDir)
  const tracePath = join(dir, fileName)
  let pending = pendingTraceFiles.get(tracePath)
  if (!pending) {
    pending = { timer: null, chunks: [] }
    pendingTraceFiles.set(tracePath, pending)
  }
  pending.chunks.push(`${JSON.stringify(projectedEvent)}\n`)
  if (!pending.timer) {
    pending.timer = setTimeout(() => {
      void flushTraceFile(userDataDir, tracePath)
    }, TRACE_FLUSH_INTERVAL_MS)
    pending.timer.unref?.()
  }
  return { ok: true, path: `traces/${fileName}` }
}
