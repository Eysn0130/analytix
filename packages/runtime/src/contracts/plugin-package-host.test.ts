import { describe, expect, it } from 'vitest'
import { pluginPackageHostRequestSchema, pluginPackageHostResponseSchema, pluginPackageViewSchema } from './plugin-package-host'

const ids = ['analytix-documents', 'analytix-spreadsheets', 'analytix-presentations', 'analytix-canvas']
const view = (packageId: string) => ({
  packageId, packageVersion: '0.1.0', displayName: packageId === 'analytix-canvas' ? 'Canvas' : packageId,
  origin: 'development-source', publishable: false, materialized: true, generationId: 'a'.repeat(64),
  activationState: 'unset', activationRevision: 0, available: false, operations: []
})

describe('fixed four-package Host contract', () => {
  it('accepts Canvas alongside the unchanged three Office package identities', () => {
    expect(pluginPackageHostResponseSchema.safeParse({ ok: true, packages: ids.map(view) }).success).toBe(true)
    for (const packageId of ids) {
      expect(pluginPackageViewSchema.safeParse(view(packageId)).success).toBe(true)
      expect(pluginPackageHostRequestSchema.safeParse({ action: 'setDesiredState', packageId,
        generationId: 'a'.repeat(64), expectedRevision: 0, desiredState: 'enabled' }).success).toBe(true)
    }
    expect(pluginPackageHostRequestSchema.safeParse({ action: 'invoke', packageId: 'analytix-canvas',
      generationId: 'a'.repeat(64), expectedRevision: 1, contributionId: 'workspace-editor', operation: 'open', input: { objectId: 'object_1' } }).success).toBe(true)
  })
  it('rejects a fifth package, unknown identities and forged public authority fields', () => {
    expect(pluginPackageHostResponseSchema.safeParse({ ok: true, packages: [...ids.map(view), view('analytix-canvas')] }).success).toBe(false)
    expect(pluginPackageViewSchema.safeParse(view('analytix-other')).success).toBe(false)
    for (const extra of [{ sourceRoot: '/private/source' }, { canvasSkillSha256: 'b'.repeat(64) }, { publishable: true }, { available: true }]) {
      expect(pluginPackageViewSchema.safeParse({ ...view('analytix-canvas'), ...extra }).success).toBe(false)
    }
    expect(pluginPackageHostRequestSchema.safeParse({ action: 'invoke', packageId: 'analytix-other',
      generationId: 'a'.repeat(64), expectedRevision: 1, contributionId: 'workspace-editor', operation: 'open', input: {} }).success).toBe(false)
    expect(pluginPackageHostRequestSchema.safeParse({ action: 'invoke', packageId: 'analytix-canvas',
      generationId: 'a'.repeat(64), expectedRevision: 1, contributionId: 'editor-adapter', operation: 'open', input: {} }).success).toBe(false)
  })
})
