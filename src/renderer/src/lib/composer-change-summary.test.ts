import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../agent/types'
import {
  collectComposerChangeSummary,
  collectCurrentTurnDiffSummary
} from './composer-change-summary'

describe('collectComposerChangeSummary', () => {
  it('summarizes successful file changes by display path', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'tool',
        id: 'tool_1',
        summary: 'edit',
        status: 'success',
        toolKind: 'file_change',
        detail: [
          'diff --git a/src/a.ts b/src/a.ts',
          '--- a/src/a.ts',
          '+++ b/src/a.ts',
          '@@ -1,2 +1,3 @@',
          ' old',
          '-remove',
          '+add',
          '+more'
        ].join('\n')
      },
      {
        kind: 'tool',
        id: 'tool_2',
        summary: 'edit',
        status: 'success',
        toolKind: 'file_change',
        filePath: 'src/a.ts',
        detail: [
          '--- a/src/a.ts',
          '+++ b/src/a.ts',
          '@@ -1 +1 @@',
          '-again',
          '+next'
        ].join('\n')
      }
    ]

    expect(collectComposerChangeSummary(blocks, '/repo')).toEqual({
      files: [{ path: 'src/a.ts', added: 3, removed: 2 }],
      added: 3,
      removed: 2
    })
  })

  it('summarizes only file changes from the current turn', () => {
    const currentPatch = [
      'diff --git a/src/current.ts b/src/current.ts',
      '--- a/src/current.ts',
      '+++ b/src/current.ts',
      '@@ -1 +1,3 @@',
      '-old',
      '+new',
      '+more',
      '+again'
    ].join('\n')
    const blocks: ChatBlock[] = [
      {
        kind: 'tool',
        id: 'tool_current',
        summary: 'edit',
        status: 'success',
        toolKind: 'file_change',
        detail: currentPatch,
        meta: { turnId: 'turn_current' }
      },
      {
        kind: 'tool',
        id: 'tool_history',
        summary: 'edit',
        status: 'success',
        toolKind: 'file_change',
        detail: [
          'diff --git a/src/history.ts b/src/history.ts',
          '--- a/src/history.ts',
          '+++ b/src/history.ts',
          '@@ -1 +1 @@',
          '-history',
          '+ignored'
        ].join('\n'),
        meta: { turnId: 'turn_history' }
      }
    ]

    expect(collectCurrentTurnDiffSummary(blocks, 'turn_current', '/repo')).toEqual({
      fileCount: 1,
      files: [{ path: 'src/current.ts', added: 3, removed: 1 }],
      added: 3,
      removed: 1
    })
  })

  it('ignores current-turn changes without a matching turn id', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'tool',
        id: 'tool_without_turn',
        summary: 'edit',
        status: 'success',
        toolKind: 'file_change',
        detail: [
          'diff --git a/src/a.ts b/src/a.ts',
          '--- a/src/a.ts',
          '+++ b/src/a.ts',
          '@@ -1 +1 @@',
          '-old',
          '+new'
        ].join('\n')
      }
    ]

    expect(collectCurrentTurnDiffSummary(blocks, 'turn_current', '/repo')).toBeNull()
  })

  it('ignores pending tools and non-diff details', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'tool',
        id: 'tool_1',
        summary: 'edit',
        status: 'running',
        toolKind: 'file_change',
        detail: 'diff --git a/a b/a'
      },
      {
        kind: 'tool',
        id: 'tool_2',
        summary: 'tool',
        status: 'success',
        toolKind: 'tool_call',
        detail: 'ok'
      }
    ]

    expect(collectComposerChangeSummary(blocks, '/repo')).toBeNull()
  })
})
