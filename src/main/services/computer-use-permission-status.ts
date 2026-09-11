import type { ComputerUsePermissions, ComputerUsePermissionState } from '../../shared/analytix-api'

export type HostPermissionProbes = {
  platform: NodeJS.Platform
  liveAccessibility: () => boolean
  configuredAccessibility: () => Promise<unknown>
  screenCapture: () => unknown
}

const captureStates = new Map<unknown, ComputerUsePermissionState>([
  ['authorized', 'granted'],
  ['granted', 'granted'],
  [undefined, 'unknown'],
  ['not determined', 'unknown'],
  ['not-determined', 'unknown']
])

function sample<T>(probe: () => T, unavailable: T): T {
  try {
    return probe()
  } catch {
    return unavailable
  }
}

/** Read OS facts without interpreting a configured grant as live authority. */
export async function readHostPermissionSnapshot(
  probes: HostPermissionProbes
): Promise<ComputerUsePermissions & { platform: NodeJS.Platform }> {
  const result: ComputerUsePermissions & { platform: NodeJS.Platform } = {
    platform: probes.platform,
    supported: true,
    needsPermission: probes.platform === 'darwin',
    accessibility: 'granted',
    screenRecording: 'granted',
    accessibilityNeedsRestart: false
  }
  if (result.needsPermission) {
    const trusted = sample(probes.liveAccessibility, false)
    const configured = await sample(probes.configuredAccessibility, Promise.resolve(undefined))
      .catch(() => undefined)
    const capture = sample(probes.screenCapture, undefined)
    result.accessibility = trusted ? 'granted' : 'denied'
    result.accessibilityNeedsRestart = configured === 'authorized' && !trusted
    result.screenRecording = captureStates.get(capture) ?? 'denied'
  }
  return result
}
