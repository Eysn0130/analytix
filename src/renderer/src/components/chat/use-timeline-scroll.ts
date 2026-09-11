import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import {
  computeBottomDistance,
  isNearBottom,
  preservePrependOffset,
  scrollTopForBottomDistance
} from '../../thread/virtualizer/bottom-distance-anchor'
import { ThreadScrollLayout } from '../../thread/virtualizer/scroll-layout'

/** Threshold (px) from the top of the scroll container that triggers
 * auto-loading earlier turns. */
const TOP_LOAD_TRIGGER_PX = 120
const MAX_STORED_SCROLL_POSITIONS = 200
const BOTTOM_DISTANCE_STATE_STEP_PX = 4
/** Distance (px) from the bottom within which the timeline is considered
 * "stuck to bottom" and will snap-scroll on new content. */
export const TIMELINE_AT_BOTTOM_PX = 32
export const TIMELINE_STICK_TO_BOTTOM_PX = TIMELINE_AT_BOTTOM_PX
export const TIMELINE_PREWORK_FOLLOW_PX = 180

export type TimelineStreamingPhase = 'idle' | 'prework' | 'final_answer'
export type TimelineFollowMode = 'static' | 'prework_watch' | 'prework_follow' | 'user_follow'

type TimelineScrollPosition = {
  scrollTop: number
  bottomDistance: number
  visibleTurnCount: number
  historyExpansionRequested: boolean
  stickToBottom: boolean
}

const scrollPositionsByThread = new Map<string, TimelineScrollPosition>()

function rememberScrollPosition(threadId: string, position: TimelineScrollPosition): void {
  scrollPositionsByThread.delete(threadId)
  scrollPositionsByThread.set(threadId, position)
  while (scrollPositionsByThread.size > MAX_STORED_SCROLL_POSITIONS) {
    const firstKey = scrollPositionsByThread.keys().next().value
    if (typeof firstKey !== 'string') break
    scrollPositionsByThread.delete(firstKey)
  }
}

type UseTimelineScrollOptions = {
  containerRef: RefObject<HTMLDivElement | null>
  activeThreadId: string | null
  pageSize: number
  autoCollapseThreshold: number
  totalTurns: number
  busy: boolean
  /** Triggers stick-to-bottom snap scroll. */
  scrollDeps: { contentKey: string; streaming: boolean; userTurnKey: string; phase?: TimelineStreamingPhase }
  onAnchorCorrected?: (data: TimelineAnchorCorrectionTrace) => void
}

type TimelineAnchorCorrectionTrace = {
  previousTop: number
  nextTop: number
  delta: number
  bottomDistance: number
  streaming: boolean
  prepend: boolean
  reset: boolean
}

export type UseTimelineScrollResult = {
  visibleTurnCount: number
  hiddenTurnCount: number
  isAtBottom: boolean
  bottomDistance: number
  loadEarlierTurns: (options?: { userInitiated?: boolean }) => void
  collapseEarlierTurns: () => void
  preserveBottomOnResize: () => void
  scrollToBottom: (options?: { behavior?: ScrollBehavior }) => void
  snapToBottomIfFollowing: () => void
  snapToBottomIfNearLive: () => void
}

export function deriveTimelineVisibleTurnCount({
  currentVisibleTurnCount,
  totalTurns,
  pageSize,
  shouldCollapseHistory,
  historyExpansionRequested
}: {
  currentVisibleTurnCount: number
  totalTurns: number
  pageSize: number
  shouldCollapseHistory: boolean
  historyExpansionRequested: boolean
}): number {
  const latestPageCount = Math.min(pageSize, totalTurns)
  if (!shouldCollapseHistory) return totalTurns
  if (historyExpansionRequested) {
    return Math.min(totalTurns, Math.max(currentVisibleTurnCount, latestPageCount))
  }
  return latestPageCount
}

export function timelineFollowDistanceForPhase(phase: TimelineStreamingPhase): number {
  return phase === 'prework' ? TIMELINE_PREWORK_FOLLOW_PX : TIMELINE_STICK_TO_BOTTOM_PX
}

export function deriveTimelineFollowMode({
  phase,
  bottomDistance,
  previousMode,
  userSubmitted = false,
  userRequestedBottom = false,
  userScrolledAway = false
}: {
  phase: TimelineStreamingPhase
  bottomDistance: number
  previousMode: TimelineFollowMode
  userSubmitted?: boolean
  userRequestedBottom?: boolean
  userScrolledAway?: boolean
}): TimelineFollowMode {
  if (userSubmitted || userRequestedBottom) return 'user_follow'
  if (userScrolledAway) return 'static'
  if (phase === 'prework') {
    if (previousMode === 'user_follow' && isNearBottom(bottomDistance, TIMELINE_PREWORK_FOLLOW_PX)) return 'user_follow'
    if (isNearBottom(bottomDistance, TIMELINE_AT_BOTTOM_PX)) return 'prework_follow'
    if (isNearBottom(bottomDistance, TIMELINE_PREWORK_FOLLOW_PX)) return 'prework_watch'
    return 'static'
  }
  if (phase === 'final_answer') {
    if (previousMode === 'user_follow' && isNearBottom(bottomDistance, TIMELINE_PREWORK_FOLLOW_PX)) {
      return 'user_follow'
    }
    return isNearBottom(bottomDistance, TIMELINE_AT_BOTTOM_PX) ? 'user_follow' : 'static'
  }
  if (previousMode === 'user_follow' && isNearBottom(bottomDistance, TIMELINE_PREWORK_FOLLOW_PX)) {
    return 'user_follow'
  }
  return isNearBottom(bottomDistance, TIMELINE_AT_BOTTOM_PX) ? 'user_follow' : 'static'
}

export function timelineFollowModeShouldAutoScroll(mode: TimelineFollowMode): boolean {
  return mode === 'user_follow' || mode === 'prework_follow'
}

/**
 * Owns the timeline scroll behaviour: stick-to-bottom snap scroll,
 * earlier-turns lazy loading, and prepend-position preservation. Pulled
 * out of `MessageTimeline` so the component body can stay focused on
 * rendering.
 */
export function useTimelineScroll({
  containerRef,
  activeThreadId,
  pageSize,
  autoCollapseThreshold,
  totalTurns,
  busy,
  scrollDeps,
  onAnchorCorrected
}: UseTimelineScrollOptions): UseTimelineScrollResult {
  const { contentKey, streaming, userTurnKey } = scrollDeps
  const streamingPhase = scrollDeps.phase ?? (streaming ? 'final_answer' : 'idle')
  const followDistancePx = timelineFollowDistanceForPhase(streamingPhase)
  const shouldCollapseHistory = totalTurns > autoCollapseThreshold
  const [visibleTurnCount, setVisibleTurnCount] = useState(() =>
    deriveTimelineVisibleTurnCount({
      currentVisibleTurnCount: 0,
      totalTurns,
      pageSize,
      shouldCollapseHistory,
      historyExpansionRequested: false
    })
  )
  const [isAtBottom, setIsAtBottom] = useState(true)
  const [bottomDistance, setBottomDistance] = useState(0)
  const hiddenTurnCount = Math.max(0, totalTurns - visibleTurnCount)

  const stickToBottomRef = useRef(true)
  const followModeRef = useRef<TimelineFollowMode>('user_follow')
  const streamingPhaseRef = useRef<TimelineStreamingPhase>(streamingPhase)
  streamingPhaseRef.current = streamingPhase
  const bottomDistanceRef = useRef(0)
  const followDistanceRef = useRef(followDistancePx)
  const visibleTurnCountRef = useRef(visibleTurnCount)
  const activeThreadIdRef = useRef(activeThreadId)
  const restoreConfigRef = useRef({ pageSize, totalTurns, shouldCollapseHistory })
  const lastUserTurnKeyRef = useRef(userTurnKey)
  const historyExpansionRequestedRef = useRef(false)
  const pendingPrependRef = useRef<{ scrollHeight: number; scrollTop: number } | null>(null)
  const prependInFlightRef = useRef(false)
  const scrollFrameRef = useRef<number | null>(null)
  const lastObservedScrollTopRef = useRef<number | null>(null)
  const userScrolledAwayFromBottomRef = useRef(false)
  const scrollLayoutRef = useRef<ThreadScrollLayout | null>(null)
  if (!scrollLayoutRef.current) {
    scrollLayoutRef.current = new ThreadScrollLayout(TIMELINE_STICK_TO_BOTTOM_PX)
  }

  useEffect(() => {
    visibleTurnCountRef.current = visibleTurnCount
  }, [visibleTurnCount])

  useEffect(() => {
    followDistanceRef.current = followDistancePx
  }, [followDistancePx])

  useEffect(() => {
    restoreConfigRef.current = { pageSize, totalTurns, shouldCollapseHistory }
  }, [pageSize, shouldCollapseHistory, totalTurns])

  const recordAnchorCorrection = useCallback(
    ({
      el,
      previousTop,
      nextTop,
      streaming,
      prepend,
      reset
    }: {
      el: HTMLDivElement
      previousTop: number
      nextTop: number
      streaming: boolean
      prepend: boolean
      reset: boolean
    }): void => {
      const delta = nextTop - previousTop
      if (Math.abs(delta) < 0.5) return
      onAnchorCorrected?.({
        previousTop: Math.round(previousTop),
        nextTop: Math.round(nextTop),
        delta: Math.round(delta),
        bottomDistance: Math.round(
          computeBottomDistance({
            totalHeight: el.scrollHeight,
            scrollTop: nextTop,
            viewportHeight: el.clientHeight
          })
        ),
        streaming,
        prepend,
        reset
      })
    },
    [onAnchorCorrected]
  )

  const loadEarlierTurns = useCallback(
    (options?: { userInitiated?: boolean }): void => {
      if (hiddenTurnCount === 0 || prependInFlightRef.current) return
      if (options?.userInitiated) {
        historyExpansionRequestedRef.current = true
      }
      const el = containerRef.current
      if (el) {
        pendingPrependRef.current = {
          scrollHeight: el.scrollHeight,
          scrollTop: el.scrollTop
        }
      }
      prependInFlightRef.current = true
      setVisibleTurnCount((count) => Math.min(totalTurns, count + pageSize))
    },
    [containerRef, hiddenTurnCount, pageSize, totalTurns]
  )

  const collapseEarlierTurns = useCallback((): void => {
    historyExpansionRequestedRef.current = false
    setVisibleTurnCount(pageSize)
  }, [pageSize])

  const saveScrollPosition = useCallback((threadId: string | null): void => {
    const normalizedThreadId = threadId?.trim()
    const el = containerRef.current
    if (!normalizedThreadId || !el) return
    const bottomDistance = computeBottomDistance({
      totalHeight: el.scrollHeight,
      scrollTop: el.scrollTop,
      viewportHeight: el.clientHeight
    })
    const followMode = deriveTimelineFollowMode({
      phase: streamingPhaseRef.current,
      bottomDistance,
      previousMode: followModeRef.current,
      userScrolledAway: userScrolledAwayFromBottomRef.current
    })
    rememberScrollPosition(normalizedThreadId, {
      scrollTop: el.scrollTop,
      bottomDistance,
      visibleTurnCount: visibleTurnCountRef.current,
      historyExpansionRequested: historyExpansionRequestedRef.current,
      stickToBottom: timelineFollowModeShouldAutoScroll(followMode)
    })
  }, [containerRef])

  const scrollToBottom = useCallback((options?: { behavior?: ScrollBehavior }): void => {
    const el = containerRef.current
    if (!el) return
    userScrolledAwayFromBottomRef.current = false
    followModeRef.current = deriveTimelineFollowMode({
      phase: streamingPhase,
      bottomDistance: 0,
      previousMode: followModeRef.current,
      userRequestedBottom: true
    })
    stickToBottomRef.current = timelineFollowModeShouldAutoScroll(followModeRef.current)
    bottomDistanceRef.current = 0
    setBottomDistance(0)
    setIsAtBottom(true)
    const previousTop = el.scrollTop
    const nextTop = scrollTopForBottomDistance({
      bottomDistance: 0,
      viewportHeight: el.clientHeight,
      totalHeight: el.scrollHeight
    })
    if (options?.behavior && options.behavior !== 'auto') {
      el.scrollTo({ top: nextTop, behavior: options.behavior })
    } else {
      el.scrollTop = nextTop
    }
    lastObservedScrollTopRef.current = nextTop
    scrollLayoutRef.current?.capture({
      totalHeight: el.scrollHeight,
      scrollTop: nextTop,
      viewportHeight: el.clientHeight
    })
    recordAnchorCorrection({
      el,
      previousTop,
      nextTop,
      streaming: false,
      prepend: false,
      reset: false
    })
  }, [containerRef, recordAnchorCorrection, streamingPhase])

  const snapToBottomIfFollowing = useCallback((): void => {
    if (userScrolledAwayFromBottomRef.current) return
    if (!timelineFollowModeShouldAutoScroll(followModeRef.current)) return
    scrollToBottom({ behavior: 'auto' })
  }, [scrollToBottom])

  const snapToBottomIfNearLive = useCallback((): void => {
    if (userScrolledAwayFromBottomRef.current) return
    const el = containerRef.current
    const currentBottomDistance = el
      ? computeBottomDistance({
          totalHeight: el.scrollHeight,
          scrollTop: el.scrollTop,
          viewportHeight: el.clientHeight
        })
      : bottomDistanceRef.current
    if (
      !timelineFollowModeShouldAutoScroll(followModeRef.current) &&
      !isNearBottom(currentBottomDistance, TIMELINE_PREWORK_FOLLOW_PX)
    ) {
      return
    }
    scrollToBottom({ behavior: 'auto' })
  }, [containerRef, scrollToBottom])

  const preserveBottomOnResize = useCallback((): void => {
    if (userScrolledAwayFromBottomRef.current) return
    if (!timelineFollowModeShouldAutoScroll(followModeRef.current)) return
    const el = containerRef.current
    if (!el) return
    const previousTop = el.scrollTop
    const nextTop = scrollTopForBottomDistance({
      bottomDistance: Math.min(bottomDistanceRef.current, followDistanceRef.current),
      viewportHeight: el.clientHeight,
      totalHeight: el.scrollHeight
    })
    if (Math.abs(previousTop - nextTop) < 0.5) return
    el.scrollTop = nextTop
    lastObservedScrollTopRef.current = nextTop
    const layoutState = scrollLayoutRef.current?.capture({
      totalHeight: el.scrollHeight,
      scrollTop: nextTop,
      viewportHeight: el.clientHeight
    })
    const distanceToBottom = layoutState?.bottomDistance ?? computeBottomDistance({
      totalHeight: el.scrollHeight,
      scrollTop: nextTop,
      viewportHeight: el.clientHeight
    })
    bottomDistanceRef.current = distanceToBottom
    setBottomDistance(distanceToBottom)
    followModeRef.current = deriveTimelineFollowMode({
      phase: streamingPhase,
      bottomDistance: distanceToBottom,
      previousMode: followModeRef.current
    })
    stickToBottomRef.current = timelineFollowModeShouldAutoScroll(followModeRef.current)
    setIsAtBottom(isNearBottom(distanceToBottom, TIMELINE_AT_BOTTOM_PX))
    recordAnchorCorrection({
      el,
      previousTop,
      nextTop,
      streaming: false,
      prepend: false,
      reset: false
    })
  }, [containerRef, recordAnchorCorrection, streamingPhase])

  // A freshly submitted user turn should become visible even if the user was
  // reading older history before pressing Enter.
  useLayoutEffect(() => {
    if (!userTurnKey || lastUserTurnKeyRef.current === userTurnKey) return
    lastUserTurnKeyRef.current = userTurnKey
    userScrolledAwayFromBottomRef.current = false
    followModeRef.current = deriveTimelineFollowMode({
      phase: streamingPhase,
      bottomDistance: 0,
      previousMode: followModeRef.current,
      userSubmitted: true
    })
    stickToBottomRef.current = timelineFollowModeShouldAutoScroll(followModeRef.current)
    setIsAtBottom(true)
    const el = containerRef.current
    if (!el) return
    const previousTop = el.scrollTop
    const nextTop = scrollTopForBottomDistance({
      bottomDistance: 0,
      viewportHeight: el.clientHeight,
      totalHeight: el.scrollHeight
    })
    el.scrollTop = nextTop
    lastObservedScrollTopRef.current = nextTop
    bottomDistanceRef.current = 0
    setBottomDistance(0)
    scrollLayoutRef.current?.capture({
      totalHeight: el.scrollHeight,
      scrollTop: nextTop,
      viewportHeight: el.clientHeight
    })
    recordAnchorCorrection({
      el,
      previousTop,
      nextTop,
      streaming: true,
      prepend: false,
      reset: false
    })
  }, [containerRef, recordAnchorCorrection, streamingPhase, userTurnKey])

  // Scroll listener: tracks stick-to-bottom + triggers lazy load.
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const onScroll = (): void => {
      const layoutState = scrollLayoutRef.current?.capture({
        totalHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        viewportHeight: el.clientHeight
      })
      const distanceToBottom = layoutState?.bottomDistance ?? computeBottomDistance({
        totalHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        viewportHeight: el.clientHeight
      })
      bottomDistanceRef.current = distanceToBottom
      setBottomDistance((value) =>
        Math.abs(value - distanceToBottom) < BOTTOM_DISTANCE_STATE_STEP_PX ? value : distanceToBottom
      )
      const nextIsAtBottom = isNearBottom(distanceToBottom, TIMELINE_AT_BOTTOM_PX)
      const previousScrollTop = lastObservedScrollTopRef.current
      const userMovedViewportUp =
        previousScrollTop !== null && el.scrollTop < previousScrollTop - 0.5
      if (userMovedViewportUp) {
        userScrolledAwayFromBottomRef.current = true
      } else if (nextIsAtBottom) {
        userScrolledAwayFromBottomRef.current = false
      }
      followModeRef.current = deriveTimelineFollowMode({
        phase: streamingPhase,
        bottomDistance: distanceToBottom,
        previousMode: followModeRef.current,
        userScrolledAway: userScrolledAwayFromBottomRef.current
      })
      stickToBottomRef.current = timelineFollowModeShouldAutoScroll(followModeRef.current)
      setIsAtBottom((value) => value === nextIsAtBottom ? value : nextIsAtBottom)
      lastObservedScrollTopRef.current = el.scrollTop
      saveScrollPosition(activeThreadId)
      if (hiddenTurnCount > 0 && el.scrollTop <= TOP_LOAD_TRIGGER_PX) {
        loadEarlierTurns({ userInitiated: true })
      }
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [activeThreadId, containerRef, hiddenTurnCount, loadEarlierTurns, saveScrollPosition, streamingPhase])

  // Snap to bottom when content changes, but only if the user was
  // already at the bottom.
  useLayoutEffect(() => {
    if (userScrolledAwayFromBottomRef.current) return
    if (!timelineFollowModeShouldAutoScroll(followModeRef.current)) return
    const el = containerRef.current
    if (!el) return
    if (scrollFrameRef.current !== null) {
      window.cancelAnimationFrame(scrollFrameRef.current)
      scrollFrameRef.current = null
    }
    const applyScroll = (): void => {
      const targetTop = scrollTopForBottomDistance({
        bottomDistance: streaming ? 0 : Math.min(bottomDistanceRef.current, followDistanceRef.current),
        viewportHeight: el.clientHeight,
        totalHeight: el.scrollHeight
      })
      const previousTop = el.scrollTop
      el.scrollTop = targetTop
      lastObservedScrollTopRef.current = targetTop
      scrollLayoutRef.current?.capture({
        totalHeight: el.scrollHeight,
        scrollTop: targetTop,
        viewportHeight: el.clientHeight
      })
      recordAnchorCorrection({
        el,
        previousTop,
        nextTop: targetTop,
        streaming,
        prepend: false,
        reset: false
      })
      bottomDistanceRef.current = streaming ? 0 : Math.min(bottomDistanceRef.current, followDistanceRef.current)
      setBottomDistance(bottomDistanceRef.current)
      setIsAtBottom(isNearBottom(bottomDistanceRef.current, TIMELINE_AT_BOTTOM_PX))
    }
    scrollFrameRef.current = window.requestAnimationFrame(() => {
      scrollFrameRef.current = null
      applyScroll()
    })
  }, [containerRef, contentKey, recordAnchorCorrection, streaming])

  // Restore the thread's own viewport on thread switch. Returning to a thread
  // should not yank the reader to the bottom unless no position is known.
  useEffect(() => {
    const previousThreadId = activeThreadIdRef.current
    if (previousThreadId && previousThreadId !== activeThreadId) {
      saveScrollPosition(previousThreadId)
    }
    activeThreadIdRef.current = activeThreadId

    const saved = activeThreadId ? scrollPositionsByThread.get(activeThreadId) : undefined
    const restoreConfig = restoreConfigRef.current
    stickToBottomRef.current = saved?.stickToBottom ?? true
    followModeRef.current = saved?.stickToBottom ? 'user_follow' : 'static'
    userScrolledAwayFromBottomRef.current = saved ? !saved.stickToBottom : false
    bottomDistanceRef.current = saved?.bottomDistance ?? 0
    setBottomDistance(bottomDistanceRef.current)
    historyExpansionRequestedRef.current = saved?.historyExpansionRequested ?? false
    pendingPrependRef.current = null
    prependInFlightRef.current = false
    setIsAtBottom(saved ? isNearBottom(saved.bottomDistance, TIMELINE_AT_BOTTOM_PX) : true)
    if (saved) {
      setVisibleTurnCount(
        deriveTimelineVisibleTurnCount({
          currentVisibleTurnCount: saved.visibleTurnCount,
          totalTurns: restoreConfig.totalTurns,
          pageSize: restoreConfig.pageSize,
          shouldCollapseHistory: restoreConfig.shouldCollapseHistory,
          historyExpansionRequested: saved.historyExpansionRequested
        })
      )
    }
    if (scrollFrameRef.current !== null) {
      window.cancelAnimationFrame(scrollFrameRef.current)
      scrollFrameRef.current = null
    }
    const el = containerRef.current
    if (el) {
      const previousTop = el.scrollTop
      const maxTop = Math.max(0, el.scrollHeight - el.clientHeight)
      const nextTop = saved && !saved.stickToBottom
        ? Math.min(saved.scrollTop, maxTop)
        : scrollTopForBottomDistance({
            bottomDistance: 0,
            viewportHeight: el.clientHeight,
            totalHeight: el.scrollHeight
          })
      el.scrollTop = nextTop
      lastObservedScrollTopRef.current = nextTop
      const layoutState = scrollLayoutRef.current?.capture({
        totalHeight: el.scrollHeight,
        scrollTop: nextTop,
        viewportHeight: el.clientHeight
      })
      const restoredBottomDistance = layoutState?.bottomDistance ?? computeBottomDistance({
        totalHeight: el.scrollHeight,
        scrollTop: nextTop,
        viewportHeight: el.clientHeight
      })
      bottomDistanceRef.current = restoredBottomDistance
      setBottomDistance(restoredBottomDistance)
      followModeRef.current = deriveTimelineFollowMode({
        phase: streamingPhaseRef.current,
        bottomDistance: restoredBottomDistance,
        previousMode: followModeRef.current,
        userScrolledAway: userScrolledAwayFromBottomRef.current
      })
      stickToBottomRef.current = timelineFollowModeShouldAutoScroll(followModeRef.current)
      setIsAtBottom(isNearBottom(restoredBottomDistance, TIMELINE_AT_BOTTOM_PX))
      recordAnchorCorrection({
        el,
        previousTop,
        nextTop,
        streaming: false,
        prepend: false,
        reset: true
      })
    }
  }, [
    activeThreadId,
    containerRef,
    recordAnchorCorrection,
    saveScrollPosition
  ])

  // Cleanup any pending rAF on unmount.
  useEffect(
    () => () => {
      saveScrollPosition(activeThreadIdRef.current)
      if (scrollFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollFrameRef.current)
      }
    },
    [saveScrollPosition]
  )

  // Re-derive visible count when the thread / collapse flag / total
  // turns change.
  useEffect(() => {
    setVisibleTurnCount((count) =>
      deriveTimelineVisibleTurnCount({
        currentVisibleTurnCount: count,
        totalTurns,
        pageSize,
        shouldCollapseHistory,
        historyExpansionRequested: historyExpansionRequestedRef.current
      })
    )
  }, [activeThreadId, pageSize, shouldCollapseHistory, totalTurns])

  // While a turn is running, keep the latest page visible without
  // mounting every historical turn. Expanding all history during SSE
  // streaming can repaint long conversations and make the viewport look
  // like it scrolled through the whole thread.
  useEffect(() => {
    if (!busy) return
    setVisibleTurnCount((count) =>
      deriveTimelineVisibleTurnCount({
        currentVisibleTurnCount: count,
        totalTurns,
        pageSize,
        shouldCollapseHistory,
        historyExpansionRequested: historyExpansionRequestedRef.current
      })
    )
  }, [busy, pageSize, shouldCollapseHistory, totalTurns])

  // After a prepend, restore scroll position so the user's viewport
  // doesn't jump.
  useEffect(() => {
    const snapshot = pendingPrependRef.current
    const el = containerRef.current
    if (!snapshot || !el) return

    pendingPrependRef.current = null
    prependInFlightRef.current = false

    requestAnimationFrame(() => {
      const previousTop = el.scrollTop
      const nextTop = scrollLayoutRef.current?.nextScrollTopAfterPrepend(
        snapshot.scrollTop,
        snapshot.scrollHeight,
        el.scrollHeight
      ) ?? preservePrependOffset({
        previousScrollTop: snapshot.scrollTop,
        previousTotalHeight: snapshot.scrollHeight,
        nextTotalHeight: el.scrollHeight
      })
      el.scrollTop = nextTop
      lastObservedScrollTopRef.current = nextTop
      scrollLayoutRef.current?.capture({
        totalHeight: el.scrollHeight,
        scrollTop: nextTop,
        viewportHeight: el.clientHeight
      })
      const nextBottomDistance = computeBottomDistance({
        totalHeight: el.scrollHeight,
        scrollTop: nextTop,
        viewportHeight: el.clientHeight
      })
      bottomDistanceRef.current = nextBottomDistance
      setBottomDistance(nextBottomDistance)
      recordAnchorCorrection({
        el,
        previousTop,
        nextTop,
        streaming: false,
        prepend: true,
        reset: false
      })
    })
  }, [containerRef, recordAnchorCorrection, visibleTurnCount])

  // If the user explicitly asked to expand history and the container
  // still has room, keep loading earlier turns until it overflows.
  useEffect(() => {
    const el = containerRef.current
    if (!el || hiddenTurnCount === 0 || prependInFlightRef.current) return
    if (!historyExpansionRequestedRef.current) return
    if (el.scrollHeight <= el.clientHeight + TOP_LOAD_TRIGGER_PX) {
      loadEarlierTurns()
    }
  }, [containerRef, hiddenTurnCount, loadEarlierTurns, visibleTurnCount])

  return {
    visibleTurnCount,
    hiddenTurnCount,
    isAtBottom,
    bottomDistance,
    loadEarlierTurns,
    collapseEarlierTurns,
    preserveBottomOnResize,
    scrollToBottom,
    snapToBottomIfFollowing,
    snapToBottomIfNearLive
  }
}
