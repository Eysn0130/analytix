import type { ThreadRow } from '../projection/thread-row-model'

const DEFAULT_ESTIMATES: Record<ThreadRow['kind'], number> = {
  emptyHero: 420,
  forkBanner: 64,
  forkPoint: 46,
  loadEarlier: 48,
  turn: 180,
  collapseEarlier: 42,
  liveOnlyTurn: 220,
  bottomSpacer: 1
}

function estimateTextHeight(text: string): number {
  return estimateTextLengthHeight(text.length)
}

function estimateTextLengthHeight(length: number): number {
  const lineCount = Math.max(1, Math.ceil(length / 86))
  return 56 + lineCount * 22
}

function estimateTextLengthBucket(length: number): number {
  return Math.max(1, Math.ceil(Math.max(0, length) / 86))
}

export class MeasuredRowCache {
  private readonly heights = new Map<string, number>()
  private readonly liveEstimates = new Map<string, { processLength: number; contentLength: number }>()

  get(rowId: string): number | undefined {
    return this.heights.get(rowId)
  }

  set(rowId: string, height: number): void {
    if (!Number.isFinite(height) || height <= 0) return
    this.heights.set(rowId, Math.ceil(height))
  }

  invalidate(rowId: string): void {
    this.heights.delete(rowId)
    this.liveEstimates.delete(rowId)
  }

  setLiveEstimate(rowId: string, estimate: { processLength: number; contentLength: number }): boolean {
    const previous = this.liveEstimates.get(rowId)
    const next = {
      processLength: Math.max(0, estimate.processLength),
      contentLength: Math.max(0, estimate.contentLength)
    }
    if (previous?.processLength === next.processLength && previous.contentLength === next.contentLength) return false
    if (
      previous &&
      estimateTextLengthBucket(previous.processLength) === estimateTextLengthBucket(next.processLength) &&
      estimateTextLengthBucket(previous.contentLength) === estimateTextLengthBucket(next.contentLength)
    ) {
      return false
    }
    this.liveEstimates.set(rowId, next)
    return true
  }

  clearLiveEstimates(validRowId?: string): boolean {
    if (validRowId) {
      let changed = false
      for (const rowId of this.liveEstimates.keys()) {
        if (rowId === validRowId) continue
        this.liveEstimates.delete(rowId)
        changed = true
      }
      return changed
    }
    const changed = this.liveEstimates.size > 0
    this.liveEstimates.clear()
    return changed
  }

  estimate(row: ThreadRow): number {
    const measured = this.heights.get(row.id)
    if (row.kind === 'turn') {
      const userText = row.turn.turn.user?.text ?? ''
      const assistantText =
        row.turn.sections.assistantContentBlocks.map((block) => block.text).join('\n\n') ||
        row.turn.liveContent
      const liveEstimate = this.liveEstimates.get(row.id)
      const assistantLength = Math.max(assistantText.length, liveEstimate?.contentLength ?? 0)
      const processWeight = row.turn.sections.processBlocks.length * 44
      const liveProcessWeight = (liveEstimate?.processLength ?? 0) > 0
        ? estimateTextLengthHeight(liveEstimate?.processLength ?? 0)
        : 0
      const generatedWeight = row.turn.generatedFileBlocks.length > 0 ? 140 : 0
      const reviewWeight = row.turn.reviewBlocks.length * 120
      const estimated = Math.max(
        DEFAULT_ESTIMATES.turn,
        estimateTextHeight(userText) +
          estimateTextLengthHeight(assistantLength) +
          processWeight +
          liveProcessWeight +
          generatedWeight +
          reviewWeight
      )
      return measured !== undefined ? Math.max(measured, estimated) : estimated
    }
    if (row.kind === 'liveOnlyTurn') {
      const liveEstimate = this.liveEstimates.get(row.id)
      const liveLength = Math.max(
        row.live.length,
        (liveEstimate?.processLength ?? 0) + (liveEstimate?.contentLength ?? 0)
      )
      const estimated = estimateTextLengthHeight(liveLength)
      return measured !== undefined ? Math.max(measured, estimated) : estimated
    }
    if (measured !== undefined) return measured
    return DEFAULT_ESTIMATES[row.kind]
  }

  prune(validRowIds: Iterable<string>): void {
    const valid = new Set(validRowIds)
    for (const rowId of this.heights.keys()) {
      if (!valid.has(rowId)) this.heights.delete(rowId)
    }
    for (const rowId of this.liveEstimates.keys()) {
      if (!valid.has(rowId)) this.liveEstimates.delete(rowId)
    }
  }
}

export function createMeasuredRowCache(): MeasuredRowCache {
  return new MeasuredRowCache()
}
