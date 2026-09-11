import type {
  AcceptedFinalProjectionBatch,
  AcceptedFinalProjectionReceiptV1,
  ChatBlock,
  ThreadUsageSnapshot
} from './types'

export function acceptedFinalProjectionReceiptFromBatch(
  batch: Pick<
    AcceptedFinalProjectionBatch,
    'batchId' | 'threadId' | 'turnId' | 'publicationCommitId' | 'lastSeq'
  >
): AcceptedFinalProjectionReceiptV1 {
  return {
    schemaVersion: 1,
    batchId: batch.batchId,
    threadId: batch.threadId,
    turnId: batch.turnId,
    publicationCommitId: batch.publicationCommitId,
    lastSeq: batch.lastSeq
  }
}

export function acceptedFinalProjectionReceiptsEqual(
  left: AcceptedFinalProjectionReceiptV1 | null | undefined,
  right: AcceptedFinalProjectionReceiptV1 | null | undefined
): boolean {
  return Boolean(
    left && right &&
    left.schemaVersion === 1 && right.schemaVersion === 1 &&
    left.batchId === right.batchId &&
    left.threadId === right.threadId &&
    left.turnId === right.turnId &&
    left.publicationCommitId === right.publicationCommitId &&
    left.lastSeq === right.lastSeq
  )
}

export function acceptedFinalProjectionBatchIsSelfConsistent(
  batch: AcceptedFinalProjectionBatch
): boolean {
  return Boolean(
    acceptedFinalProjectionReceiptsEqual(
      batch.receipt,
      acceptedFinalProjectionReceiptFromBatch(batch)
    ) &&
    Number.isSafeInteger(batch.firstSeq) && batch.firstSeq > 0 &&
    Number.isSafeInteger(batch.lastSeq) &&
    batch.lastSeq === batch.firstSeq + (batch.terminalError ? 3 : 2) &&
    batch.assistant.meta?.turnId === batch.turnId &&
    batch.assistant.acceptedFinalView?.acceptedFinalDigest === batch.publicationCommitId &&
    batch.terminal.acceptedFinalDigest === batch.publicationCommitId &&
    (!batch.terminalError || batch.terminalError.meta?.turnId === batch.turnId)
  )
}

export function acceptedFinalProjectionIsAlreadyCommitted(
  blocks: ChatBlock[],
  lastSeq: number,
  batch: AcceptedFinalProjectionBatch
): boolean {
  if (lastSeq !== batch.lastSeq) return false
  const assistants = blocks.filter((block) =>
    block.kind === 'assistant' &&
    acceptedFinalProjectionReceiptsEqual(block.acceptedFinalProjectionReceipt, batch.receipt)
  )
  if (assistants.length !== 1) return false
  const assistant = assistants[0]
  if (!assistant || assistant.kind !== 'assistant' || assistant.id !== batch.assistant.id ||
      assistant.text !== batch.assistant.text ||
      assistant.createdAt !== batch.assistant.createdAt ||
      assistant.meta?.turnId !== batch.turnId ||
      assistant.acceptedFinalView?.acceptedFinalDigest !== batch.publicationCommitId ||
      JSON.stringify(canonicalJsonValue(assistant.acceptedFinalView)) !==
        JSON.stringify(canonicalJsonValue(batch.assistant.acceptedFinalView)) ||
      assistant.acceptedFinalProjectionTerminal?.status !== batch.terminal.status ||
      assistant.acceptedFinalProjectionTerminal?.createdAt !== batch.terminal.createdAt ||
      assistant.acceptedFinalProjectionTerminal?.acceptedFinalDigest !==
        batch.terminal.acceptedFinalDigest ||
      assistant.acceptedFinalProjectionTerminal?.terminalReason !==
        batch.terminal.terminalReason) {
    return false
  }
  if (!batch.terminalError) return true
  const errors = blocks.filter((block) =>
    block.kind === 'system' &&
    block.id === batch.terminalError?.id &&
    block.text === batch.terminalError.text &&
    block.createdAt === batch.terminalError.createdAt &&
    block.code === batch.terminalError.code &&
    block.detail === batch.terminalError.detail &&
    block.severity === batch.terminalError.severity &&
    block.meta?.turnId === batch.turnId
  )
  return errors.length === 1
}

function canonicalJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalJsonValue)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .filter(([, entry]) => entry !== undefined)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, entry]) => [key, canonicalJsonValue(entry)])
  )
}

export function acceptedFinalUsageSnapshotsEqual(
  left: ThreadUsageSnapshot | null | undefined,
  right: ThreadUsageSnapshot | null | undefined
): boolean {
  if (!left || !right) return false
  return JSON.stringify(canonicalJsonValue(left)) === JSON.stringify(canonicalJsonValue(right))
}

export function acceptedFinalProjectionHasCandidateReceipt(
  blocks: ChatBlock[],
  batch: AcceptedFinalProjectionBatch
): boolean {
  return blocks.some((block) =>
    block.kind === 'assistant' &&
    (block.id === batch.assistant.id ||
      (Boolean(block.acceptedFinalProjectionReceipt) &&
        acceptedFinalProjectionReceiptsEqual(block.acceptedFinalProjectionReceipt, batch.receipt)))
  )
}
