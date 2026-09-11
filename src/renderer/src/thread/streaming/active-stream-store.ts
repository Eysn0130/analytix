import { useSyncExternalStore } from 'react'
import {
  createThreadTraceEvent,
  PersistedThreadTraceSink
} from '../tracing/thread-performance-trace'

export type ActiveStreamSnapshot = {
  threadId: string | null
  turnId: string | null
  liveAssistant: string
  liveAssistantContent: string
  lastSeq: number
  startedAt: number | null
  version: number
}

export type ActiveStreamMetrics = {
  threadId: string | null
  turnId: string | null
  assistantLength: number
  processLength: number
  contentLength: number
  lastSeq: number
  startedAt: number | null
  version: number
}

export type ActiveStreamEstimateMetrics = {
  threadId: string | null
  turnId: string | null
  processLength: number
  contentLength: number
  processBucket: number
  contentBucket: number
  version: number
}

const EMPTY_SNAPSHOT: ActiveStreamSnapshot = {
  threadId: null,
  turnId: null,
  liveAssistant: '',
  liveAssistantContent: '',
  lastSeq: 0,
  startedAt: null,
  version: 0
}

const EMPTY_METRICS: ActiveStreamMetrics = {
  threadId: null,
  turnId: null,
  assistantLength: 0,
  processLength: 0,
  contentLength: 0,
  lastSeq: 0,
  startedAt: null,
  version: 0
}

const EMPTY_ESTIMATE_METRICS: ActiveStreamEstimateMetrics = {
  threadId: null,
  turnId: null,
  processLength: 0,
  contentLength: 0,
  processBucket: 0,
  contentBucket: 0,
  version: 0
}

const PENDING_TURN_PREFIX = 'pending:'
const ESTIMATED_CHARS_PER_LINE = 86

type StreamKey = {
  threadId: string
  turnId: string
}

type ActiveStreamState = {
  threadId: string
  turnId: string
  assistantChunks: string[]
  assistantSplit: AssistantThinkSplitState
  assistantLength: number
  liveAssistantCache: string | null
  snapshot: ActiveStreamSnapshot | null
  metrics: ActiveStreamMetrics | null
  estimateMetrics: ActiveStreamEstimateMetrics | null
  lastSeq: number
  startedAt: number | null
  version: number
}

type AssistantThinkSplitMode = 'pending' | 'content'

type AssistantThinkSplitState = {
  mode: AssistantThinkSplitMode
  contentChunks: string[]
  contentLength: number
  contentCache: string | null
}

const streamsByThread = new Map<string, Map<string, ActiveStreamState>>()
const listeners = new Set<() => void>()
const traceSink = new PersistedThreadTraceSink()
let latestKey: StreamKey | null = null
let version = 0

function emit(): void {
  listeners.forEach((listener) => listener())
}

function traceStreamUpdate(state: ActiveStreamState, reason: number): void {
  traceSink.record(
    createThreadTraceEvent('thread.active_stream.updated', {
      threadId: state.threadId,
      data: {
        reason,
        version: state.version,
        lastSeq: state.lastSeq,
        assistantChars: state.assistantLength
      }
    })
  )
}

function normalize(value: string | null | undefined): string | null {
  return value?.trim() || null
}

export function pendingActiveStreamTurnId(
  threadId: string | null | undefined,
  userId?: string | null | undefined
): string {
  const normalizedThreadId = normalize(threadId) ?? 'thread'
  const normalizedUserId = normalize(userId) ?? 'turn'
  return `${PENDING_TURN_PREFIX}${normalizedThreadId}:${normalizedUserId}`
}

export function isPendingActiveStreamTurnId(turnId: string | null | undefined): boolean {
  return normalize(turnId)?.startsWith(PENDING_TURN_PREFIX) === true
}

function createAssistantThinkSplitState(): AssistantThinkSplitState {
  return {
    mode: 'pending',
    contentChunks: [],
    contentLength: 0,
    contentCache: ''
  }
}

function appendContentChunk(state: AssistantThinkSplitState, text: string): void {
  if (!text) return
  state.contentChunks.push(text)
  state.contentLength += text.length
  state.contentCache = null
}

function materializeChunks(chunks: string[], cache: string | null): string {
  return cache ?? chunks.join('')
}

function materializeContentText(state: AssistantThinkSplitState): string {
  const text = materializeChunks(state.contentChunks, state.contentCache)
  state.contentCache = text
  return text
}
function buildAssistantThinkSplitFromChunks(chunks: readonly string[]): AssistantThinkSplitState {
  const state = createAssistantThinkSplitState()
  const publicText = chunks.join('')
  if (publicText) {
    state.mode = 'content'
    appendContentChunk(state, publicText)
  }
  return state
}

function streamAssistantText(state: ActiveStreamState): string {
  const liveAssistant = state.liveAssistantCache ?? materializeContentText(state.assistantSplit)
  state.liveAssistantCache = liveAssistant
  return liveAssistant
}

function assistantDisplayContent(state: ActiveStreamState): string {
  const split = state.assistantSplit
  if (split.mode === 'pending') return ''
  return materializeContentText(split)
}

function assistantDisplayContentLength(state: ActiveStreamState): number {
  const split = state.assistantSplit
  if (split.mode === 'pending') return 0
  return split.contentLength
}

function findLatestKey(): StreamKey | null {
  let latest: ActiveStreamState | null = null
  for (const turnSnapshots of streamsByThread.values()) {
    for (const candidate of turnSnapshots.values()) {
      if (!latest || candidate.version > latest.version) latest = candidate
    }
  }
  if (!latest) return null
  return { threadId: latest.threadId, turnId: latest.turnId }
}

function getThreadStreams(threadId: string): Map<string, ActiveStreamState> {
  let threadSnapshots = streamsByThread.get(threadId)
  if (!threadSnapshots) {
    threadSnapshots = new Map()
    streamsByThread.set(threadId, threadSnapshots)
  }
  return threadSnapshots
}

function streamSnapshot(state: ActiveStreamState): ActiveStreamSnapshot {
  if (state.snapshot?.version === state.version) return state.snapshot
  const liveAssistant = streamAssistantText(state)
  state.snapshot = {
    threadId: state.threadId,
    turnId: state.turnId,
    liveAssistant,
    liveAssistantContent: assistantDisplayContent(state),
    lastSeq: state.lastSeq,
    startedAt: state.startedAt,
    version: state.version
  }
  return state.snapshot
}

function streamMetrics(state: ActiveStreamState): ActiveStreamMetrics {
  if (state.metrics?.version === state.version) return state.metrics
  const contentLength = assistantDisplayContentLength(state)
  state.metrics = {
    threadId: state.threadId,
    turnId: state.turnId,
    assistantLength: state.assistantLength,
    processLength: 0,
    contentLength,
    lastSeq: state.lastSeq,
    startedAt: state.startedAt,
    version: state.version
  }
  return state.metrics
}

function streamEstimateMetrics(state: ActiveStreamState): ActiveStreamEstimateMetrics {
  const processLength = 0
  const contentLength = assistantDisplayContentLength(state)
  const processBucket = Math.max(0, Math.ceil(processLength / ESTIMATED_CHARS_PER_LINE))
  const contentBucket = Math.max(0, Math.ceil(contentLength / ESTIMATED_CHARS_PER_LINE))
  const previous = state.estimateMetrics
  if (
    previous &&
    previous.processBucket === processBucket &&
    previous.contentBucket === contentBucket
  ) {
    return previous
  }
  state.estimateMetrics = {
    threadId: state.threadId,
    turnId: state.turnId,
    processLength,
    contentLength,
    processBucket,
    contentBucket,
    version: state.version
  }
  return state.estimateMetrics
}

function replaceStream(input: Omit<ActiveStreamSnapshot, 'version' | 'liveAssistantContent'>): void {
  const threadId = normalize(input.threadId)
  const turnId = normalize(input.turnId)
  if (!threadId || !turnId) return
  version += 1
	const assistantSplit = createAssistantThinkSplitState()
	const publicAssistant = ''
	const nextState: ActiveStreamState = {
    threadId,
    turnId,
	assistantChunks: publicAssistant ? [publicAssistant] : [],
	assistantSplit,
	assistantLength: publicAssistant.length,
	liveAssistantCache: publicAssistant,
    snapshot: null,
    metrics: null,
    estimateMetrics: null,
    lastSeq: input.lastSeq,
    startedAt: input.startedAt,
    version
  }
  getThreadStreams(threadId).set(turnId, nextState)
  latestKey = { threadId, turnId }
  traceStreamUpdate(nextState, 1)
  emit()
}

export function getActiveStreamSnapshot(): ActiveStreamSnapshot {
  if (!latestKey) return EMPTY_SNAPSHOT
  const state = streamsByThread.get(latestKey.threadId)?.get(latestKey.turnId)
  return state ? streamSnapshot(state) : EMPTY_SNAPSHOT
}

export function getActiveStreamSnapshotFor(
  threadId: string | null | undefined,
  turnId?: string | null | undefined
): ActiveStreamSnapshot {
  const normalizedThreadId = normalize(threadId)
  const normalizedTurnId = normalize(turnId)
  if (!normalizedThreadId) return EMPTY_SNAPSHOT
  const threadSnapshots = streamsByThread.get(normalizedThreadId)
  if (!threadSnapshots) return EMPTY_SNAPSHOT
  if (normalizedTurnId) {
    const state = threadSnapshots.get(normalizedTurnId)
    return state ? streamSnapshot(state) : EMPTY_SNAPSHOT
  }
  if (latestKey?.threadId === normalizedThreadId) {
    const state = threadSnapshots.get(latestKey.turnId)
    return state ? streamSnapshot(state) : EMPTY_SNAPSHOT
  }
  let latest: ActiveStreamState | null = null
  for (const candidate of threadSnapshots.values()) {
    if (!latest || candidate.version > latest.version) latest = candidate
  }
  return latest ? streamSnapshot(latest) : EMPTY_SNAPSHOT
}

export function getActiveStreamMetricsFor(
  threadId: string | null | undefined,
  turnId?: string | null | undefined
): ActiveStreamMetrics {
  const normalizedThreadId = normalize(threadId)
  const normalizedTurnId = normalize(turnId)
  if (!normalizedThreadId) return EMPTY_METRICS
  const threadSnapshots = streamsByThread.get(normalizedThreadId)
  if (!threadSnapshots) return EMPTY_METRICS
  if (normalizedTurnId) {
    const state = threadSnapshots.get(normalizedTurnId)
    return state ? streamMetrics(state) : EMPTY_METRICS
  }
  if (latestKey?.threadId === normalizedThreadId) {
    const state = threadSnapshots.get(latestKey.turnId)
    return state ? streamMetrics(state) : EMPTY_METRICS
  }
  let latest: ActiveStreamState | null = null
  for (const candidate of threadSnapshots.values()) {
    if (!latest || candidate.version > latest.version) latest = candidate
  }
  return latest ? streamMetrics(latest) : EMPTY_METRICS
}

export function getActiveStreamEstimateMetricsFor(
  threadId: string | null | undefined,
  turnId?: string | null | undefined
): ActiveStreamEstimateMetrics {
  const normalizedThreadId = normalize(threadId)
  const normalizedTurnId = normalize(turnId)
  if (!normalizedThreadId) return EMPTY_ESTIMATE_METRICS
  const threadSnapshots = streamsByThread.get(normalizedThreadId)
  if (!threadSnapshots) return EMPTY_ESTIMATE_METRICS
  if (normalizedTurnId) {
    const state = threadSnapshots.get(normalizedTurnId)
    return state ? streamEstimateMetrics(state) : EMPTY_ESTIMATE_METRICS
  }
  if (latestKey?.threadId === normalizedThreadId) {
    const state = threadSnapshots.get(latestKey.turnId)
    return state ? streamEstimateMetrics(state) : EMPTY_ESTIMATE_METRICS
  }
  let latest: ActiveStreamState | null = null
  for (const candidate of threadSnapshots.values()) {
    if (!latest || candidate.version > latest.version) latest = candidate
  }
  return latest ? streamEstimateMetrics(latest) : EMPTY_ESTIMATE_METRICS
}

export function subscribeActiveStream(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function resetActiveStream(threadId: string | null, options: {
  turnId?: string | null
  liveAssistant?: string
  lastSeq?: number
  startedAt?: number | null
} = {}): void {
  const normalizedThreadId = normalize(threadId)
  const normalizedTurnId = normalize(options.turnId)
  if (!normalizedThreadId) {
    clearActiveStream()
    return
  }
  if (!normalizedTurnId) {
    clearActiveStream(normalizedThreadId)
    return
  }
  replaceStream({
    threadId: normalizedThreadId,
    turnId: normalizedTurnId,
    liveAssistant: options.liveAssistant ?? '',
    lastSeq: options.lastSeq ?? 0,
    startedAt: options.startedAt ?? null
  })
}

export function appendActiveStreamDeltas(input: {
  threadId: string | null | undefined
  turnId: string | null | undefined
  assistant?: string
  lastSeq?: number
  startedAt?: number | null
}): void {
  const threadId = normalize(input.threadId)
  const turnId = normalize(input.turnId)
  if (!threadId || !turnId) return
  const existing = streamsByThread.get(threadId)?.get(turnId)
  const lastSeq = Math.max(existing?.lastSeq ?? 0, input.lastSeq ?? 0)
  if (existing?.lastSeq === lastSeq) return
  if (!existing) {
    replaceStream({
      threadId,
      turnId,
      liveAssistant: '',
      lastSeq,
      startedAt: input.startedAt ?? Date.now()
    })
    return
  }
  existing.lastSeq = lastSeq
  existing.startedAt = existing.startedAt ?? input.startedAt ?? Date.now()
  existing.version = ++version
  existing.snapshot = null
  existing.metrics = null
  latestKey = { threadId, turnId }
  traceStreamUpdate(existing, 2)
  emit()
}

export function migrateActiveStreamTurn(
  threadId: string | null | undefined,
  fromTurnId: string | null | undefined,
  toTurnId: string | null | undefined
): void {
  const normalizedThreadId = normalize(threadId)
  const normalizedFromTurnId = normalize(fromTurnId)
  const normalizedToTurnId = normalize(toTurnId)
  if (!normalizedThreadId || !normalizedFromTurnId || !normalizedToTurnId) return
  if (normalizedFromTurnId === normalizedToTurnId) return

  const threadSnapshots = streamsByThread.get(normalizedThreadId)
  const source = threadSnapshots?.get(normalizedFromTurnId)
  if (!source) return
  const target = threadSnapshots?.get(normalizedToTurnId)
  threadSnapshots?.delete(normalizedFromTurnId)
  version += 1
  if (!target) {
    source.turnId = normalizedToTurnId
    source.version = version
    source.snapshot = null
    source.metrics = null
    source.estimateMetrics = null
    getThreadStreams(normalizedThreadId).set(normalizedToTurnId, source)
    latestKey = { threadId: normalizedThreadId, turnId: normalizedToTurnId }
    traceStreamUpdate(source, 3)
    emit()
    return
  }
  const nextState: ActiveStreamState = {
    threadId: normalizedThreadId,
    turnId: normalizedToTurnId,
    // Pending buckets receive early deltas before the durable runtime turn id
    // arrives. If the durable bucket already exists, its chunks are later in
    // the same turn, so append them after the pending source to preserve the
    // visible stream order.
    assistantChunks: [...source.assistantChunks, ...target.assistantChunks],
    assistantSplit: buildAssistantThinkSplitFromChunks([...source.assistantChunks, ...target.assistantChunks]),
    assistantLength: target.assistantLength + source.assistantLength,
    liveAssistantCache: null,
    snapshot: null,
    metrics: null,
    estimateMetrics: null,
    lastSeq: Math.max(target.lastSeq, source.lastSeq),
    startedAt: target.startedAt ?? source.startedAt,
    version
  }
  getThreadStreams(normalizedThreadId).set(normalizedToTurnId, nextState)
  latestKey = { threadId: normalizedThreadId, turnId: normalizedToTurnId }
  traceStreamUpdate(nextState, 3)
  emit()
}

export function clearActiveStream(threadId?: string | null, turnId?: string | null): void {
  const normalizedThreadId = normalize(threadId)
  const normalizedTurnId = normalize(turnId)
  let changed = false
  if (!normalizedThreadId) {
    changed = streamsByThread.size > 0
    streamsByThread.clear()
  } else if (!normalizedTurnId) {
    changed = streamsByThread.delete(normalizedThreadId)
  } else {
    const threadSnapshots = streamsByThread.get(normalizedThreadId)
    if (threadSnapshots?.delete(normalizedTurnId)) {
      changed = true
      if (threadSnapshots.size === 0) streamsByThread.delete(normalizedThreadId)
    }
  }
  if (!changed) return
  latestKey = findLatestKey()
  version += 1
  emit()
}

export function useActiveStreamSnapshot(
  threadId: string | null | undefined,
  turnId?: string | null | undefined
): ActiveStreamSnapshot {
  const normalizedThreadId = threadId?.trim() || null
  const normalizedTurnId = turnId?.trim() || null
  return useSyncExternalStore(
    subscribeActiveStream,
    () => normalizedThreadId ? getActiveStreamSnapshotFor(normalizedThreadId, normalizedTurnId) : EMPTY_SNAPSHOT,
    () => normalizedThreadId ? getActiveStreamSnapshotFor(normalizedThreadId, normalizedTurnId) : EMPTY_SNAPSHOT
  )
}

export function useActiveStreamMetrics(
  threadId: string | null | undefined,
  turnId?: string | null | undefined
): ActiveStreamMetrics {
  const normalizedThreadId = threadId?.trim() || null
  const normalizedTurnId = turnId?.trim() || null
  return useSyncExternalStore(
    subscribeActiveStream,
    () => normalizedThreadId ? getActiveStreamMetricsFor(normalizedThreadId, normalizedTurnId) : EMPTY_METRICS,
    () => normalizedThreadId ? getActiveStreamMetricsFor(normalizedThreadId, normalizedTurnId) : EMPTY_METRICS
  )
}

export function useActiveStreamEstimateMetrics(
  threadId: string | null | undefined,
  turnId?: string | null | undefined
): ActiveStreamEstimateMetrics {
  const normalizedThreadId = threadId?.trim() || null
  const normalizedTurnId = turnId?.trim() || null
  return useSyncExternalStore(
    subscribeActiveStream,
    () => normalizedThreadId ? getActiveStreamEstimateMetricsFor(normalizedThreadId, normalizedTurnId) : EMPTY_ESTIMATE_METRICS,
    () => normalizedThreadId ? getActiveStreamEstimateMetricsFor(normalizedThreadId, normalizedTurnId) : EMPTY_ESTIMATE_METRICS
  )
}
