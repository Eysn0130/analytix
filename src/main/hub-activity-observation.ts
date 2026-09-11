export type HubActivityKind =
  | 'moduleLoad'
  | 'serviceInstance'
  | 'refreshTimer'
  | 'tokenRead'
  | 'request'
  | 'fallback'

export type HubActivityObservation = Readonly<{
  schemaVersion: 1
  moduleLoad: number
  serviceInstance: number
  refreshTimer: number
  tokenRead: number
  request: number
  fallback: number
  anyActivity: boolean
}>

export type HubActivityObservationStage =
  | 'ordinary_startup_ready'
  | 'activity_changed'

const counters: Record<HubActivityKind, number> = {
  moduleLoad: 0,
  serviceInstance: 0,
  refreshTimer: 0,
  tokenRead: 0,
  request: 0,
  fallback: 0
}

const observationEnabled = process.env.ANALYTIX_HUB_ACTIVITY_OBSERVATION === '1'

export function observeHubActivity(): HubActivityObservation {
  const observation = {
    schemaVersion: 1 as const,
    moduleLoad: counters.moduleLoad,
    serviceInstance: counters.serviceInstance,
    refreshTimer: counters.refreshTimer,
    tokenRead: counters.tokenRead,
    request: counters.request,
    fallback: counters.fallback
  }
  return {
    ...observation,
    anyActivity: Object.values(counters).some((value) => value > 0)
  }
}

export function emitHubActivityObservation(stage: HubActivityObservationStage): void {
  if (!observationEnabled) return
  console.info('[hub-activity-observation]', JSON.stringify({
    stage,
    ...observeHubActivity()
  }))
}

export function recordHubActivity(kind: HubActivityKind): void {
  counters[kind] += 1
  emitHubActivityObservation('activity_changed')
}

export function resetHubActivityForTests(): void {
  for (const kind of Object.keys(counters) as HubActivityKind[]) counters[kind] = 0
}
