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
  it('keeps Goal continuation items out of human navigation and numbering without classifying human prompt prose', () => {
    const source = { kind: 'goal' as const, goalId: 'goal', revision: 1, round: 2 }
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'human', text: 'Human opening', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'goal-steering', text: 'INTERNAL_GOAL_STEERING', source, meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'goal-opening', text: 'INTERNAL_GOAL_OPENING', source, meta: { turnId: 'turn-2' } },
      { kind: 'user', id: 'human-prose', text: '<goal_round>Human-authored text</goal_round>', meta: { turnId: 'turn-3' } }
    ]
    const snapshot = projection(blocks)
    expect(snapshot.userMessageNavigationItems.map(item => [item.id, item.label, item.preview])).toEqual([
      ['human:user', '1', 'Human opening'],
      ['human-prose:user', '2', '<goal_round>Human-authored text</goal_round>']
    ])
    expect(JSON.stringify(snapshot.userMessageNavigationItems)).not.toContain('INTERNAL_GOAL_')
    const rows = snapshot.rows.filter(row => row.kind === 'turn')
    expect(rows).toHaveLength(3)
    expect(rows[0].turn.turn.blocks).toContain(blocks[1])
    expect(rows[1].turn.turn.user).toBe(blocks[2])
    expect(rows[1].turn.userNavigationItemId).toBeUndefined()
    expect(projection(blocks, 1).userMessageNavigationItems.map(item => [item.id, item.label])).toEqual([['human-prose:user', '2']])
  })

  it('keeps same-runtime-turn steering inputs ordered under one live owner and separate navigation nodes', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'opening', text: 'Request', meta: { turnId: 'turn-1' } },
      { kind: 'assistant', id: 'progress', text: 'First progress', meta: { turnId: 'turn-1' } },
      { kind: 'tool', id: 'first-tool', summary: 'Read', status: 'success', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'pending', text: 'First direction', meta: { turnId: 'turn-1', steeringStatus: 'pending' } },
      { kind: 'tool', id: 'next-tool', summary: 'Read again', status: 'running', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'admitted', text: 'Second direction', meta: { turnId: 'turn-1', steeringStatus: 'admitted', admittedSeq: 42 } }
    ]
    const snapshot = projection(blocks, 0, {
      busy: true, currentTurnId: 'turn-1', currentTurnUserId: 'opening', live: 'One live answer'
    })
    const rows = snapshot.rows.filter(row => row.kind === 'turn')
    expect(rows).toHaveLength(1)
    expect(rows[0].turn.turn.user).toBe(blocks[0])
    expect(rows[0].turn.turn.blocks).toEqual(blocks.slice(1))
    expect(rows[0].turn).toMatchObject({ isLiveTurn: true, isProcessing: true, liveContent: 'One live answer' })
    expect(rows[0].turn.sections.processBlocks.map(block => block.id)).toEqual(blocks.slice(1).map(block => block.id))
    expect(snapshot.userMessageNavigationItems.map(item => [item.id, item.turnKey, item.label, item.preview])).toEqual([
      ['opening:user', 'opening', '1', 'Request'],
      ['pending:user', 'opening', '2', 'First direction'],
      ['admitted:user', 'opening', '3', 'Second direction']
    ])
    expect(snapshot.userMessageNavigationItems.map(item => item.responsePreview)).toEqual(['First progress', undefined, undefined])
    expect(snapshot.anchors.map(anchor => anchor.key)).toEqual(['opening', 'pending', 'admitted'])
    expect(snapshot.rows.filter(row => row.kind === 'liveOnlyTurn')).toEqual([])
  })

  it.each(['completed', 'failed', 'aborted'] as const)('retires live and stale pending work on authoritative %s settlement', status => {
    const snapshot = projection([
      { kind: 'user', id: 'opening', text: 'Request', meta: { turnId: 'turn-1' } },
      { kind: 'tool', id: 'stale-running', summary: 'Read', status: 'running', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'steering', text: 'Direction', meta: { turnId: 'turn-1', steeringStatus: 'admitted' } },
      { kind: 'assistant', id: 'final', text: 'Settled answer', meta: { turnId: 'turn-1' } }
    ], 0, { busy: true, currentTurnId: 'turn-1', currentTurnUserId: 'opening', live: 'Retained live buffer',
      authoritativeTurnStatuses: { 'turn-1': status } })
    const rows = snapshot.rows.filter(row => row.kind === 'turn')
    expect(rows).toHaveLength(1)
    expect(rows[0].turn).toMatchObject({ isLiveTurn: false, isProcessing: false, hasLiveStream: false, live: '', timing: { status } })
    expect(rows[0].turn.sections.assistantContentBlocks.map(block => block.id)).toEqual(['final'])
    expect(snapshot.rows.some(row => row.kind === 'liveOnlyTurn')).toBe(false)
    expect(projection([], 0, { busy: true, currentTurnId: 'turn-1', live: 'Retained buffer',
      authoritativeTurnStatuses: { 'turn-1': status } }).rows.some(row => row.kind === 'liveOnlyTurn')).toBe(false)
  })

  it('keeps an assistant followed by steering before that input after settlement', () => {
    const snapshot = projection([
      { kind: 'user', id: 'opening', text: 'Request', meta: { turnId: 'turn-1' } },
      { kind: 'assistant', id: 'closing', text: 'Closing answer', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'late-steering', text: 'Direction', meta: { turnId: 'turn-1', steeringStatus: 'admitted' } }
    ], 0, { authoritativeTurnStatuses: { 'turn-1': 'completed' } })
    const row = snapshot.rows.find(row => row.kind === 'turn')!
    expect(row.turn.sections.processBlocks.map(block => block.id)).toEqual(['closing', 'late-steering'])
    expect(row.turn.sections.assistantContentBlocks).toEqual([])
  })

  it('uses explicit runtime identity across turns even when the current user id is stale', () => {
    const snapshot = projection([
      { kind: 'user', id: 'u1', text: 'First', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'u2', text: 'Second', meta: { turnId: 'turn-2' } },
      { kind: 'tool', id: 'late-first-tool', summary: 'First owner only', status: 'success', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'steer-first', text: 'First direction', meta: { turnId: 'turn-1', steeringStatus: 'admitted' } }
    ], 0, { busy: true, currentTurnId: 'turn-2', currentTurnUserId: 'u1', live: 'Second owner only' })
    const rows = snapshot.rows.filter(row => row.kind === 'turn')
    expect(rows.map(row => row.turn.turn.blocks.map(block => block.id))).toEqual([['late-first-tool', 'steer-first'], []])
    expect(rows.map(row => row.turn.isLiveTurn)).toEqual([false, true])
    expect(rows.map(row => row.turn.live)).toEqual(['', 'Second owner only'])
  })

  it('preserves user-bound no-ID history and only uses the current user fallback for an unidentified turn', () => {
    const blocks: ChatBlock[] = [
      { kind: 'assistant', id: 'intro', text: 'Welcome' },
      { kind: 'user', id: 'legacy-one', text: 'One' },
      { kind: 'assistant', id: 'legacy-answer', text: 'Answer' },
      { kind: 'user', id: 'legacy-two', text: 'Two' }
    ]
    const snapshot = projection(blocks, 0, { busy: true, currentTurnId: 'turn-new', currentTurnUserId: 'legacy-two', live: 'Legacy live' })
    const rows = snapshot.rows.filter(row => row.kind === 'turn')
    expect(rows.map(row => row.id)).toEqual(['turn:intro', 'turn:legacy-one', 'turn:legacy-two'])
    expect(rows.map(row => row.turn.isLiveTurn)).toEqual([false, false, true])
    expect(snapshot.userMessageNavigationItems.map(item => item.id)).toEqual(['legacy-one:user', 'legacy-two:user'])
    expect(projection(blocks, 1).userMessageNavigationItems.map(item => item.label)).toEqual(['1', '2'])
  })

  it('counts every hidden runtime-turn input when numbering navigation', () => {
    const snapshot = projection([
      { kind: 'user', id: 'u1', text: 'First', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'steer', text: 'Direction', meta: { turnId: 'turn-1' } },
      { kind: 'user', id: 'u2', text: 'Second', meta: { turnId: 'turn-2' } }
    ], 1)
    expect(snapshot.userMessageNavigationItems.map(item => [item.id, item.label])).toEqual([['u2:user', '3']])
  })

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

  it('projects the live estimate start separately from authoritative final duration', () => {
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
    expect(row?.turn.timing.durationMs).toBeUndefined()
    expect(row?.turn.timing.runningStartedAt).toBe(1_000)
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


it('keeps legacy neutral telemetry outside the mounted turn projection while preserving errors and retry', () => {
  const blocks: ChatBlock[] = [
    { kind: 'user', id: 'user', text: 'Human input', meta: { turnId: 'turn' } },
    { kind: 'system', id: 'neutral', text: 'Model request started', meta: { turnId: 'turn', stage: 'post_send' } },
    { kind: 'system', id: 'error', text: 'Actual error', severity: 'error', meta: { turnId: 'turn', stage: 'post_send' } },
    { kind: 'system', id: 'warning', text: 'Actual warning', severity: 'warning', meta: { turnId: 'turn', stage: 'post_send' } },
    { kind: 'system', id: 'retry', text: 'Actual retry', meta: { turnId: 'turn', providerRetry: { attempt: 2 } } }
  ]
  const snapshot = projection(blocks)
  const row = snapshot.rows.find(row => row.kind === 'turn')!
  expect(row.kind === 'turn' && row.turn.sections.processBlocks.map(block => block.id)).toEqual(['error', 'warning', 'retry'])
  expect(blocks.map(block => block.id)).toEqual(['user', 'neutral', 'error', 'warning', 'retry'])
})
it('uses admitted running authority independently of local busy while terminal status fences it', () => {
  const blocks: ChatBlock[] = [{ kind: 'user', id: 'u', text: 'Input', meta: { turnId: 'turn' } }]
  for (const authority of [{ runtimeRunning: true }, { authoritativeTurnStatuses: { turn: 'running' as const } }]) {
    const rows = projection(blocks, 0, { currentTurnId: 'turn', busy: false, ...authority }).rows.filter(row => row.kind === 'turn')
    expect(rows).toHaveLength(1)
    expect(rows[0].turn).toMatchObject({ isLiveTurn: true, isProcessing: true })
  }
  const rows = projection(blocks, 0, { currentTurnId: 'turn', runtimeRunning: true,
    authoritativeTurnStatuses: { turn: 'completed' } }).rows.filter(row => row.kind === 'turn')
  expect(rows[0].turn).toMatchObject({ isLiveTurn: false, isProcessing: false })
})
