import { Readable } from 'node:stream'
import { canonicalScheduleUTC, preserveScheduleTaskHostDelivery, requestScheduleAdmission, scheduleAdmissionRequestSchemaV1, type StoredScheduledTaskV1, type ScheduleAdmissionRequestV1, type ScheduleRuntimeDeps } from './schedule-runtime-helpers'
import { buildScheduleTaskUpdate, scheduleTaskEditable } from '../shared/schedule-task-mutation'
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
    loadFresh: vi.fn(async () => current),
    mutateScheduleTasks: vi.fn(async <T>(mutator: (latest: AppSettingsV1) => { tasks: ScheduledTaskV1[]; enabled?: boolean; value: T }) => {
      const result = mutator(current)
      const changed = result.tasks !== current.schedule.tasks || (result.enabled !== undefined && result.enabled !== current.schedule.enabled)
      current = { ...current, schedule: mergeScheduleSettings(current.schedule, {
        tasks: result.tasks, ...(result.enabled === undefined ? {} : { enabled: result.enabled })
      }) }
      current.schedule.tasks = preserveScheduleTaskHostDelivery(current.schedule.tasks, result.tasks)
      return { settings: current, value: result.value, changed }
    }),
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


describe('schedule task configuration transactions', () => {
  it.each(['update', 'delete'] as const)('returns the committed normalized create receipt before a later %s', async operation => {
    const { runtime, store } = createRuntime(settingsWith())
    vi.spyOn(runtime, 'sync').mockImplementation(() => {})
    let committed!: ScheduledTaskV1
    const privateRuntime = runtime as unknown as { ensureNextRuns: () => Promise<void> }
    vi.spyOn(privateRuntime, 'ensureNextRuns').mockImplementation(async () => {
      committed = structuredClone(store.read().schedule.tasks[0])
      await store.mutateScheduleTasks(latest => ({ tasks: operation === 'delete' ? [] : latest.schedule.tasks.map(task => ({
        ...task, title: 'Later catalog title', prompt: 'Later catalog prompt', lastThreadId: 'later-run-thread'
      })), value: undefined }))
    })
    const readback = vi.spyOn(runtime, 'listTasks')
    const receipt = await runtime.createTaskFromInput({ title: '  Created title  ', prompt: 'Created prompt',
      schedule: { kind: 'interval', everyMinutes: 30 } })
    expect(receipt).toEqual(committed)
    expect(receipt).toMatchObject({ title: 'Created title', prompt: 'Created prompt', lastThreadId: '', schedule: { kind: 'interval', everyMinutes: 30 } })
    expect(receipt.id).toBeTruthy()
    expect(readback).not.toHaveBeenCalled()
    expect(store.read().schedule.tasks).toEqual(operation === 'delete' ? [] : [expect.objectContaining({ title: 'Later catalog title' })])
  })
  it('merges different editable fields but rejects a second edit of the same field', async () => {
    const baseline = makeTask()
    const { runtime, store } = createRuntime(settingsWith([baseline]))
    vi.spyOn(runtime, 'sync').mockImplementation(() => {})
    const rename = buildScheduleTaskUpdate(baseline, { ...scheduleTaskEditable(baseline), title: 'Renamed' })
    const reprompt = buildScheduleTaskUpdate(baseline, { ...scheduleTaskEditable(baseline), prompt: 'Changed prompt' })
    expect((await runtime.updateTaskConfiguration(rename)).kind).toBe('success')
    expect((await runtime.updateTaskConfiguration(reprompt)).kind).toBe('success')
    expect((await runtime.updateTaskConfiguration({ ...rename, patch: { title: 'Stale title' } })).kind).toBe('conflict')
    expect(store.read().schedule.tasks[0]).toMatchObject({ title: 'Renamed', prompt: 'Changed prompt' })
  })

  it('preserves latest one-time completion and next run fields during a title-only edit', async () => {
    const baseline = makeTask()
    const { runtime, store } = createRuntime(settingsWith([baseline]))
    vi.spyOn(runtime, 'sync').mockImplementation(() => {})
    await store.mutateScheduleTasks(latest => ({ tasks: latest.schedule.tasks.map(task => ({
      ...task, enabled: false, lastStatus: 'success', lastMessage: 'schedule_task_completed',
      nextRunAt: '2027-01-01T00:00:00.000Z', lastThreadId: 'actual-thread', updatedAt: 'runtime-update'
    })), value: undefined }))
    const result = await runtime.updateTaskConfiguration(buildScheduleTaskUpdate(baseline, { ...scheduleTaskEditable(baseline), title: 'After completion' }))
    expect(result).toMatchObject({ kind: 'success', task: { title: 'After completion', enabled: false,
      lastStatus: 'success', lastThreadId: 'actual-thread', nextRunAt: '2027-01-01T00:00:00.000Z' } })
  })

  it('does not resurrect a deleted task through late background updates or next-run maintenance', async () => {
    const task = settingsWith([makeTask()]).schedule.tasks[0]
    const { runtime, store } = createRuntime(settingsWith([task]))
    vi.spyOn(runtime, 'sync').mockImplementation(() => {})
    await runtime.deleteTask({ taskId: task.id, expected: scheduleTaskEditable(task) })
    const privateRuntime = runtime as unknown as {
      updateTask: (id: string, updater: (task: ScheduledTaskV1) => ScheduledTaskV1) => Promise<AppSettingsV1>
      ensureNextRuns: () => Promise<void>
    }
    await privateRuntime.updateTask(task.id, current => ({ ...current, lastStatus: 'success' }))
    await privateRuntime.ensureNextRuns()
    expect(store.read().schedule.tasks).toEqual([])
    expect((await runtime.updateTaskConfiguration(buildScheduleTaskUpdate(task, { ...scheduleTaskEditable(task), title: 'Late' }))).kind).toBe('not_found')
  })
})


it('commits delayed text detection against the latest task catalog', async () => {
  const existing = makeTask()
  const { runtime, store } = createRuntime(settingsWith([existing]))
  vi.spyOn(runtime, 'sync').mockImplementation(() => {})
  let finish!: (value: unknown) => void
  let entered!: () => void
  const started = new Promise<void>(resolve => { entered = resolve })
  const detection = new Promise<unknown>(resolve => { finish = resolve })
  vi.spyOn(runtime as unknown as { runPrompt: (settings: AppSettingsV1, options: Record<string, unknown>) => Promise<unknown> }, 'runPrompt')
    .mockImplementation(async () => { entered(); return detection })
  const creation = runtime.createScheduledTaskFromText('Remind me tomorrow to ship the review.')
  await started
  await runtime.updateTaskConfiguration(buildScheduleTaskUpdate(existing, { ...scheduleTaskEditable(existing), title: 'Edited while detecting' }))
  finish({ ok: true, text: JSON.stringify({ shouldCreateTask: true, scheduleAt: '2099-06-03T09:00:00.000Z', reminderBody: 'ship the review', taskName: 'Ship review' }) })
  expect(await creation).toMatchObject({ kind: 'created' })
  expect(store.read().schedule.tasks).toHaveLength(2)
  expect(store.read().schedule.tasks.find(task => task.id === existing.id)?.title).toBe('Edited while detecting')
})

it.each([false, true])('does not resurrect a deleted task after a late monitor result (failure=%s)', async failure => {
  const task = settingsWith([makeTask()]).schedule.tasks[0]
  const { runtime, store } = createRuntime(settingsWith([task]))
  vi.spyOn(runtime, 'sync').mockImplementation(() => {})
  let complete!: () => void
  let reject!: (error: Error) => void
  const wait = new Promise<void>((resolve, fail) => { complete = resolve; reject = fail })
  const privateRuntime = runtime as unknown as {
    waitForAssistantText: (...args: unknown[]) => Promise<void>
    monitorTaskTurn: (taskId: string, threadId: string, turnId: string) => Promise<void>
  }
  vi.spyOn(privateRuntime, 'waitForAssistantText').mockReturnValue(wait)
  const pending = privateRuntime.monitorTaskTurn(task.id, 'actual-thread', 'actual-turn')
  await Promise.resolve()
  await runtime.deleteTask({ taskId: task.id, expected: scheduleTaskEditable(task) })
  if (failure) reject(new Error('Synthetic monitor failure')); else complete()
  await pending
  expect(store.read().schedule.tasks).toEqual([])
})

describe('Schedule actual saved delivery pages', () => {
  it('paginates saved ACK identities in Host append order and does not infer legacy origin/history from lastThreadId', async () => {
    const origin = { threadId: 'original-thread', turnId: 'original-turn', toolCallId: 'call_host_' + 'a'.repeat(64), grantId: 'historical-grant' }
    const records = Array.from({ length: 23 }, (_, index) => ({ attemptId: `attempt-${index}`, scheduledAt: '2026-10-11T10:00:00.000Z', deliveredAt: '2026-10-11T10:00:01.000Z', messageId: `message-${index}`, prompt: `SAVED_${index}` }))
    const settings = settingsWith([makeTask({ id: 'native', lastThreadId: 'last-run' }), makeTask({ id: 'legacy', lastThreadId: 'legacy-last-run' })])
    Object.assign(settings.schedule.tasks[0], { hostDelivery: { schemaVersion: 1, origin, records } })
    const { runtime } = createRuntime(settings)
    const first = await runtime.readTaskHistory({ taskId: 'native', limit: 20 })
    expect(first.kind).toBe('success')
    if (first.kind !== 'success') throw new Error('Expected page')
    expect(first.records.map(record => record.attemptId)).toEqual(records.slice(3).reverse().map(record => record.attemptId))
    records.push({ ...records[0], attemptId: 'new-ack', messageId: 'new-message' })
    const older = await runtime.readTaskHistory({ taskId: 'native', limit: 20, before: first.nextBefore })
    expect(older).toMatchObject({ kind: 'success', records: [...records.slice(0, 3)].reverse() })
    expect(await runtime.readTaskHistory({ taskId: 'legacy', limit: 20 })).toEqual({ kind: 'success', records: [], deliveryKind: 'legacy-headless' })
    expect(await runtime.readTaskHistory({ taskId: 'native', limit: 20, before: 'foreign-task-cursor' })).toMatchObject({ kind: 'conflict' })
    expect((await runtime.listTasks())[0]).toMatchObject({ originalThreadId: 'original-thread', lastThreadId: 'last-run' })
    expect(JSON.stringify(await runtime.listTasks())).not.toContain('historical-grant')
  })
})


describe('Schedule origin admission and frozen recovery', () => {
  const origin = { threadId: 'original-thread', turnId: 'original-turn', toolCallId: 'call_host_' + 'a'.repeat(64), grantId: 'historical-grant' }
  const receiptFor = (request: ScheduleAdmissionRequestV1, delivery: 'start' | 'steer' = 'start') => ({
    taskId: request.taskId, attemptId: request.attemptId, scheduledAt: request.scheduledAt,
    threadId: origin.threadId, turnId: 'actual-turn', messageId: 'actual-message',
    delivery, deliveredAt: '2026-10-12T11:00:01.123456789Z'
  })
  const native = () => Object.assign(makeTask({ id: 'native', prompt: '  FROZEN_PROMPT 😀  ', model: 'selected-model', providerId: 'selected-provider' }),
    { hostDelivery: { schemaVersion: 1 as const, origin, records: [] } })
  const setup = (localDisplayRequest: NonNullable<ScheduleRuntimeDeps['localDisplayRequest']>, initial?: AppSettingsV1) => {
    const settings = initial ?? settingsWith([], { enabled: false })
    if (!initial) settings.schedule.tasks = [native()]
    const store = createStore(settings), runtimeRequest = vi.fn(), runtime = new ScheduleRuntime({ store: store as never,
      runtimeRequest, localDisplayRequest, logError: vi.fn() })
    vi.spyOn(runtime, 'sync').mockImplementation(() => {})
    return { runtime, store, runtimeRequest }
  }
  it.each(['start', 'steer'] as const)('persists before submit and records only a real typed %s ACK, without creating or patching a thread', async delivery => {
    let read!: () => AppSettingsV1
    const local = vi.fn(async (_settings, path: string, body: string) => {
      const request = JSON.parse(body) as ScheduleAdmissionRequestV1
      expect(path).toBe('/v1/local-display/schedule-admission')
      expect((read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery?.pending).toEqual(request)
      return { ok: true, status: 200, body: JSON.stringify({ schemaVersion: 1, status: 'accepted', receipt: receiptFor(request, delivery) }) }
    })
    const { runtime, store, runtimeRequest } = setup(local); read = store.read
    const preferences = structuredClone(store.read().runtime)
    expect(await runtime.runTask('native')).toMatchObject({ ok: true, threadId: 'original-thread', turnId: 'actual-turn' })
    expect(runtimeRequest).not.toHaveBeenCalled()
    const request = JSON.parse(local.mock.calls[0][2])
    expect(request).toMatchObject({ kind: 'submit', origin, prompt: '  FROZEN_PROMPT 😀  ', execution: { model: 'selected-model', providerId: 'selected-provider', mode: 'agent' } })
    expect(request.scheduledAt).not.toMatch(/\.000Z$/)
    const saved = store.read().schedule.tasks[0] as StoredScheduledTaskV1
    expect(saved.hostDelivery?.pending).toBeUndefined()
    expect(saved.hostDelivery?.records).toEqual([{ attemptId: request.attemptId, scheduledAt: request.scheduledAt,
      deliveredAt: '2026-10-12T11:00:01.123456789Z', messageId: 'actual-message', prompt: request.prompt }])
    expect(saved.lastMessage).toBe('schedule_task_accepted')
    expect(store.read().runtime).toEqual(preferences)
  })
  it('queries the same persisted identity after restart and edits, retaining immutable prompt and selection after an unknown submit', async () => {
    const first = vi.fn(async () => { throw new Error('Synthetic lost ACK') })
    const { runtime, store } = setup(first)
    expect(await runtime.runTask('native')).toMatchObject({ ok: false })
    const frozen = (store.read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery!.pending!
    expect((await runtime.readTaskHistory({ taskId: 'native', limit: 20 }))).toMatchObject({ records: [] })
    const before = store.read().schedule.tasks[0]
    await runtime.updateTaskConfiguration(buildScheduleTaskUpdate(before, { ...scheduleTaskEditable(before), prompt: 'CURRENT_PROMPT', enabled: false, model: 'current-model' }))
    const query = vi.fn(async (_settings, _path, body: string) => ({ ok: true, status: 200,
      body: JSON.stringify({ schemaVersion: 1, status: 'accepted', receipt: receiptFor(JSON.parse(body)) }) }))
    const restarted = new ScheduleRuntime({ store: store as never, runtimeRequest: vi.fn(), localDisplayRequest: query, logError: vi.fn() })
    expect(await restarted.runTask('native')).toMatchObject({ ok: true })
    expect(JSON.parse(query.mock.calls[0][2])).toEqual({ ...frozen, kind: 'query' })
    expect((store.read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery?.records[0].prompt).toBe(frozen.prompt)
    expect(store.read().schedule.tasks[0].prompt).toBe('CURRENT_PROMPT')
    expect(store.read().schedule.tasks[0].enabled).toBe(false)
  })
  it('retains pending on 503 and query not_found, and never submits a fresh attempt automatically', async () => {
    const local = vi.fn().mockResolvedValueOnce({ ok: false, status: 503, body: JSON.stringify({ schemaVersion: 1, status: 'unknown', code: 'schedule_admission_unavailable' }) })
      .mockResolvedValue({ ok: true, status: 200, body: JSON.stringify({ schemaVersion: 1, status: 'not_found' }) })
    const { runtime, store } = setup(local)
    await runtime.runTask('native'); await runtime.runTask('native'); await runtime.runTask('native')
    const requests = local.mock.calls.map(call => JSON.parse(call[2]))
    expect(requests.map(item => item.kind)).toEqual(['submit', 'query', 'query'])
    expect(new Set(requests.map(item => item.attemptId)).size).toBe(1)
    expect((store.read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery?.records).toEqual([])
    expect(store.read().schedule.tasks[0].lastRunAt).toBe('')
  })
  it.each([['rejected', 409, 'schedule_admission_unavailable'], ['conflict', 409, 'schedule_admission_conflict'], ['invalid', 400, undefined]] as const)('honors typed non-2xx %s without creating a History entry', async (status, http, code) => {
    const local = vi.fn(async () => ({ ok: false, status: http, body: JSON.stringify({ schemaVersion: 1, status, ...(code ? { code } : {}) }) }))
    const { runtime, store } = setup(local)
    expect(await runtime.runTask('native')).toMatchObject({ ok: false })
    const saved = store.read().schedule.tasks[0] as StoredScheduledTaskV1
    expect(saved.hostDelivery?.pending).toBeUndefined(); expect(saved.hostDelivery?.records).toEqual([])
    expect(saved.lastStatus).toBe('error'); expect(saved.lastRunAt).toBe('')
  })
  it('does not resurrect a deleted task on late acceptance', async () => {
    let respond!: (value: { ok: boolean; status: number; body: string }) => void
    const local = vi.fn((_settings: AppSettingsV1, _path: string, _body: string) => new Promise<{ ok: boolean; status: number; body: string }>(resolve => { respond = resolve }))
    const { runtime, store } = setup(local), pending = runtime.runTask('native')
    await vi.waitFor(() => expect(local).toHaveBeenCalledTimes(1))
    const request = JSON.parse(local.mock.calls[0][2])
    await runtime.deleteTask({ taskId: 'native', expected: scheduleTaskEditable(store.read().schedule.tasks[0]) })
    respond({ ok: true, status: 200, body: JSON.stringify({ schemaVersion: 1, status: 'accepted', receipt: receiptFor(request) }) })
    expect(await pending).toMatchObject({ ok: false }); expect(store.read().schedule.tasks).toEqual([])
  })
  it('treats mismatched or malformed receipts as unknown and preserves the frozen attempt', async () => {
    const local = vi.fn(async (_settings, _path, body: string) => ({ ok: true, status: 200,
      body: JSON.stringify({ schemaVersion: 1, status: 'accepted', receipt: { ...receiptFor(JSON.parse(body)), threadId: 'foreign-thread' } }) }))
    const { runtime, store } = setup(local)
    expect(await runtime.runTask('native')).toMatchObject({ ok: false })
    expect((store.read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery?.pending).toBeDefined()
    expect((store.read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery?.records).toEqual([])
  })
  it('recovers a pending ACK from the existing timer even after scheduling is disabled, without a new submit', async () => {
    const local = vi.fn(async (_settings: AppSettingsV1, _path: string, body: string) => {
      const request = JSON.parse(body) as ScheduleAdmissionRequestV1
      return request.kind === 'submit' ? { ok: false, status: 503, body: JSON.stringify({ schemaVersion: 1, status: 'unknown', code: 'schedule_admission_unavailable' }) }
        : { ok: true, status: 200, body: JSON.stringify({ schemaVersion: 1, status: 'accepted', receipt: receiptFor(request) }) }
    })
    const { runtime, store } = setup(local)
    await runtime.runTask('native')
    await (runtime as unknown as { tick: () => Promise<void> }).tick()
    await vi.waitFor(() => expect((store.read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery?.records).toHaveLength(1))
    const requests = local.mock.calls.map(call => JSON.parse(call[2]))
    expect(requests.map(item => item.kind)).toEqual(['submit', 'query'])
    expect(requests[1]).toEqual({ ...requests[0], kind: 'query' })
    expect(store.read().schedule.enabled).toBe(false)
  })
  it('freezes the real due instant for an automatic delivery and advances only after acceptance', async () => {
    const settings = settingsWith([], { enabled: true })
    settings.schedule.tasks = [{ ...native(), nextRunAt: '2026-10-11T10:00:00.000Z',
      schedule: { kind: 'interval', everyMinutes: 60, timeOfDay: '09:00', atTime: '' } }]
    const local = vi.fn(async (_settings: AppSettingsV1, _path: string, body: string) => ({ ok: true, status: 200,
      body: JSON.stringify({ schemaVersion: 1, status: 'accepted', receipt: receiptFor(JSON.parse(body)) }) }))
    const { runtime, store } = setup(local, settings)
    expect(await (runtime as unknown as { runTaskInternal: (task: ScheduledTaskV1, scheduled: boolean) => Promise<unknown> }).runTaskInternal(settings.schedule.tasks[0], true)).toMatchObject({ ok: true })
    expect(JSON.parse(local.mock.calls[0][2]).scheduledAt).toBe('2026-10-11T10:00:00Z')
    expect(Date.parse(store.read().schedule.tasks[0].nextRunAt)).toBeGreaterThan(Date.now())
  })
  it('validates exact UTC and UTF-8 request bounds before private transport', async () => {
    const request = { schemaVersion: 1, kind: 'submit', taskId: 'native', origin, attemptId: '00000000-0000-4000-8000-000000000000', scheduledAt: '2026-10-12T10:00:00Z', prompt: '😀', execution: {} } as const
    expect(canonicalScheduleUTC('2026-10-12T10:00:00.000Z')).toBe(request.scheduledAt)
    expect(canonicalScheduleUTC('2026-10-12T10:00:00.123400000Z')).toBe('2026-10-12T10:00:00.1234Z')
    expect(scheduleAdmissionRequestSchemaV1.safeParse(request).success).toBe(true)
    for (const patch of [{ origin: { ...origin, toolCallId: 'invented' } }, { prompt: '\ud800' }, { scheduledAt: '2026-10-12T10:00:00.000Z' }, { execution: { model: '字'.repeat(86) } }, { schemaVersion: 2 }]) {
      expect(scheduleAdmissionRequestSchemaV1.safeParse({ ...request, ...patch }).success).toBe(false)
    }
    const localDisplayRequest = vi.fn(async () => ({ ok: false, status: 409, body: JSON.stringify({ schemaVersion: 1, status: 'conflict', code: 'schedule_admission_conflict' }) }))
    expect(await requestScheduleAdmission({ localDisplayRequest } as never, settingsWith(), request)).toMatchObject({ status: 'conflict' })
    expect(localDisplayRequest).toHaveBeenCalledTimes(1)
  })
})


describe('Main authenticated native Schedule creation', () => {
  const origin = { threadId: 'original-thread', turnId: 'original-turn', toolCallId: 'call_host_' + 'a'.repeat(64), grantId: 'historical-grant' }
  const input = { title: 'Native', prompt: 'Create prompt', schedule: { kind: 'manual' } }
  async function request(secret: string, headers: Record<string, string>, body: unknown) {
    const { runtime, store } = createRuntime(settingsWith([], { enabled: false, internal: { port: 8787, secret } }))
    vi.spyOn(runtime, 'sync').mockImplementation(() => {})
    const req = Object.assign(Readable.from([Buffer.from(JSON.stringify(body))]), { method: 'POST', url: '/schedule/internal/create', headers })
    let status = 0, response = ''
    const res = { writeHead: (value: number) => { status = value }, end: (value: string) => { response = value } }
    await (runtime as unknown as { handleInternalRequest: (req: unknown, res: unknown) => Promise<void> }).handleInternalRequest(req, res)
    return { status, response: JSON.parse(response), store }
  }
  it('atomically persists the four exact Host identities under current helper Bearer and keeps receipt origin-free', async () => {
    const result = await request('helper-secret', { authorization: 'Bearer helper-secret', 'x-analytix-schedule-origin': 'typed-v1' }, { input, origin })
    expect(result.status).toBe(200)
    expect((result.store.read().schedule.tasks[0] as StoredScheduledTaskV1).hostDelivery).toEqual({ schemaVersion: 1, origin, records: [] })
    expect(JSON.stringify(result.response)).not.toContain('historical-grant')
  })
  it.each([
    ['', { 'x-analytix-schedule-origin': 'typed-v1' }, { input, origin }, 401],
    ['helper-secret', { authorization: 'Bearer other', 'x-analytix-schedule-origin': 'typed-v1' }, { input, origin }, 401],
    ['helper-secret', { authorization: 'Bearer helper-secret' }, { input, origin }, 400],
    ['helper-secret', { authorization: 'Bearer helper-secret', 'x-analytix-schedule-origin': 'typed-v1' }, { input, origin: null }, 400],
    ['helper-secret', { authorization: 'Bearer helper-secret' }, { input: { ...input, origin } }, 400]
  ])('rejects unauthenticated or mixed origin input before a task write', async (secret, headers, body, expected) => {
    const result = await request(secret as string, headers as Record<string, string>, body)
    expect(result.status).toBe(expected); expect(result.store.read().schedule.tasks).toEqual([])
  })
})
