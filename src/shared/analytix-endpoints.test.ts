import { describe, expect, it } from 'vitest'
import * as endpoints from './analytix-endpoints'

const FORBIDDEN_PUBLIC_ENDPOINT = /reasonix|\/kun(?:\/|$)|\/deepseek(?:\/|$)|\/runtime\/go|workflow|create-loop|subagent|autoresearch|mcp-indexer|session-api/

describe('analytix endpoint builders', () => {
	it('exposes the exact Provider Registry portable-manifest endpoint', () => {
		expect(endpoints.ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_PATH)
			.toBe('/v1/provider-registry/portable-manifest')
		expect(endpoints.ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_TEMPLATE)
			.toBe('/v1/provider-registry/portable-manifest')
	})

	it('URL-encodes route ids so runtime paths cannot inject extra route segments or query strings', () => {
    const threadId = 'thr/with space?x=1#frag'
    const taskId = 'task/with space?x=1#frag'
    const turnId = 'turn/with space?x=1#frag'
    const checkpointId = 'axcp/with space?x=1#frag'
    const approvalId = 'appr/with space?x=1#frag'
    const inputId = 'input/with space?x=1#frag'
    const sessionId = 'sess/with space?x=1#frag'
    const attachmentId = 'att/with space?x=1#frag'
    const memoryId = 'mem/with space?x=1#frag'
    const caseProjectId = 'case/with space?x=1#frag'
    const workspacePath = '/tmp/project with space?x=1#frag'
    const encodedThread = encodeURIComponent(threadId)
    const encodedTask = encodeURIComponent(taskId)
    const encodedTurn = encodeURIComponent(turnId)
    const encodedCheckpoint = encodeURIComponent(checkpointId)
    const encodedCaseProject = encodeURIComponent(caseProjectId)

    expect(endpoints.analytixThreadPath(threadId)).toBe(`/v1/threads/${encodedThread}`)
    expect(endpoints.analytixThreadSummaryPath(threadId)).toBe(`/v1/threads/${encodedThread}/summary`)
    expect(endpoints.analytixThreadSummaryTaskOutputPath(threadId, taskId))
      .toBe(`/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/output`)
    expect(endpoints.analytixThreadSummaryTaskKillPath(threadId, taskId))
      .toBe(`/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/kill`)
    expect(endpoints.analytixThreadSummaryTaskRestartPath(threadId, taskId))
      .toBe(`/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/restart`)
    expect(endpoints.analytixThreadForkPath(threadId)).toBe(`/v1/threads/${encodedThread}/fork`)
    expect(endpoints.analytixThreadGoalPath(threadId)).toBe(`/v1/threads/${encodedThread}/goal`)
    expect(endpoints.analytixThreadTodosPath(threadId)).toBe(`/v1/threads/${encodedThread}/todos`)
    expect(endpoints.analytixThreadCompactPath(threadId)).toBe(`/v1/threads/${encodedThread}/compact`)
    expect(endpoints.analytixThreadReviewPath(threadId)).toBe(`/v1/threads/${encodedThread}/review`)
    expect(endpoints.analytixThreadTurnsPath(threadId)).toBe(`/v1/threads/${encodedThread}/turns`)
    expect(endpoints.analytixThreadRewindPath(threadId)).toBe(`/v1/threads/${encodedThread}/rewind`)
    expect(endpoints.analytixThreadSteerPath(threadId, turnId))
      .toBe(`/v1/threads/${encodedThread}/turns/${encodedTurn}/steer`)
    expect(endpoints.analytixThreadInterruptPath(threadId, turnId))
      .toBe(`/v1/threads/${encodedThread}/turns/${encodedTurn}/interrupt`)
    expect(endpoints.analytixThreadEventsPath(threadId)).toBe(`/v1/threads/${encodedThread}/events`)
    expect(endpoints.analytixThreadCheckpointRewindPlanPath(threadId, checkpointId))
      .toBe(`/v1/threads/${encodedThread}/checkpoints/${encodedCheckpoint}/rewind-plan`)
    expect(endpoints.analytixThreadCheckpointRewindApplyPath(threadId, checkpointId))
      .toBe(`/v1/threads/${encodedThread}/checkpoints/${encodedCheckpoint}/rewind-apply`)
    expect(endpoints.analytixApprovalPath(approvalId)).toBe(`/v1/approvals/${encodeURIComponent(approvalId)}`)
    expect(endpoints.analytixUserInputPath(inputId)).toBe(`/v1/user-inputs/${encodeURIComponent(inputId)}`)
    expect(endpoints.analytixSessionResumePath(sessionId)).toBe(`/v1/sessions/${encodeURIComponent(sessionId)}/resume-thread`)
    expect(endpoints.analytixAttachmentPath(attachmentId)).toBe(`/v1/attachments/${encodeURIComponent(attachmentId)}`)
    expect(endpoints.analytixAttachmentContentPath(attachmentId)).toBe(`/v1/attachments/${encodeURIComponent(attachmentId)}/content`)
    expect(endpoints.analytixMemoryRecordPath(memoryId)).toBe(`/v1/memory/${encodeURIComponent(memoryId)}`)
    expect(endpoints.analytixCaseProjectThreadsPath(caseProjectId)).toBe(`/v1/case-projects/${encodedCaseProject}/threads`)
    expect(endpoints.analytixCaseProjectDetailPath(caseProjectId)).toBe(`/v1/case-projects/${encodedCaseProject}/detail`)
    expect(endpoints.analytixWorkspaceStatusPath(workspacePath)).toBe(`/v1/workspace/status?path=${encodeURIComponent(workspacePath)}`)
  })

  it('keeps shared runtime endpoint templates analytix-owned and canonical', () => {
    const exportedStrings = Object.values(endpoints).filter((value) => typeof value === 'string') as string[]

    expect(exportedStrings).toContain('/health')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/list')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/wait')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/output')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/kill')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/restart')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/recover')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/steer')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/pause')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/resume')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/isolation-review')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/isolation-reject')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/isolation-cleanup')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/isolation-accept')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/isolation-conflict-report')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/isolation-repair-check')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/isolation-repair-accept')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/child-todos')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/child-todos/project')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/child-todos/reject')
    expect(exportedStrings).toContain('/v1/runtime/task-jobs/child-todos/accept')
    expect(exportedStrings).toContain('/v1/threads/{id}/summary')
    expect(exportedStrings).toContain('/v1/threads/{id}/summary/tasks/{task}/output')
    expect(exportedStrings).toContain('/v1/threads/{id}/summary/tasks/{task}/kill')
    expect(exportedStrings).toContain('/v1/threads/{id}/summary/tasks/{task}/restart')
    expect(exportedStrings).toContain('/v1/threads/{id}/events')
    expect(exportedStrings).toContain('/v1/threads/{id}/rewind')
    expect(exportedStrings).toContain('/v1/approvals/{id}')
    expect(exportedStrings).toContain('/v1/user-inputs/{id}')
    expect(exportedStrings).toContain('/v1/sessions/{id}/resume-thread')
    expect(exportedStrings).toContain('/v1/case-projects')
    expect(exportedStrings).toContain('/v1/case-projects/{id}/threads')
    expect(exportedStrings).toContain('/v1/case-projects/{id}/detail')
    expect(exportedStrings).not.toContain('/v1/user-input/{id}')
    for (const value of exportedStrings) {
      expect(value).not.toMatch(FORBIDDEN_PUBLIC_ENDPOINT)
      expect(value === '/health' || value.startsWith('/v1/')).toBe(true)
    }
  })
})
