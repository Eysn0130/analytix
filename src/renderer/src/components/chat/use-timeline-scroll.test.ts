import { describe, expect, it } from 'vitest'
import { isNearBottom } from '../../thread/virtualizer/bottom-distance-anchor'
import {
  deriveTimelineFollowMode,
  deriveTimelineVisibleTurnCount,
  TIMELINE_AT_BOTTOM_PX,
  TIMELINE_PREWORK_FOLLOW_PX,
  timelineFollowDistanceForPhase,
  timelineFollowModeShouldAutoScroll,
  TIMELINE_STICK_TO_BOTTOM_PX
} from './use-timeline-scroll'

describe('deriveTimelineVisibleTurnCount', () => {
  it('keeps long conversations on the latest page instead of expanding all turns', () => {
    expect(
      deriveTimelineVisibleTurnCount({
        currentVisibleTurnCount: 18,
        totalTurns: 36,
        pageSize: 18,
        shouldCollapseHistory: true,
        historyExpansionRequested: false
      })
    ).toBe(18)
  })

  it('renders every turn for short conversations below the collapse threshold', () => {
    expect(
      deriveTimelineVisibleTurnCount({
        currentVisibleTurnCount: 5,
        totalTurns: 6,
        pageSize: 18,
        shouldCollapseHistory: false,
        historyExpansionRequested: false
      })
    ).toBe(6)
  })

  it('preserves a user-expanded history window while new turns arrive', () => {
    expect(
      deriveTimelineVisibleTurnCount({
        currentVisibleTurnCount: 36,
        totalTurns: 50,
        pageSize: 18,
        shouldCollapseHistory: true,
        historyExpansionRequested: true
      })
    ).toBe(36)
  })

  it('caps a user-expanded history window at the current turn count', () => {
    expect(
      deriveTimelineVisibleTurnCount({
        currentVisibleTurnCount: 36,
        totalTurns: 24,
        pageSize: 18,
        shouldCollapseHistory: true,
        historyExpansionRequested: true
      })
    ).toBe(24)
  })
})

describe('TIMELINE_STICK_TO_BOTTOM_PX', () => {
  it('keeps visible bottom state and live-follow threshold aligned', () => {
    expect(isNearBottom(TIMELINE_AT_BOTTOM_PX, TIMELINE_AT_BOTTOM_PX)).toBe(true)
    expect(isNearBottom(TIMELINE_AT_BOTTOM_PX + 1, TIMELINE_AT_BOTTOM_PX)).toBe(false)
    expect(TIMELINE_STICK_TO_BOTTOM_PX).toBe(TIMELINE_AT_BOTTOM_PX)
    expect(isNearBottom(TIMELINE_STICK_TO_BOTTOM_PX, TIMELINE_STICK_TO_BOTTOM_PX)).toBe(true)
    expect(isNearBottom(TIMELINE_STICK_TO_BOTTOM_PX + 1, TIMELINE_STICK_TO_BOTTOM_PX)).toBe(false)
  })
})

describe('timelineFollowDistanceForPhase', () => {
  it('uses a wider follow band for prework and strict bottom follow for final answers', () => {
    expect(timelineFollowDistanceForPhase('prework')).toBe(TIMELINE_PREWORK_FOLLOW_PX)
    expect(timelineFollowDistanceForPhase('final_answer')).toBe(TIMELINE_STICK_TO_BOTTOM_PX)
    expect(timelineFollowDistanceForPhase('idle')).toBe(TIMELINE_STICK_TO_BOTTOM_PX)
  })
})

describe('deriveTimelineFollowMode', () => {
  it('uses explicit user intent and keeps live prework follow within its guard band', () => {
    expect(deriveTimelineFollowMode({
      phase: 'prework',
      bottomDistance: 80,
      previousMode: 'static',
      userSubmitted: true
    })).toBe('user_follow')

    expect(deriveTimelineFollowMode({
      phase: 'prework',
      bottomDistance: 80,
      previousMode: 'user_follow'
    })).toBe('user_follow')

    expect(deriveTimelineFollowMode({
      phase: 'prework',
      bottomDistance: 80,
      previousMode: 'static'
    })).toBe('prework_watch')

    expect(timelineFollowModeShouldAutoScroll('prework_watch')).toBe(false)
    expect(timelineFollowModeShouldAutoScroll('prework_follow')).toBe(true)
    expect(timelineFollowModeShouldAutoScroll('user_follow')).toBe(true)
  })

  it('keeps final answer following strict to the bottom band', () => {
    expect(deriveTimelineFollowMode({
      phase: 'final_answer',
      bottomDistance: TIMELINE_AT_BOTTOM_PX,
      previousMode: 'prework_follow'
    })).toBe('user_follow')
    expect(deriveTimelineFollowMode({
      phase: 'final_answer',
      bottomDistance: TIMELINE_AT_BOTTOM_PX + 1,
      previousMode: 'prework_follow'
    })).toBe('static')
  })

  it('keeps an existing user follow through the prework to final-answer phase change', () => {
    expect(deriveTimelineFollowMode({
      phase: 'final_answer',
      bottomDistance: TIMELINE_PREWORK_FOLLOW_PX,
      previousMode: 'user_follow'
    })).toBe('user_follow')

    expect(deriveTimelineFollowMode({
      phase: 'final_answer',
      bottomDistance: TIMELINE_PREWORK_FOLLOW_PX + 1,
      previousMode: 'user_follow'
    })).toBe('static')
  })

  it('keeps an existing user follow during the final-answer to idle settling frame', () => {
    expect(deriveTimelineFollowMode({
      phase: 'idle',
      bottomDistance: TIMELINE_PREWORK_FOLLOW_PX,
      previousMode: 'user_follow'
    })).toBe('user_follow')

    expect(deriveTimelineFollowMode({
      phase: 'idle',
      bottomDistance: TIMELINE_PREWORK_FOLLOW_PX + 1,
      previousMode: 'user_follow'
    })).toBe('static')
  })

  it('lets an explicit upward user scroll break final-answer bottom follow', () => {
    expect(deriveTimelineFollowMode({
      phase: 'idle',
      bottomDistance: 12,
      previousMode: 'user_follow',
      userScrolledAway: true
    })).toBe('static')

    expect(deriveTimelineFollowMode({
      phase: 'final_answer',
      bottomDistance: TIMELINE_AT_BOTTOM_PX,
      previousMode: 'user_follow',
      userScrolledAway: true
    })).toBe('static')
  })
})
