import {
  computeBottomDistance,
  isNearBottom,
  preserveBottomDistance,
  preservePrependOffset,
  type ScrollMetrics
} from './bottom-distance-anchor'

export type ThreadScrollLayoutState = {
  stickToBottom: boolean
  bottomDistance: number
}

export class ThreadScrollLayout {
  constructor(private readonly stickinessPx?: number) {}

  private state: ThreadScrollLayoutState = {
    stickToBottom: true,
    bottomDistance: 0
  }

  capture(metrics: ScrollMetrics): ThreadScrollLayoutState {
    const bottomDistance = computeBottomDistance(metrics)
    this.state = {
      bottomDistance,
      stickToBottom: isNearBottom(bottomDistance, this.stickinessPx)
    }
    return this.state
  }

  nextScrollTopAfterResize(metrics: ScrollMetrics, nextTotalHeight: number): number | null {
    if (!this.state.stickToBottom) return null
    return preserveBottomDistance({ previous: metrics, nextTotalHeight })
  }

  nextScrollTopAfterPrepend(previousScrollTop: number, previousTotalHeight: number, nextTotalHeight: number): number {
    return preservePrependOffset({ previousScrollTop, previousTotalHeight, nextTotalHeight })
  }
}
