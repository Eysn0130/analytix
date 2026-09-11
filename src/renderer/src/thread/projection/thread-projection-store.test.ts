import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../../agent/types'
import { buildThreadProjection } from './thread-projection-store'
import { reduceThreadDeltas } from './thread-event-reducer'

function projection(
  blocks: ChatBlock[],
  hiddenTurnCount = 0,
  overrides: Partial<Parameters<typeof buildThreadProjection>[0]> = {}
) {
  return buildThreadProjection({
    blocks,
    live: '',
    activeThreadId: 'thread-1',
    workspaceRoot: '/tmp',
    currentTurnUserId: null,
    turnStartedAtByUserId: {},
    turnDurationByUserId: {},
    busy: false,
    tickNow: 1_000,
    hiddenTurnCount,
    pageSize: 2,
    autoCollapseThreshold: 4,
    activeThreadGoal: null,
    turnPreview: (turn, fallback) => turn.user?.text ?? fallback,
    turnTitle: (index) => `Turn ${index}`,
    ...overrides
  })
}

describe('thread projection store', () => {
  it('builds stable turn rows without deriving inside React renderers', () => {
    const snapshot = projection([
      { kind: 'user', id: 'u1', text: 'Hello' },
      { kind: 'assistant', id: 'a1', text: 'Hi' },
      { kind: 'user', id: 'u2', text: 'Next' },
      { kind: 'tool', id: 'tool1', summary: 'Read', status: 'success' }
    ])

    expect(snapshot.rows.filter((row) => row.kind === 'turn').map((row) => row.id)).toEqual([
      'turn:u1',
      'turn:u2'
    ])
    const second = snapshot.rows.find(
      (row): row is Extract<typeof row, { kind: 'turn' }> => row.kind === 'turn' && row.id === 'turn:u2'
    )
    expect(second?.turn.sections.processBlocks.map((block) => block.id)).toEqual(['tool1'])
  })

  it('adds load earlier and jump anchors for hidden turns', () => {
    const snapshot = projection([
      { kind: 'user', id: 'u1', text: 'One' },
      { kind: 'assistant', id: 'a1', text: 'Answer' },
      { kind: 'user', id: 'u2', text: 'Two' },
      { kind: 'assistant', id: 'a2', text: 'Answer' },
      { kind: 'user', id: 'u3', text: 'Three' },
      { kind: 'assistant', id: 'a3', text: 'Answer' }
    ], 1)

    expect(snapshot.rows[0]).toMatchObject({ kind: 'loadEarlier', hiddenCount: 1 })
    expect(snapshot.anchors.map((anchor) => anchor.key)).toEqual(['u2', 'u3'])
    expect(snapshot.anchors.map((anchor) => anchor.label)).toEqual(['2', '3'])
    expect(snapshot.userMessageNavigationItems.map((item) => item.id)).toEqual(['u2:user', 'u3:user'])
    expect(snapshot.userMessageNavigationItems.map((item) => item.turnKey)).toEqual(['u2', 'u3'])
    expect(snapshot.userMessageNavigationItems.map((item) => item.responsePreview)).toEqual(['Answer', 'Answer'])
  })

  it('derives user navigation tooltip context chips from turn files', () => {
    const snapshot = projection([
      {
        kind: 'user',
        id: 'u1',
        text: 'Review these files',
        meta: {
          fileReferences: [
            {
              path: '/tmp/src/thread-row-model.ts',
              relativePath: 'src/thread-row-model.ts',
              name: 'thread-row-model.ts',
              kind: 'file'
            },
            {
              path: '/tmp/src/thread-projection-store.ts',
              relativePath: 'src/thread-projection-store.ts',
              name: 'thread-projection-store.ts',
              kind: 'file'
            }
          ]
        }
      },
      {
        kind: 'tool',
        id: 'tool1',
        summary: 'Edited file',
        status: 'success',
        toolKind: 'file_change',
        filePath: '/tmp/src/base-shell.css'
      }
    ])

    expect(snapshot.userMessageNavigationItems[0]).toMatchObject({
      contextChips: [
        { label: 'thread-row-model.ts', icon: 'typescript' },
        { label: 'thread-projection-store.ts', icon: 'typescript' }
      ],
      contextChipOverflowCount: 1
    })
  })

  it('omits user navigation tooltip context chips when no turn files exist', () => {
    const snapshot = projection([
      {
        kind: 'user',
        id: 'u1',
        text: 'Hello',
        meta: {
          fileReferences: [
            {
              path: '/tmp/Downloads',
              relativePath: 'Downloads',
              name: 'Downloads',
              kind: 'directory'
            }
          ]
        }
      },
      { kind: 'assistant', id: 'a1', text: 'Hi' }
    ])

    expect(snapshot.userMessageNavigationItems[0].contextChips).toBeUndefined()
    expect(snapshot.userMessageNavigationItems[0].contextChipOverflowCount).toBeUndefined()
  })

  it('reduces runtime deltas into projection live state', () => {
    expect(
      reduceThreadDeltas(
        { liveAssistant: 'A', lastSeq: 2 },
        [
          { kind: 'agent_message', text: 'B', seq: 4 }
        ]
      )
	).toEqual({ liveAssistant: 'AB', lastSeq: 4 })
  })

  it('attaches live processing only to the matching turn', () => {
    const snapshot = projection(
      [
        { kind: 'user', id: 'u1', text: 'first', meta: { turnId: 'turn-1' } },
        { kind: 'assistant', id: 'a1', text: 'old answer', meta: { turnId: 'turn-1' } },
        { kind: 'user', id: 'u2', text: 'second', meta: { turnId: 'turn-2' } },
        { kind: 'assistant', id: 'a2', text: 'new answer', meta: { turnId: 'turn-2' } }
      ],
      0,
      {
        currentTurnId: 'turn-1',
        currentTurnUserId: 'u1',
        busy: true
      }
    )

    const turns = snapshot.rows.filter((row): row is Extract<typeof row, { kind: 'turn' }> => row.kind === 'turn')
    expect(turns.map((row) => row.turn.isLiveTurn)).toEqual([true, false])
    expect(turns.map((row) => row.turn.isProcessing)).toEqual([true, false])
  })

  it('lets live turn timing keep moving past a stale recorded duration', () => {
    const snapshot = projection(
      [
        { kind: 'user', id: 'u1', text: 'still running', meta: { turnId: 'turn-1' } }
      ],
      0,
      {
        currentTurnId: 'turn-1',
        currentTurnUserId: 'u1',
        turnStartedAtByUserId: { u1: 1_000 },
        turnDurationByUserId: { u1: 2_000 },
        busy: true,
        tickNow: 9_000
      }
    )

    const row = snapshot.rows.find((item): item is Extract<typeof item, { kind: 'turn' }> => item.kind === 'turn')
    expect(row?.turn.timing.durationMs).toBe(8_000)
  })

  it('scopes live-only row ids by thread and turn to isolate measurements', () => {
    const first = projection([], 0, {
      activeThreadId: 'thread-a',
      currentTurnId: 'turn-a',
      busy: true
    })
    const second = projection([], 0, {
      activeThreadId: 'thread-b',
      currentTurnId: 'turn-b',
      busy: true
    })

    const firstLive = first.rows.find((row) => row.kind === 'liveOnlyTurn')
    const secondLive = second.rows.find((row) => row.kind === 'liveOnlyTurn')

    expect(firstLive?.id).toBe('live-only-turn:thread-a:turn-a')
    expect(secondLive?.id).toBe('live-only-turn:thread-b:turn-b')
  })
})
