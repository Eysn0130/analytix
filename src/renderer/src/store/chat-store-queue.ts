import type { ChatBlock } from '../agent/types'
import type { ChatState, QueuedMessagePauseReason, QueuedUserMessage } from './chat-store-types'

type QueueState = Pick<ChatState, 'activeThreadId' | 'queuedMessages' | 'queuedMessagePauses'>
const emptyQueue: QueuedUserMessage[] = Object.freeze([]) as unknown as QueuedUserMessage[]
const views = new WeakMap<QueuedUserMessage[], Map<string, QueuedUserMessage[]>>()

/** A stable selected-thread view; unowned legacy entries never acquire a thread. */
export function queuedMessagesForThread(state: QueueState, threadId = state.activeThreadId): QueuedUserMessage[] {
  if (!threadId) return emptyQueue
  let byThread = views.get(state.queuedMessages)
  if (!byThread) { byThread = new Map(); views.set(state.queuedMessages, byThread) }
  let messages = byThread.get(threadId)
  if (!messages) {
    messages = state.queuedMessages.filter(message => message.threadId === threadId)
    byThread.set(threadId, messages)
  }
  return messages
}

export function queuedPauseReason(state: QueueState, threadId = state.activeThreadId): QueuedMessagePauseReason {
  return threadId ? state.queuedMessagePauses?.[threadId] ?? null : null
}

export function queuePausePatch(state: QueueState, reason: QueuedMessagePauseReason, threadId = state.activeThreadId): Partial<ChatState> {
  if (!threadId) return {}
  const pauses = { ...state.queuedMessagePauses }
  if (reason === null) delete pauses[threadId]
  else pauses[threadId] = reason
  return { queuedMessagePauses: pauses }
}

/** Terminal events may pause work, but cannot release an explicit Stop/failed pause. */
export function terminalQueuePausePatch(state: QueueState, failed: boolean, threadId = state.activeThreadId): Partial<ChatState> {
  return queuedPauseReason(state, threadId) !== null || !failed ? {} : queuePausePatch(state, 'failed', threadId)
}

export function queueMessagesPatch(state: QueueState, threadId: string, messages: QueuedUserMessage[]): Partial<ChatState> {
  return { queuedMessages: [...state.queuedMessages.filter(message => message.threadId !== threadId),
    ...messages.filter(message => message.threadId === threadId)] }
}

/** Stable intent IDs deduplicate recovery; identical text with different IDs stays distinct. */
export function appendQueuedMessages(state: QueueState, messages: QueuedUserMessage[]): Partial<ChatState> {
  const next = [...state.queuedMessages]
  for (const message of messages) {
    if (!message.threadId || !message.id || next.some(item => item.threadId === message.threadId && item.id === message.id)) continue
    next.push(message)
  }
  return { queuedMessages: next }
}

/** A recovered intent revokes only an in-flight ticket with the same stable ID. */
export function recoverQueuedMessages(state: QueueState, messages: QueuedUserMessage[]): Partial<ChatState> {
  const next = [...state.queuedMessages]
  for (const message of messages) {
    if (!message.threadId || !message.id) continue
    const index = next.findIndex(item => item.threadId === message.threadId && item.id === message.id)
    if (index < 0) next.push({ ...message, sending: false })
    else if (next[index].sending) next[index] = { ...message, sending: false }
  }
  return { queuedMessages: next }
}

/** Confirmed deletion or authority purge retires only its local owners. */
export function retireQueuedThreads(state: QueueState, threadIds: Iterable<string>): Partial<ChatState> {
  const retired = new Set(threadIds)
  const pauses = { ...state.queuedMessagePauses }
  for (const threadId of retired) delete pauses[threadId]
  return { queuedMessages: state.queuedMessages.filter(message => !retired.has(message.threadId)),
    queuedMessagePauses: pauses }
}

/** Consume only a real Core user-item receipt with the same thread and intent ID. */
export function consumeAcceptedQueuedMessages(state: QueueState, threadId: string | undefined, blocks: readonly ChatBlock[]): Partial<ChatState> {
  if (!threadId) return {}
  const accepted = new Set(blocks.flatMap(block => block.kind === 'user' && block.meta?.steeringStatus === 'accepted' &&
    block.meta.turnId?.trim() && block.meta.clientUserMessageId?.trim() ? [block.meta.clientUserMessageId] : []))
  if (!state.queuedMessages.some(message => message.threadId === threadId && accepted.has(message.id))) return {}
  const queuedMessages = state.queuedMessages.filter(message => message.threadId !== threadId || !accepted.has(message.id))
  return { queuedMessages, ...(!queuedMessages.some(message => message.threadId === threadId) ? queuePausePatch(state, null, threadId) : {}) }
}
