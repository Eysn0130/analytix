import type {
  PublicToolCallArgumentsProjectionV1,
  PublicToolResultProjectionV1,
  TurnItem
} from '../contracts/items.js'
import type { ReviewOutput, ReviewTarget } from '../contracts/review.js'

export type ItemEntity = TurnItem

type PublicToolCallItem = Extract<TurnItem, { kind: 'tool_call' }>
type PublicToolResultItem = Extract<TurnItem, { kind: 'tool_result' }>

/** Attempt-private provider call state. Never persist or expose this shape. */
export type InternalToolCallItem = Omit<PublicToolCallItem, 'arguments'> & {
  arguments: Record<string, unknown>
  summary?: string
}

/** Attempt-private tool output. Never persist or expose this shape. */
export type InternalToolResultItem = Omit<PublicToolResultItem, 'output'> & {
  output: unknown
}

export function makePublicToolCallArgumentsProjection(): PublicToolCallArgumentsProjectionV1 {
  return {
    schemaVersion: 1,
    projectionKind: 'withheld',
    disclosure: 'metadata_only',
    messageKey: 'tool_arguments_withheld',
    privatePayloadWithheld: true,
    factAnswerAllowed: false,
    evidenceAuthority: false
  }
}

export function makePublicToolResultWithheldProjection(input: {
  status: 'completed' | 'failed' | 'blocked' | 'cancelled' | 'unknown'
  legacy?: boolean
}): PublicToolResultProjectionV1 {
  if (input.legacy) {
    return {
      schemaVersion: 1,
      projectionKind: 'withheld',
      disclosure: 'metadata_only',
      status: input.status,
      code: 'legacy_output_withheld',
      messageKey: 'legacy_output_withheld',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    }
  }
  return {
    schemaVersion: 1,
    projectionKind: 'withheld',
    disclosure: 'metadata_only',
    status: input.status,
    code: 'tool_output_private',
    messageKey: 'tool_output_withheld',
    privatePayloadWithheld: true,
    factAnswerAllowed: false,
    evidenceAuthority: false
  }
}

export function makeUserItem(input: {
  id: string
  turnId: string
  threadId: string
  text: string
  displayText?: string
  delivery?: 'steer'
  attachmentIds?: string[]
  fileReferences?: Array<{ path: string; relativePath: string; name: string; kind?: 'file' | 'directory' }>
  workspaceCheckpointId?: string
}): TurnItem {
  const attachmentIds = input.attachmentIds?.filter((id) => id.trim().length > 0)
  const fileReferences = input.fileReferences
    ?.map((reference) => ({
      path: reference.path.trim(),
      relativePath: reference.relativePath.trim(),
      name: reference.name.trim(),
      ...(reference.kind === 'directory' ? { kind: 'directory' as const } : { kind: 'file' as const })
    }))
    .filter((reference) => reference.path && reference.relativePath && reference.name)
  const displayText = input.displayText?.trim()
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'user',
    status: 'completed',
    createdAt: new Date().toISOString(),
    finishedAt: new Date().toISOString(),
    kind: 'user_message',
    text: input.text,
    ...(displayText && displayText !== input.text ? { displayText } : {}),
    ...(input.delivery === 'steer' ? { delivery: 'steer' as const } : {}),
    ...(attachmentIds?.length ? { attachmentIds } : {}),
    ...(fileReferences?.length ? { fileReferences } : {}),
    ...(input.workspaceCheckpointId ? { workspaceCheckpointId: input.workspaceCheckpointId } : {})
  }
}

export function makeAssistantTextItem(input: {
  id: string
  turnId: string
  threadId: string
  text: string
  status?: 'running' | 'completed' | 'failed'
}): TurnItem {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'assistant',
    status: input.status ?? 'running',
    createdAt: new Date().toISOString(),
    kind: 'assistant_text',
    text: input.text
  }
}

export type InternalAssistantReasoningItem = {
  id: string
  turnId: string
  threadId: string
  role: 'assistant'
  status: 'pending' | 'running' | 'completed' | 'failed' | 'aborted'
  createdAt: string
  finishedAt?: string
  kind: 'assistant_reasoning'
  text: string
  signature?: string
}

export function makeAssistantReasoningItem(input: {
  id: string
  turnId: string
  threadId: string
  text: string
  signature?: string
  status?: 'running' | 'completed' | 'failed'
}): InternalAssistantReasoningItem {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'assistant',
    status: input.status ?? 'running',
    createdAt: new Date().toISOString(),
    kind: 'assistant_reasoning',
    text: input.text,
    ...(input.signature ? { signature: input.signature } : {})
  }
}

export function makePrivateToolCallItem(input: {
  id: string
  turnId: string
  threadId: string
  callId: string
  toolName: string
  toolKind?: 'tool_call' | 'command_execution' | 'file_change' | 'subagent'
  /** Attempt-private arguments are deliberately not copied into the public item. */
  arguments: Readonly<Record<string, unknown>>
  summary?: string
  status?: 'pending' | 'running' | 'completed' | 'failed'
}): InternalToolCallItem {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'tool',
    status: input.status ?? 'pending',
    createdAt: new Date().toISOString(),
    kind: 'tool_call',
    toolName: input.toolName,
    callId: input.callId,
    toolKind: input.toolKind ?? 'tool_call',
    arguments: { ...input.arguments },
    ...(input.summary ? { summary: input.summary } : {})
  }
}

export function makeToolCallItem(
  input: Parameters<typeof makePrivateToolCallItem>[0]
): PublicToolCallItem {
  return projectToolCallItemForPublic(makePrivateToolCallItem(input))
}

export function makePrivateToolResultItem(input: {
  id: string
  turnId: string
  threadId: string
  callId: string
  toolName: string
  toolKind?: 'tool_call' | 'command_execution' | 'file_change' | 'subagent'
  output: unknown
  isError?: boolean
  status?: 'pending' | 'running' | 'completed' | 'failed' | 'aborted'
  finishedAt?: string
}): InternalToolResultItem {
  const isError = input.isError ?? (input.status === 'failed' || input.status === 'aborted')
  const status = input.status ?? (isError ? 'failed' : 'completed')
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'tool',
    status,
    createdAt: new Date().toISOString(),
    ...(input.finishedAt
      ? { finishedAt: input.finishedAt }
      : status === 'completed' || status === 'failed' || status === 'aborted'
        ? { finishedAt: new Date().toISOString() }
        : {}),
    kind: 'tool_result',
    toolName: input.toolName,
    callId: input.callId,
    toolKind: input.toolKind ?? 'tool_call',
    output: input.output,
    isError
  }
}

export function makeToolResultItem(
  input: Parameters<typeof makePrivateToolResultItem>[0]
): PublicToolResultItem {
  return projectToolResultItemForPublic(makePrivateToolResultItem(input))
}

export function projectToolCallItemForPublic(item: InternalToolCallItem): PublicToolCallItem {
  const {
    summary: _summary,
    arguments: _arguments,
    ...metadata
  } = item
  return {
    ...metadata,
    arguments: makePublicToolCallArgumentsProjection()
  }
}

export function projectToolResultItemForPublic(item: InternalToolResultItem): PublicToolResultItem {
  const lifecycleStatus = item.isError
    ? (item.status === 'aborted' ? 'aborted' : 'failed')
    : item.status === 'failed' || item.status === 'aborted'
      ? 'completed'
      : item.status
  const publicStatus = lifecycleStatus === 'completed'
    ? 'completed'
    : lifecycleStatus === 'failed'
      ? 'failed'
      : lifecycleStatus === 'aborted'
        ? 'cancelled'
        : 'unknown'
  const { output: _output, ...metadata } = item
  return {
    ...metadata,
    status: lifecycleStatus,
    output: makePublicToolResultWithheldProjection({ status: publicStatus })
  }
}

export function finalizePublicToolResultItem(
  item: PublicToolResultItem,
  status: 'completed' | 'failed' | 'aborted',
  finishedAt: string
): PublicToolResultItem {
  const isError = status !== 'completed'
  return {
    ...item,
    status,
    finishedAt,
    isError,
    output: makePublicToolResultWithheldProjection({
      status: status === 'completed'
        ? 'completed'
        : status === 'aborted'
          ? 'cancelled'
          : 'failed'
    })
  }
}

export function makeApprovalItem(input: {
  id: string
  turnId: string
  threadId: string
  approvalId: string
  toolName: string
  summary: string
}): Extract<TurnItem, { kind: 'approval' }> {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'tool',
    createdAt: new Date().toISOString(),
    kind: 'approval',
    approvalId: input.approvalId,
    toolName: input.toolName,
    summary: input.summary,
    status: 'pending'
  }
}

export function makeUserInputItem(input: {
  id: string
  turnId: string
  threadId: string
  inputId: string
  prompt: string
  questions?: Array<{
    header: string
    id: string
    question: string
    options: Array<{ label: string; description: string }>
  }>
}): TurnItem {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'tool',
    createdAt: new Date().toISOString(),
    kind: 'user_input',
    inputId: input.inputId,
    prompt: input.prompt,
    questions: input.questions ?? [],
    status: 'pending'
  }
}

export function makeCompactionItem(input: {
  id: string
  turnId: string
  threadId: string
  summary: string
  replacedTokens: number
  pinnedConstraints: string[]
  auto?: boolean
  sourceDigest?: string
  digestMarker?: string
  sourceItemIds?: string[]
}): TurnItem {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'system',
    status: 'completed',
    createdAt: new Date().toISOString(),
    finishedAt: new Date().toISOString(),
    kind: 'compaction',
    summary: input.summary,
    replacedTokens: input.replacedTokens,
    ...(input.auto !== undefined ? { auto: input.auto } : {}),
    pinnedConstraints: input.pinnedConstraints,
    ...(input.sourceDigest ? { sourceDigest: input.sourceDigest } : {}),
    ...(input.digestMarker ? { digestMarker: input.digestMarker } : {}),
    ...(input.sourceItemIds ? { sourceItemIds: [...input.sourceItemIds] } : {})
  }
}

export function makeReviewItem(input: {
  id: string
  turnId: string
  threadId: string
  target: ReviewTarget
  title: string
  status?: 'running' | 'completed' | 'failed' | 'aborted'
  reviewText?: string
  output?: ReviewOutput
  finishedAt?: string
}): TurnItem {
  const status = input.status ?? 'running'
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'assistant',
    status,
    createdAt: new Date().toISOString(),
    ...(input.finishedAt
      ? { finishedAt: input.finishedAt }
      : status === 'completed' || status === 'failed' || status === 'aborted'
        ? { finishedAt: new Date().toISOString() }
        : {}),
    kind: 'review',
    target: input.target,
    title: input.title,
    ...(input.reviewText ? { reviewText: input.reviewText } : {}),
    ...(input.output ? { output: input.output } : {})
  }
}

export function makeErrorItem(input: {
  id: string
  turnId: string
  threadId: string
  message: string
  code?: string
  details?: unknown
  severity?: 'info' | 'warning' | 'error'
}): TurnItem {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'system',
    status: 'failed',
    createdAt: new Date().toISOString(),
    finishedAt: new Date().toISOString(),
    kind: 'error',
    message: input.message,
    ...(input.code ? { code: input.code } : {}),
    ...(input.details !== undefined ? { details: input.details } : {}),
    ...(input.severity ? { severity: input.severity } : {})
  }
}
