import { afterEach, describe, expect, it, vi } from 'vitest'
import { GoalResumeCoordinator } from '../src/shared/goal-resume-coordinator.js'

describe('GoalResumeCoordinator console projection', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps startup failure fallback value-free and returns false', async () => {
    const threadSentinel = 'thread-/private/analytix-rc-k-s02/STARTUP_THREAD'
    const errorSentinel = 'error-/private/analytix-rc-k-s02/STARTUP_ERROR'
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const coordinator = new GoalResumeCoordinator({
      launch: vi.fn(),
      getActiveGoalKey: async () => {
        throw new Error(errorSentinel)
      },
      isThreadBusy: vi.fn()
    })

    await expect(coordinator.resumeInterrupted(threadSentinel)).resolves.toBe(false)

    expect(warn).toHaveBeenCalledTimes(1)
    expect(warn).toHaveBeenCalledWith(
      '[analytix] event=ANALYTIX_GOAL_RESUME_STARTUP_FAILED'
    )
    const serializedWarnings = JSON.stringify(warn.mock.calls)
    expect(serializedWarnings).not.toContain(threadSentinel)
    expect(serializedWarnings).not.toContain(errorSentinel)
    coordinator.shutdown()
  })

  it('keeps scheduled launch failure fallback value-free after attempting the launch', async () => {
    const threadSentinel = 'thread-/private/analytix-rc-k-s02/LAUNCH_THREAD'
    const goalSentinel = 'goal-/private/analytix-rc-k-s02/LAUNCH_GOAL'
    const errorSentinel = 'error-/private/analytix-rc-k-s02/LAUNCH_ERROR'
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const launch = vi.fn(async () => {
      throw new Error(errorSentinel)
    })
    let fire!: () => void
    let scheduledDelayMs: number | undefined
    const coordinator = new GoalResumeCoordinator({
      launch,
      getActiveGoalKey: async () => goalSentinel,
      isThreadBusy: async () => false,
      setTimer: (fn, delayMs) => {
        fire = fn
        scheduledDelayMs = delayMs
        return { cancel: vi.fn() }
      }
    })

    expect(coordinator.noteGoalTurnFailed({
      threadId: threadSentinel,
      goalKey: goalSentinel,
      madeProgress: false
    })).toBe('scheduled')
    expect(scheduledDelayMs).toBe(2_000)

    fire()
    await vi.waitFor(() => expect(warn).toHaveBeenCalledTimes(1))

    expect(launch).toHaveBeenCalledTimes(1)
    expect(launch).toHaveBeenCalledWith(threadSentinel)
    expect(warn).toHaveBeenCalledWith(
      '[analytix] event=ANALYTIX_GOAL_RESUME_LAUNCH_FAILED'
    )
    const serializedWarnings = JSON.stringify(warn.mock.calls)
    expect(serializedWarnings).not.toContain(threadSentinel)
    expect(serializedWarnings).not.toContain(goalSentinel)
    expect(serializedWarnings).not.toContain(errorSentinel)
    coordinator.shutdown()
  })
})
