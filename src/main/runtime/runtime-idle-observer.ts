type Activity = 'active' | 'idle' | 'invalid'
const activeStates = new Set(['queued', 'in_progress', 'started', 'running'])

/** The activity predicate and an authoritative idle observation are distinct. */
export function classifyRuntimeActivity(body: string): Activity {
  let document: unknown
  try {
    document = JSON.parse(body)
  } catch {
    return 'invalid'
  }
  if (!document || typeof document !== 'object' || Array.isArray(document)) return 'invalid'
  const rows = (document as { threads?: unknown }).threads
  if (!Array.isArray(rows)) return 'invalid'
  for (const row of rows) {
    if (!row || typeof row !== 'object' || Array.isArray(row)) continue
    if (activeStates.has(row.status)) return 'active'
    if (!Array.isArray(row.turns)) continue
    for (const turn of row.turns) {
      if (turn && typeof turn === 'object' && !Array.isArray(turn) && activeStates.has(turn.status)) {
        return 'active'
      }
    }
  }
  return 'idle'
}

export type RuntimeIdleObservation = 'idle' | 'timeout' | 'unavailable'
type IdleProbe = {
  read: () => Promise<{ ok: boolean; body: string }>
  sleep: (milliseconds: number) => Promise<void>
  timeoutMs: number
  intervalMs: number
}

/** Serialize observations; only a valid idle list completes the wait as idle. */
export async function observeRuntimeIdle(probe: IdleProbe): Promise<RuntimeIdleObservation> {
  const deadline = Date.now() + Math.max(0, probe.timeoutMs)
  const observe = async (): Promise<'active' | 'idle' | 'unavailable'> => {
    try {
      const response = await probe.read()
      const activity = response.ok ? classifyRuntimeActivity(response.body) : 'invalid'
      return activity === 'invalid' ? 'unavailable' : activity
    } catch {
      return 'unavailable'
    }
  }
  let activity = await observe()
  while (activity === 'active' && Date.now() < deadline) {
    await probe.sleep(Math.min(probe.intervalMs, Math.max(0, deadline - Date.now())))
    activity = await observe()
  }
  return activity === 'active' ? 'timeout' : activity
}
