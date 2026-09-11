import { describe, expect, it } from 'vitest'
import {
  analytixApprovalPath,
  analytixAttachmentContentPath,
  analytixMemoryRecordPath,
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
  isSafeOpenExternalUrl,
  runtimeRequestPayloadSchema,
  scheduleTaskFromTextPayloadSchema,
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

describe('app-ipc-schemas', () => {
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
        body: '{"jobIds":["job-1"],"jobId":"job-1","threadId":"thr_1","message":"continue"}'
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
      { path: analytixUserInputPath(inputId), method: 'POST', body: '{}' },
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
        tasks: [{
          id: 'task-1',
          title: 'Daily review',
          enabled: true,
          prompt: 'Review the repo',
          workspaceRoot: '/tmp/schedule',
          clawChannelId: 'channel-1',
          providerId: 'minimax-token-plan',
          model: 'auto',
          reasoningEffort: 'high',
          mode: 'agent',
          schedule: {
            kind: 'daily',
            everyMinutes: 60,
            timeOfDay: '09:30',
            atTime: ''
          },
          lastStatus: 'idle'
        }]
      }
    })

    expect(payload.schedule?.internal?.port).toBe(9788)
    expect(payload.schedule?.providerId).toBe('minimax-token-plan')
    expect(payload.schedule?.tasks?.[0]?.schedule?.kind).toBe('daily')
    expect(payload.schedule?.tasks?.[0]?.reasoningEffort).toBe('high')
    expect(payload.schedule?.tasks?.[0]?.clawChannelId).toBe('channel-1')
    expect(payload.schedule?.tasks?.[0]?.providerId).toBe('minimax-token-plan')

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

  it('accepts write export payloads', () => {
    const payload = writeExportPayloadSchema.parse({
      path: '/tmp/workspace/draft.md',
      format: 'docx',
      content: '# Draft',
      typography: {
        fontPreset: 'custom',
        customFontFamily: "'FangSong', serif",
        fontSizePx: 21,
        lineHeight: 2
      }
    })

    expect(payload.path).toBe('/tmp/workspace/draft.md')
    expect(payload.format).toBe('docx')
    expect(payload.content).toBe('# Draft')
    expect(payload.typography?.fontSizePx).toBe(21)
  })

  it('rejects renderer-supplied write export workspace authority', () => {
    expect(() => writeExportPayloadSchema.parse({
      path: '/tmp/workspace/draft.md',
      workspaceRoot: '/tmp/renderer-forged-workspace',
      format: 'docx',
      content: '# Draft'
    })).toThrow()
  })

  it('accepts write rich clipboard payloads', () => {
    const payload = writeRichClipboardPayloadSchema.parse({
      path: '/tmp/workspace/draft.md',
      workspaceRoot: '/tmp/workspace',
      content: '# Draft'
    })

    expect(payload.path).toBe('/tmp/workspace/draft.md')
    expect(payload.content).toBe('# Draft')
  })
})
