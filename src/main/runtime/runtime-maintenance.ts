import { z } from 'zod'
import type { AppSettingsV1 } from '../../shared/app-settings'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'
import {
  captureCurrentFinalPublicationAuthorityPin,
  getRuntimeBaseUrlForSettings,
  isCurrentFinalPublicationAuthorityPin,
  runtimeAuthHeaders,
  type ManagedFinalPublicationAuthorityPinV1
} from './analytix-adapter'
import type { BundledFundsMaintenanceLeaseV1 } from './bundled-funds-on-demand'

const Ready = z.object({
  schemaVersion: z.literal(1),
  runtimePid: z.number().int().positive(),
  state: z.literal('idle'),
  lease: z.string().regex(/^[A-Za-z0-9_-]{43}$/u),
  expiresAtUnixMs: z.number().int().safe()
}).strict()
const Committed = z.object({
  schemaVersion: z.literal(1),
  runtimePid: z.number().int().positive(),
  committed: z.literal(true)
}).strict()
const Released = z.object({
  schemaVersion: z.literal(1),
  runtimePid: z.number().int().positive(),
  released: z.literal(true)
}).strict()

export function parseRuntimeMaintenanceReadyV1(
  body: Buffer,
  pin: ManagedFinalPublicationAuthorityPinV1,
  now: number
): { lease: string; expiresAtUnixMs: number } | null {
  try {
    const ready = Ready.parse(parseStrictJsonObject(body, { maxBytes: 1024, maxDepth: 2, maxTokens: 16 }))
    if (ready.runtimePid !== pin.runtimePid ||
      ready.expiresAtUnixMs <= now + 1_000 ||
      ready.expiresAtUnixMs > now + 31_000) return null
    return { lease: ready.lease, expiresAtUnixMs: ready.expiresAtUnixMs }
  } catch {
    return null
  }
}

export async function prepareCurrentRuntimeMaintenanceV1(
  settings: AppSettingsV1,
  pin: ManagedFinalPublicationAuthorityPinV1
): Promise<BundledFundsMaintenanceLeaseV1 | null> {
  if (!isCurrentFinalPublicationAuthorityPin(pin)) return null
  const baseUrl = getRuntimeBaseUrlForSettings(settings)
  if (new URL(baseUrl).origin !== pin.runtimeUrl) return null
  const headers = runtimeAuthHeaders(settings)
  headers.set('Content-Type', 'application/json')
  let response: Response
  try {
    response = await fetch(`${baseUrl}/v1/runtime/quiescence`, {
      method: 'POST', headers, body: '{"operation":"prepare"}', signal: AbortSignal.timeout(10_000)
    })
  } catch {
    return null
  }
  if (!response.ok || !isCurrentFinalPublicationAuthorityPin(pin)) return null
  const body = Buffer.from(await response.arrayBuffer())
  const ready = parseRuntimeMaintenanceReadyV1(body, pin, Date.now())
  body.fill(0)
  if (!ready || !isCurrentFinalPublicationAuthorityPin(pin)) return null
  return {
    expiresAtUnixMs: ready.expiresAtUnixMs,
    isCurrent: () => isCurrentFinalPublicationAuthorityPin(pin),
    commitStop: async () => {
      if (!isCurrentFinalPublicationAuthorityPin(pin) || Date.now() + 1_000 >= ready.expiresAtUnixMs) return false
      try {
        const committed = await fetch(`${baseUrl}/v1/runtime/quiescence`, {
          method: 'POST', headers,
          body: JSON.stringify({ operation: 'commit-stop', lease: ready.lease }),
          signal: AbortSignal.timeout(5_000)
        })
        if (!committed.ok || !isCurrentFinalPublicationAuthorityPin(pin)) return false
        const body = Buffer.from(await committed.arrayBuffer())
        const parsed = Committed.safeParse(parseStrictJsonObject(body, {
          maxBytes: 256, maxDepth: 2, maxTokens: 8
        }))
        body.fill(0)
        return parsed.success && parsed.data.runtimePid === pin.runtimePid &&
          isCurrentFinalPublicationAuthorityPin(pin)
      } catch {
        return false
      }
    },
    release: async () => {
      if (!isCurrentFinalPublicationAuthorityPin(pin)) return
      const released = await fetch(`${baseUrl}/v1/runtime/quiescence`, {
        method: 'POST', headers,
        body: JSON.stringify({ operation: 'release', lease: ready.lease }),
        signal: AbortSignal.timeout(5_000)
      })
      if (!isCurrentFinalPublicationAuthorityPin(pin)) return
      if (!released.ok) throw new Error('Runtime maintenance release was not confirmed.')
      const body = Buffer.from(await released.arrayBuffer())
      try {
        const parsed = Released.safeParse(parseStrictJsonObject(body, {
          maxBytes: 256, maxDepth: 2, maxTokens: 8
        }))
        if (!parsed.success || parsed.data.runtimePid !== pin.runtimePid ||
          !isCurrentFinalPublicationAuthorityPin(pin)) {
          throw new Error('Runtime maintenance release identity was not confirmed.')
        }
      } finally {
        body.fill(0)
      }
    }
  }
}

export function currentRuntimeMaintenancePinV1(): ManagedFinalPublicationAuthorityPinV1 | null {
  return captureCurrentFinalPublicationAuthorityPin()
}
