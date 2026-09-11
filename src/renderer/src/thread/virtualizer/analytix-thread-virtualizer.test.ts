import { describe, expect, it } from 'vitest'
import type { ThreadRow } from '../projection/thread-row-model'
import {
  computeBottomDistance,
  preserveBottomDistance,
  preservePrependOffset,
  scrollTopForBottomDistance
} from './bottom-distance-anchor'
import { computeVirtualThreadWindow } from './analytix-thread-virtualizer'
import { createMeasuredRowCache } from './measured-row-cache'
import { ResizeObserverBatcher } from './resize-observer-batcher'

function row(id: string): ThreadRow {
  return { kind: 'bottomSpacer', id }
}

function liveTurnRow(id: string): ThreadRow {
  return {
    kind: 'turn',
    id,
    turn: {
      key: id,
      turn: {
        user: { kind: 'user', id: `${id}:user`, text: 'Question' },
        blocks: []
      },
      absoluteIndex: 0,
      isLatest: true,
      isLiveTurn: true,
      isProcessing: true,
      hasLiveStream: true,
      live: '',
      liveProcessText: '',
      liveContent: '',
      timing: {},
      pendingRuntimeWork: false,
      sections: {
        processBlocks: [],
        assistantContentBlocks: [],
        compactionBlocks: [],
        generatedFileBlocks: [],
        turnFileChanges: []
      },
      reviewBlocks: [],
      generatedFileBlocks: [],
      devPreviewCard: null,
      showForkPoint: false
    }
  }
}

describe('AnalytixThreadVirtualizer', () => {
  it('computes a measured viewport window with before and after spacers', () => {
    const cache = createMeasuredRowCache()
    const rows = Array.from({ length: 10 }, (_, index) => row(`row-${index}`))
    rows.forEach((item) => cache.set(item.id, 100))

    const result = computeVirtualThreadWindow({
      rows,
      cache,
      scrollTop: 350,
      viewportHeight: 200,
      overscanPx: 50
    })

    expect(result.totalHeight).toBe(1000)
    expect(result.items.map((item) => item.row.id)).toEqual(['row-2', 'row-3', 'row-4', 'row-5', 'row-6'])
    expect(result.beforeHeight).toBe(200)
    expect(result.afterHeight).toBe(300)
  })

  it('accounts for the row gap in total height and spacers', () => {
    const cache = createMeasuredRowCache()
    const rows = Array.from({ length: 4 }, (_, index) => row(`row-${index}`))
    rows.forEach((item) => cache.set(item.id, 100))

    const result = computeVirtualThreadWindow({
      rows,
      cache,
      scrollTop: 150,
      viewportHeight: 100,
      overscanPx: 0,
      rowGapPx: 32
    })

    expect(result.totalHeight).toBe(496)
    expect(result.items.map((item) => item.row.id)).toEqual(['row-1'])
    expect(result.beforeHeight).toBe(132)
    expect(result.afterHeight).toBe(232)
  })

  it('keeps rows that touch either viewport boundary', () => {
    const cache = createMeasuredRowCache()
    const rows = [row('before'), row('after')]
    rows.forEach((item) => cache.set(item.id, 100))

    const result = computeVirtualThreadWindow({
      rows,
      cache,
      scrollTop: 100,
      viewportHeight: 0,
      overscanPx: 0
    })

    expect(result.items.map((item) => item.row.id)).toEqual(['before', 'after'])
    expect(result.startIndex).toBe(0)
    expect(result.endIndex).toBe(1)
  })

  it('returns the full height as the after spacer when the window misses all rows', () => {
    const cache = createMeasuredRowCache()
    const rows = [row('one'), row('two')]
    rows.forEach((item) => cache.set(item.id, 100))

    const result = computeVirtualThreadWindow({
      rows,
      cache,
      scrollTop: 500,
      viewportHeight: 100,
      overscanPx: 0
    })

    expect(result).toEqual({
      items: [],
      totalHeight: 200,
      beforeHeight: 0,
      afterHeight: 200,
      startIndex: 0,
      endIndex: -1
    })
  })

  it('keeps bottom distance stable when rows grow near the bottom', () => {
    const previous = { scrollTop: 700, viewportHeight: 300, totalHeight: 1_000 }
    expect(computeBottomDistance(previous)).toBe(0)
    expect(preserveBottomDistance({ previous, nextTotalHeight: 1_180 })).toBe(880)
    expect(scrollTopForBottomDistance({ bottomDistance: 40, viewportHeight: 300, totalHeight: 1_180 })).toBe(840)
  })

  it('preserves the first visible position when history is prepended', () => {
    expect(
      preservePrependOffset({
        previousScrollTop: 120,
        previousTotalHeight: 900,
        nextTotalHeight: 1_260
      })
    ).toBe(480)
  })

  it('batches resize observations into one measurement flush', () => {
    const callbacks: Array<() => void> = []
    const flushed: unknown[] = []
    const batcher = new ResizeObserverBatcher({
      scheduleFrame: (callback) => {
        callbacks.push(callback)
        return callbacks.length
      },
      cancelFrame: () => undefined,
      onMeasurements: (measurements) => flushed.push(measurements)
    })

    batcher.record('a', 10.2)
    batcher.record('b', 20)
    expect(flushed).toHaveLength(0)
    callbacks[0]?.()
    expect(flushed).toEqual([[{ rowId: 'a', height: 11 }, { rowId: 'b', height: 20 }]])
  })

  it('lets active-stream live estimates grow an unmeasured turn row before ResizeObserver catches up', () => {
    const cache = createMeasuredRowCache()
    const item = liveTurnRow('live-turn')
    const before = cache.estimate(item)

    expect(cache.setLiveEstimate(item.id, { processLength: 2400, contentLength: 6400 })).toBe(true)
    const after = cache.estimate(item)

    expect(after).toBeGreaterThan(before)
    expect(cache.clearLiveEstimates()).toBe(true)
    expect(cache.estimate(item)).toBe(before)
  })

  it('only reports live estimate changes when text crosses an estimated line bucket', () => {
    const cache = createMeasuredRowCache()
    const item = liveTurnRow('live-turn')

    expect(cache.setLiveEstimate(item.id, { processLength: 1, contentLength: 1 })).toBe(true)
    expect(cache.setLiveEstimate(item.id, { processLength: 20, contentLength: 80 })).toBe(false)
    expect(cache.setLiveEstimate(item.id, { processLength: 20, contentLength: 87 })).toBe(true)
  })

  it('lets active-stream live estimates exceed a stale measured turn height', () => {
    const cache = createMeasuredRowCache()
    const item = liveTurnRow('live-turn')
    cache.set(item.id, 220)
    expect(cache.estimate(item)).toBe(220)

    cache.setLiveEstimate(item.id, { processLength: 3600, contentLength: 7200 })

    expect(cache.estimate(item)).toBeGreaterThan(220)
  })

  it('prunes live estimates for rows that are no longer valid', () => {
    const cache = createMeasuredRowCache()
    const item = liveTurnRow('live-turn')
    cache.setLiveEstimate(item.id, { processLength: 0, contentLength: 6400 })
    expect(cache.estimate(item)).toBeGreaterThan(180)

    cache.prune(['other-row'])

    expect(cache.estimate(item)).toBe(180)
  })
})
