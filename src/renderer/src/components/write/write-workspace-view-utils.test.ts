import { afterEach, describe, expect, it, vi } from 'vitest'
import { computeWriteDocumentStats, inlineAgentPosition } from './write-workspace-view-utils'

afterEach(() => vi.unstubAllGlobals())

describe('selection menu viewport coordinates', () => {
  it.each([0.75, 1, 1.25])('anchors to the actual selection with body zoom %s', (zoom) => {
    vi.stubGlobal('document', { body: {} })
    vi.stubGlobal('window', { innerWidth: 1200, getComputedStyle: () => ({ zoom: String(zoom) }) })
    const position = inlineAgentPosition({ anchorRect: { left: 720, top: 160, bottom: 180, width: 160 } })!
    expect((position.left + position.width / 2) * zoom).toBeCloseTo(800)
    expect(position.anchorTop * zoom).toBeCloseTo(160)
    expect(position.anchorBottom * zoom).toBeCloseTo(180)
    expect((position.left + position.width) * zoom).toBeLessThan(1200)
  })

  it('keeps a menu inside a viewport narrower than its preferred width', () => {
    vi.stubGlobal('document', { body: {} })
    vi.stubGlobal('window', { innerWidth: 240, getComputedStyle: () => ({ zoom: '1' }) })
    const position = inlineAgentPosition({ anchorRect: { left: 220, top: 40, bottom: 60, width: 12 } })!
    expect(position.left).toBeGreaterThanOrEqual(16)
    expect(position.left + position.width).toBeLessThanOrEqual(224)
  })
})

describe('computeWriteDocumentStats', () => {
  it('counts visible markdown text instead of syntax markers', () => {
    const stats = computeWriteDocumentStats('# 标题\n\n- 第一项\n- 第二项 **加粗**\n', true)

    expect(stats).toEqual({ characterCount: 10 })
  })

  it('counts non-whitespace characters for plain text files', () => {
    const stats = computeWriteDocumentStats('Hello world\n  2026  ', false)

    expect(stats).toEqual({ characterCount: 14 })
  })
})
