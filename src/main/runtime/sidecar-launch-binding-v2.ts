import { createHmac, timingSafeEqual } from 'node:crypto'

const PURPOSE = 'analytix.runtime-sidecar-launch-binding/v2'
const SHA256_PATTERN = /^[a-f0-9]{64}$/
const MAX_SAFE_GENERATION = Number.MAX_SAFE_INTEGER

export type ControlledArtifactSidecarLaunchBindingV2 = Readonly<{
  runtimeURL: string
  runtimePID: number
  controlledArtifactHostURL: string
  backendGeneration: number
  allocationRecordDigest: string
  tlsRootCertificateSHA256: string
  tlsLeafSPKISHA256: string
  controlledArtifactHostReady: boolean
  runtimeTokenConfigured: true
  persistenceRootsConfigured: true
  productionRuntime: true
}>

export function controlledArtifactSidecarLaunchBindingProofV2(
  secretText: string,
  binding: ControlledArtifactSidecarLaunchBindingV2
): string {
  const secret = decodeSecret(secretText)
  try {
    validateBinding(binding)
    const fields = [
      binding.runtimeURL,
      String(binding.runtimePID),
      binding.controlledArtifactHostURL,
      String(binding.backendGeneration),
      binding.allocationRecordDigest,
      binding.tlsRootCertificateSHA256,
      binding.tlsLeafSPKISHA256,
      String(binding.controlledArtifactHostReady),
      String(binding.runtimeTokenConfigured),
      String(binding.persistenceRootsConfigured),
      String(binding.productionRuntime)
    ]
    const mac = createHmac('sha256', secret)
    mac.update(PURPOSE, 'utf8')
    mac.update(Buffer.from([0]))
    for (const field of fields) {
      const bytes = Buffer.from(field, 'utf8')
      const length = Buffer.allocUnsafe(4)
      length.writeUInt32BE(bytes.length)
      mac.update(length)
      mac.update(bytes)
      length.fill(0)
    }
    return mac.digest('hex')
  } finally {
    secret.fill(0)
  }
}

export function verifyControlledArtifactSidecarLaunchBindingProofV2(
  secretText: string,
  binding: ControlledArtifactSidecarLaunchBindingV2,
  candidate: string
): boolean {
  if (!SHA256_PATTERN.test(candidate)) return false
  let expected = ''
  try {
    expected = controlledArtifactSidecarLaunchBindingProofV2(secretText, binding)
    return timingSafeEqual(Buffer.from(expected, 'hex'), Buffer.from(candidate, 'hex'))
  } catch {
    return false
  } finally {
    expected = ''
  }
}

function decodeSecret(secretText: string): Buffer {
  if (typeof secretText !== 'string' || secretText.length === 0) {
    throw new Error('runtime sidecar launch binding input is invalid')
  }
  const secret = Buffer.from(secretText, 'base64url')
  if (secret.length !== 32 || secret.toString('base64url') !== secretText) {
    secret.fill(0)
    throw new Error('runtime sidecar launch binding input is invalid')
  }
  return secret
}

function validateBinding(binding: ControlledArtifactSidecarLaunchBindingV2): void {
  if (
    !validLoopbackOrigin(binding.runtimeURL, 'http:') ||
    !Number.isSafeInteger(binding.runtimePID) || binding.runtimePID <= 0 ||
    !validLoopbackOrigin(binding.controlledArtifactHostURL, 'https:') ||
    !Number.isSafeInteger(binding.backendGeneration) || binding.backendGeneration <= 0 ||
    binding.backendGeneration > MAX_SAFE_GENERATION ||
    !SHA256_PATTERN.test(binding.allocationRecordDigest) ||
    !SHA256_PATTERN.test(binding.tlsRootCertificateSHA256) ||
    !SHA256_PATTERN.test(binding.tlsLeafSPKISHA256) ||
    binding.runtimeTokenConfigured !== true || binding.persistenceRootsConfigured !== true ||
    binding.productionRuntime !== true
  ) {
    throw new Error('runtime sidecar launch binding input is invalid')
  }
}

function validLoopbackOrigin(value: string, protocol: 'http:' | 'https:'): boolean {
  try {
    if (value !== value.trim()) return false
    const parsed = new URL(value)
    const hostname = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '')
    const port = Number(parsed.port)
    return parsed.protocol === protocol && parsed.username === '' && parsed.password === '' &&
      parsed.pathname === '/' && parsed.search === '' && parsed.hash === '' &&
      (hostname === '127.0.0.1' || hostname === '::1') &&
      Number.isInteger(port) && port > 0 && port <= 65_535
  } catch {
    return false
  }
}
