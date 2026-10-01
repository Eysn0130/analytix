import { describe, expect, it, vi } from 'vitest'
import { createPluginPackageHostHandler } from './plugin-package-host-ipc'
import { pluginPackageViewSchema } from '../../../packages/runtime/src/contracts/plugin-package-host'

const pkg = { packageId: 'analytix-documents', packageVersion: '0.1.0', displayName: 'Documents',
  origin: 'development-source', publishable: false, materialized: true, generationId: 'a'.repeat(64),
  activationState: 'recorded', desiredState: 'enabled', activationRevision: 1, activationId: 'b'.repeat(64),
  available: false, unavailableReason: 'adapter_unavailable', operations: [] }
const set = { action: 'setDesiredState', packageId: pkg.packageId, generationId: pkg.generationId,
  expectedRevision: 0, desiredState: 'enabled' }

describe('protected package host IPC', () => {
  it('rejects renderer-forged native attestations before Core transport', async () => {
    const transport = vi.fn()
    const handler = createPluginPackageHostHandler(transport, 'renderer')
    for (const operation of ['capture-selection', 'proposal-accept', 'commit-object', 'model-selection-propose']) {
      const result = await handler({ action: 'invoke', packageId: pkg.packageId,
        generationId: pkg.generationId, expectedRevision: 1, contributionId: 'workspace-editor',
        operation, input: {} })
      expect(result).toMatchObject({ ok: false, code: 'identity_invalid' })
    }
    expect(transport).not.toHaveBeenCalled()
  })
  it('keeps a persisted enabled package distinct from an available adapter', async () => {
    const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, package: pkg }) }))
    expect(await createPluginPackageHostHandler(transport)(set)).toEqual({ ok: true, package: pkg })
    expect(transport.mock.calls).toHaveLength(1)
    expect(pluginPackageViewSchema.safeParse({ ...pkg, available: true }).success).toBe(false)
  })
  it('rejects forged roots and lossy CAS revisions before transport', async () => {
    const transport = vi.fn()
    for (const input of [{ ...set, root: '/other' }, { ...set, expectedRevision: Number.MAX_SAFE_INTEGER }, { ...set, expectedRevision: 0.5 }]) {
      expect(await createPluginPackageHostHandler(transport)(input)).toMatchObject({ ok: false, code: 'invalid_request' })
    }
    expect(transport).not.toHaveBeenCalled()
  })
  it('reconciles late or mismatched state responses without reporting success', async () => {
    for (const mutation of [{ ...pkg, generationId: 'c'.repeat(64) }, { ...pkg, activationRevision: 3 }, { ...pkg, desiredState: 'disabled' }]) {
      const transport = async () => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, package: mutation }) })
      expect(await createPluginPackageHostHandler(transport)(set)).toMatchObject({ ok: false, relist: true })
    }
    expect(await createPluginPackageHostHandler(async () => { throw new Error('private path') })(set)).toEqual({
      ok: false, code: 'unavailable', message: 'Plugin result could not be confirmed. Refresh its current state.', relist: true
    })
  })
})
