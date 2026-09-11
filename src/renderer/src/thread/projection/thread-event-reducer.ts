import type { ThreadDeltaEvent } from '../../agent/types'

export type ThreadProjectionLiveState = {
  liveAssistant: string
  lastSeq: number
}

export function reduceThreadDeltas(
  state: ThreadProjectionLiveState,
  deltas: ThreadDeltaEvent[]
): ThreadProjectionLiveState {
  let liveAssistant = state.liveAssistant
  let lastSeq = state.lastSeq

  for (const delta of deltas) {
    if (typeof delta.seq === 'number') lastSeq = Math.max(lastSeq, delta.seq)
    liveAssistant += delta.text
  }

  if (
    liveAssistant === state.liveAssistant &&
    lastSeq === state.lastSeq
  ) {
    return state
  }

  return { liveAssistant, lastSeq }
}
