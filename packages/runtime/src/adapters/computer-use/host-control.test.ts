import { describe, expect, it } from 'vitest'
import { computeDisplayDims, mapDisplayToLogical } from './host-control.js'

describe('computeDisplayDims', () => {
  it('passes small screens through unscaled', () => {
    expect(computeDisplayDims(1024, 768, 1280)).toEqual({ width: 1024, height: 768, scale: 1 })
  })

  it('caps the long edge and preserves aspect ratio', () => {
    const dims = computeDisplayDims(2560, 1600, 1280)
    expect(dims.width).toBe(1280)
    expect(dims.height).toBe(800)
    expect(dims.scale).toBeCloseTo(0.5)
  })

  it('caps tall screens by height', () => {
    const dims = computeDisplayDims(1080, 1920, 1280)
    expect(dims.height).toBe(1280)
    expect(dims.width).toBe(720)
  })
})

describe('mapDisplayToLogical', () => {
  it('maps display pixels back to logical points on a downscaled screen', () => {
    expect(mapDisplayToLogical(640, 400, 2560, 1600, 1280)).toEqual({ x: 1280, y: 800 })
    expect(mapDisplayToLogical(0, 0, 2560, 1600, 1280)).toEqual({ x: 0, y: 0 })
  })

  it('is identity when the screen is within the cap', () => {
    expect(mapDisplayToLogical(100, 200, 1024, 768, 1280)).toEqual({ x: 100, y: 200 })
  })

  it('clamps to screen bounds', () => {
    expect(mapDisplayToLogical(99999, 99999, 1024, 768, 1280)).toEqual({ x: 1023, y: 767 })
  })
})
