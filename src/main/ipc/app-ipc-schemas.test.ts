import { writePrototypeFilePayloadSchema, writePrototypeAuthorizePayloadSchema, writePrototypeRevokePayloadSchema } from './app-ipc-schemas'
import { workspaceFileReadPayloadSchema, workspacePresentMediaReadPayloadSchema } from './app-ipc-schemas'
import { describe, expect, it, vi } from 'vitest'
import { userInputLocalSelectorSchemaV1 } from '../../../packages/runtime/src/contracts/user-input-local-display'
import {
  analytixApprovalPath,
  analytixAttachmentContentPath,
  analytixMemoryRecordPath,
  analytixMessageFeedbackPath,
  analytixMessageFeedbackItemPath,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_KILL_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_LIST_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_RESTART_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_RESUME_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_WAIT_PATH,
  analytixSessionResumePath,
  analytixThreadCheckpointRewindApplyPath,
  analytixThreadCheckpointRewindPlanPath,
  analytixThreadForkPath,
  analytixThreadInterruptPath,
  analytixThreadPath,
  analytixThreadRewindPath,
  analytixThreadSummaryPath,
  analytixThreadSummaryTaskKillPath,
  analytixThreadSummaryTaskOutputPath,
  analytixThreadSummaryTaskRestartPath,
  analytixThreadSteerPath,
  analytixThreadTurnsPath,
  analytixUserInputPath,
  analytixWorkspaceStatusPath
} from '../../shared/analytix-endpoints'
import {
  clawImInstallPollPayloadSchema,
  clawImTelegramTokenPayloadSchema,
  gitCheckpointCreatePayloadSchema,
  gitCheckpointRestorePayloadSchema,
  logErrorPayloadSchema,
  isSafeOpenExternalUrl,
  runtimeRequestPayloadSchema,
  scheduleTaskFromTextPayloadSchema,
  scheduleTaskCreatePayloadSchema,
  scheduleTaskUpdatePayloadSchema,
  scheduleTaskDeletePayloadSchema,
  settingsPatchSchema,
  shellOpenExternalUrlSchema,
  skillListPayloadSchema,
  sseStartPayloadSchema,
  threadHandoffCompleteSwitchPayloadSchema,
  threadHandoffFailSwitchPayloadSchema,
  threadHandoffStartPayloadSchema,
  workspaceDirectoryCreatePayloadSchema,
  workspaceDirectoryTargetPayloadSchema,
  workspaceEntryDeletePayloadSchema,
  workspaceEntryRenamePayloadSchema,
  backgroundTaskActionPayloadSchema,
  backgroundTaskOutputPayloadSchema,
  backgroundTaskRegisterPayloadSchema,
  backgroundTaskThreadPayloadSchema,
  writeExportPayloadSchema,
  writeRichClipboardPayloadSchema,
  writeInlineCompletionPayloadSchema
} from './app-ipc-schemas'

describe('closed renderer SSE rejection logs', () => {
  const categories = [
    ['sse-event-active', 'renderer_sse_event_active'],
    ['sse-event-aborted', 'renderer_sse_event_aborted'],
    ['sse-event-closed', 'renderer_sse_event_closed'],
    ['sse-ack_false-active', 'renderer_sse_ack_false_active'],
    ['sse-ack_false-aborted', 'renderer_sse_ack_false_aborted'],
    ['sse-ack_false-closed', 'renderer_sse_ack_false_closed'],
    ['sse-ack_transport-active', 'renderer_sse_ack_transport_active'],
    ['sse-ack_transport-aborted', 'renderer_sse_ack_transport_aborted'],
    ['sse-ack_transport-closed', 'renderer_sse_ack_transport_closed']
  ] as const
  const privateCanary = 'SSE_LOG_PRIVATE_CANARY_7F3C'

  it('projects every admitted lifecycle category to its exact code and drops all detail extras', () => {
    for (const [category, code] of categories) {
      const projected = logErrorPayloadSchema.parse({ category, message: privateCanary,
        detail: { code: privateCanary, reasonCode: privateCanary,
          diagnostic: { body: privateCanary }, payload: privateCanary,
          id: privateCanary, path: `/private/${privateCanary}`, locator: privateCanary,
          seq: 99, retryable: true, resultCount: 99, sha256: 'a'.repeat(64) } })
      expect(projected).toEqual({ code })
      expect(JSON.stringify(projected)).not.toContain(privateCanary)
    }
  })

  it('does not read or enumerate hostile detail fields in any admitted lifecycle category', () => {
    const readExtra = vi.fn(() => { throw new Error(privateCanary) })
    const enumerate = vi.fn((): never => { throw new Error(privateCanary) })
    const detail = {}
    for (const key of ['retryable', 'resultCount', 'diagnostic', 'payload', 'id', 'path', 'locator', 'code', 'reasonCode']) {
      Object.defineProperty(detail, key, { enumerable: true, get: readExtra })
    }
    const hostile = new Proxy(detail, { ownKeys: enumerate })
    for (const [category, code] of categories) {
      expect(logErrorPayloadSchema.parse({ category, message: privateCanary, detail: hostile })).toEqual({ code })
    }
    expect(readExtra).not.toHaveBeenCalled()
    expect(enumerate).not.toHaveBeenCalled()
  })

  it('keeps the existing closed fallback for unknown or spoofed SSE categories', () => {
    for (const category of ['sse-event-unknown', 'sse-event-active-extra', 'sse-ack_false-active-extra',
      'sse-ack_transport-closed-extra', 'SSE-event-active', privateCanary]) {
      expect(logErrorPayloadSchema.parse({ category, message: privateCanary,
        detail: { retryable: false, resultCount: 3, code: 'renderer_sse_event_active',
          reasonCode: privateCanary, diagnostic: privateCanary, payload: privateCanary,
          id: privateCanary, path: `/private/${privateCanary}`, locator: privateCanary } }))
        .toEqual({ code: 'renderer_diagnostic_failed', retryable: false, resultCount: 3 })
    }
  })

  it('keeps strict top-level admission and existing non-SSE scalar projection', () => {
    const payload = { category: 'sse-event-active', message: privateCanary }
    expect(logErrorPayloadSchema.parse(payload)).toEqual({ code: 'renderer_sse_event_active' })
    for (const key of ['diagnostic', 'payload', 'id', 'path', 'locator']) {
      expect(logErrorPayloadSchema.safeParse({ ...payload, [key]: privateCanary }).success).toBe(false)
    }
    expect(logErrorPayloadSchema.parse({ category: 'send-message', message: privateCanary,
      detail: { retryable: true, resultCount: 2, payload: privateCanary } }))
      .toEqual({ code: 'renderer_send_message_failed', retryable: true, resultCount: 2 })
  })
})

describe('app-ipc-schemas', () => {
  describe('single message feedback admission', () => {
    const list = analytixMessageFeedbackPath('thread')
    const item = analytixMessageFeedbackItemPath('thread', 'message')
    const version = '11111111-1111-4111-8111-111111111111'
    const request = (path: string, method: string, body?: unknown) => ({
      path, method, ...(body === undefined ? {} : { body: JSON.stringify(body) })
    })

    it('admits only the list and versioned item operations', () => {
      for (const payload of [
        request(list, 'GET'),
        request(item, 'PUT', { rating: 'positive', ifVersion: null }),
        request(item, 'PUT', { rating: 'negative', ifVersion: version, category: 'other', note: 'Optional synthetic rating detail' }),
        request(item, 'DELETE', { ifVersion: version })
      ]) expect(runtimeRequestPayloadSchema.parse(payload)).toEqual(payload)
    })

    it('rejects route aliases, malformed targets and unsupported methods', () => {
      for (const path of [
        `${item}?view=all`, `${item}?`, `${item}#fragment`, `${item}#`, `${item}/extra`, `${item}/`,
        '/v1/threads/thread/message-feedback/%',
        '/v1/threads/thread/message-feedback/%2Fmessage',
        '/v1/threads/thread/message-feedback/%00message',
        '/v1/threads/thread/message-feedback/.hidden',
        '/v1/threads/thread/message-feedback/message..alias',
        `/v1/threads/${'a'.repeat(256)}/message-feedback/message`,
        `/v1/threads/thread/message-feedback/${'a'.repeat(256)}`,
        '/v1/threads/thread/message-feedback/alias/../message',
        '//localhost/v1/threads/thread/message-feedback/message'
      ]) expect(runtimeRequestPayloadSchema.safeParse(request(path, 'PUT', { rating: 'positive', ifVersion: null })).success).toBe(false)
      for (const payload of [
        request(list, 'GET', {}), request(list, 'PUT', { rating: 'positive', ifVersion: null }),
        request(item, 'GET'), request(item, 'POST', {}), request(item, 'PATCH', {})
      ]) expect(runtimeRequestPayloadSchema.safeParse(payload).success).toBe(false)
    })

    it('requires closed bodies, UUIDv4 preconditions and well-formed bounded notes', () => {
      for (const body of [
        {}, { rating: 'positive' }, { rating: 'neutral', ifVersion: null },
        { rating: 'positive', ifVersion: 'not-a-version' },
        { rating: 'positive', ifVersion: null, extra: true },
        { rating: 'positive', ifVersion: null, note: null },
        { rating: 'positive', ifVersion: null, category: null },
        { rating: 'positive', ifVersion: null, category: 'unrecognized' },
        { rating: 'positive', ifVersion: null, note: '\ud800' },
        { rating: 'positive', ifVersion: null, note: '界'.repeat(2731) }
      ]) expect(runtimeRequestPayloadSchema.safeParse(request(item, 'PUT', body)).success).toBe(false)
      for (const body of [{}, { ifVersion: null }, { ifVersion: version, extra: true }]) {
        expect(runtimeRequestPayloadSchema.safeParse(request(item, 'DELETE', body)).success).toBe(false)
      }
      expect(runtimeRequestPayloadSchema.safeParse(request(item, 'PUT', { rating: 'positive', ifVersion: null, note: 'x'.repeat(8192) })).success).toBe(true)
      expect(runtimeRequestPayloadSchema.safeParse(request(item, 'PUT', { rating: 'positive', ifVersion: null, note: 'x'.repeat(8193) })).success).toBe(false)
      expect(runtimeRequestPayloadSchema.safeParse({ path: item, method: 'PUT', body: '{invalid' }).success).toBe(false)
      expect(runtimeRequestPayloadSchema.safeParse({ path: item, method: 'PUT', body: ' '.repeat(65537) }).success).toBe(false)
    })

    it('preserves the raw JSON body for independent Go duplicate-key validation', () => {
      const body = '{"rating":"positive","rating":"negative","ifVersion":null}'
      expect(runtimeRequestPayloadSchema.parse({ path: item, method: 'PUT', body }).body).toBe(body)
    })
  })

  it('normalizes runtime request paths', () => {
    const payload = runtimeRequestPayloadSchema.parse({
      path: 'v1/threads?limit=1',
      method: 'GET'
    })

    expect(payload.path).toBe('/v1/threads?limit=1')
  })

  it('accepts the Analytix runtime info endpoint', () => {
    const payload = runtimeRequestPayloadSchema.parse({
      path: '/v1/runtime/info',
      method: 'GET'
    })

    expect(payload.path).toBe('/v1/runtime/info')
  })

  it('accepts the Analytix runtime tool diagnostics endpoint', () => {
    const payload = runtimeRequestPayloadSchema.parse({
      path: '/v1/runtime/tools',
      method: 'GET'
    })

    expect(payload.path).toBe('/v1/runtime/tools')
  })

  it('keeps every Provider Registry route outside generic runtime requests', () => {
    const providerRegistryRequests = [
      { path: '/v1/provider-registry', method: 'GET' },
      { path: '/v1/provider-registry', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/recover', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/providers/provider-alpha', method: 'GET' },
      { path: '/v1/provider-registry/providers/provider-alpha', method: 'PATCH', body: '{}' },
      { path: '/v1/provider-registry/providers/provider-alpha', method: 'DELETE', body: '{}' },
      { path: '/v1/provider-registry/providers/provider-alpha/select', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/providers/provider-alpha/disconnect', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/providers/provider-alpha/credential', method: 'PUT', body: '{}' },
      { path: '/v1/provider-registry/_private/account-credentials/status', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/account-credentials/put', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/account-credentials/resolve', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/account-credentials/mutate', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/protected-recovery/prepare', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/protected-recovery/confirm-destination', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/protected-recovery/create-bundle', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/protected-recovery/apply', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/protected-recovery/finalize', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/protected-recovery/recover', method: 'POST', body: '{}' },
      { path: '/v1/provider-registry/_private/protected-recovery/rollback', method: 'POST', body: '{}' }
    ]

    for (const request of providerRegistryRequests) {
      expect(runtimeRequestPayloadSchema.safeParse(request).success).toBe(false)
    }
  })

  it('accepts Analytix runtime task job endpoints for background subagents', () => {
    for (const path of [
      ANALYTIX_RUNTIME_TASK_JOBS_LIST_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_WAIT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_KILL_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_RESTART_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_RESUME_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_PATH,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_PATH
    ]) {
      expect(runtimeRequestPayloadSchema.parse({
        path,
        method: 'POST',
        body: path === ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH
          ? '{"mode":"inspect","jobId":"job-1","threadId":"thr_1"}'
          : '{"jobIds":["job-1"],"jobId":"job-1","threadId":"thr_1","message":"continue"}'
      }).path).toBe(path)
    }
  })

  it('accepts the Analytix skills endpoint', () => {
    const payload = runtimeRequestPayloadSchema.parse({
      path: '/v1/skills',
      method: 'GET'
    })

    expect(payload.path).toBe('/v1/skills')
  })

  it('accepts Analytix attachment and memory endpoints', () => {
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/attachments',
      method: 'POST',
      body: JSON.stringify({
        name: 'note.txt',
        dataBase64: 'aGVsbG8=',
        threadId: 'thr_1',
        workspace: '/workspace'
      })
    }).path).toBe('/v1/attachments')
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/attachments/att_1/content?thread_id=thr_1&workspace=%2Fworkspace',
      method: 'GET'
    }).path).toBe('/v1/attachments/att_1/content?thread_id=thr_1&workspace=%2Fworkspace')
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/memory',
      method: 'POST',
      body: '{}'
    }).path).toBe('/v1/memory')
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/memory/mem_1',
      method: 'PATCH',
      body: '{}'
    }).path).toBe('/v1/memory/mem_1')
  })

  it('rejects unscoped or private attachment requests at the main boundary', () => {
    for (const payload of [
      { path: '/v1/attachments', method: 'POST', body: '{}' },
      {
        path: '/v1/attachments',
        method: 'POST',
        body: JSON.stringify({
          name: 'note.txt',
          dataBase64: 'aGVsbG8=',
          threadId: 'thr_1',
          workspace: '/workspace',
          localFilePath: '/tmp/private.txt'
        })
      },
      { path: '/v1/attachments/att_1/content?thread_id=thr_1', method: 'GET' },
      { path: '/v1/attachments/att_1?thread_id=thr_1&workspace=%2Fworkspace&case_id=case_1', method: 'GET' }
    ]) {
      expect(runtimeRequestPayloadSchema.safeParse(payload).success).toBe(false)
    }
  })

  it('accepts shared endpoint builder output for encoded dynamic ids', () => {
    const threadId = 'thr/with space?x=1#frag'
    const turnId = 'turn/with space?x=1#frag'
    const checkpointId = 'axcp/with space?x=1#frag'
    const approvalId = 'appr/with space?x=1#frag'
    const inputId = 'input/with space?x=1#frag'
    const sessionId = 'sess/with space?x=1#frag'
    const attachmentId = 'att/with space?x=1#frag'
    const memoryId = 'mem/with space?x=1#frag'

    const allowedRequests = [
      { path: analytixThreadPath(threadId), method: 'GET' },
      { path: analytixThreadPath(threadId), method: 'PATCH', body: '{}' },
      { path: analytixThreadForkPath(threadId), method: 'POST', body: '{}' },
      { path: analytixThreadTurnsPath(threadId), method: 'POST', body: '{}' },
      { path: analytixThreadRewindPath(threadId), method: 'POST', body: '{}' },
      { path: analytixThreadSummaryPath(threadId), method: 'GET' },
      { path: analytixThreadSummaryTaskOutputPath(threadId, 'task/with space?x=1#frag'), method: 'GET' },
      { path: analytixThreadSummaryTaskKillPath(threadId, 'task/with space?x=1#frag'), method: 'POST', body: '{}' },
      { path: analytixThreadSummaryTaskRestartPath(threadId, 'task/with space?x=1#frag'), method: 'POST', body: '{}' },
      { path: analytixThreadSteerPath(threadId, turnId), method: 'POST', body: '{}' },
      { path: analytixThreadInterruptPath(threadId, turnId), method: 'POST', body: '{}' },
      { path: analytixThreadCheckpointRewindPlanPath(threadId, checkpointId), method: 'POST', body: '{}' },
      { path: analytixThreadCheckpointRewindApplyPath(threadId, checkpointId), method: 'POST', body: '{}' },
      { path: analytixApprovalPath(approvalId), method: 'POST', body: '{}' },
      { path: analytixUserInputPath(inputId), method: 'POST', body: JSON.stringify({ cancelled: true }) },
      { path: analytixSessionResumePath(sessionId), method: 'POST', body: '{}' },
      {
        path: `${analytixAttachmentContentPath(attachmentId)}?thread_id=${encodeURIComponent(threadId)}&workspace=${encodeURIComponent('/tmp/project with spaces')}`,
        method: 'GET'
      },
      { path: analytixMemoryRecordPath(memoryId), method: 'PATCH', body: '{}' },
      { path: analytixWorkspaceStatusPath('/tmp/project with spaces'), method: 'GET' }
    ] as const

    for (const request of allowedRequests) {
      expect(runtimeRequestPayloadSchema.parse(request).path).toBe(request.path)
    }
  })

  it('accepts skill list payloads with an optional workspace root', () => {
    expect(skillListPayloadSchema.parse({
      workspaceRoot: ' /tmp/workspace '
    })).toEqual({ workspaceRoot: '/tmp/workspace' })
    expect(skillListPayloadSchema.parse({})).toEqual({})
  })

  it('accepts git checkpoint payloads for desktop workspace rollback', () => {
    expect(gitCheckpointCreatePayloadSchema.parse({
      workspaceRoot: ' /tmp/workspace ',
      threadId: ' thread-1 '
    })).toEqual({
      workspaceRoot: '/tmp/workspace',
      threadId: 'thread-1'
    })
    expect(gitCheckpointRestorePayloadSchema.parse({
      checkpointId: ' gcp_1 '
    })).toEqual({ checkpointId: 'gcp_1' })
  })

  it('validates thread handoff IPC payloads strictly', () => {
    expect(threadHandoffStartPayloadSchema.parse({
      direction: 'to-worktree',
      sourceThreadId: ' thr_1 ',
      sourceWorkspace: ' /tmp/project ',
      localBranch: ' main '
    })).toEqual({
      direction: 'to-worktree',
      sourceThreadId: 'thr_1',
      sourceWorkspace: '/tmp/project',
      localBranch: 'main'
    })

    expect(() => threadHandoffStartPayloadSchema.parse({
      direction: 'to-space',
      sourceThreadId: 'thr_1',
      sourceWorkspace: '/tmp/project'
    })).toThrow()
    expect(() => threadHandoffStartPayloadSchema.parse({
      direction: 'to-worktree',
      sourceThreadId: 'thr_1'
    })).toThrow()
    expect(() => threadHandoffStartPayloadSchema.parse({
      direction: 'to-local',
      sourceThreadId: 'thr_1',
      sourceWorkspace: '/tmp/pool-0',
      localWorkspace: '/tmp/project',
      projectPath: '/tmp/project',
      extra: true
    })).toThrow()
    expect(() => threadHandoffCompleteSwitchPayloadSchema.parse({
      operationId: ''
    })).toThrow()
    expect(threadHandoffFailSwitchPayloadSchema.parse({
      operationId: ' op_1 ',
      message: ' failed '
    })).toEqual({
      operationId: 'op_1',
      message: 'failed'
    })
  })

  it('validates background task IPC payloads strictly for desktop summary actions', () => {
    expect(backgroundTaskThreadPayloadSchema.parse({
      threadId: ' thr_1 '
    })).toEqual({ threadId: 'thr_1' })

    expect(backgroundTaskActionPayloadSchema.parse({
      threadId: ' thr_1 ',
      taskId: ' task_1 '
    })).toEqual({ threadId: 'thr_1', taskId: 'task_1' })

    expect(backgroundTaskOutputPayloadSchema.parse({
      threadId: ' thr_1 ',
      taskId: ' task_1 ',
      offset: 0,
      limit: 2_000_000
    })).toEqual({
      threadId: 'thr_1',
      taskId: 'task_1',
      offset: 0,
      limit: 2_000_000
    })

    expect(backgroundTaskRegisterPayloadSchema.parse({
      record: {
        id: ' task_1 ',
        threadId: ' thr_1 ',
        turnId: ' turn_1 ',
        itemId: ' item_1 ',
        pid: 1234,
        processId: ' proc_1234 ',
        attemptId: ' attempt_1 ',
        status: 'running',
        source: 'summary-command',
        outputBytes: 6,
        startedAt: ' 2026-06-29T00:00:00.000Z '
      }
    })).toEqual({
      record: {
        id: 'task_1',
        threadId: 'thr_1',
        turnId: 'turn_1',
        itemId: 'item_1',
        pid: 1234,
        processId: 'proc_1234',
        attemptId: 'attempt_1',
        status: 'running',
        source: 'summary-command',
        outputBytes: 6,
        startedAt: '2026-06-29T00:00:00.000Z'
      }
    })

    expect(() => backgroundTaskOutputPayloadSchema.parse({
      threadId: 'thr_1',
      taskId: 'task_1',
      offset: -1
    })).toThrow()
    for (const forbidden of ['output', 'stdout', 'stderr', 'error', 'reasoning', 'command', 'cwd']) {
      expect(() => backgroundTaskRegisterPayloadSchema.parse({
        record: {
          id: 'task_1',
          threadId: 'thr_1',
          status: 'running',
          source: 'summary-command',
          [forbidden]: 'PRIVATE_VALUE'
        }
      })).toThrow()
    }
    expect(() => backgroundTaskOutputPayloadSchema.parse({
      threadId: 'thr_1',
      taskId: 'task_1',
      limit: 0
    })).toThrow()
    expect(() => backgroundTaskRegisterPayloadSchema.parse({
      record: {
        id: 'task_1',
        threadId: 'thr_1',
        status: 'running',
        source: 'summary-command',
        extra: true
      }
    })).toThrow()
    expect(() => backgroundTaskRegisterPayloadSchema.parse({
      record: {
        id: 'task_1',
        threadId: 'thr_1',
        status: 'unknown',
        source: 'summary-command'
      }
    })).toThrow()
    expect(() => backgroundTaskRegisterPayloadSchema.parse({
      record: {
        id: 'task_1',
        threadId: 'thr_1',
        status: 'running',
        source: 'reasonix-task'
      }
    })).toThrow()
  })

  it('accepts Analytix thread goal endpoints', () => {
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/threads/thr_1/goal',
      method: 'GET'
    }).path).toBe('/v1/threads/thr_1/goal')
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/threads/thr_1/goal',
      method: 'POST',
      body: '{}'
    }).path).toBe('/v1/threads/thr_1/goal')
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/threads/thr_1/goal',
      method: 'DELETE'
    }).path).toBe('/v1/threads/thr_1/goal')
  })

  it('accepts the Analytix thread review endpoint', () => {
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/threads/thr_1/review',
      method: 'POST',
      body: '{"target":{"kind":"uncommittedChanges"}}'
    }).path).toBe('/v1/threads/thr_1/review')
  })

  it('accepts the Analytix checkpoint rewind plan endpoint', () => {
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/threads/thr_1/checkpoints/axcp_123/rewind-plan',
      method: 'POST',
      body: '{"scope":"combined"}'
    }).path).toBe('/v1/threads/thr_1/checkpoints/axcp_123/rewind-plan')
  })

  it('accepts the Analytix checkpoint rewind apply endpoint', () => {
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/threads/thr_1/checkpoints/axcp_123/rewind-apply',
      method: 'POST',
      body: '{"confirmation":{"confirmed":true,"destructive":true,"phrase":"APPLY_CHECKPOINT_REWIND"}}'
    }).path).toBe('/v1/threads/thr_1/checkpoints/axcp_123/rewind-apply')
  })

  it('accepts the LLM debug rounds endpoint', () => {
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/debug/llm-rounds',
      method: 'GET'
    }).path).toBe('/v1/debug/llm-rounds')
  })

  it('accepts thread event replay through the modeled Analytix API surface', () => {
    expect(runtimeRequestPayloadSchema.parse({
      path: '/v1/threads/thr_1/events?since_seq=0',
      method: 'GET'
    }).path).toBe('/v1/threads/thr_1/events?since_seq=0')
  })

  it('rejects runtime request paths outside the modeled Analytix API surface', () => {
    expect(() =>
      runtimeRequestPayloadSchema.parse({
        path: '/v1/runtime/secrets',
        method: 'GET'
      })
    ).toThrow(/runtime request path is not allowed/)
  })

  it('rejects forbidden upstream public runtime request routes explicitly', () => {
    const forbiddenRoutes = [
      { path: 'v1/reasonix/session?thread_id=thr_1', method: 'GET' },
      { path: '/v1/runtime/go/threads', method: 'GET' },
      { path: '/v1/workflow', method: 'GET' },
      { path: '/v1/workflows', method: 'POST' },
      { path: '/v1/create-loop', method: 'POST' },
      { path: '/v1/subagents', method: 'GET' },
      { path: '/v1/autoresearch', method: 'POST' },
      { path: '/v1/mcp-indexer', method: 'GET' }
    ] as const

    for (const route of forbiddenRoutes) {
      expect(() => runtimeRequestPayloadSchema.parse(route)).toThrow(
        /runtime request path is not allowed/
      )
    }
  })

  it('rejects unencoded dynamic route ids and singular user-input compatibility paths', () => {
    const rejectedRoutes = [
      { path: '/v1/threads/thr/raw/turns', method: 'POST' },
      { path: '/v1/threads/thr/raw/rewind', method: 'POST' },
      { path: '/v1/threads/thr/raw/turns/turn/raw/steer', method: 'POST' },
      { path: '/v1/threads/thr/raw/checkpoints/axcp/raw/rewind-plan', method: 'POST' },
      { path: '/v1/approvals/appr/raw', method: 'POST' },
      { path: '/v1/user-input/input_raw', method: 'POST' },
      { path: '/v1/sessions/sess/raw/resume-thread', method: 'POST' },
      { path: '/v1/memory/mem/raw', method: 'PATCH' },
      { path: '/v1/attachments/att/raw/content', method: 'GET' }
    ] as const

    for (const route of rejectedRoutes) {
      expect(() => runtimeRequestPayloadSchema.parse(route)).toThrow(
        /runtime request path is not allowed/
      )
    }
  })

  it('rejects runtime request methods that do not match the modeled endpoint', () => {
    expect(() =>
      runtimeRequestPayloadSchema.parse({
        path: '/v1/usage',
        method: 'POST'
      })
    ).toThrow(/runtime request path is not allowed/)
  })

  it('accepts a valid settings patch for analytix and write settings', () => {
    const payload = settingsPatchSchema.parse({
      theme: 'dark',
      motionPreference: 'on',
      runtime: {
          port: 9000,
          model: 'deepseek-chat',
          modelProfiles: {
            'custom-vision-model': {
              aliases: ['custom-vision'],
              contextWindowTokens: 128000,
              inputModalities: ['text', 'image'],
              outputModalities: ['text'],
              supportsToolCalling: true,
              messageParts: ['text', 'image_url']
            }
          },
          tokenEconomy: {
            enabled: true,
            compressToolResults: false,
            historyHygiene: {
              maxToolResultTokens: 4000,
              maxCumulativeToolResultTokens: 0,
              keepRecentToolResults: 0
            }
          },
          subagents: {
            maxParallel: 4,
            maxChildRuns: 24,
            defaultToolPolicy: 'inherit',
            defaultProfile: 'reviewer',
            profiles: {
              reviewer: {
                prompt: 'Review the implementation.',
                model: 'deepseek-v4-pro',
                effort: 'high',
                toolPolicy: 'readOnly',
                tools: ['grep', 'read']
              }
            }
          }
        },
      write: {
        inlineCompletion: {
          model: 'deepseek-v4-pro',
          maxTokens: 128
        },
        selectionAssist: {
          infographicPrompt: '手绘风格信息图。',
          quickActions: [
            { id: 'polish', label: '润色一下', prompt: '请润色这段文字。' },
            { id: 'custom-1', label: '', prompt: '' }
          ]
        },
        typography: {
          fontPreset: 'custom',
          customFontFamily: "'FangSong', serif",
          fontSizePx: 21,
          lineHeight: 2,
          textAlign: 'justify'
        }
      },
      disabledSkillIds: ['test-skill-08']
    })

    expect(payload.runtime?.port).toBe(9000)
    expect(payload.motionPreference).toBe('on')
    expect(payload.runtime?.modelProfiles?.['custom-vision-model']?.inputModalities).toEqual(['text', 'image'])
    expect(payload.runtime?.tokenEconomy?.enabled).toBe(true)
    expect(payload.runtime?.tokenEconomy?.historyHygiene?.maxToolResultTokens).toBe(4000)
    expect(payload.runtime?.tokenEconomy?.historyHygiene?.maxCumulativeToolResultTokens).toBe(0)
    expect(payload.runtime?.tokenEconomy?.historyHygiene?.keepRecentToolResults).toBe(0)
    expect(payload.runtime?.subagents?.profiles?.reviewer?.tools).toEqual(['grep', 'read'])
    expect(payload.write?.inlineCompletion?.model).toBe('deepseek-v4-pro')
    expect(payload.write?.selectionAssist?.infographicPrompt).toBe('手绘风格信息图。')
    expect(payload.write?.selectionAssist?.quickActions).toHaveLength(2)
    expect(payload.write?.typography?.textAlign).toBe('justify')
    expect(payload.disabledSkillIds).toEqual(['test-skill-08'])
  })

  it('accepts media generation settings and provider capability patches', () => {
    const payload = settingsPatchSchema.parse({
      provider: {
        providers: [{
          id: 'minimax',
          name: 'MiniMax',
          baseUrl: 'https://api.minimaxi.com/anthropic',
          endpointFormat: 'messages',
          models: ['MiniMax-M3'],
          textToSpeech: {
            protocol: 'minimax-t2a',
            baseUrl: 'https://api.minimax.io',
            models: ['speech-2.8-hd']
          },
          music: {
            protocol: 'minimax-music',
            baseUrl: 'https://api.minimax.io',
            models: ['music-2.6']
          },
          video: {
            protocol: 'minimax-video',
            baseUrl: 'https://api.minimax.io',
            models: ['MiniMax-Hailuo-2.3']
          }
        }]
      },
      runtime: {
          textToSpeech: {
            enabled: true,
            providerId: 'minimax',
            protocol: 'minimax-t2a',
            model: 'speech-2.8-hd',
            voice: 'male-qn-qingse',
            format: 'mp3',
            timeoutMs: 120000
          },
          musicGeneration: {
            enabled: true,
            providerId: 'minimax',
            protocol: 'minimax-music',
            model: 'music-2.6',
            format: 'mp3',
            timeoutMs: 300000
          },
          videoGeneration: {
            enabled: true,
            providerId: 'minimax',
            protocol: 'minimax-video',
            model: 'MiniMax-Hailuo-2.3',
            defaultDuration: 6,
            defaultResolution: '1080P',
            timeoutMs: 900000,
            pollIntervalMs: 10000
          }
        }
    })

    expect(payload.provider?.providers?.[0]?.textToSpeech?.models).toEqual(['speech-2.8-hd'])
    expect(payload.runtime?.textToSpeech?.enabled).toBe(true)
    expect(payload.runtime?.musicGeneration?.model).toBe('music-2.6')
    expect(payload.runtime?.videoGeneration?.defaultResolution).toBe('1080P')
  })

  it('accepts schedule settings patches and task payloads', () => {
    const payload = settingsPatchSchema.parse({
      schedule: {
        enabled: true,
        keepAwake: true,
        defaultWorkspaceRoot: '/tmp/schedule',
        providerId: 'minimax-token-plan',
        model: 'deepseek-v4-flash',
        mode: 'plan',
        promptPrefix: 'Use the project checklist.',
        skills: {
          defaultNames: ['review'],
          extraDirs: ['/tmp/skills']
        },
        internal: {
          port: 9788,
          secret: 'secret'
        },

      }
    })

    expect(payload.schedule?.internal?.port).toBe(9788)
    expect(payload.schedule?.providerId).toBe('minimax-token-plan')

    const fromText = scheduleTaskFromTextPayloadSchema.parse({
      text: 'Remind me tomorrow morning to ship the review',
      workspaceRoot: '/tmp/schedule',
      clawChannelId: 'channel-1',
      modelHint: 'deepseek-v4-pro',
      mode: 'agent'
    })

    expect(fromText.workspaceRoot).toBe('/tmp/schedule')
    expect(fromText.clawChannelId).toBe('channel-1')
    expect(fromText.modelHint).toBe('deepseek-v4-pro')
  })

  it('strips legacy settings keys before validating settings patches', () => {
    const payload = settingsPatchSchema.parse({
      locale: 'zh',
      disabledSkillIds: ['legacy-skill'],
      agent: { auto_plan: true },
      agentProvider: 'kun',
      agents: {
        kun: {
          model: 'kun-model',
          apiKey: 'sk-kun'
        }
      },
      autoPlan: true,
      auto_plan: true,
      deepseek: { apiKey: 'sk-deepseek' },
      reasonix: { model: 'legacy-reasoner' },
      quickChat: { enabled: true },
      provider: {
        providers: [{
          id: 'legacy-vision-provider',
          imageRecognition: { enabled: true }
        }]
      },
      runtime: {
        port: 9001,
        imageRecognition: { enabled: true }
      }
    })

    expect(payload.locale).toBe('zh')
    expect(payload.provider?.providers?.[0]?.imageRecognition).toEqual({ enabled: true })
    expect(payload.runtime?.port).toBe(9001)
    expect(payload.runtime?.imageRecognition).toEqual({ enabled: true })
    expect(payload.disabledSkillIds).toEqual(['legacy-skill'])
    expect('agent' in payload).toBe(false)
    expect('agentProvider' in payload).toBe(false)
    expect('agents' in payload).toBe(false)
    expect('autoPlan' in payload).toBe(false)
    expect('auto_plan' in payload).toBe(false)
    expect('deepseek' in payload).toBe(false)
    expect('reasonix' in payload).toBe(false)
    expect('quickChat' in payload).toBe(false)
  })

  it('rejects legacy agent-shaped keys inside runtime settings patches', () => {
    expect(() => settingsPatchSchema.parse({
      runtime: {
        port: 9001,
        autoPlan: true
      }
    })).toThrow()

    expect(() => settingsPatchSchema.parse({
      runtime: {
        model: 'deepseek-v4-flash',
        agentProvider: 'reasonix',
        agents: {
          reasonix: {
            model: 'reasonix-runtime-model'
          }
        },
        reasonix: {
          model: 'reasonix-runtime-model'
        }
      }
    })).toThrow()
  })

  it('accepts persisted claw channel welcome markers in full settings snapshots', () => {
    const payload = settingsPatchSchema.parse({
      claw: {
        channels: [{
          id: 'channel-1',
          provider: 'weixin',
          label: 'weixin agent',
          enabled: true,
          model: 'auto',
          threadId: '',
          workspaceRoot: '',
          agentProfile: {
            name: 'weixin agent',
            description: '',
            identity: '',
            personality: '',
            userContext: '',
            replyRules: ''
          },
          conversations: [],
          welcomeSentAt: '2026-06-10T00:00:00.000Z',
          createdAt: '2026-06-10T00:00:00.000Z',
          updatedAt: '2026-06-10T00:00:00.000Z'
        }]
      }
    })

    expect(payload.claw?.channels?.[0]?.welcomeSentAt).toBe('2026-06-10T00:00:00.000Z')
  })

  it('accepts key-free Telegram account metadata and rejects credential drafts in settings patches', () => {
    const payload = settingsPatchSchema.parse({
      claw: {
        channels: [{
          id: 'channel-telegram',
          provider: 'telegram',
          label: 'telegram agent',
          enabled: true,
          model: 'auto',
          threadId: '',
          workspaceRoot: '',
          platformAccount: {
            kind: 'telegram',
            accountId: 'channel-telegram',
            allowedChatIds: '1001,1002',
            botUsername: 'analytix_bot',
            createdAt: '2026-06-10T00:00:00.000Z'
          },
          agentProfile: {
            name: 'telegram agent',
            description: '',
            identity: '',
            personality: '',
            userContext: '',
            replyRules: ''
          },
          conversations: [],
          createdAt: '2026-06-10T00:00:00.000Z',
          updatedAt: '2026-06-10T00:00:00.000Z'
        }]
      }
    })

    expect(payload.claw?.channels?.[0]?.provider).toBe('telegram')
    expect(payload.claw?.channels?.[0]?.platformAccount).toMatchObject({
      kind: 'telegram',
      accountId: 'channel-telegram',
      allowedChatIds: '1001,1002'
    })
    expect(settingsPatchSchema.safeParse({
      claw: {
        channels: [{
          id: 'channel-telegram',
          provider: 'telegram',
          platformCredential: {
            kind: 'telegram',
            botToken: '123456789:AA_mock_token_with_enough_length'
          }
        }]
      }
    }).success).toBe(false)
  })

  it('accepts partial key-free provider profiles and rejects credential fields', () => {
    expect(settingsPatchSchema.safeParse({
      provider: {
        apiKey: 'synthetic-secret',
        providers: [{ id: 'deepseek', apiKey: 'synthetic-secret' }]
      }
    }).success).toBe(false)

    const payload = settingsPatchSchema.parse({
      provider: {
        providers: [{
          id: 'deepseek',
          endpointFormat: 'responses'
        }]
      }
    })

    expect(payload.provider?.providers?.[0]).toEqual({
      id: 'deepseek',
      endpointFormat: 'responses'
    })
  })

  it('accepts partial keyboard shortcut binding maps in settings patches', () => {
    const payload = settingsPatchSchema.parse({
      keyboardShortcuts: {
        bindings: {
          settings: ['Ctrl+,']
        }
      }
    })

    expect(payload.keyboardShortcuts?.bindings?.settings).toEqual(['Ctrl+,'])
  })

  it('accepts a configurable stream idle timeout in runtime tuning patches', () => {
    const payload = settingsPatchSchema.parse({
      runtime: {
          runtimeTuning: {
            streamIdleTimeoutMs: 300000,
            stepLimits: {
              userGlobalMaxModelSteps: 24,
              plannerMaxModelSteps: 8
            }
          },
          quality: {
            enabled: false,
            strictness: 'strict',
            ignoreRules: ['quality-overused-font'],
            ignoreFiles: ['vendor/**'],
            maxFindings: 24
          }
        }
    })

    expect(payload.runtime?.runtimeTuning?.streamIdleTimeoutMs).toBe(300000)
    expect(payload.runtime?.runtimeTuning?.stepLimits?.userGlobalMaxModelSteps).toBe(24)
    expect(payload.runtime?.runtimeTuning?.stepLimits?.plannerMaxModelSteps).toBe(8)
    expect(payload.runtime?.quality?.strictness).toBe('strict')
    expect(payload.runtime?.quality?.ignoreFiles).toEqual(['vendor/**'])
  })

  it('rejects an out-of-range stream idle timeout', () => {
    expect(() =>
      settingsPatchSchema.parse({
        runtime: { runtimeTuning: { streamIdleTimeoutMs: -1 } }
      })
    ).toThrow()
  })

  it('rejects out-of-range runtime step limits', () => {
    expect(() =>
      settingsPatchSchema.parse({
        runtime: { runtimeTuning: { stepLimits: { defaultMaxModelSteps: -1 } } }
      })
    ).toThrow()
  })

  it('rejects unknown settings patch fields', () => {
    expect(() =>
      settingsPatchSchema.parse({
        runtime: {
            mysteryFlag: true
          }
      })
    ).toThrow(/Unrecognized key/)
  })

  it('rejects unknown schedule patch fields', () => {
    expect(() =>
      settingsPatchSchema.parse({
        schedule: {
          tasks: [{
            id: 'task-1',
            prompt: 'Run',
            schedule: { kind: 'manual' },
            legacyClawOnlyField: true
          }]
        }
      })
    ).toThrow(/Unrecognized key/)
  })

  it('allows only safe external URL protocols', () => {
    expect(isSafeOpenExternalUrl('https://deepseek.com')).toBe(true)
    expect(isSafeOpenExternalUrl('http://127.0.0.1:5173')).toBe(true)
    expect(isSafeOpenExternalUrl('mailto:security@example.invalid')).toBe(true)
    expect(isSafeOpenExternalUrl('javascript:alert(1)')).toBe(false)
    expect(isSafeOpenExternalUrl('file:///tmp/test')).toBe(false)
    expect(() => shellOpenExternalUrlSchema.parse('javascript:alert(1)')).toThrow(
      /Only http, https, and mailto URLs are allowed/
    )
  })

  it('rejects invalid SSE payloads', () => {
    expect(() =>
      sseStartPayloadSchema.parse({
        threadId: 'thread-1',
        sinceSeq: -1
      })
    ).toThrow()
  })

  it('keeps SSE start payloads on the analytix thread cursor contract', () => {
    expect(sseStartPayloadSchema.parse({
      threadId: ' thread-1 ',
      sinceSeq: 7,
      streamId: ' stream-1 '
    })).toEqual({
      threadId: 'thread-1',
      sinceSeq: 7,
      streamId: 'stream-1'
    })

    expect(sseStartPayloadSchema.parse({
      threadId: 'thread-1',
      sinceSeq: 7
    })).toEqual({
      threadId: 'thread-1',
      sinceSeq: 7
    })

    expect(() =>
      sseStartPayloadSchema.parse({
        threadId: 'thread-1',
        sinceSeq: 7,
        reasonixSessionId: 'session-1'
      })
    ).toThrow(/Unrecognized key/)
  })

  it('accepts an explicit finite SSE boundary and rejects unsafe or reversed cursors', () => {
    expect(sseStartPayloadSchema.parse({ threadId: 't', sinceSeq: 0, replayThroughSeq: 0 }))
      .toEqual({ threadId: 't', sinceSeq: 0, replayThroughSeq: 0 })
    for (const replayThroughSeq of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1, 3]) {
      expect(() => sseStartPayloadSchema.parse({ threadId: 't', sinceSeq: 4, replayThroughSeq })).toThrow()
    }
  })

  it('accepts long Feishu install device codes', () => {
    const deviceCode = 'x'.repeat(2_048)
    const payload = clawImInstallPollPayloadSchema.parse({
      provider: 'feishu',
      deviceCode
    })

    expect(payload.deviceCode).toBe(deviceCode)
  })

  it('keeps Telegram token binding out of the QR poll schema', () => {
    const payload = clawImTelegramTokenPayloadSchema.parse({
      botToken: ' 123456789:AA_mock_token_with_enough_length ',
      allowedChatIds: ' 1001, 1002 '
    })

    expect(payload).toEqual({
      botToken: '123456789:AA_mock_token_with_enough_length',
      allowedChatIds: '1001, 1002'
    })
    expect(() =>
      clawImInstallPollPayloadSchema.parse({
        provider: 'telegram',
        deviceCode: 'abc'
      })
    ).toThrow()
  })

  it('accepts workspace directory payloads without a child path', () => {
    const payload = workspaceDirectoryTargetPayloadSchema.parse({
      workspaceRoot: '/tmp/workspace'
    })

    expect(payload.workspaceRoot).toBe('/tmp/workspace')
    expect(payload.path).toBeUndefined()
  })

  it('accepts workspace directory create payloads', () => {
    const payload = workspaceDirectoryCreatePayloadSchema.parse({
      workspaceRoot: '/tmp/workspace',
      path: 'notes'
    })

    expect(payload.path).toBe('notes')
  })

  it('accepts workspace rename payloads', () => {
    const payload = workspaceEntryRenamePayloadSchema.parse({
      workspaceRoot: '/tmp/workspace',
      path: '/tmp/workspace/draft.md',
      newName: 'final.md'
    })

    expect(payload.newName).toBe('final.md')
  })

  it('accepts workspace delete payloads', () => {
    const payload = workspaceEntryDeletePayloadSchema.parse({
      workspaceRoot: '/tmp/workspace',
      path: '/tmp/workspace/draft.md'
    })

    expect(payload.path).toBe('/tmp/workspace/draft.md')
  })

  it('accepts structured inline completion payloads', () => {
    const payload = writeInlineCompletionPayloadSchema.parse({
      prefix: '## Heading\n\nSome intro',
      suffix: '',
      mode: 'edit',
      workspaceRoot: '/tmp/workspace',
      currentFilePath: '/tmp/workspace/notes.md',
      cursor: {
        line: 3,
        column: 10
      },
      context: {
        language: 'markdown',
        currentLinePrefix: 'Some intro',
        currentLineSuffix: '',
        previousLine: '',
        previousNonEmptyLine: '## Heading',
        nextLine: '',
        indentation: '',
        signals: {
          list: false,
          quote: false,
          heading: false,
          table: false,
          atLineEnd: true,
          endsWithSentencePunctuation: false,
          previousLineEndsWithSentencePunctuation: false,
          prefersNewLineCompletion: false,
          paragraphBreakOpportunity: false
        }
      },
      policy: {
        name: 'precision-inline-v2',
        instruction: 'Return only the inserted text.',
        acceptanceCriteria: ['Keep it short.'],
        rejectionCriteria: ['Do not ramble.']
      },
      preview: {
        local: 'Some intro',
        documentTail: '## Heading Some intro'
      },
      editCandidate: {
        kind: 'paragraph',
        from: 12,
        to: 22,
        startLine: 3,
        startColumn: 1,
        endLine: 3,
        endColumn: 10,
        original: 'Some intro',
        selectedText: 'Some'
      },
      recentEdits: [{
        source: 'user',
        ageMs: 1_200,
        filePath: '/tmp/workspace/notes.md',
        from: 12,
        to: 16,
        deletedText: 'Old',
        insertedText: 'Some',
        beforeContext: '',
        afterContext: ' intro'
      }],
      model: 'deepseek-v4-pro'
    })

    expect(payload.model).toBe('deepseek-v4-pro')
    expect(payload.mode).toBe('edit')
    expect(payload.workspaceRoot).toBe('/tmp/workspace')
    expect(payload.cursor.line).toBe(3)
    expect(payload.editCandidate?.kind).toBe('paragraph')
    expect(payload.recentEdits?.[0].insertedText).toBe('Some')
  })

  it('accepts identity-only export and clipboard bindings, never Renderer bytes or paths', () => {
    const binding = { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), threadId: 'main-thread',
      baseRevision: 'c'.repeat(64), draftVersion: 'd'.repeat(48) }
    const payload = writeExportPayloadSchema.parse({ ...binding, format: 'docx', typography: { fontSizePx: 21 } })
    expect(payload).toMatchObject({ ...binding, format: 'docx', typography: { fontSizePx: 21 } })
    expect(writeRichClipboardPayloadSchema.parse(binding)).toEqual(binding)
    for (const field of ['workspaceRoot', 'path', 'content', 'principal', 'scopeId']) {
      expect(writeExportPayloadSchema.safeParse({ ...binding, format: 'docx', [field]: 'forged' }).success).toBe(false)
      expect(writeRichClipboardPayloadSchema.safeParse({ ...binding, [field]: 'forged' }).success).toBe(false)
    }
    expect(writeExportPayloadSchema.safeParse({ path: '/tmp/workspace/draft.md', format: 'docx', content: '# Draft' }).success).toBe(false)
  })

})

it('never admits protected process or question reads through generic runtime IPC', () => {
  const requestId = 'input_' + 'a'.repeat(64)
  const questionRead = userInputLocalSelectorSchemaV1.parse({
    threadId: 'thread-a', turnId: 'turn-a', itemId: 'item_' + requestId, requestId
  })
  for (const path of ['/v1/local-display/process/read', '/v1/local-display/process/target', '/v1/local-display/user-input']) {
    const body = JSON.stringify(path === '/v1/local-display/user-input' ? questionRead : {})
    for (const method of ['GET', 'POST', 'PATCH', 'PUT', 'DELETE']) {
      for (const candidate of [path, path.slice(1), path + '?viewerToken=forged', path + '/', path + '/reveal']) {
        expect(runtimeRequestPayloadSchema.safeParse({ path: candidate, method, body }).success).toBe(false)
      }
    }
  }
  expect(runtimeRequestPayloadSchema.safeParse({
    path: '/v1/local-display/user-input', method: 'POST', body: JSON.stringify(questionRead),
    headers: { 'X-Analytix-Local-Display': 'typed-v1' }
  }).success).toBe(false)
  expect(sseStartPayloadSchema.safeParse({ threadId: 'thread-a', sinceSeq: 0, viewerToken: 'forged' }).success).toBe(false)
})

it('keeps protected preview reads identity-only and ordinary workspace reads unchanged', () => {
  const protectedRead = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64) }
  expect(workspaceFileReadPayloadSchema.parse({ protectedRead })).toEqual({ protectedRead })
  const ordinary = { path: '/synthetic/ordinary.go', workspaceRoot: '/synthetic', line: 9 }
  expect(workspaceFileReadPayloadSchema.parse(ordinary)).toEqual(ordinary)
  for (const value of [{ ...ordinary, protectedRead }, { protectedRead, preview: 'source' }, { protectedRead: { ...protectedRead, path: '/forged' } }, { preview: 'source', path: '/forged' }, { protectedRead: {} }]) {
    expect(workspaceFileReadPayloadSchema.safeParse(value).success).toBe(false)
  }
})

describe('user-input closed request boundary', () => {
  const parse = (value: unknown) => runtimeRequestPayloadSchema.safeParse({
    path: '/v1/user-inputs/input_owned', method: 'POST', body: JSON.stringify(value)
  }).success
  it('admits one typed batch, legacy single answer, and whole-request cancellation', () => {
    expect(parse({ answers: [
      { id: 'input_owned_1', kind: 'answer', optionIds: ['input_owned_1_option_1'], customText: 'custom' },
      { id: 'input_owned_2', kind: 'skip' }
    ] })).toBe(true)
    expect(parse({ answers: [{ id: 'q1', label: 'Yes', value: 'Yes' }] })).toBe(true)
    expect(parse({ cancelled: true })).toBe(true)
  })
  it.each([
    {}, { answers: [] },
    { cancelled: true, answers: [{ id: 'input_owned_1', kind: 'skip' }] },
    { answers: [{ id: 'input_owned_1', kind: 'skip', customText: 'ignored' }] },
    { answers: [{ id: 'input_owned_1', kind: 'answer', optionIds: [], customText: '  ' }] },
    { answers: [{ id: 'input_owned_1', kind: 'answer', optionIds: ['selected'], customText: '' }] },
    { answers: [{ id: 'input_owned_1', kind: 'answer', optionIds: ['same', 'same'] }] },
    { answers: [{ id: 'input_owned_1', kind: 'skip' }, { id: 'input_owned_1', kind: 'skip' }] },
    { answers: [{ id: 'input_owned_1', kind: 'skip' }, { id: 'q1', value: 'legacy' }] },
    { answers: [{ id: 'q1', value: 'legacy', privateAuthority: 'ignored' }] }
  ])('rejects malformed, mixed or duplicate answer batches', value => {
    expect(parse(value)).toBe(false)
  })
})

describe('closed model preference settings input', () => {
  const preference = {
    id: 'provider-alpha', baseUrl: 'https://provider.invalid/v1', endpointFormat: 'chat_completions',
    modelProfiles: { 'model-alpha': { aliases: ['alpha'], reasoning: {
      supportedEfforts: ['off', 'high'], defaultEffort: 'high', requestProtocol: 'deepseek-chat-completions'
    } } }
  }
  it('admits key-free on, explicit off and inherited preferences through normal settings', () => {
    const patch = { provider: { modelPreferences: [preference] } }
    expect(settingsPatchSchema.parse(patch)).toEqual(patch)
    expect(settingsPatchSchema.parse({ provider: { modelPreferences: [{ ...preference,
      modelProfiles: { 'model-alpha': { reasoning: null } }
    }] } }).provider?.modelPreferences?.[0].modelProfiles['model-alpha'].reasoning).toBeNull()
    expect(settingsPatchSchema.parse({ provider: {} }).provider).not.toHaveProperty('modelPreferences')
  })
  it.each([
    { ...preference, apiKey: 'synthetic-rejected-credential' },
    { ...preference, models: ['unadmitted-model'] },
    { ...preference, proxy: 'https://proxy.invalid' },
    { ...preference, endpointFormat: ['messages'] },
    { ...preference, baseUrl: 'https://provider.invalid/path\\segment' },
    { ...preference, baseUrl: 'https://user:synthetic@provider.invalid' },
    { ...preference, baseUrl: 'https://provider.invalid?api_key=synthetic' },
    { ...preference, modelProfiles: { 'model-alpha': { supportsToolCalling: true } } },
    { ...preference, modelProfiles: { 'model-alpha': { aliases: Array(51).fill('alias') } } },
    { ...preference, modelProfiles: { 'model-alpha': { reasoning: { supportedEfforts: ['off'], defaultEffort: 'high', requestProtocol: 'none' } } } },
    { ...preference, modelProfiles: { 'model-alpha': {}, 'MODEL-ALPHA': {} } }
  ])('rejects unsupported preference authority or malformed fields %#', (value) => {
    expect(settingsPatchSchema.safeParse({ provider: { modelPreferences: [value] } }).success).toBe(false)
  })
  it('preserves and validates prototype-named own model keys', () => {
    const profiles = Object.fromEntries([['__proto__', { reasoning: null }]])
    const parsed = settingsPatchSchema.parse({ provider: { modelPreferences: [{ ...preference, modelProfiles: profiles }] } })
    const readback = parsed.provider!.modelPreferences![0].modelProfiles
    expect(Object.hasOwn(readback, '__proto__')).toBe(true)
    expect(readback['__proto__'].reasoning).toBeNull()
    const invalid = Object.fromEntries([['__proto__', { apiKey: 'synthetic-rejected' }]])
    expect(settingsPatchSchema.safeParse({ provider: { modelPreferences: [{ ...preference, modelProfiles: invalid }] } }).success).toBe(false)
  })
  it('bounds runtime projection bytes using the existing Go JSON encoding', () => {
    const modelProfiles = Object.fromEntries(Array.from({ length: 100 }, (_, index) => [`model-${index}`, {
      aliases: Array.from({ length: 50 }, (_, alias) => '<'.repeat(126) + String(alias).padStart(2, '0'))
    }]))
    const value = { ...preference, modelProfiles }
    expect(new TextEncoder().encode(JSON.stringify({ providers: [value] })).byteLength).toBeLessThan(1_048_576)
    expect(settingsPatchSchema.safeParse({ provider: { modelPreferences: [value] } }).success).toBe(false)
  })
  it('rejects duplicate provider bindings and bounds preference inventories', () => {
    expect(settingsPatchSchema.safeParse({ provider: { modelPreferences: [preference, preference] } }).success).toBe(false)
    expect(settingsPatchSchema.safeParse({ provider: { modelPreferences: Array(129).fill(preference) } }).success).toBe(false)
    const modelProfiles = Object.fromEntries(Array.from({ length: 201 }, (_, index) => [`model-${index}`, {}]))
    expect(settingsPatchSchema.safeParse({ provider: { modelPreferences: [{ ...preference, modelProfiles }] } }).success).toBe(false)
  })
})

describe('normal user-input policy Settings IPC', () => {
  it.each([{ policy: 'legacy' }, { policy: 'timed' }, { policy: 'timed', defaultTimeoutSeconds: null },
    { policy: 'timed', defaultTimeoutSeconds: -1 }, { policy: 'timed', defaultTimeoutSeconds: 2147483 }])('admits the strict policy atom %j', userInput => {
    expect(settingsPatchSchema.parse({ runtime: { userInput } }).runtime?.userInput).toEqual(userInput)
  })
  it.each([{}, { defaultTimeoutSeconds: 120 }, { policy: 'timed', defaultTimeoutSeconds: 0 },
    { policy: 'timed', defaultTimeoutSeconds: 1.5 }, { policy: 'timed', defaultTimeoutSeconds: 2147484 },
    { policy: 'timed', extra: true }])('rejects incomplete or injected policy %j', userInput => {
    expect(settingsPatchSchema.safeParse({ runtime: { userInput } }).success).toBe(false)
  })
})

describe('timed user-input action transport admission', () => {
  const path = analytixUserInputPath('input_' + 'a'.repeat(64))
  it('admits only the closed continue body and bounded late attempt fields', () => {
    expect(runtimeRequestPayloadSchema.safeParse({ path, method: 'POST', body: JSON.stringify({ action: 'continue' }) }).success).toBe(true)
    expect(runtimeRequestPayloadSchema.safeParse({ path, method: 'POST', body: JSON.stringify({ action: 'continue', callId: 'injected' }) }).success).toBe(false)
    expect(runtimeRequestPayloadSchema.safeParse({ path, method: 'GET' }).success).toBe(false)
  })
})


describe('normal performance and usage settings IPC', () => {
  it('accepts only the two display modes through the existing strict patch schema', () => {
    expect(settingsPatchSchema.parse({ performanceUsageMode: 'compact' })).toEqual({ performanceUsageMode: 'compact' })
    expect(settingsPatchSchema.parse({ performanceUsageMode: 'detailed' })).toEqual({ performanceUsageMode: 'detailed' })
    for (const performanceUsageMode of ['verbose', null, 0, {}]) {
      expect(settingsPatchSchema.safeParse({ performanceUsageMode }).success).toBe(false)
    }
  })
})


it('rejects old task-array settings writes and validates closed task mutations', () => {
  for (const tasks of [[], [{ id: 'task-1' }]]) expect(() => settingsPatchSchema.parse({ schedule: { tasks } })).toThrow()
  const config = { title: 'Daily', enabled: true, prompt: 'Check', workspaceRoot: '/synthetic/work', clawChannelId: '', providerId: '',
    model: 'deepseek-v4-flash', reasoningEffort: 'high', mode: 'agent', schedule: { kind: 'daily', everyMinutes: 60, timeOfDay: '09:00', atTime: '' } }
  const newConfig = { ...config, schedule: { ...config.schedule, timeZone: 'UTC' } }
  expect(scheduleTaskCreatePayloadSchema.parse(newConfig)).toEqual(newConfig)
  expect(() => scheduleTaskCreatePayloadSchema.parse(config)).toThrow()
  expect(() => scheduleTaskCreatePayloadSchema.parse({ ...config, id: 'foreign-id' })).toThrow()
  expect(() => scheduleTaskUpdatePayloadSchema.parse({ taskId: 'task-1', patch: { title: 'New' }, expected: {} })).toThrow()
  expect(() => scheduleTaskUpdatePayloadSchema.parse({ taskId: 'task-1', patch: { lastThreadId: 'foreign-thread' }, expected: {} })).toThrow()
  expect(() => scheduleTaskUpdatePayloadSchema.parse({ taskId: 'task-1', patch: { enabled: true }, expected: { enabled: false } })).toThrow()
  expect(scheduleTaskUpdatePayloadSchema.parse({ taskId: 'task-1', patch: { title: 'New' }, expected: { title: 'Daily' } }).patch.title).toBe('New')
  expect(scheduleTaskDeletePayloadSchema.parse({ taskId: 'task-1', expected: config }).expected).toEqual(config)
})


it('keeps Present current-file reads separate from frozen Read and ordinary path requests', () => {
  const protectedPresent = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64), index: 0 }
  expect(workspaceFileReadPayloadSchema.parse({ protectedPresent })).toEqual({ protectedPresent })
  for (const payload of [{ protectedPresent, path: '/forged' }, { protectedPresent, workspaceRoot: '/forged' },
    { protectedPresent, protectedRead: protectedPresent }, { protectedPresent: { ...protectedPresent, index: 8 } },
    { protectedPresent: { ...protectedPresent, path: '/forged' } }]) {
    expect(workspaceFileReadPayloadSchema.safeParse(payload).success).toBe(false)
  }
})

it('validates Schedule calendar writes while retaining old daily CAS expected values', () => {
  const old = { kind: 'daily', everyMinutes: 60, timeOfDay: '09:00', atTime: '' }
  const daily = { ...old, timeZone: 'America/New_York' }
  expect(scheduleTaskUpdatePayloadSchema.safeParse({ taskId: 'a', patch: { schedule: daily }, expected: { enabled: true, schedule: old } }).success).toBe(true)
  for (const schedule of [{ ...daily, timeZone: '' }, { ...daily, timeZone: 'EST' }, { ...old, kind: 'weekly', timeZone: 'UTC', weekdays: [1, 1] }, { ...old, kind: 'weekly', weekdays: [1] }, { ...old, kind: 'cron', timeZone: 'UTC', expression: '0 0 * * MON' }]) {
    expect(scheduleTaskCreatePayloadSchema.safeParse({ title: 'A', prompt: 'Synthetic', schedule }).success).toBe(false)
  }
  for (const schedule of [{ ...old, kind: 'weekly', timeZone: 'UTC', weekdays: [1, 2, 3, 4, 5] }, { ...old, kind: 'cron', timeZone: 'UTC', expression: '0 9 * * 1-5' }]) {
    expect(scheduleTaskCreatePayloadSchema.safeParse({ title: 'A', prompt: 'Synthetic', schedule }).success).toBe(true)
  }
})

it('keeps Schedule History read bounded and rejects caller-origin or ACK injection', async () => {
  const { scheduleTaskHistoryPayloadSchema } = await import('./app-ipc-schemas')
  expect(scheduleTaskHistoryPayloadSchema.parse({ taskId: 'a', limit: 20, before: 'actual-attempt' })).toEqual({ taskId: 'a', limit: 20, before: 'actual-attempt' })
  for (const request of [{ taskId: 'a', limit: 21 }, { taskId: 'a', limit: 20, origin: { threadId: 'forged' } }, { taskId: 'a', limit: 20, records: [] }]) expect(scheduleTaskHistoryPayloadSchema.safeParse(request).success).toBe(false)
})


it('Present media admits only a current indexed selector or a separate ordinary path request', () => {
  const protectedPresent = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64), index: 0 }
  expect(workspacePresentMediaReadPayloadSchema.parse({ protectedPresent })).toEqual({ protectedPresent })
  expect(workspacePresentMediaReadPayloadSchema.parse({ path: '/ordinary.png', workspaceRoot: '/ordinary' })).toEqual({ path: '/ordinary.png', workspaceRoot: '/ordinary' })
  for (const value of [{ protectedPresent, path: '/forged' }, { protectedPresent, workspaceRoot: '/forged' },
    { protectedPresent, protectedRead: protectedPresent }, { protectedRead: protectedPresent }, { protectedPresent: null },
    { protectedPresent: { ...protectedPresent, index: 8 } }, { protectedPresent: { ...protectedPresent, path: '/forged' } }]) {
    expect(workspacePresentMediaReadPayloadSchema.safeParse(value).success).toBe(false)
  }
})


it('reuses the Present media identity schema for Excel without protected-path or frozen Read mixing', () => {
  const protectedPresent = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64), index: 0 }
  expect(workspacePresentMediaReadPayloadSchema.parse({ protectedPresent })).toEqual({ protectedPresent })
  for (const extension of ['xlsx', 'xls', 'csv', 'tsv']) {
    const ordinary = { path: `/ordinary/actual.${extension}`, workspaceRoot: '/ordinary' }
    expect(workspacePresentMediaReadPayloadSchema.parse(ordinary)).toEqual(ordinary)
  }
  for (const payload of [
    { protectedPresent, path: '/forged/book.xlsx' },
    { protectedPresent, workspaceRoot: '/forged' },
    { protectedPresent, preview: 'source' },
    { protectedPresent, protectedRead: protectedPresent },
    { protectedRead: protectedPresent },
    { protectedPresent: { ...protectedPresent, path: '/forged/book.xlsx' } },
    { protectedPresent: { ...protectedPresent, format: 'xlsx' } },
    { protectedPresent: { ...protectedPresent, index: 8 } },
    { path: '/ordinary/actual.csv', preview: 'source' }
  ]) expect(workspacePresentMediaReadPayloadSchema.safeParse(payload).success).toBe(false)
})

describe('Host child-input IPC closed intent', () => {
  const path = ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH
  const inspect = { mode: 'inspect', threadId: 'parent', jobId: 'old-job' }
  const submit = { ...inspect, mode: 'start_or_steer', expectedRunId: 'current-job', expectedChildTurnId: 'current-turn',
    message: 'continue', clientMessageId: 'd3a26000-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }
  const request = (body: unknown, target = path) => ({ path: target, method: 'POST', body: JSON.stringify(body) })
  it('admits inspect and exact expected-identity submission without renderer authority', () => {
    for (const body of [inspect, submit]) expect(runtimeRequestPayloadSchema.safeParse(request(body)).success).toBe(true)
  })
  it('rejects implicit legacy modes, caller authority, stale envelope shapes and noncanonical UUIDs', () => {
    for (const body of [
      { threadId: 'parent', jobId: 'old-job', message: 'continue' }, { ...submit, mode: 'steer' },
      { ...inspect, message: 'continue' }, { ...submit, expectedRunId: undefined },
      { ...submit, sourceTurnId: 'turn' }, { ...submit, sourceToolCallId: 'call' },
      { ...submit, clientMessageId: 'steer_timestamp' }, { ...submit, runtimeToken: 'FORGED' },
      { ...submit, message: ' continue ' }, { ...submit, message: '界'.repeat(4001) }
    ]) expect(runtimeRequestPayloadSchema.safeParse(request(body)).success).toBe(false)
    for (const target of [path + '?mode=inspect', path + '#fragment', path + '/']) {
      expect(runtimeRequestPayloadSchema.safeParse(request(inspect, target)).success).toBe(false)
    }
  })
})

describe('HTML preview IPC schemas', () => {
  it('admits only closed modes and exact grant revocation', () => {
    expect(writePrototypeFilePayloadSchema.parse({ path: 'page.htm', workspaceRoot: '/workspace', mode: 'workspace-html' }).mode).toBe('workspace-html')
    for (const extra of [{ mode: 'anything' }, { protectedRead: {} }, { preload: 'unsafe' }]) {
      expect(writePrototypeFilePayloadSchema.safeParse({ path: 'page.htm', workspaceRoot: '/workspace', ...extra }).success).toBe(false)
    }
    expect(writePrototypeRevokePayloadSchema.safeParse({ grantId: 'a'.repeat(48) }).success).toBe(true)
    for (const payload of [{ grantId: '' }, { grantId: 'persist:session' }, { grantId: 'a'.repeat(48), ownerId: 99 }]) {
      expect(writePrototypeRevokePayloadSchema.safeParse(payload).success).toBe(false)
    }
  })
})

it('keeps protected prototype authorization separate from external path opening', () => {
  const protectedPresent = { threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64),
    resultItemId: 'item_result_' + 'b'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 0 }
  const request = { protectedPresent, mode: 'workspace-html' }
  expect(writePrototypeAuthorizePayloadSchema.safeParse(request).success).toBe(true)
  expect(writePrototypeFilePayloadSchema.safeParse(request).success).toBe(false)
  expect(writePrototypeAuthorizePayloadSchema.safeParse({ ...request, path: '/forged.html' }).success).toBe(false)
})
