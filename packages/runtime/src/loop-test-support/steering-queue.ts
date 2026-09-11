/**
 * Mid-turn steering queue. The renderer posts steering text while a
 * turn is running; the queue collects those messages and injects them
 * as user inputs at the next safe loop boundary. The queue is cleared
 * on turn completion or interruption.
 */
export type SteeringQueueEntry = {
  id?: string
  clientUserMessageId?: string
  text: string
  displayText?: string
  attachmentIds?: string[]
  fileReferences?: Array<{ path: string; relativePath: string; name: string; kind?: 'file' | 'directory' }>
  admittedAt?: string
}

export class SteeringQueue {
  private readonly buffer: SteeringQueueEntry[] = []
  private turnId: string | null = null

  setTurn(turnId: string | null): void {
    if (this.turnId !== turnId) {
      this.buffer.length = 0
    }
    this.turnId = turnId
  }

  enqueue(turnId: string, input: string | SteeringQueueEntry): void {
    if (this.turnId !== turnId) {
      this.buffer.length = 0
      this.turnId = turnId
    }
    const entry = typeof input === 'string' ? { text: input } : input
    const trimmed = entry.text.trim()
    if (!trimmed) return
    this.buffer.push({
      ...entry,
      text: trimmed,
      ...(entry.displayText?.trim() ? { displayText: entry.displayText.trim() } : {}),
      ...(entry.clientUserMessageId?.trim() ? { clientUserMessageId: entry.clientUserMessageId.trim() } : {}),
      ...(entry.id?.trim() ? { id: entry.id.trim() } : {}),
      ...(entry.attachmentIds?.length ? { attachmentIds: [...entry.attachmentIds] } : {}),
      ...(entry.fileReferences?.length ? { fileReferences: [...entry.fileReferences] } : {})
    })
  }

  /**
   * Drain queued steering messages and return them. The loop calls
   * this at safe boundaries (after a model response, before the next
   * model request). Returns an empty array when nothing is pending.
   */
  drain(): SteeringQueueEntry[] {
    if (this.buffer.length === 0) return []
    const out = [...this.buffer]
    this.buffer.length = 0
    return out
  }

  /**
   * Peek at the queued text without removing it. Used by the UI to
   * show pending steering in a "pending injection" indicator.
   */
  peek(): SteeringQueueEntry[] {
    return [...this.buffer]
  }

  clear(): void {
    this.buffer.length = 0
    this.turnId = null
  }
}
