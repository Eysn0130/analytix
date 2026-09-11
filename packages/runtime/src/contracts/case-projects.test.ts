import { describe, expect, it } from 'vitest'
import {
  CaseProjectDetailResponseV1Schema,
  CaseProjectListResponseV1Schema,
  CaseProjectThreadsResponseV1Schema
} from './case-projects.js'

const project = {
  id: 'case_0123456789abcdef01234567',
  name: 'case-a',
  rootPath: '/cases/a',
  updatedAt: '2026-07-22T00:00:00Z',
  threadCount: 1,
  runningCount: 0,
  archivedCount: 0,
  lastThreadId: 'thread-1',
  lastPreview: '',
  dataSizeEstimate: 0,
  status: 'ready' as const
}

const thread = {
  id: 'thread-1',
  title: 'Case A',
  workspace: '/cases/a',
  model: 'deepseek-chat',
  mode: 'agent' as const,
  status: 'idle' as const,
  approvalPolicy: 'on-request' as const,
  sandboxMode: 'workspace-write' as const,
  relation: 'primary' as const,
  createdAt: '2026-07-22T00:00:00Z',
  updatedAt: '2026-07-22T00:00:00Z',
  messageCount: 0,
  turnCount: 0,
  historyAuthority: 'case_boundary_only_v1' as const
}

describe('case project public response v1', () => {
  it('accepts exact list, thread, and detail responses', () => {
    expect(CaseProjectListResponseV1Schema.parse({
      caseProjects: [project],
      indexStatus: 'ready'
    })).toEqual({ caseProjects: [project], indexStatus: 'ready' })
    expect(CaseProjectThreadsResponseV1Schema.parse({ threads: [thread] }))
      .toEqual({ threads: [thread] })
    expect(CaseProjectDetailResponseV1Schema.parse({ project, threads: [thread] }))
      .toEqual({ project, threads: [thread] })
  })

  it('rejects missing counts, unknown fields, and null detail authority', () => {
    const { threadCount: _threadCount, ...missingCount } = project
    expect(CaseProjectListResponseV1Schema.safeParse({
      caseProjects: [missingCount],
      indexStatus: 'ready'
    }).success).toBe(false)
    expect(CaseProjectListResponseV1Schema.safeParse({
      caseProjects: [{ ...project, rawPath: '/private/case.csv' }],
      indexStatus: 'ready'
    }).success).toBe(false)
    expect(CaseProjectThreadsResponseV1Schema.safeParse({
      threads: [{ ...thread, reasoning: 'private' }]
    }).success).toBe(false)
    expect(CaseProjectDetailResponseV1Schema.safeParse({ project: null, threads: [] }).success)
      .toBe(false)
  })
})
