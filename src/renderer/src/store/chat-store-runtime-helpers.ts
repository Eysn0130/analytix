import type {
  ChatBlock,
  NormalizedThread,
  RuntimeDisclosureMetadata,
  UserMessageEventPayload,
  UserInputStatusPayload
} from '../agent/types'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import i18n from '../i18n'
import { normalizeWorkspaceRoot } from '../lib/workspace-path'
import { shouldAutoTitleThread } from '../lib/thread-title'
import type { ChatState, UserInputDraftInterruption } from './chat-store-types'
import { userInputLocalSelectorSchemaV1 } from '../../../../packages/runtime/src/contracts/user-input-local-display'

type ThreadDetailProviderLike = {
  getThreadDetail: (threadId: string) => Promise<{ blocks: ChatBlock[] }>
}

type UserInputBlock = Extract<ChatBlock, { kind: 'user_input' }>

/** Request telemetry belongs to diagnostics, never an ordinary transcript seat. */
export function isNeutralProviderRequestStage(stage: unknown): boolean {
  return stage === 'pre_send' || stage === 'post_send' || stage === 'response_received'
}

function hasUserInputIdentity(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

/** Resolve only a unique observed pair; item and request namespaces cannot alias another block. */
export function findUserInputBlockIndex(blocks: ChatBlock[], itemId: string, requestId: string): number {
  if (!hasUserInputIdentity(itemId) || !hasUserInputIdentity(requestId)) return -1
  const related = blocks.flatMap((block, index) =>
    block.id === itemId || block.id === requestId ||
    block.kind === 'user_input' && (block.requestId === requestId || block.requestId === itemId)
      ? [index] : [])
  if (related.length !== 1) return -1
  const index = related[0]
  const block = blocks[index]
  return block.kind === 'user_input' && block.id === itemId && block.requestId === requestId ? index : -1
}

/** A missing legacy request field is usable only when its observed item alias is unique. */
export function findUserInputStatusBlockIndex(blocks: ChatBlock[], event: UserInputStatusPayload): number {
  if (!hasUserInputIdentity(event.itemId) ||
      event.requestId !== undefined && !hasUserInputIdentity(event.requestId)) return -1
  const candidates = blocks.flatMap((block, index) =>
    block.kind === 'user_input' && (event.requestId !== undefined
      ? block.requestId === event.requestId
      : block.id === event.itemId || block.requestId === event.itemId) ? [index] : [])
  if (candidates.length !== 1) return -1
  const index = candidates[0]
  const block = blocks[index]
  if (block.kind !== 'user_input' ||
      event.itemId !== block.id && event.itemId !== block.requestId ||
      findUserInputBlockIndex(blocks, block.id, block.requestId) !== index) return -1
  if ((block.status === 'submitted' || block.status === 'cancelled' || block.status === 'continued') && block.status !== event.status) return -1
  return index
}

/** Repeated requests can refresh their own pending card, never replace a different observed identity. */
export function upsertUserInputBlock(blocks: ChatBlock[], incoming: UserInputBlock): ChatBlock[] {
  if (!hasUserInputIdentity(incoming.id) || !hasUserInputIdentity(incoming.requestId)) return blocks
  const related = blocks.some(block => block.id === incoming.id || block.id === incoming.requestId ||
    block.kind === 'user_input' && (block.requestId === incoming.requestId || block.requestId === incoming.id))
  if (!related) return [...blocks, incoming]
  const index = findUserInputBlockIndex(blocks, incoming.id, incoming.requestId)
  if (index < 0) return blocks
  const current = blocks[index]
  if (current.kind !== 'user_input' || current.status !== 'pending') return blocks
  const next: UserInputBlock = {
    ...current,
    ...incoming,
    createdAt: current.createdAt ?? incoming.createdAt,
    status: incoming.status,
    questions: incoming.questions,
    answers: incoming.answers ?? current.answers,
    errorMessage: incoming.errorMessage ?? current.errorMessage,
    meta: { ...(current.meta ?? {}), ...(incoming.meta ?? {}) }
  }
  const updated = [...blocks]
  updated[index] = next
  return updated
}

export function threadBelongsToWorkspace(
  thread: { workspace?: string },
  workspaceRoot: string
): boolean {
  const normalizedWorkspace = normalizeWorkspaceRoot(workspaceRoot)
  if (!normalizedWorkspace) return false
  return normalizeWorkspaceRoot(thread.workspace) === normalizedWorkspace
}

export function hasPendingRuntimeWork(block: ChatBlock): boolean {
  if (block.kind === 'tool') return block.status === 'running'
  if (block.kind === 'compaction') return block.status === 'running'
  if (block.kind === 'review') return block.status === 'running'
  if (block.kind === 'approval') return block.status === 'pending'
  if (block.kind === 'user_input') return block.status === 'pending'
  return false
}

function assistantBlockHasVisibleContent(block: Extract<ChatBlock, { kind: 'assistant' }>): boolean {
  const withoutThink = block.text.replace(/<think>[\s\S]*?(?:<\/think>|$)/g, '').trim()
  return withoutThink.length > 0
}

export function threadHasPendingRuntimeWork(blocks: ChatBlock[]): boolean {
  let pendingInCurrentTurn = false

  for (const block of blocks) {
    if (block.kind === 'user') {
      pendingInCurrentTurn = false
      continue
    }
    if (hasPendingRuntimeWork(block)) {
      pendingInCurrentTurn = true
      continue
    }
    if (pendingInCurrentTurn && block.kind === 'assistant' && assistantBlockHasVisibleContent(block)) {
      pendingInCurrentTurn = false
    }
  }

  return pendingInCurrentTurn
}

/** Retire only this terminal owner's live retry seat; independent diagnostics stay. */
export function retireProviderRetryStatusesForTurn(blocks: ChatBlock[], turnId: string): ChatBlock[] {
  if (!turnId.trim()) return blocks
  const retryCaptions = new Set([
    '模型服务正在重试',
    'Provider request is retrying',
    'providerRetryingStatus',
    'common:providerRetryingStatus',
    ...['en', 'zh', i18n.language].map(lng => i18n.t('common:providerRetryingStatus', { lng, defaultValue: '' }))
  ].filter(Boolean))
  let changed = false
  const next: ChatBlock[] = []
  for (const block of blocks) {
    if (block.kind !== 'system' || block.meta?.turnId !== turnId || !block.meta.providerRetry) {
      next.push(block)
      continue
    }
    changed = true
    const meta = { ...block.meta }
    delete meta.providerRetry
    const retryCaption = retryCaptions.has(block.text)
    const independent = block.severity !== undefined && block.severity !== 'info' || Boolean(block.code || block.detail) ||
      Object.entries(meta).some(([key, value]) => key !== 'turnId' && value !== undefined)
    if (retryCaption && !independent) continue
    next.push({ ...block, meta, ...(retryCaption ? { text: i18n.t('common:dshProviderRequestDetails') } : {}) })
  }
  return changed ? next : blocks
}

export function settlePendingRuntimeWorkAfterInterrupt(blocks: ChatBlock[]): ChatBlock[] {
  let changed = false
  const next = blocks.map((block): ChatBlock => {
    if (block.kind === 'tool' && block.status === 'running') {
      changed = true
      return { ...block, status: 'error' as const }
    }
    if (block.kind === 'compaction' && block.status === 'running') {
      changed = true
      return { ...block, status: 'error' as const }
    }
    if (block.kind === 'review' && block.status === 'running') {
      changed = true
      return { ...block, status: 'error' as const }
    }
    if (block.kind === 'approval' && block.status === 'pending') {
      changed = true
      return { ...block, status: 'error' as const }
    }
    if (block.kind === 'user_input' && block.status === 'pending') {
      changed = true
      return { ...block, status: 'error' as const, errorMessage: 'User input outcome is unavailable.' }
    }
    return block
  })
  return changed ? next : blocks
}

export function threadSnapshotLooksRunning(blocks: ChatBlock[], threadStatus?: string): boolean {
  if (threadStatus != null && threadStatus.trim()) {
    return runtimeStatusLooksRunning(threadStatus)
  }
  return threadHasPendingRuntimeWork(blocks)
}

export function findLatestUserBlockId(blocks: ChatBlock[]): string | null {
  for (let idx = blocks.length - 1; idx >= 0; idx -= 1) {
    const block = blocks[idx]
    if (block?.kind === 'user') return block.id
  }
  return null
}

export function upsertUserBlock(blocks: ChatBlock[], ev: UserMessageEventPayload): ChatBlock[] {
  const clientUserMessageId = ev.meta?.clientUserMessageId?.trim()
  const nextBlock: ChatBlock = {
    kind: 'user',
    id: ev.itemId,
    createdAt: ev.createdAt,
    text: projectOrdinaryPublicText(ev.text),
    ...(ev.source ? {source:{...ev.source}} : {}),
    ...(ev.modelLabel ? { modelLabel: ev.modelLabel } : {}),
    ...(ev.managedBy ? { managedBy: ev.managedBy } : {}),
    ...(ev.meta ? { meta: ev.meta } : {})
  }
  const existingIndex = blocks.findIndex((block) =>
    block.kind === 'user' &&
    (
      block.id === ev.itemId ||
      (clientUserMessageId && block.meta?.clientUserMessageId === clientUserMessageId)
    )
  )
  if (existingIndex < 0) return [...blocks, nextBlock]
  const current = blocks[existingIndex]
  if (!current || current.kind !== 'user') return blocks
  const isSteeringBlock = Boolean(
    clientUserMessageId ||
    current.meta?.clientUserMessageId ||
    current.meta?.steeringStatus
  )
  const meta = mergeRuntimeDisclosureMeta(
    current.meta,
    isSteeringBlock
      ? { ...(nextBlock.meta ?? {}), steeringStatus: 'accepted' as const }
      : nextBlock.meta
  )
  const merged: ChatBlock = {
    ...current,
    ...nextBlock,
    createdAt: current.createdAt ?? nextBlock.createdAt,
    ...(meta ? { meta } : {})
  }
  const next = [...blocks]
  next[existingIndex] = merged
  return next
}

function mergeRuntimeDisclosureMeta(
  current: RuntimeDisclosureMetadata | undefined,
  next: RuntimeDisclosureMetadata | undefined
): RuntimeDisclosureMetadata | undefined {
  if (!current && !next) return undefined
  return {
    ...(current ?? {}),
    ...(next ?? {})
  }
}

export function reconcileOptimisticUserBlock(
  blocks: ChatBlock[],
  optimisticId: string,
  runtimeId: string,
  fallbackText?: string,
  modelLabel?: string
): ChatBlock[] {
  return blocks.map((block) => {
    if (block.kind !== 'user' || block.id !== optimisticId) return block
    return {
      ...block,
      id: runtimeId,
      ...(fallbackText && !block.text.trim()
        ? { text: projectOrdinaryPublicText(fallbackText) }
        : {}),
      ...(modelLabel && !block.modelLabel ? { modelLabel } : {})
    }
  })
}

export function collectAssistantTextForTurn(
  blocks: ChatBlock[],
  userBlockId: string,
  liveAssistant: string
): string {
  const userIndex = blocks.findIndex((block) => block.kind === 'user' && block.id === userBlockId)
  if (userIndex < 0) return liveAssistant.trim()
  const parts: string[] = []
  for (let index = userIndex + 1; index < blocks.length; index += 1) {
    const block = blocks[index]
    if (block.kind === 'user') break
    if (block.kind === 'assistant' && block.text.trim()) {
      parts.push(block.text.trim())
    }
  }
  if (liveAssistant.trim()) parts.push(liveAssistant.trim())
  return parts.join('\n\n').trim()
}

export function clearedThreadSelection(): Pick<
  ChatState,
  | 'activeThreadId'
  | 'activeThreadGoal'
  | 'activeGoalActivation'
  | 'activeThreadTodos'
  | 'blocks'
  | 'lastSeq'
  | 'liveAssistant'
  | 'busy'
  | 'currentTurnId'
  | 'currentTurnUserId'
  | 'turnStartedAtByUserId'
  | 'turnDurationByUserId'
  | 'turnUsageById'
  | 'rewoundTurnIds'
  | 'lastTurnUsage'
  | 'inspectorSelectedId'
> {
  return {
    activeThreadId: null,
    activeThreadGoal: null,
    activeGoalActivation: null,
    activeThreadTodos: null,
    blocks: [],
    lastSeq: 0,
    liveAssistant: '',
    busy: false,
    currentTurnId: null,
    currentTurnUserId: null,
    turnStartedAtByUserId: {},
    turnDurationByUserId: {},
    turnUsageById: {},
    rewoundTurnIds: {},
    lastTurnUsage: null,
    inspectorSelectedId: null
  }
}

export async function findReusableEmptyThreadId(
  state: ChatState,
  provider: ThreadDetailProviderLike,
  workspaceRoot: string,
  isReusableThread: (thread: NormalizedThread) => boolean = () => true
): Promise<string | null> {
  const normalizedWorkspace = normalizeWorkspaceRoot(workspaceRoot)
  if (!normalizedWorkspace) return null

  const activeThread = state.activeThreadId
    ? state.threads.find((thread) => thread.id === state.activeThreadId)
    : null
  if (
    activeThread &&
    isReusableThread(activeThread) &&
    shouldAutoTitleThread(activeThread) &&
    normalizeWorkspaceRoot(activeThread.workspace) === normalizedWorkspace &&
    !threadHasUserMessage(state.blocks)
  ) {
    return activeThread.id
  }

  const candidates = state.threads
    .filter(
      (thread) =>
        thread.id !== activeThread?.id &&
        isReusableThread(thread) &&
        shouldAutoTitleThread(thread) &&
        normalizeWorkspaceRoot(thread.workspace) === normalizedWorkspace
    )
    .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))

  for (const thread of candidates) {
    try {
      const { blocks } = await provider.getThreadDetail(thread.id)
      if (!threadHasUserMessage(blocks)) return thread.id
    } catch {
      /* ignore and keep checking other candidates */
    }
  }

  return null
}

function runtimeStatusLooksRunning(status?: string): boolean {
  const normalized = status?.trim().toLowerCase()
  return normalized === 'running'
    || normalized === 'in_progress'
    || normalized === 'queued'
    || normalized === 'started'
}

function threadHasUserMessage(blocks: ChatBlock[]): boolean {
  return blocks.some((block) => block.kind === 'user')
}

/** Keep only bounded Question save warning metadata across a normal leaf detach. */
export function updateUserInputDraftInterruption(
  current: Record<string, UserInputDraftInterruption> | undefined, key: string, value: UserInputDraftInterruption | null,
  deletedThreadIds: Record<string, true> | undefined = undefined
): Record<string, UserInputDraftInterruption> {
  const entries = current ?? {}
  try {
    const parsed = userInputLocalSelectorSchemaV1.safeParse(JSON.parse(key))
    if (!parsed.success || JSON.stringify(parsed.data) !== key) return entries
    if (value !== null && deletedThreadIds?.[parsed.data.threadId]) return entries
    if (value === null) {
      if (!Object.prototype.hasOwnProperty.call(entries, key)) return entries
      const next = { ...entries }; delete next[key]; return next
    }
    if (typeof value.unsent !== 'boolean' || value.revision !== null &&
        (typeof value.revision !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value.revision))) return entries
    const next = { ...entries }; delete next[key]
    // Match the Host's existing signed-draft bound; no raw draft, timer or
    // capability is placed in this metadata-only record or browser storage.
    while (Object.keys(next).length >= 4096) delete next[Object.keys(next)[0]]
    next[key] = { revision: value.revision, unsent: value.unsent }
    return next
  } catch { return entries }
}

export function forgetUserInputDraftInterruptionsForThread(
  current: Record<string, UserInputDraftInterruption> | undefined, threadId: string
): Record<string, UserInputDraftInterruption> {
  return Object.fromEntries(Object.entries(current ?? {}).filter(([key]) => {
    try {
      const parsed = userInputLocalSelectorSchemaV1.safeParse(JSON.parse(key))
      return parsed.success && parsed.data.threadId !== threadId
    } catch { return false }
  }))
}

/** A confirmed deletion prevents a former mounted leaf from recreating its warning. */
export function markDeletedUserInputDraftThread(current: Record<string, true> | undefined, threadId: string): Record<string, true> {
  const next = { ...current }; delete next[threadId]
  while (Object.keys(next).length >= 4096) delete next[Object.keys(next)[0]]
  next[threadId] = true
  return next
}
