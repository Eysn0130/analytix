import { describe, expect, it } from 'vitest'

import {
  MACOS_TITLEBAR_SAFE_LEFT,
  MACOS_TRAFFIC_LIGHT_CLUSTER_WIDTH,
  MACOS_TRAFFIC_LIGHT_GAP,
  MACOS_TRAFFIC_LIGHT_POSITION
} from './window-chrome'

describe('window chrome constants', () => {
  it('keeps macOS traffic lights aligned with the shell toolbar row', () => {
    expect(MACOS_TRAFFIC_LIGHT_POSITION).toEqual({ x: 16, y: 16 })
    expect(MACOS_TRAFFIC_LIGHT_GAP).toBe(18)
    expect(MACOS_TITLEBAR_SAFE_LEFT).toBe(
      MACOS_TRAFFIC_LIGHT_POSITION.x + MACOS_TRAFFIC_LIGHT_CLUSTER_WIDTH + MACOS_TRAFFIC_LIGHT_GAP
    )
    expect(MACOS_TITLEBAR_SAFE_LEFT).toBe(100)
  })
})
