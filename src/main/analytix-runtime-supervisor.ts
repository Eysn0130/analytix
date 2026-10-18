/**
 * Crash-loop budget and status contract for the GUI-managed Analytix
 * runtime. The supervisor in index.ts consumes these to auto-restart a
 * crashed runtime with backoff, and to stop retrying (circuit break)
 * when the runtime is crashing faster than it can recover.
 */

import type { AnalytixRuntimeStatusPayload } from '../shared/analytix-api'

/** Shared with preload/renderer; the payload travels over `runtime:status`. */
export type AnalytixRuntimeStatus = AnalytixRuntimeStatusPayload

export type RestartVerdict =
  | { allowed: true; attempt: number; delayMs: number }
  | { allowed: false; attempt: number; delayMs: 0 }

export type RestartBudgetOptions = {
  windowMs: number
  maxRestarts: number
  baseDelayMs?: number
  delayFactor?: number
  now?: () => number
}

export const MAX_RESTART_DELAY_MS = 2_147_483_647

/**
 * A restart must not terminate a runtime child while its ready handshake is
 * still owned by an in-flight ensure. Settle that ensure first, then run one
 * fresh stop/start sequence. The old ensure error is intentionally ignored:
 * the replacement restart remains the authoritative outcome.
 */
export async function restartAfterPendingEnsure<T>(
  pendingEnsure: Promise<T> | null,
  restart: () => Promise<void>
): Promise<void> {
  if (pendingEnsure) {
    try {
      await pendingEnsure
    } catch {
      // A failed ensure still has to release its child before replacement.
    }
  }
  await restart()
}

export type SettingsRuntimeStopReceipt = Readonly<{
  child: object
  generation: number
  cancelEpoch: number
}>

/** Only a child actually stopped by the serialized Settings apply can be resumed. */
export class SettingsRuntimeStopHandoff {
  private cancelEpoch = 0
  private pending: SettingsRuntimeStopReceipt | null = null
  private stopping: SettingsRuntimeStopReceipt | null = null

  begin(child: object, generation: number): SettingsRuntimeStopReceipt {
    const receipt = { child, generation, cancelEpoch: this.cancelEpoch }
    this.stopping = receipt
    return receipt
  }

  owns(receipt: SettingsRuntimeStopReceipt): boolean {
    return receipt.cancelEpoch === this.cancelEpoch
  }

  cancellationWitness(): () => boolean {
    const epoch = this.cancelEpoch
    return () => epoch === this.cancelEpoch
  }

  confirmStopped(receipt: SettingsRuntimeStopReceipt, currentChild: object | null): boolean {
    if (this.stopping !== receipt || !this.owns(receipt) || currentChild !== null) return false
    this.stopping = null
    this.pending = receipt
    return true
  }

  hasPending(): boolean {
    return this.pending !== null
  }

  admit(generation: number, currentGeneration: number, issuer?: SettingsRuntimeStopReceipt): SettingsRuntimeStopReceipt | null {
    const receipt = this.pending
    if (!receipt || !this.owns(receipt) || generation !== currentGeneration) return null
    return generation > receipt.generation || (issuer === receipt && generation === receipt.generation)
      ? receipt : null
  }

  async startAfterPort<T>(
    receipt: SettingsRuntimeStopReceipt,
    resolvePort: () => Promise<T>,
    admitted: () => boolean,
    start: (settings: T) => Promise<void>
  ): Promise<T | null> {
    if (this.pending !== receipt || !this.owns(receipt) || !admitted()) return null
    const settings = await resolvePort()
    if (this.pending !== receipt || !this.owns(receipt) || !admitted()) return null
    await start(settings)
    this.complete(receipt)
    return settings
  }

  complete(receipt: SettingsRuntimeStopReceipt): void {
    if (this.pending === receipt) this.pending = null
  }

  cancel(): void {
    this.cancelEpoch += 1
    this.pending = null
    this.stopping = null
  }
}

function finiteNumber(value: number | undefined, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

/**
 * Sliding-window restart budget: allows up to `maxRestarts` attempts per
 * `windowMs`, with exponential backoff delays (base, base*factor, ...).
 * Once the window is saturated the caller should circuit-break and wait
 * for a manual restart instead of burning CPU on a crash loop.
 */
export class RestartBudget {
  private readonly windowMs: number
  private readonly maxRestarts: number
  private readonly baseDelayMs: number
  private readonly delayFactor: number
  private readonly now: () => number
  private attempts: number[] = []

  constructor(options: RestartBudgetOptions) {
    this.windowMs = Math.max(1, finiteNumber(options.windowMs, 60_000))
    this.maxRestarts = Math.max(1, finiteNumber(options.maxRestarts, 3))
    this.baseDelayMs = Math.max(0, finiteNumber(options.baseDelayMs, 1_000))
    this.delayFactor = Math.max(1, finiteNumber(options.delayFactor, 3))
    this.now = options.now ?? (() => Date.now())
  }

  /** Ask for one restart attempt; records it when allowed. */
  note(): RestartVerdict {
    const at = this.now()
    this.attempts = this.attempts.filter((t) => at - t < this.windowMs)
    if (this.attempts.length >= this.maxRestarts) {
      return { allowed: false, attempt: this.attempts.length, delayMs: 0 }
    }
    this.attempts.push(at)
    const attempt = this.attempts.length
    return {
      allowed: true,
      attempt,
      delayMs: Math.min(
        MAX_RESTART_DELAY_MS,
        Math.round(this.baseDelayMs * Math.pow(this.delayFactor, attempt - 1))
      )
    }
  }

  /** Forget past attempts after the runtime proved stable again. */
  reset(): void {
    this.attempts = []
  }
}
