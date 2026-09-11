import { HostController, type HostControlAvailability, type HostControlBackend } from './host-control.js'
import { OpenComputerUseBackend, type OpenComputerUseBackendOptions } from './open-computer-use-backend.js'

export type HostControlBackendSelection = {
  backend: HostControlBackend
  readiness: HostControlAvailability
  preferredBackendId: string
  selectedBackendId: string
  fallbackReason?: string
}

export type HostControlBackendFactoryOptions = {
  maxImageDimension?: number
  maxImageBytes?: number
  openComputerUse?: OpenComputerUseBackendOptions
}

export async function selectHostControlBackend(
  options: HostControlBackendFactoryOptions = {}
): Promise<HostControlBackendSelection> {
  const openBackend = new OpenComputerUseBackend({
    ...(options.openComputerUse ?? {}),
    maxImageDimension: options.maxImageDimension,
    maxImageBytes: options.maxImageBytes
  })
  const openReady = await openBackend.ensureReady()
  if (openReady.available) {
    return {
      backend: openBackend,
      readiness: openReady,
      preferredBackendId: 'analytix-computer-use',
      selectedBackendId: openBackend.id
    }
  }

  const nut = new HostController({ maxImageDimension: options.maxImageDimension })
  const nutReady = await nut.ensureReady()
  return {
    backend: nut,
    readiness: nutReady,
    preferredBackendId: 'analytix-computer-use',
    selectedBackendId: nut.id,
    fallbackReason: openReady.reason
  }
}
