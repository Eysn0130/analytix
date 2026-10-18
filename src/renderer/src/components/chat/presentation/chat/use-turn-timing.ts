import { useEffect, useMemo, useRef, useState } from 'react'
import type { AuthoritativeTurnStatuses, AuthoritativeTurnTimings } from '../../../../agent/types'
import { getProvider } from '../../../../agent/registry'
import { isPublicProjectionRevoked } from '../../../../lib/public-projection-revocation'

const EMPTY_TIMINGS: AuthoritativeTurnTimings = {}
const EMPTY_STATUSES: AuthoritativeTurnStatuses = {}
const EMPTY = { timings: EMPTY_TIMINGS, statuses: EMPTY_STATUSES }

/** One existing authorized detail read at mount/settlement, never a clock poll.
 * A selection generation fences A → B → A responses as well as revocation. */
export function useTurnTiming(threadId: string | null, settlementKey: string,
  supplied?: AuthoritativeTurnTimings, suppliedStatuses?: AuthoritativeTurnStatuses) {
  const provider = useMemo(() => getProvider(), [])
  const generation = useRef(0)
  const [state, setState] = useState<{ threadId: string; timings: AuthoritativeTurnTimings; statuses: AuthoritativeTurnStatuses } | null>(null)
  useEffect(() => {
    const epoch = ++generation.current
    setState(null)
    if (!threadId || supplied !== undefined || isPublicProjectionRevoked(threadId)) return
    let active = true
    void provider.getThreadDetail(threadId).then(detail => {
      if (!active || generation.current !== epoch || isPublicProjectionRevoked(threadId)) return
      setState({ threadId, timings: detail.authoritativeTurnTimings ?? EMPTY_TIMINGS,
        statuses: detail.authoritativeTurnStatuses ?? EMPTY_STATUSES })
    }).catch(() => { /* Missing facts remain unknown; the transcript owner reports load errors. */ })
    return () => { active = false; generation.current += 1 }
  }, [provider, threadId, settlementKey, supplied])
  if (!threadId || isPublicProjectionRevoked(threadId)) return EMPTY
  if (supplied !== undefined) return { timings: supplied, statuses: suppliedStatuses ?? EMPTY_STATUSES }
  return state?.threadId === threadId ? state : EMPTY
}
