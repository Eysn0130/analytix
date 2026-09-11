import { describe, expect, it } from 'vitest'
import { compileMemoryContext } from '../src/memory/memory-context-compiler.js'

describe('memory context compiler', () => {
  it('deduplicates memory anchors, enforces limits, and reports omitted ids', () => {
    const result = compileMemoryContext([
      {
        id: 'mem_1',
        scope: 'workspace',
        content: 'Use pnpm for frontend installs and prefer TypeScript for UI work.',
        confidence: 1,
        tags: []
      },
      {
        id: 'mem_2',
        scope: 'workspace',
        content: '  use pnpm for frontend installs and prefer typescript for ui work.  ',
        confidence: 1,
        tags: []
      },
      {
        id: 'mem_3',
        scope: 'workspace',
        content: 'This is a very long project memory about provider endpoint format behavior and custom endpoint paths.',
        confidence: 0.8,
        tags: []
      },
      {
        id: 'mem_4',
        scope: 'workspace',
        content: 'Another relevant memory that should be omitted by maxReferences.',
        confidence: 0.8,
        tags: []
      }
    ], {
      maxContextChars: 240,
      maxItemChars: 95,
      maxReferences: 2
    })

    const instruction = result.instructions.join('\n')
    expect(result.memoryIds).toEqual(['mem_1', 'mem_3'])
    expect(result.omittedMemoryIds).toEqual(['mem_2', 'mem_4'])
    expect(result.duplicateCount).toBe(1)
    expect(result.retainedCount).toBe(2)
    expect(result.originalCount).toBe(4)
    expect(result.truncated).toBe(true)
    expect(result.totalChars).toBeLessThanOrEqual(240)
    expect(instruction).toContain('[mem_1]')
    expect(instruction).toContain('[mem_3]')
    expect(instruction).toContain('...[truncated]')
    expect(instruction).not.toContain('[mem_2]')
    expect(instruction).not.toContain('[mem_4]')
  })

  it('returns an empty compiled context for blank memory content', () => {
    const result = compileMemoryContext([
      {
        id: 'mem_blank',
        scope: 'workspace',
        content: '   ',
        confidence: 1,
        tags: []
      }
    ])

    expect(result.instructions).toEqual([])
    expect(result.memoryIds).toEqual([])
    expect(result.omittedMemoryIds).toEqual(['mem_blank'])
    expect(result.totalChars).toBe(0)
  })
})
