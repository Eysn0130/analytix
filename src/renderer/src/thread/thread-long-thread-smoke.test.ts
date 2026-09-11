import { describe, expect, it } from 'vitest'
import { sanitizeThreadTraceEvent } from '@shared/thread-trace'
import type { ChatBlock } from '../agent/types'
import { buildThreadProjection } from './projection/thread-projection-store'
import { computeVirtualThreadWindow } from './virtualizer/analytix-thread-virtualizer'
import { createMeasuredRowCache } from './virtualizer/measured-row-cache'
import { ThreadScrollLayout } from './virtualizer/scroll-layout'

const LONG_MARKDOWN = [
  'Here is a structured update with markdown.',
  '',
  '```ts',
  'export function answer(value: number): number {',
  '  return value * 2',
  '}',
  '```',
  '',
  '- Keep streaming lightweight.',
  '- Finalize rich markdown later.'
].join('\n')

function makeLongThreadBlocks(turnCount: number): ChatBlock[] {
  const blocks: ChatBlock[] = []
  for (let index = 0; index < turnCount; index += 1) {
    blocks.push({
      kind: 'user',
      id: `user-${index}`,
      text: `Turn ${index}: inspect workspace and explain the next change.`
    })
    if (index % 5 === 0) {
      blocks.push({
        kind: 'tool',
        id: `tool-${index}`,
        summary: 'Read source file',
        status: 'success',
        toolKind: 'tool_call',
        detail: `Read ${index + 1} files and returned bounded context.`
      })
    }
    if (index % 9 === 0) {
      blocks.push({
        kind: 'approval',
        id: `approval-${index}`,
        approvalId: `approval-${index}`,
        summary: 'Allow command execution?',
        status: index % 18 === 0 ? 'allowed' : 'pending'
      })
    }
    if (index % 11 === 0) {
      blocks.push({
        kind: 'user_input',
        id: `input-${index}`,
        requestId: `input-${index}`,
        questions: [
          {
            header: 'Scope',
            id: 'scope',
            question: 'Which verification scope should be used?',
            options: [
              { label: 'Focused', description: 'Run targeted checks.' },
              { label: 'Full', description: 'Run the full suite.' }
            ]
          }
        ],
        status: 'pending'
      })
    }
    if (index % 13 === 0) {
      blocks.push({
        kind: 'tool',
        id: `file-change-${index}`,
        summary: 'Updated file',
        status: 'success',
        toolKind: 'file_change',
        filePath: `/workspace/src/file-${index}.ts`,
        detail: [
          `--- a/src/file-${index}.ts`,
          `+++ b/src/file-${index}.ts`,
          '@@',
          '-old line',
          '+new line'
        ].join('\n')
      })
    }
    if (index % 17 === 0) {
      blocks.push({
        kind: 'review',
        id: `review-${index}`,
        title: 'Review summary',
        status: 'success',
        reviewText: 'No blocking findings in this bounded smoke fixture.'
      })
    }
    blocks.push({
      kind: 'assistant',
      id: `assistant-${index}`,
      text: `${LONG_MARKDOWN}\n\nCompleted turn ${index}.`
    })
  }
  return blocks
}

describe('long thread renderer smoke', () => {
  it('projects, virtualizes, anchors, and sanitizes a 120-turn streaming thread', () => {
    const blocks = makeLongThreadBlocks(120)
    const projection = buildThreadProjection({
      blocks,
      live: `${LONG_MARKDOWN}\n\nstreaming delta`,
      activeThreadId: 'thread-smoke',
      workspaceRoot: '/workspace',
      currentTurnUserId: 'user-119',
      turnStartedAtByUserId: { 'user-119': 1_000 },
      turnDurationByUserId: {},
      busy: true,
      tickNow: 2_000,
      hiddenTurnCount: 0,
      pageSize: 18,
      autoCollapseThreshold: 24,
      activeThreadGoal: null,
      turnPreview: (turn, fallback) => turn.user?.text ?? fallback,
      turnTitle: (index) => `Turn ${index}`
    })

    expect(projection.turns).toHaveLength(120)
    expect(projection.anchors).toHaveLength(120)
    expect(projection.userMessageNavigationItems).toHaveLength(120)
    expect(projection.rows.length).toBeGreaterThan(120)
    const latestTurn = [...projection.rows].reverse().find((row) => row.kind === 'turn')
    expect(latestTurn?.kind === 'turn' && latestTurn.turn.hasLiveStream).toBe(true)

    const cache = createMeasuredRowCache()
    for (const [index, row] of projection.rows.entries()) {
      if (index % 7 === 0) cache.set(row.id, 160 + index)
    }
    const windowed = computeVirtualThreadWindow({
      rows: projection.rows,
      cache,
      scrollTop: 7_200,
      viewportHeight: 820,
      overscanPx: 900,
      rowGapPx: 32
    })
    expect(windowed.totalHeight).toBeGreaterThan(20_000)
    expect(windowed.items.length).toBeGreaterThan(0)
    expect(windowed.items.length).toBeLessThan(projection.rows.length)
    expect(windowed.beforeHeight + windowed.afterHeight).toBeGreaterThan(0)

    const layout = new ThreadScrollLayout()
    layout.capture({ totalHeight: 30_000, scrollTop: 29_200, viewportHeight: 800 })
    expect(layout.nextScrollTopAfterResize(
      { totalHeight: 30_000, scrollTop: 29_200, viewportHeight: 800 },
      30_640
    )).toBe(29_840)
    layout.capture({ totalHeight: 30_000, scrollTop: 12_000, viewportHeight: 800 })
    expect(layout.nextScrollTopAfterResize(
      { totalHeight: 30_000, scrollTop: 12_000, viewportHeight: 800 },
      30_640
    )).toBeNull()
    expect(layout.nextScrollTopAfterPrepend(240, 30_000, 31_280)).toBe(1_520)

    const trace = sanitizeThreadTraceEvent({
      name: 'thread.react.commit_sample',
      timestamp: 1,
      threadId: 'thread-smoke',
      data: {
        rows: projection.rows.length,
        renderedRows: windowed.items.length,
        virtualized: true,
        prompt: 'must not be persisted',
        output: LONG_MARKDOWN
      }
    })
    expect(trace.data).toEqual({
      rows: projection.rows.length,
      renderedRows: windowed.items.length,
      virtualized: true
    })
    expect(JSON.stringify(trace)).not.toContain('must not be persisted')
    expect(JSON.stringify(trace)).not.toContain('export function answer')
  })
})
