import type { ThreadRow } from '../projection/thread-row-model'
import type { MeasuredRowCache } from './measured-row-cache'

export type VirtualThreadItem = {
  row: ThreadRow
  index: number
  offsetTop: number
  height: number
  extentHeight: number
}

export type VirtualThreadWindow = {
  items: VirtualThreadItem[]
  totalHeight: number
  beforeHeight: number
  afterHeight: number
  startIndex: number
  endIndex: number
}

export type ThreadVirtualizerInput = {
  rows: ThreadRow[]
  cache: Pick<MeasuredRowCache, 'estimate'>
  scrollTop: number
  viewportHeight: number
  overscanPx?: number
  rowGapPx?: number
}

type RowLayout = VirtualThreadItem & {
  offsetBottom: number
}

// Clean-room implementation from the Analytix-owned row/window contract and
// focused tests. Third-party extracted virtualizer code is not an input.
function layoutRows(
  rows: ThreadRow[],
  cache: Pick<MeasuredRowCache, 'estimate'>,
  rowGapPx: number
): RowLayout[] {
  const layout: RowLayout[] = []
  let offsetTop = 0

  for (const [index, row] of rows.entries()) {
    const height = cache.estimate(row)
    const extentHeight = height + (index + 1 < rows.length ? rowGapPx : 0)
    const offsetBottom = offsetTop + extentHeight
    layout.push({ row, index, offsetTop, offsetBottom, height, extentHeight })
    offsetTop = offsetBottom
  }

  return layout
}

export function computeVirtualThreadWindow({
  rows,
  cache,
  scrollTop,
  viewportHeight,
  overscanPx = 720,
  rowGapPx = 0
}: ThreadVirtualizerInput): VirtualThreadWindow {
  const startPx = Math.max(0, scrollTop - overscanPx)
  const endPx = Math.max(startPx, scrollTop + viewportHeight + overscanPx)
  const layout = layoutRows(rows, cache, rowGapPx)
  const totalHeight = layout.at(-1)?.offsetBottom ?? 0
  const visible = layout.filter(
    ({ offsetTop, offsetBottom }) => offsetBottom >= startPx && offsetTop <= endPx
  )

  if (visible.length === 0) {
    return {
      items: [],
      totalHeight,
      beforeHeight: 0,
      afterHeight: totalHeight,
      startIndex: 0,
      endIndex: -1
    }
  }

  const first = visible[0]
  const last = visible.at(-1)!
  const items = visible.map(({ row, index, offsetTop, height, extentHeight }) => ({
    row,
    index,
    offsetTop,
    height,
    extentHeight
  }))
  const beforeHeight = first.offsetTop
  const afterHeight = Math.max(0, totalHeight - last.offsetBottom)
  const startIndex = first.index
  const endIndex = last.index

  return { items, totalHeight, beforeHeight, afterHeight, startIndex, endIndex }
}

export class AnalytixThreadVirtualizer {
  constructor(private readonly cache: Pick<MeasuredRowCache, 'estimate'>) {}

  getWindow(input: Omit<ThreadVirtualizerInput, 'cache'>): VirtualThreadWindow {
    return computeVirtualThreadWindow({ ...input, cache: this.cache })
  }
}
