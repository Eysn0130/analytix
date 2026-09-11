import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsPatch,
  type AppSettingsV1
} from '@shared/app-settings'
import {
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
  analytixThreadSteerPath
} from '@shared/analytix-endpoints'
import { rendererRuntimeClient } from './runtime-client'

function settings(workspaceRoot = '/tmp/workspace'): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot,
    log: { enabled: false, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    claw: defaultClawSettings(),
    schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

afterEach(() => {
  rendererRuntimeClient.invalidateSettings()
  vi.unstubAllGlobals()
})

describe('rendererRuntimeClient', () => {
  it('caches settings reads until invalidated', async () => {
    const getSettings = vi.fn(async () => settings('/tmp/first'))
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings,
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest: vi.fn(),
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    const first = await rendererRuntimeClient.getSettings()
    const second = await rendererRuntimeClient.getSettings()

    expect(first.workspaceRoot).toBe('/tmp/first')
    expect(second).toBe(first)
    expect(getSettings).toHaveBeenCalledTimes(1)
  })

  it('refreshes the cache after setSettings', async () => {
    const getSettings = vi.fn(async () => settings('/tmp/first'))
    const setSettings = vi.fn(async () => settings('/tmp/second'))
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings,
          setSettings
        },
        runtime: {
          runtimeRequest: vi.fn(),
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await rendererRuntimeClient.getSettings()
    const next = await rendererRuntimeClient.setSettings({ workspaceRoot: '/tmp/next' })
    const cached = await rendererRuntimeClient.getSettings()

    expect(next.workspaceRoot).toBe('/tmp/second')
    expect(cached).toBe(next)
    expect(getSettings).toHaveBeenCalledTimes(1)
    expect(setSettings).toHaveBeenCalledTimes(1)
  })

  it('passes top-level runtime settings patches through window.analytix.settings without legacy aliases', async () => {
    const getSettings = vi.fn(async () => settings('/tmp/initial'))
    const setSettings = vi.fn(async (partial: AppSettingsPatch) => ({
      ...settings('/tmp/next'),
      runtime: {
        ...defaultAnalytixRuntimeSettings(),
        model: partial.runtime?.model ?? 'deepseek-chat',
        approvalPolicy: partial.runtime?.approvalPolicy ?? 'on-request'
      }
    }))
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings,
          setSettings
        },
        runtime: {
          runtimeRequest: vi.fn(),
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      },
      get kun() {
        throw new Error('legacy bridge alias should not be read')
      },
      get reasonix() {
        throw new Error('legacy bridge alias should not be read')
      }
    })

    const patch = { runtime: { model: 'deepseek-reasoner', approvalPolicy: 'never' as const } }
    const next = await rendererRuntimeClient.setSettings(patch)
    const cached = await rendererRuntimeClient.getSettings()

    expect(next.runtime.model).toBe('deepseek-reasoner')
    expect(next.runtime.approvalPolicy).toBe('never')
    expect(cached).toBe(next)
    expect(setSettings).toHaveBeenCalledWith(patch)
    expect(getSettings).not.toHaveBeenCalled()
  })

  it('forwards explicit runtime restarts through the preload bridge', async () => {
    const restartRuntime = vi.fn(async () => undefined)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest: vi.fn(),
          restartRuntime,
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.restartRuntime()).resolves.toBeUndefined()
    expect(restartRuntime).toHaveBeenCalledTimes(1)
  })

  it('passes runtime requests through window.analytix without rewriting arguments', async () => {
    const response = { ok: true, status: 200, body: '{"ok":true}' }
    const runtimeRequest = vi.fn(async () => response)
    const path = analytixThreadSteerPath(
      'thr/with space?x=1#frag',
      'turn/with space?x=1#frag'
    )
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      },
      get kun() {
        throw new Error('legacy bridge alias should not be read')
      },
      get reasonix() {
        throw new Error('legacy bridge alias should not be read')
      }
    })

    await expect(rendererRuntimeClient.runtimeRequest(path)).resolves.toEqual(response)
    await expect(rendererRuntimeClient.runtimeRequest(path, 'POST')).resolves.toEqual(response)
    await expect(rendererRuntimeClient.runtimeRequest(path, 'POST', '{"action":"step"}')).resolves.toEqual(response)

    expect(path).toContain('%2F')
    expect(runtimeRequest).toHaveBeenNthCalledWith(1, path)
    expect(runtimeRequest).toHaveBeenNthCalledWith(2, path, 'POST')
    expect(runtimeRequest).toHaveBeenNthCalledWith(3, path, 'POST', '{"action":"step"}')
  })

  it('exposes a minimal background task job list helper over the canonical runtime path', async () => {
    const response = { ok: true, status: 200, body: '{"jobs":[]}' }
    const runtimeRequest = vi.fn(async () => response)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.listTaskJobs({ threadId: 'thr_1', active: true })).resolves.toEqual(response)

    expect(runtimeRequest).toHaveBeenCalledWith(
      ANALYTIX_RUNTIME_TASK_JOBS_LIST_PATH,
      'POST',
      '{"threadId":"thr_1","active":true}'
    )
  })

  it('exposes a subagent steer helper over the canonical runtime path', async () => {
    const response = { ok: true, status: 200, body: '{"status":"queued"}' }
    const runtimeRequest = vi.fn(async () => response)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.steerTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      message: 'narrow the scope'
    })).resolves.toEqual(response)

    expect(runtimeRequest).toHaveBeenCalledWith(
      ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","message":"narrow the scope"}'
    )
  })

  it('exposes task job pause, resume, and kill helpers over canonical runtime paths', async () => {
    const response = { ok: true, status: 200, body: '{"status":"requested"}' }
    const runtimeRequest = vi.fn(async () => response)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.pauseTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'pause_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.resumeTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'resume_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.killTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      reason: 'stop'
    })).resolves.toEqual(response)

    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"pause_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2,
      ANALYTIX_RUNTIME_TASK_JOBS_RESUME_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"resume_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      3,
      ANALYTIX_RUNTIME_TASK_JOBS_KILL_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","reason":"stop"}'
    )
  })

  it('exposes task job wait, output, and restart helpers over canonical runtime paths', async () => {
    const response = { ok: true, status: 200, body: '{"status":"running"}' }
    const runtimeRequest = vi.fn(async () => response)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.waitTaskJobs({
      threadId: 'thr_1',
      jobIds: ['job_1'],
      timeoutMs: 1000
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.outputTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      tail: true
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.restartTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.recoverTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      deliveryId: 'delivery_1'
    })).resolves.toEqual(response)

    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      ANALYTIX_RUNTIME_TASK_JOBS_WAIT_PATH,
      'POST',
      '{"threadId":"thr_1","jobIds":["job_1"],"timeoutMs":1000}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2,
      ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","tail":true}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      3,
      ANALYTIX_RUNTIME_TASK_JOBS_RESTART_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      4,
      ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","deliveryId":"delivery_1"}'
    )
  })

  it('exposes a task job isolation review helper over the canonical runtime path', async () => {
    const response = { ok: true, status: 200, body: '{"status":"reviewed"}' }
    const runtimeRequest = vi.fn(async () => response)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.reviewTaskJobIsolation({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'review_1'
    })).resolves.toEqual(response)

    expect(runtimeRequest).toHaveBeenCalledWith(
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"review_1"}'
    )
  })

  it('exposes task job isolation reject, cleanup, and accept helpers over canonical runtime paths', async () => {
    const response = { ok: true, status: 200, body: '{"status":"rejected"}' }
    const runtimeRequest = vi.fn(async () => response)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.rejectTaskJobIsolation({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'reject_1',
      reason: 'not needed'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.cleanupTaskJobIsolation({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'cleanup_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.acceptTaskJobIsolation({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'accept_1',
      approvalId: 'approval_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.conflictReportTaskJobIsolation({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'conflict_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.repairCheckTaskJobIsolation({
      threadId: 'thr_1',
      jobId: 'job_1',
      conflictReportId: 'conflict_1',
      repairPatch: 'patch'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.repairAcceptTaskJobIsolation({
      threadId: 'thr_1',
      jobId: 'job_1',
      repairReviewId: 'repair_review_1',
      approvalId: 'approval_1'
    })).resolves.toEqual(response)

    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"reject_1","reason":"not needed"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"cleanup_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      3,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"accept_1","approvalId":"approval_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      4,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"conflict_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      5,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","conflictReportId":"conflict_1","repairPatch":"patch"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      6,
      ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","repairReviewId":"repair_review_1","approvalId":"approval_1"}'
    )
  })

  it('exposes child todo projection helpers over canonical runtime paths', async () => {
    const response = { ok: true, status: 200, body: '{"status":"proposed"}' }
    const runtimeRequest = vi.fn(async () => response)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest,
          restartRuntime: vi.fn(),
          startSse: vi.fn(),
          stopSse: vi.fn(),
          onSseEvent: vi.fn(),
          onSseEnd: vi.fn(),
          onSseError: vi.fn()
        }
      }
    })

    await expect(rendererRuntimeClient.childTodosTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.projectChildTodosTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      clientRequestId: 'project_1'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.rejectChildTodoProjectionTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      projectionId: 'projection_1',
      reason: 'parent chose not to adopt'
    })).resolves.toEqual(response)
    await expect(rendererRuntimeClient.acceptChildTodoProjectionTaskJob({
      threadId: 'thr_1',
      jobId: 'job_1',
      projectionId: 'projection_1',
      approvalId: 'approval_1',
      expectedParentTodosUpdatedAt: '2026-07-07T00:00:00Z',
      selectedChildTodoIds: ['child_1']
    })).resolves.toEqual(response)

    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","clientRequestId":"project_1"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      3,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","projectionId":"projection_1","reason":"parent chose not to adopt"}'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      4,
      ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_PATH,
      'POST',
      '{"threadId":"thr_1","jobId":"job_1","projectionId":"projection_1","approvalId":"approval_1","expectedParentTodosUpdatedAt":"2026-07-07T00:00:00Z","selectedChildTodoIds":["child_1"]}'
    )
  })

  it('passes SSE control and listener handlers through window.analytix unchanged', async () => {
    const offEvent = vi.fn()
    const offEnd = vi.fn()
    const offError = vi.fn()
    const startSse = vi.fn(async () => ({ streamId: 'stream-renderer' }))
    const ackSseEvent = vi.fn(async () => true)
    const stopSse = vi.fn(async () => true)
    const onSseEvent = vi.fn(() => offEvent)
    const onSseEnd = vi.fn(() => offEnd)
    const onSseError = vi.fn(() => offError)
    const eventHandler = vi.fn()
    const endHandler = vi.fn()
    const errorHandler = vi.fn()
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(),
          setSettings: vi.fn()
        },
        runtime: {
          runtimeRequest: vi.fn(),
          restartRuntime: vi.fn(),
          startSse,
          ackSseEvent,
          stopSse,
          onSseEvent,
          onSseEnd,
          onSseError
        }
      },
      get kun() {
        throw new Error('legacy bridge alias should not be read')
      },
      get reasonix() {
        throw new Error('legacy bridge alias should not be read')
      }
    })

    await expect(rendererRuntimeClient.startSse('thr/with space?x=1#frag', 7, 'stream-renderer')).resolves.toEqual({
      streamId: 'stream-renderer'
    })
    await expect(rendererRuntimeClient.ackSseEvent('stream-renderer', 8)).resolves.toBe(true)
    const acceptedFinalAck = {
      batchId: '7'.repeat(64),
      threadId: 'thread-case',
      turnId: 'turn-case',
      publicationCommitId: '9'.repeat(64)
    }
    await expect(rendererRuntimeClient.ackSseEvent(
      'stream-renderer', 9, acceptedFinalAck
    )).resolves.toBe(true)
    await expect(rendererRuntimeClient.stopSse('stream-renderer')).resolves.toBe(true)
    expect(rendererRuntimeClient.onSseEvent(eventHandler)).toBe(offEvent)
    expect(rendererRuntimeClient.onSseEnd(endHandler)).toBe(offEnd)
    expect(rendererRuntimeClient.onSseError(errorHandler)).toBe(offError)

    expect(startSse).toHaveBeenCalledWith('thr/with space?x=1#frag', 7, 'stream-renderer')
    expect(ackSseEvent).toHaveBeenCalledWith('stream-renderer', 8)
    expect(ackSseEvent).toHaveBeenCalledWith('stream-renderer', 9, acceptedFinalAck)
    expect(stopSse).toHaveBeenCalledWith('stream-renderer')
    expect(onSseEvent).toHaveBeenCalledWith(eventHandler)
    expect(onSseEnd).toHaveBeenCalledWith(endHandler)
    expect(onSseError).toHaveBeenCalledWith(errorHandler)
  })
})
