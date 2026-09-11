/**
 * Goal auto-resume coordinator.
 *
 * Goal mode runs as a single long-lived turn that self-continues within its own
 * loop. When that turn ends abnormally while the goal is still active, this
 * coordinator owns the cross-turn resume policy:
 *
 * - Relaunch after a failed goal turn and after a runtime restart strands an
 *   active goal.
 * - Bound consecutive no-progress failures with exponential backoff.
 * - Leave domain effects injected so the policy remains unit-testable.
 */

export type GoalResumeTimer = { cancel: () => void }

export type GoalResumeCoordinatorDeps = {
  launch: (threadId: string) => Promise<void>
  getActiveGoalKey: (threadId: string) => Promise<string | null>
  isThreadBusy: (threadId: string) => Promise<boolean>
  setTimer?: (fn: () => void, delayMs: number) => GoalResumeTimer
  log?: (message: string) => void
  maxNoProgressAttempts?: number
  baseDelayMs?: number
  maxDelayMs?: number
}

export const DEFAULT_MAX_GOAL_RESUME_NO_PROGRESS_ATTEMPTS = 5
export const DEFAULT_GOAL_RESUME_BASE_DELAY_MS = 2_000
export const DEFAULT_GOAL_RESUME_MAX_DELAY_MS = 60_000

type ThreadResumeState = {
  goalKey: string
  attempts: number
  timer?: GoalResumeTimer
}

type GoalResumeConsoleEventCode =
  | 'ANALYTIX_GOAL_RESUME_STARTUP_FAILED'
  | 'ANALYTIX_GOAL_RESUME_LAUNCH_FAILED'

function defaultSetTimer(fn: () => void, delayMs: number): GoalResumeTimer {
  const handle = setTimeout(fn, delayMs)
  if (typeof (handle as { unref?: () => void }).unref === 'function') {
    ;(handle as { unref: () => void }).unref()
  }
  return { cancel: () => clearTimeout(handle) }
}

export class GoalResumeCoordinator {
  private readonly deps: GoalResumeCoordinatorDeps
  private readonly setTimer: (fn: () => void, delayMs: number) => GoalResumeTimer
  private readonly maxNoProgressAttempts: number
  private readonly baseDelayMs: number
  private readonly maxDelayMs: number
  private readonly state = new Map<string, ThreadResumeState>()
  private shuttingDown = false

  constructor(deps: GoalResumeCoordinatorDeps) {
    this.deps = deps
    this.setTimer = deps.setTimer ?? defaultSetTimer
    this.maxNoProgressAttempts =
      deps.maxNoProgressAttempts ?? DEFAULT_MAX_GOAL_RESUME_NO_PROGRESS_ATTEMPTS
    this.baseDelayMs = deps.baseDelayMs ?? DEFAULT_GOAL_RESUME_BASE_DELAY_MS
    this.maxDelayMs = deps.maxDelayMs ?? DEFAULT_GOAL_RESUME_MAX_DELAY_MS
  }

  noteGoalTurnFailed(input: {
    threadId: string
    goalKey: string
    madeProgress: boolean
  }): 'scheduled' | 'exhausted' | 'skipped' {
    if (this.shuttingDown) return 'skipped'
    const { threadId, goalKey, madeProgress } = input
    let entry = this.state.get(threadId)
    if (!entry || entry.goalKey !== goalKey) {
      entry?.timer?.cancel()
      entry = { goalKey, attempts: 0 }
      this.state.set(threadId, entry)
    }
    entry.timer?.cancel()
    entry.timer = undefined
    if (madeProgress) entry.attempts = 0
    else entry.attempts += 1
    if (entry.attempts > this.maxNoProgressAttempts) {
      this.state.delete(threadId)
      return 'exhausted'
    }
    const delayMs = Math.min(
      this.maxDelayMs,
      this.baseDelayMs * 2 ** Math.max(0, entry.attempts - 1)
    )
    entry.timer = this.setTimer(() => {
      void this.fire(threadId, goalKey)
    }, delayMs)
    return 'scheduled'
  }

  async resumeInterrupted(threadId: string): Promise<boolean> {
    if (this.shuttingDown) return false
    try {
      const goalKey = await this.deps.getActiveGoalKey(threadId)
      if (!goalKey) return false
      if (await this.deps.isThreadBusy(threadId)) return false
      this.state.get(threadId)?.timer?.cancel()
      this.state.set(threadId, { goalKey, attempts: 0 })
      await this.deps.launch(threadId)
      return true
    } catch (error) {
      this.log(
        `goal resume on startup failed for ${threadId}: ${String(error)}`,
        'ANALYTIX_GOAL_RESUME_STARTUP_FAILED'
      )
      return false
    }
  }

  clear(threadId: string): void {
    const entry = this.state.get(threadId)
    if (!entry) return
    entry.timer?.cancel()
    this.state.delete(threadId)
  }

  shutdown(): void {
    this.shuttingDown = true
    for (const entry of this.state.values()) entry.timer?.cancel()
    this.state.clear()
  }

  private async fire(threadId: string, goalKey: string): Promise<void> {
    if (this.shuttingDown) return
    const entry = this.state.get(threadId)
    if (!entry || entry.goalKey !== goalKey) return
    entry.timer = undefined
    try {
      const currentKey = await this.deps.getActiveGoalKey(threadId)
      if (currentKey !== goalKey) {
        this.state.delete(threadId)
        return
      }
      if (await this.deps.isThreadBusy(threadId)) return
      await this.deps.launch(threadId)
    } catch (error) {
      this.log(
        `goal resume launch failed for ${threadId}: ${String(error)}`,
        'ANALYTIX_GOAL_RESUME_LAUNCH_FAILED'
      )
    }
  }

  private log(message: string, eventCode: GoalResumeConsoleEventCode): void {
    if (this.deps.log) this.deps.log(message)
    else console.warn(`[analytix] event=${eventCode}`)
  }
}
