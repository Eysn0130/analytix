import { describe, expect, it } from 'vitest'
import {
  ListThreadsResponse,
  SetThreadTodosRequest,
  ThreadSummaryResponse,
  ThreadSummaryTaskMutationResponse,
  ThreadSummaryTaskOutputResponse,
  ThreadTodoListSchema
} from './threads.js'
import {
  TaskJobKillResponseV1Schema,
  TaskJobListResponseV1Schema,
  TaskJobOutputResponseV1Schema,
  TaskJobSummaryV1Schema,
  TaskJobWaitResponseV1Schema,
  taskJobSummaryV1
} from './task-job-output.js'

const PRIVATE_OUTPUT_SENTINEL = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'

describe('Thread todo lifecycle contracts', () => {
  const item = {
    id: 'todo-1',
    content: 'Run tool',
    status: 'failed' as const,
    statusReasonCode: 'tool_failed' as const,
    createdAt: '2026-07-22T00:00:00Z',
    updatedAt: '2026-07-22T00:00:01Z'
  }

  it('preserves valid failed and canceled status reasons', () => {
    expect(ThreadTodoListSchema.safeParse({
      threadId: 'thread-1', updatedAt: item.updatedAt, items: [item, {
        ...item, id: 'todo-2', status: 'canceled', statusReasonCode: 'user_canceled'
      }]
    }).success).toBe(true)
  })

  it('rejects missing, mismatched, or non-terminal status reasons', () => {
    for (const invalid of [
      { ...item, statusReasonCode: undefined },
      { ...item, status: 'canceled', statusReasonCode: 'tool_failed' },
      { ...item, status: 'pending', statusReasonCode: 'tool_failed' },
      { ...item, statusReasonCode: 'model_claimed_failure' }
    ]) {
      expect(ThreadTodoListSchema.safeParse({
        threadId: 'thread-1', updatedAt: item.updatedAt, items: [invalid]
      }).success).toBe(false)
    }
  })

  it('rejects duplicate ids, multiple active items, and unknown properties', () => {
    expect(SetThreadTodosRequest.safeParse({
      todos: [
        { id: 'same', content: 'one', status: 'pending' },
        { id: 'same', content: 'two', status: 'pending' }
      ]
    }).success).toBe(false)
    expect(SetThreadTodosRequest.safeParse({
      todos: [
        { content: 'one', status: 'in_progress' },
        { content: 'two', status: 'in_progress' }
      ]
    }).success).toBe(false)
    expect(SetThreadTodosRequest.safeParse({
      todos: [{ content: 'one', status: 'pending', unknown: true }]
    }).success).toBe(false)
    expect(SetThreadTodosRequest.safeParse({ todos: [], unknown: true }).success).toBe(false)
  })
})

describe('ListThreadsResponse', () => {
  it('accepts the exact boundary-only summary without synthesizing a workspace', () => {
    const summary = {
      id: 'thread-1',
      title: 'Case thread',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      createdAt: '2026-07-22T00:00:00Z',
      updatedAt: '2026-07-22T00:00:01Z',
      messageCount: 1,
      turnCount: 1,
      latestTurnId: 'turn-1',
      historyAuthority: 'case_boundary_only_v1'
    }

    expect(ListThreadsResponse.safeParse({ threads: [summary] }).success).toBe(true)
    expect(ListThreadsResponse.safeParse({
      threads: [{ ...summary, privateField: 'must-not-pass' }]
    }).success).toBe(false)
  })

  it('accepts and retains the durable Go goal id', () => {
    const goal = {
      id: 'goal-1',
      threadId: 'thread-1',
      objective: 'Complete the repository task',
      status: 'active' as const,
      tokenBudget: null,
      tokensUsed: 0,
      timeUsedSeconds: 0,
      evidenceLedger: [],
      createdAt: '2026-08-03T00:00:00Z',
      updatedAt: '2026-08-03T00:00:01Z'
    }
    const summary = {
      id: 'thread-1',
      title: 'Repository task',
      workspace: '/isolated/repository',
      model: 'deepseek-chat',
      mode: 'agent' as const,
      status: 'idle' as const,
      approvalPolicy: 'auto' as const,
      sandboxMode: 'danger-full-access' as const,
      relation: 'primary' as const,
      goal,
      createdAt: '2026-08-03T00:00:00Z',
      updatedAt: '2026-08-03T00:00:01Z'
    }

    const parsed = ListThreadsResponse.safeParse({ threads: [summary] })
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.data.threads[0].goal).toEqual(goal)
    }
  })
})

describe('ThreadSummaryTaskOutputResponse', () => {
  it('rejects arbitrary available output even without a reasoning marker', () => {
    const result = ThreadSummaryTaskOutputResponse.safeParse({
      schemaVersion: 1,
      availability: 'available',
      taskId: 'taskjob:job-1',
      status: 'completed',
      output: PRIVATE_OUTPUT_SENTINEL,
      offset: 0,
      nextOffset: PRIVATE_OUTPUT_SENTINEL.length,
      outputBytes: PRIVATE_OUTPUT_SENTINEL.length,
      truncated: false
    })
    expect(result.success).toBe(false)
    expect(JSON.stringify(result)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
  })

  it('accepts a metadata-only withheld variant without output or cursor fields', () => {
    const result = ThreadSummaryTaskOutputResponse.safeParse({
      schemaVersion: 1,
      availability: 'withheld',
      taskId: 'taskjob:job-1',
      status: 'completed',
      reasonCode: 'security_bound_child_output',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    })
    expect(result.success).toBe(true)
  })

  it('accepts private tool output only as a closed metadata-only withheld variant', () => {
    const result = ThreadSummaryTaskOutputResponse.safeParse({
      schemaVersion: 1,
      availability: 'withheld',
      taskId: 'command:call-1',
      status: 'error',
      reasonCode: 'tool_output_private',
      outputWithheld: true,
      outputTrustStatus: 'private_tool_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    })
    expect(result.success).toBe(true)
  })

  it('binds each withheld reason to the matching task identity and lifecycle family', () => {
    const child = {
      schemaVersion: 1, availability: 'withheld', taskId: 'taskjob:job-1', status: 'completed',
      reasonCode: 'security_bound_child_output', outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output', factAnswerAllowed: false,
      evidenceAuthority: false, canReadOutput: false, canContinueParent: false
    }
    const command = {
      schemaVersion: 1, availability: 'withheld', taskId: 'command:call-1', status: 'error',
      reasonCode: 'tool_output_private', outputWithheld: true,
      outputTrustStatus: 'private_tool_output', factAnswerAllowed: false,
      evidenceAuthority: false, canReadOutput: false, canContinueParent: false
    }
    expect(ThreadSummaryTaskOutputResponse.safeParse(child).success).toBe(true)
    expect(ThreadSummaryTaskOutputResponse.safeParse(command).success).toBe(true)
    expect(ThreadSummaryTaskOutputResponse.safeParse({ ...child, taskId: command.taskId }).success).toBe(false)
    expect(ThreadSummaryTaskOutputResponse.safeParse({ ...command, taskId: child.taskId }).success).toBe(false)
  })

  it.each([
    {
      taskId: 'taskjob:job-1', status: 'completed', output: 'legacy bypass', offset: 0,
      nextOffset: 13, outputBytes: 13, truncated: false
    },
    {
      schemaVersion: 1, availability: 'withheld', taskId: 'taskjob:job-1', status: 'completed',
      reasonCode: 'security_bound_child_output', outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false,
      output: 'private child final'
    },
    {
      schemaVersion: 1, availability: 'withheld', taskId: 'taskjob:job-1', status: 'completed',
      reasonCode: 'model_claimed_safe', outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false
    },
    {
      schemaVersion: 1, availability: 'withheld', taskId: 'taskjob:job-1', status: 'completed',
      reasonCode: 'security_bound_child_output', outputWithheld: true, outputTrustStatus: 'private_tool_output',
      factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false
    },
    {
      schemaVersion: 1, availability: 'withheld', taskId: 'taskjob:job-1', status: 'completed',
      reasonCode: 'tool_output_private', outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false
    }
  ])('rejects legacy, fact-bearing, unknown, or model-selected withheld shapes', (value) => {
    expect(ThreadSummaryTaskOutputResponse.safeParse(value).success).toBe(false)
  })
})

describe('TaskJobOutputResponseV1Schema', () => {
  it('accepts only closed withheld metadata and rejects arbitrary available output', () => {
    const available = {
      schemaVersion: 1,
      availability: 'available',
      jobId: 'job-1',
      status: 'completed',
      output: PRIVATE_OUTPUT_SENTINEL,
      offset: 0,
      nextOffset: PRIVATE_OUTPUT_SENTINEL.length,
      outputBytes: PRIVATE_OUTPUT_SENTINEL.length,
      truncated: false
    }
    const withheld = {
      schemaVersion: 1,
      availability: 'withheld',
      jobId: 'job-2',
      status: 'completed',
      reasonCode: 'security_bound_child_output',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }
    const rejected = TaskJobOutputResponseV1Schema.safeParse(available)
    expect(rejected.success).toBe(false)
    expect(JSON.stringify(rejected)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    expect(TaskJobOutputResponseV1Schema.safeParse(withheld).success).toBe(true)
    expect(TaskJobOutputResponseV1Schema.safeParse({ ...withheld, output: 'private child final' }).success).toBe(false)
    expect(TaskJobOutputResponseV1Schema.safeParse({ ...available, outputWithheld: true }).success).toBe(false)
  })
})

describe('TaskJobSummaryV1Schema', () => {
  const closed = taskJobSummaryV1({
    id: 'job-1', kind: 'background-shell', status: 'completed', background: true
  })

  it('accepts one exact metadata projection across list, wait, and kill', () => {
    expect(TaskJobSummaryV1Schema.parse(closed)).toEqual(closed)
    expect(TaskJobListResponseV1Schema.parse({ jobs: [closed], count: 1 })).toEqual({ jobs: [closed], count: 1 })
    expect(TaskJobWaitResponseV1Schema.parse({ jobs: [closed] })).toEqual({ jobs: [closed] })
    expect(TaskJobKillResponseV1Schema.parse({ job: closed })).toEqual({ job: closed })
  })

  it.each(['output', 'error', 'reasoning', 'diagnostics', 'outputBytes', 'offset', 'nextOffset'])(
    'rejects the private or diagnostic field %s',
    (field) => {
      const rejected = TaskJobSummaryV1Schema.safeParse({ ...closed, [field]: PRIVATE_OUTPUT_SENTINEL })
      expect(rejected.success).toBe(false)
      expect(JSON.stringify(rejected)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    }
  )

  it('maps unknown durable lifecycle text to fixed values without reflecting it', () => {
    const projected = taskJobSummaryV1({
      id: 'job-2', kind: PRIVATE_OUTPUT_SENTINEL, status: PRIVATE_OUTPUT_SENTINEL
    })
    expect(projected.kind).toBe('unknown')
    expect(projected.status).toBe('unknown')
    expect(JSON.stringify(projected)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
  })

  it('rejects lifecycle flags that contradict the closed status', () => {
    expect(TaskJobSummaryV1Schema.safeParse({ ...closed, terminal: false }).success).toBe(false)
    expect(TaskJobSummaryV1Schema.safeParse({ ...closed, active: true }).success).toBe(false)
  })
})

describe('ThreadSummaryResponse', () => {
  const task = {
    schemaVersion: 1 as const,
    id: 'taskjob:job-1',
    kind: 'task' as const,
    status: 'running' as const,
    background: false,
    terminal: false,
    outputWithheld: true as const,
    outputTrustStatus: 'untrusted_child_output' as const,
    factAnswerAllowed: false as const,
    evidenceAuthority: false as const,
    canReadOutput: false as const,
    canContinueParent: false as const
  }
  const subagent = {
    schemaVersion: 1 as const,
    id: 'run:child-1',
    key: 'run:child-1',
    parentThreadId: 'thread-1',
    childRunId: 'child-1',
    childThreadId: 'child-thread-1',
    status: 'active' as const,
    rawStatus: 'running' as const,
    outputWithheld: true as const,
    outputTrustStatus: 'untrusted_child_output' as const,
    factAnswerAllowed: false as const,
    evidenceAuthority: false as const,
    canContinueParent: false as const,
    canReadOutput: false as const,
    canOpenThread: true,
    canKill: false as const,
    canRestart: false as const,
    updatedAt: '2026-07-20T00:00:01Z'
  }
  const valid = {
    threadId: 'thread-1',
    generatedAt: '2026-07-20T00:00:02Z',
    latestSeq: 1,
    subagents: [subagent],
    tasks: [task],
    outputs: [],
    sources: [],
    sideChats: [],
    backgroundProcesses: []
  }

  it('accepts a closed, same-parent, metadata-only summary', () => {
    expect(ThreadSummaryResponse.safeParse(valid).success).toBe(true)
    expect(ThreadSummaryTaskMutationResponse.safeParse({ task }).success).toBe(true)
  })

  it('accepts only closed reasoning-effort metadata', () => {
    for (const effort of ['auto', 'off', 'low', 'medium', 'high', 'max'] as const) {
      expect(ThreadSummaryResponse.safeParse({
        ...valid,
        subagents: [{ ...subagent, effort }]
      }).success).toBe(true)
    }
    for (const effort of ['', ' high ', 'HIGH', PRIVATE_OUTPUT_SENTINEL]) {
      expect(ThreadSummaryResponse.safeParse({
        ...valid,
        subagents: [{ ...subagent, effort }]
      }).success).toBe(false)
    }
  })

  it('accepts only non-negative integer child execution bounds', () => {
    expect(ThreadSummaryResponse.safeParse({
      ...valid,
      subagents: [{ ...subagent, maxModelSteps: 8, timeBudgetMs: 180_000 }]
    }).success).toBe(true)
    for (const [field, value] of [
      ['maxModelSteps', -1],
      ['maxModelSteps', 1.5],
      ['timeBudgetMs', -1],
      ['timeBudgetMs', 1.5]
    ] as const) {
      expect(ThreadSummaryResponse.safeParse({
        ...valid,
        subagents: [{ ...subagent, [field]: value }]
      }).success).toBe(false)
    }
  })

  it.each(['outputs', 'sources', 'backgroundProcesses'] as const)(
    'rejects non-empty fact-bearing %s before controlled authority exists',
    (field) => {
      expect(ThreadSummaryResponse.safeParse({ ...valid, [field]: [{ id: 'forged' }] }).success).toBe(false)
    }
  )

  it('rejects foreign parents, invalid timestamps, and lifecycle mismatches', () => {
    expect(ThreadSummaryResponse.safeParse({
      ...valid,
      subagents: [{ ...subagent, parentThreadId: 'thread-2' }]
    }).success).toBe(false)
    expect(ThreadSummaryResponse.safeParse({ ...valid, generatedAt: 'not-a-time' }).success).toBe(false)
    expect(ThreadSummaryResponse.safeParse({
      ...valid,
      subagents: [{ ...subagent, status: 'done' }]
    }).success).toBe(false)
  })

  it('allows only closed subagent lifecycle metadata in a case-bound summary', () => {
    const caseBound = {
      ...valid,
      tasks: [],
      historyAuthority: 'case_boundary_only_v1' as const
    }
    expect(ThreadSummaryResponse.safeParse(caseBound).success).toBe(true)
    expect(ThreadSummaryResponse.safeParse({
      ...caseBound,
      tasks: [task]
    }).success).toBe(false)
    expect(ThreadSummaryResponse.safeParse({
      ...caseBound,
      sideChats: [{
        threadId: 'side-1',
        title: 'Side',
        status: 'idle',
        relation: 'side',
        parentThreadId: 'thread-1',
        createdAt: '2026-07-20T00:00:00Z',
        updatedAt: '2026-07-20T00:00:01Z'
      }]
    }).success).toBe(false)
  })
})
