import { describe, expect, it } from 'vitest'
import { timelineResponseSpacerHeight } from './MessageTimeline'

describe('timelineResponseSpacerHeight', () => {
  it('uses a viewport-aware live spacer and collapses to a stable idle sentinel', () => {
    expect(timelineResponseSpacerHeight(true)).toBe('clamp(72px, 16vh, 180px)')
    expect(timelineResponseSpacerHeight(false)).toBe(1)
  })
})
