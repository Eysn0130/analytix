import type { ThreadDeltaEvent } from '../../agent/types'

export type StreamingTextSnapshot = {
  assistant: string
  lastSeq: number
  turnId: string | null
}

type StreamingTextBucket = {
  assistant: string
  lastSeq: number
  turnId: string | null
}

const DEFAULT_BUCKET_KEY = '__default__'

export class StreamingTextBuffer {
  private buckets = new Map<string, StreamingTextBucket>()

  private getBucket(turnId: string | null): StreamingTextBucket {
    const key = turnId ?? DEFAULT_BUCKET_KEY
    let bucket = this.buckets.get(key)
    if (!bucket) {
      bucket = { assistant: '', lastSeq: 0, turnId }
      this.buckets.set(key, bucket)
    }
    return bucket
  }

  append(deltas: ThreadDeltaEvent[]): void {
    for (const delta of deltas) {
      const deltaTurnId = delta.turnId?.trim() || null
      const bucket = this.getBucket(deltaTurnId)
      if (typeof delta.seq === 'number') bucket.lastSeq = Math.max(bucket.lastSeq, delta.seq)
      bucket.assistant += delta.text
    }
  }

  drain(): StreamingTextSnapshot | null {
    return this.drainAll()[0] ?? null
  }

  drainAll(): StreamingTextSnapshot[] {
    if (this.buckets.size === 0) return []
    const snapshots = Array.from(this.buckets.values())
      .filter((bucket) => bucket.assistant || bucket.lastSeq !== 0)
      .map((bucket) => ({
        assistant: bucket.assistant,
        lastSeq: bucket.lastSeq,
        turnId: bucket.turnId
      }))
    this.buckets.clear()
    return snapshots
  }

  clear(): void {
    this.buckets.clear()
  }

  get empty(): boolean {
    return this.buckets.size === 0
  }
}
