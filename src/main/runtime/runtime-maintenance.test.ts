import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AppSettingsV1 } from '../../shared/app-settings'
import type { ManagedFinalPublicationAuthorityPinV1 } from './analytix-adapter'
import { prepareCurrentRuntimeMaintenanceV1 } from './runtime-maintenance'

const state = vi.hoisted(() => ({ currentPid: 4101 }))
vi.mock('./analytix-adapter', () => ({
  captureCurrentFinalPublicationAuthorityPin: () => null,
  getRuntimeBaseUrlForSettings: () => 'http://127.0.0.1:4100',
  isCurrentFinalPublicationAuthorityPin: (pin: { runtimePid: number }) =>
    pin.runtimePid === state.currentPid,
  runtimeAuthHeaders: () => new Headers({ Authorization: 'Bearer test-only' })
}))

const pin = {
  runtimePid: 4101,
  runtimeUrl: 'http://127.0.0.1:4100',
  generation: 1,
  keyId: 'test',
  publicKey: 'test'
} as ManagedFinalPublicationAuthorityPinV1

afterEach(() => {
  vi.unstubAllGlobals()
  state.currentPid = 4101
})

describe('runtime maintenance exact lease client', () => {
  it('does not accept a lost release request as recovery or send it to a new PID', async () => {
    const leaseId = 'a'.repeat(43)
    let releaseAttempts = 0
    const fetch = vi.fn(async (_url: string, init: RequestInit) => {
      const request = JSON.parse(String(init.body)) as { operation: string; lease?: string }
      expect(request.lease).toBeUndefined()
      return Response.json({ schemaVersion: 1, runtimePid: 4101, state: 'idle',
        lease: leaseId, expiresAtUnixMs: Date.now() + 30_000 })
    })
    vi.stubGlobal('fetch', fetch)
    const lease = await prepareCurrentRuntimeMaintenanceV1({} as AppSettingsV1, pin)
    expect(lease).not.toBeNull()

    fetch.mockImplementation(async (_url: string, init: RequestInit) => {
      const request = JSON.parse(String(init.body)) as { operation: string; lease: string }
      expect(request.lease).toBe(leaseId)
      if (request.operation === 'commit-stop') throw new Error('commit ACK lost')
      if (request.operation === 'release') {
        releaseAttempts += 1
        if (releaseAttempts === 1) throw new Error('release request lost')
        return Response.json({ schemaVersion: 1, runtimePid: 4101, released: true })
      }
      throw new Error('unexpected operation')
    })
    await expect(lease!.commitStop()).resolves.toBe(false)
    await expect(lease!.release()).rejects.toThrow('release request lost')
    await expect(lease!.release()).resolves.toBeUndefined()
    expect(releaseAttempts).toBe(2)

    state.currentPid = 4102
    await expect(lease!.release()).resolves.toBeUndefined()
    expect(releaseAttempts).toBe(2)
  })
})
