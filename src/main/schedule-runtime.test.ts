import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  mergeScheduleSettings,
  type AppSettingsPatch,
  type AppSettingsV1,
  type ClawImChannelV1,
  type ScheduledTaskV1
} from '../shared/app-settings'
import { ScheduleRuntime, computeScheduleNextRunAt, scheduledThreadTitle } from './schedule-runtime'

function makeTask(patch: Partial<ScheduledTaskV1> = {}): ScheduledTaskV1 {
  const schedule = {
    kind: 'manual' as const,
    everyMinutes: 60,
    timeOfDay: '09:00',
    atTime: '',
    ...patch.schedule
  }
  return {
    id: 'task-1',
    title: 'Task 1',
    enabled: true,
    prompt: 'Run the task',
    workspaceRoot: '/tmp/workspace',
    clawChannelId: '',
    model: 'auto',
    reasoningEffort: 'medium',
    mode: 'agent',
    createdAt: '2026-06-02T00:00:00.000Z',
    updatedAt: '2026-06-02T00:00:00.000Z',
    lastRunAt: '',
    nextRunAt: '',
    lastStatus: 'idle',
    lastMessage: '',
    lastThreadId: '',
    ...patch,
    schedule
  }
}

function makeClawChannel(patch: Partial<ClawImChannelV1> = {}): ClawImChannelV1 {
  return {
    id: 'channel-1',
    provider: 'feishu',
    label: 'Feishu Agent',
    enabled: true,
    model: 'deepseek-v4-flash',
    threadId: '',
    workspaceRoot: '/tmp/claw-workspace',
    agentProfile: {
      name: 'Ops Claw',
      description: '',
      identity: 'You are the operations assistant.',
      personality: '',
      userContext: '',
      replyRules: ''
    },
    conversations: [],
    createdAt: '2026-06-02T00:00:00.000Z',
    updatedAt: '2026-06-02T00:00:00.000Z',
    ...patch
  }
}

function settingsWith(
  tasks: ScheduledTaskV1[] = [],
  schedulePatch: AppSettingsPatch['schedule'] = {}
): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: {
        ...defaultAnalytixRuntimeSettings()
      },
    workspaceRoot: '/tmp/workspace',
    log: { enabled: true, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    claw: defaultClawSettings(),
    schedule: mergeScheduleSettings(defaultScheduleSettings(), {
      enabled: true,
      tasks,
      ...schedulePatch
    }),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

function createStore(initial: AppSettingsV1) {
  let current = initial
  return {
    load: vi.fn(async () => current),
    patch: vi.fn(async (partial: AppSettingsPatch) => {
      current = {
        ...current,
        schedule: mergeScheduleSettings(current.schedule, partial.schedule),
        claw: current.claw
      }
      return current
    }),
    read: () => current
  }
}

function createRuntime(initial: AppSettingsV1, runtimeRequest = vi.fn()) {
  const store = createStore(initial)
  const logError = vi.fn()
  const runtime = new ScheduleRuntime({
    store: store as never,
    runtimeRequest: runtimeRequest as never,
    logError
  })
  return { runtime, store, runtimeRequest, logError }
}

describe('ScheduleRuntime', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('computes nextRunAt for supported schedule kinds', () => {
    const from = new Date('2026-06-02T00:00:00.000Z')

    expect(computeScheduleNextRunAt(makeTask(), from)).toBe('')
    expect(computeScheduleNextRunAt(makeTask({
      schedule: { kind: 'interval', everyMinutes: 15, timeOfDay: '09:00', atTime: '' }
    }), from)).toBe('2026-06-02T00:15:00.000Z')
    expect(computeScheduleNextRunAt(makeTask({
      schedule: {
        kind: 'at',
        everyMinutes: 60,
        timeOfDay: '09:00',
        atTime: '2026-06-03T09:00:00.000+08:00'
      }
    }), from)).toBe('2026-06-03T09:00:00.000+08:00')
  })

  it('builds compact Scheduled task thread titles from task names', () => {
    expect(scheduledThreadTitle('每日A股行情盘')).toBe('[Scheduled task] 每日A股')
    expect(scheduledThreadTitle('Task 1')).toBe('[Scheduled task] Task')
    expect(scheduledThreadTitle('   ')).toBe('[Scheduled task]')
  })

  it('routes detector model work through the Go runtime with key-free intent', async () => {
    const { runtime, store, runtimeRequest } = createRuntime(settingsWith())
    vi.spyOn(runtime, 'sync').mockImplementation(() => undefined)
    const runPrompt = vi.spyOn(runtime as unknown as {
      runPrompt: (settings: AppSettingsV1, options: Record<string, unknown>) => Promise<unknown>
    }, 'runPrompt').mockResolvedValue({
      ok: true,
      threadId: 'thr_detector',
      turnId: 'turn_detector',
      text: JSON.stringify({
        shouldCreateTask: true,
        scheduleAt: '2099-06-03T09:00:00.000Z',
        reminderBody: 'ship the review',
        taskName: 'Ship review'
      }),
      message: 'Completed'
    })

    const result = await runtime.createScheduledTaskFromText('Remind me tomorrow to ship the review.', {
      workspaceRoot: '/tmp/schedule',
      modelHint: 'deepseek-v4-flash',
      mode: 'plan'
    })

    expect(result).toMatchObject({ kind: 'created', title: 'Ship review reminder' })
    expect(runPrompt).toHaveBeenCalledWith(
      expect.any(Object),
      expect.objectContaining({
        workspaceRoot: '/tmp/schedule',
        model: 'deepseek-v4-flash',
        reasoningEffort: 'medium',
        mode: 'plan',
        waitForResult: true,
        maxModelSteps: 1,
        deleteThreadAfterResult: true
      })
    )
    expect(JSON.stringify(runPrompt.mock.calls[0]?.[1])).not.toMatch(/apiKey|credentialRef|baseUrl|endpointFormat|proxyUrl/)
    expect(store.read().schedule.tasks).toHaveLength(1)
    expect(store.read().claw.tasks).toEqual([])
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('deletes an ephemeral detector thread when turn startup fails', async () => {
    const runtimeRequest = vi.fn(async (_settings, path, init) => {
      if (path === '/v1/threads') {
        return { ok: true, status: 200, body: JSON.stringify({ id: 'thr_detector_failed' }) }
      }
      if (path === '/v1/threads/thr_detector_failed/turns') {
        return { ok: false, status: 503, body: '{}' }
      }
      if (path === '/v1/threads/thr_detector_failed' && init?.method === 'DELETE') {
        return { ok: true, status: 200, body: '{}' }
      }
      return { ok: false, status: 404, body: '{}' }
    })
    const { runtime } = createRuntime(settingsWith(), runtimeRequest)
    vi.spyOn(runtime, 'sync').mockImplementation(() => undefined)

    await expect(runtime.createScheduledTaskFromText('Remind me tomorrow to ship the review.')).resolves.toEqual({
      kind: 'noop'
    })
    expect(runtimeRequest).toHaveBeenCalledWith(
      expect.any(Object),
      '/v1/threads/thr_detector_failed',
      { method: 'DELETE' }
    )
  })

  it('does not return or log raw detector errors and source prompts', async () => {
    const promptSentinel = 'Remind me tomorrow to PROMPT_SENTINEL_9F8A.'
    const errorSentinel = 'MARKER_FREE_DETECTOR_ERROR_SENTINEL'
    const { runtime, logError } = createRuntime(settingsWith())
    vi.spyOn(runtime as unknown as {
      runPrompt: (settings: AppSettingsV1, options: Record<string, unknown>) => Promise<unknown>
    }, 'runPrompt').mockRejectedValue(new Error(errorSentinel))

    await expect(runtime.createScheduledTaskFromText(promptSentinel)).resolves.toEqual({
      kind: 'error',
      message: 'Failed to create scheduled task.'
    })
    expect(JSON.stringify(logError.mock.calls)).not.toContain(promptSentinel)
    expect(JSON.stringify(logError.mock.calls)).not.toContain(errorSentinel)
  })

  it('starts a Analytix thread with a Schedule title and records running status', async () => {
    const task = makeTask({ reasoningEffort: 'max' })
    const runtimeRequest = vi.fn(async (_settings, path, init) => {
      if (path === '/v1/threads') {
        return { ok: true, status: 200, body: JSON.stringify({ id: 'thr_1' }) }
      }
      if (path === '/v1/threads/thr_1' && init?.method === 'PATCH') {
        return { ok: true, status: 200, body: '{}' }
      }
      if (path === '/v1/threads/thr_1/turns') {
        return { ok: true, status: 202, body: JSON.stringify({ turnId: 'turn_1' }) }
      }
      throw new Error(`unexpected path ${path}`)
    })
    const { runtime, store } = createRuntime(settingsWith([task]), runtimeRequest)
    ;(runtime as unknown as { monitorTaskTurn: () => void }).monitorTaskTurn = vi.fn()

    await expect(runtime.runTask(task.id)).resolves.toMatchObject({
      ok: true,
      threadId: 'thr_1',
      turnId: 'turn_1'
    })

    const createRequest = runtimeRequest.mock.calls.find(([, path, init]) =>
      path === '/v1/threads' && init?.method === 'POST'
    )?.[2]?.body
    const turnRequest = runtimeRequest.mock.calls.find(([, path]) =>
      path === '/v1/threads/thr_1/turns'
    )?.[2]?.body
    expect(JSON.parse(String(createRequest))).toMatchObject({
      title: '[Scheduled task] Task',
      workspace: '/tmp/workspace',
      model: 'deepseek-v4-flash',
      mode: 'agent',
      approvalPolicy: 'never',
      sandboxMode: 'workspace-write'
    })
    expect(JSON.parse(String(turnRequest))).toMatchObject({
      model: 'deepseek-v4-flash',
      reasoningEffort: 'max',
      approvalPolicy: 'never',
      sandboxMode: 'workspace-write',
      // Headless turn: a user_input request would hang until timeout.
      disableUserInput: true
    })
    expect(store.read().schedule.tasks[0]).toMatchObject({
      lastStatus: 'running',
      lastThreadId: 'thr_1',
      lastMessage: 'schedule_task_started'
    })
  })

  it('does not persist arbitrary runtime failure bodies as task messages', async () => {
    const sentinel = 'MARKER_FREE_RUNTIME_FAILURE_SENTINEL'
    const task = makeTask()
    const runtimeRequest = vi.fn(async () => ({
      ok: false,
      status: 503,
      body: JSON.stringify({ message: sentinel })
    }))
    const { runtime, store } = createRuntime(settingsWith([task]), runtimeRequest)

    await expect(runtime.runTask(task.id)).resolves.toEqual({
      ok: false,
      message: 'Failed to create thread.'
    })
    expect(store.read().schedule.tasks[0]).toMatchObject({
      lastStatus: 'error',
      lastMessage: 'schedule_task_failed'
    })
    expect(JSON.stringify(store.read())).not.toContain(sentinel)
  })

  it('runs selected Claw channel scheduled tasks with the Claw persona', async () => {
    const channel = makeClawChannel()
    const task = makeTask({
      clawChannelId: channel.id,
      workspaceRoot: '',
      model: channel.model
    })
    const initial = settingsWith([task])
    initial.claw.channels = [channel]
    const runtimeRequest = vi.fn(async (_settings, path, init) => {
      if (path === '/v1/threads') {
        return { ok: true, status: 200, body: JSON.stringify({ id: 'thr_claw' }) }
      }
      if (path === '/v1/threads/thr_claw' && init?.method === 'PATCH') {
        return { ok: true, status: 200, body: '{}' }
      }
      if (path === '/v1/threads/thr_claw/turns') {
        return { ok: true, status: 202, body: JSON.stringify({ turnId: 'turn_claw' }) }
      }
      throw new Error(`unexpected path ${path}`)
    })
    const { runtime } = createRuntime(initial, runtimeRequest)
    ;(runtime as unknown as { monitorTaskTurn: () => void }).monitorTaskTurn = vi.fn()

    await expect(runtime.runTask(task.id)).resolves.toMatchObject({
      ok: true,
      threadId: 'thr_claw',
      turnId: 'turn_claw'
    })

    const createRequest = runtimeRequest.mock.calls.find(([, path, init]) =>
      path === '/v1/threads' && init?.method === 'POST'
    )?.[2]?.body
    const turnRequest = runtimeRequest.mock.calls.find(([, path]) =>
      path === '/v1/threads/thr_claw/turns'
    )?.[2]?.body
    expect(JSON.parse(String(createRequest))).toMatchObject({
      workspace: '/tmp/claw-workspace',
      model: 'deepseek-v4-flash',
      approvalPolicy: 'never',
      sandboxMode: 'workspace-write'
    })
    const turnBody = JSON.parse(String(turnRequest))
    expect(turnBody).toMatchObject({
      approvalPolicy: 'never',
      sandboxMode: 'workspace-write'
    })
    expect(turnBody.prompt).toContain('[Connect Phone managed instructions]')
    expect(turnBody.prompt).toContain('[Agent name]\nOps Claw')
    expect(turnBody.prompt).toContain('Run the task')
  })

  it('reads assistant text from the real Analytix thread detail shape', async () => {
    const task = makeTask()
    const runtimeRequest = vi.fn(async (_settings, path, init) => {
      if (path === '/v1/threads') {
        return { ok: true, status: 200, body: JSON.stringify({ id: 'thr_1' }) }
      }
      if (path === '/v1/threads/thr_1' && init?.method === 'PATCH') {
        return { ok: true, status: 200, body: '{}' }
      }
      if (path === '/v1/threads/thr_1/turns') {
        return { ok: true, status: 202, body: JSON.stringify({ turnId: 'turn_1' }) }
      }
      if (path === '/v1/threads/thr_1' && init?.method === 'GET') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            id: 'thr_1',
            status: 'idle',
            turns: [
              {
                id: 'turn_1',
                status: 'completed',
                items: [{ kind: 'assistant_text', text: 'scheduled task completed' }]
              }
            ]
          })
        }
      }
      throw new Error(`unexpected path ${path}`)
    })
    const { runtime } = createRuntime(settingsWith([task]), runtimeRequest)

    const result = await (runtime as unknown as {
      runPrompt: (
        settingsArg: AppSettingsV1,
        options: {
          prompt: string
          title: string
          workspaceRoot: string
          model: string
          reasoningEffort: ScheduledTaskV1['reasoningEffort']
          mode: ScheduledTaskV1['mode']
          waitForResult: boolean
          responseTimeoutMs: number
        }
      ) => Promise<{ ok: boolean; text?: string }>
    }).runPrompt(settingsWith([task]), {
      prompt: 'hello',
      title: 'demo',
      workspaceRoot: '/tmp/workspace',
      model: 'auto',
      reasoningEffort: 'medium',
      mode: 'agent',
      waitForResult: true,
      responseTimeoutMs: 2_000
    })

    expect(result).toMatchObject({ ok: true, text: 'scheduled task completed' })
  })

  it('waits for the current scheduled turn to complete before returning final text', async () => {
    const task = makeTask()
    let getCount = 0
    const runtimeRequest = vi.fn(async (_settings, path, init) => {
      if (path === '/v1/threads') {
        return { ok: true, status: 200, body: JSON.stringify({ id: 'thr_1' }) }
      }
      if (path === '/v1/threads/thr_1' && init?.method === 'PATCH') {
        return { ok: true, status: 200, body: '{}' }
      }
      if (path === '/v1/threads/thr_1/turns') {
        return { ok: true, status: 202, body: JSON.stringify({ turnId: 'turn_current' }) }
      }
      if (path === '/v1/threads/thr_1' && init?.method === 'GET') {
        getCount += 1
        return {
          ok: true,
          status: 200,
          body: JSON.stringify(getCount === 1
            ? {
                id: 'thr_1',
                status: 'running',
                turns: [
                  {
                    id: 'turn_previous',
                    status: 'completed',
                    items: [{ kind: 'assistant_text', text: 'previous scheduled reply' }]
                  },
                  {
                    id: 'turn_current',
                    status: 'running',
                    items: [{ kind: 'assistant_text', text: 'intermediate scheduled reply' }]
                  }
                ]
              }
            : {
                id: 'thr_1',
                status: 'idle',
                turns: [
                  {
                    id: 'turn_previous',
                    status: 'completed',
                    items: [{ kind: 'assistant_text', text: 'previous scheduled reply' }]
                  },
                  {
                    id: 'turn_current',
                    status: 'completed',
                    items: [
                      { kind: 'assistant_text', text: 'intermediate scheduled reply' },
                      { kind: 'assistant_text', text: 'final scheduled reply' }
                    ]
                  }
                ]
              })
        }
      }
      throw new Error(`unexpected path ${path}`)
    })
    const { runtime } = createRuntime(settingsWith([task]), runtimeRequest)

    const result = await (runtime as unknown as {
      runPrompt: (
        settingsArg: AppSettingsV1,
        options: {
          prompt: string
          title: string
          workspaceRoot: string
          model: string
          reasoningEffort: ScheduledTaskV1['reasoningEffort']
          mode: ScheduledTaskV1['mode']
          waitForResult: boolean
          responseTimeoutMs: number
        }
      ) => Promise<{ ok: boolean; text?: string }>
    }).runPrompt(settingsWith([task]), {
      prompt: 'hello',
      title: 'demo',
      workspaceRoot: '/tmp/workspace',
      model: 'auto',
      reasoningEffort: 'medium',
      mode: 'agent',
      waitForResult: true,
      responseTimeoutMs: 2_500
    })

    expect(result).toMatchObject({ ok: true, text: 'final scheduled reply' })
    expect(getCount).toBe(2)
  })

  it('does not return historical scheduled text when the current turn fails', async () => {
    const task = makeTask()
    const runtimeRequest = vi.fn(async (_settings, path, init) => {
      if (path === '/v1/threads') {
        return { ok: true, status: 200, body: JSON.stringify({ id: 'thr_1' }) }
      }
      if (path === '/v1/threads/thr_1' && init?.method === 'PATCH') {
        return { ok: true, status: 200, body: '{}' }
      }
      if (path === '/v1/threads/thr_1/turns') {
        return { ok: true, status: 202, body: JSON.stringify({ turnId: 'turn_current' }) }
      }
      if (path === '/v1/threads/thr_1' && init?.method === 'GET') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            id: 'thr_1',
            status: 'idle',
            turns: [
              {
                id: 'turn_previous',
                status: 'completed',
                items: [{ kind: 'assistant_text', text: 'previous scheduled reply' }]
              },
              {
                id: 'turn_current',
                status: 'failed',
                error: 'MARKER_FREE_TURN_ERROR_SENTINEL',
                items: []
              }
            ]
          })
        }
      }
      throw new Error(`unexpected path ${path}`)
    })
    const { runtime } = createRuntime(settingsWith([task]), runtimeRequest)

    await expect((runtime as unknown as {
      runPrompt: (
        settingsArg: AppSettingsV1,
        options: {
          prompt: string
          title: string
          workspaceRoot: string
          model: string
          reasoningEffort: ScheduledTaskV1['reasoningEffort']
          mode: ScheduledTaskV1['mode']
          waitForResult: boolean
          responseTimeoutMs: number
        }
      ) => Promise<{ ok: boolean; text?: string }>
    }).runPrompt(settingsWith([task]), {
      prompt: 'hello',
      title: 'demo',
      workspaceRoot: '/tmp/workspace',
      model: 'auto',
      reasoningEffort: 'medium',
      mode: 'agent',
      waitForResult: true,
      responseTimeoutMs: 2_000
    })).rejects.toThrow('Agent turn failed.')
  })

  it('disables one-time tasks after monitored completion', async () => {
    const task = makeTask({
      lastStatus: 'running',
      schedule: {
        kind: 'at',
        everyMinutes: 60,
        timeOfDay: '09:00',
        atTime: '2099-06-03T09:00:00.000Z'
      }
    })
    const { runtime, store } = createRuntime(settingsWith([task]))
    ;(runtime as unknown as {
      waitForAssistantText: () => Promise<string>
    }).waitForAssistantText = vi.fn(async () => 'MARKER_FREE_ASSISTANT_RESULT_SENTINEL')

    await (runtime as unknown as {
      monitorTaskTurn: (taskId: string, threadId: string, turnId: string) => Promise<void>
    }).monitorTaskTurn(task.id, 'thr_1', 'turn_1')

    expect(store.read().schedule.tasks[0]).toMatchObject({
      enabled: false,
      nextRunAt: '',
      lastStatus: 'success',
      lastMessage: 'schedule_task_completed',
      lastThreadId: 'thr_1'
    })
    expect(JSON.stringify(store.read())).not.toContain('MARKER_FREE_ASSISTANT_RESULT_SENTINEL')
  })

  it('does not persist or log arbitrary monitored turn errors', async () => {
    const sentinel = 'MARKER_FREE_CHILD_ERROR_SENTINEL'
    const task = makeTask({ lastStatus: 'running' })
    const { runtime, store, logError } = createRuntime(settingsWith([task]))
    ;(runtime as unknown as {
      waitForAssistantText: () => Promise<string>
    }).waitForAssistantText = vi.fn(async () => {
      throw new Error(sentinel)
    })

    await (runtime as unknown as {
      monitorTaskTurn: (taskId: string, threadId: string, turnId: string) => Promise<void>
    }).monitorTaskTurn(task.id, 'thr_1', 'turn_1')

    expect(store.read().schedule.tasks[0]).toMatchObject({
      lastStatus: 'error',
      lastMessage: 'schedule_task_failed'
    })
    expect(JSON.stringify(store.read())).not.toContain(sentinel)
    expect(JSON.stringify(logError.mock.calls)).not.toContain(sentinel)
  })

  it('does not auto-run manual tasks during tick', async () => {
    const task = makeTask({
      schedule: { kind: 'manual', everyMinutes: 60, timeOfDay: '09:00', atTime: '' },
      nextRunAt: '2026-06-02T00:00:00.000Z'
    })
    const runtimeRequest = vi.fn()
    const { runtime } = createRuntime(settingsWith([task]), runtimeRequest)

    await (runtime as unknown as { tick: () => Promise<void> }).tick()

    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('marks interrupted running tasks as errors during next-run recovery', async () => {
    const task = makeTask({
      lastStatus: 'running',
      schedule: { kind: 'interval', everyMinutes: 10, timeOfDay: '09:00', atTime: '' }
    })
    const initial = settingsWith([task])
    const { runtime, store } = createRuntime(initial)

    await (runtime as unknown as {
      ensureNextRuns: (settings: AppSettingsV1) => Promise<void>
    }).ensureNextRuns(initial)

    expect(store.read().schedule.tasks[0].lastStatus).toBe('error')
    expect(store.read().schedule.tasks[0].lastMessage).toBe('schedule_task_interrupted')
    expect(Date.parse(store.read().schedule.tasks[0].nextRunAt)).toBeGreaterThan(0)
  })

  it('uses the power save blocker only for enabled automatic schedules', () => {
    const started = new Set<number>()
    const powerSaveBlocker = {
      start: vi.fn(() => {
        started.add(1)
        return 1
      }),
      stop: vi.fn((id: number) => {
        started.delete(id)
      }),
      isStarted: vi.fn((id: number) => started.has(id))
    }
    const runtime = new ScheduleRuntime({
      store: createStore(settingsWith()) as never,
      runtimeRequest: vi.fn() as never,
      logError: vi.fn(),
      powerSaveBlocker
    })
    const scheduled = settingsWith([
      makeTask({ schedule: { kind: 'daily', everyMinutes: 60, timeOfDay: '09:00', atTime: '' } })
    ], { keepAwake: true })

    ;(runtime as unknown as { syncPowerSaveBlocker: (settings: AppSettingsV1) => void })
      .syncPowerSaveBlocker(scheduled)
    expect(powerSaveBlocker.start).toHaveBeenCalledWith('prevent-app-suspension')

    ;(runtime as unknown as { syncPowerSaveBlocker: (settings: AppSettingsV1) => void })
      .syncPowerSaveBlocker({ ...scheduled, schedule: { ...scheduled.schedule, keepAwake: false } })
    expect(powerSaveBlocker.stop).toHaveBeenCalledWith(1)
  })
})
