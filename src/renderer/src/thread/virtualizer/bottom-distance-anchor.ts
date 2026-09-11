export const DEFAULT_BOTTOM_STICKINESS_PX = 96

export type ScrollMetrics = {
  scrollTop: number
  viewportHeight: number
  totalHeight: number
}

export function computeBottomDistance({ scrollTop, viewportHeight, totalHeight }: ScrollMetrics): number {
  return Math.max(0, totalHeight - scrollTop - viewportHeight)
}

export function isNearBottom(distance: number, threshold = DEFAULT_BOTTOM_STICKINESS_PX): boolean {
  return distance <= threshold
}

export function scrollTopForBottomDistance({
  bottomDistance,
  viewportHeight,
  totalHeight
}: {
  bottomDistance: number
  viewportHeight: number
  totalHeight: number
}): number {
  return Math.max(0, totalHeight - viewportHeight - Math.max(0, bottomDistance))
}

export function preserveBottomDistance({
  previous,
  nextTotalHeight
}: {
  previous: ScrollMetrics
  nextTotalHeight: number
}): number {
  return scrollTopForBottomDistance({
    bottomDistance: computeBottomDistance(previous),
    viewportHeight: previous.viewportHeight,
    totalHeight: nextTotalHeight
  })
}

export function preservePrependOffset({
  previousScrollTop,
  previousTotalHeight,
  nextTotalHeight
}: {
  previousScrollTop: number
  previousTotalHeight: number
  nextTotalHeight: number
}): number {
  return previousScrollTop + Math.max(0, nextTotalHeight - previousTotalHeight)
}
