import { mkdtemp, readFile, realpath, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  showSaveDialog: vi.fn(),
  closeDisplay: vi.fn(),
  revokeDisplay: vi.fn()
}))

vi.mock('electron', () => ({
  dialog: { showSaveDialog: mocks.showSaveDialog },
  BrowserWindow: class {},
  MessageChannelMain: class {}
}))

import { ElectronControlledArtifactReleaseEffectsV2 } from './release-effects-v2'

beforeEach(() => {
  mocks.showSaveDialog.mockReset()
  mocks.closeDisplay.mockReset()
  mocks.revokeDisplay.mockReset()
})

describe('Electron controlled artifact V2 release selection', () => {
  it('freezes an export target as a main-only capability without creating a file', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-export-v2-')
    const targetPath = join(root, 'verified.json')
    const window = { isDestroyed: () => false }
    try {
      mocks.showSaveDialog.mockResolvedValue({ canceled: false, filePath: targetPath })
      const effects = await ElectronControlledArtifactReleaseEffectsV2.open(
        join(root, 'journal'),
        () => window as never
      )
      const selected = await effects.select('export', new AbortController().signal)

      expect(selected.canceled).toBe(false)
      if (selected.canceled) throw new Error('expected an export target')
      expect(selected.target.targetIdentityDigest()).toMatch(/^[a-f0-9]{64}$/u)
      expect(() => JSON.stringify(selected.target)).toThrow('controlled_artifact_file_effect_v2_failed')
      expect(mocks.showSaveDialog).toHaveBeenCalledWith(window, expect.objectContaining({
        properties: ['createDirectory', 'showOverwriteConfirmation']
      }))
      await expect(readFile(targetPath)).rejects.toMatchObject({ code: 'ENOENT' })
      await effects.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('selects an isolated display target bound to the exact current renderer generation', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-display-v2-')
    try {
      const createDisplayTarget = vi.fn(() => ({
        targetIdentityDigest: () => 'a'.repeat(64),
        createPreparation: () => vi.fn(),
        revoke: mocks.revokeDisplay,
        close: mocks.closeDisplay
      }))
      const isRendererCurrent = vi.fn((id: number, generation: number) =>
        id === 42 && generation === 7)
      const effects = await ElectronControlledArtifactReleaseEffectsV2.open(
        join(root, 'journal'),
        () => null,
        { isRendererCurrent, createDisplayTarget }
      )
      const selected = await effects.select('display', new AbortController().signal, {
        webContentsId: 42,
        rendererGeneration: 7
      })
      expect(selected.canceled).toBe(false)
      if (selected.canceled) throw new Error('expected a display target')
      expect(selected.target.targetIdentityDigest()).toBe('a'.repeat(64))
      expect(createDisplayTarget).toHaveBeenCalledWith({
        webContentsId: 42,
        rendererGeneration: 7,
        isRendererCurrent
      })
      expect(mocks.showSaveDialog).not.toHaveBeenCalled()
      await effects.close()
      expect(mocks.closeDisplay).toHaveBeenCalledTimes(1)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('fails display closed when the initiating renderer authority is absent or stale', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-display-stale-v2-')
    try {
      const createDisplayTarget = vi.fn()
      const effects = await ElectronControlledArtifactReleaseEffectsV2.open(
        join(root, 'journal'),
        () => null,
        { isRendererCurrent: () => false, createDisplayTarget }
      )
      await expect(effects.select('display', new AbortController().signal, {
        webContentsId: 42,
        rendererGeneration: 7
      })).rejects.toThrow('controlled_artifact_release_effect_v2_unavailable')
      expect(createDisplayTarget).not.toHaveBeenCalled()
      await effects.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('revokes only the exact active display into reauthorization and keeps it closeable', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-display-revoke-v2-')
    try {
      const effects = await ElectronControlledArtifactReleaseEffectsV2.open(
        join(root, 'journal'),
        () => null,
        {
          isRendererCurrent: (id, generation) => id === 42 && generation === 7,
          createDisplayTarget: () => ({
            targetIdentityDigest: () => 'b'.repeat(64),
            bindInvocationContext: vi.fn(),
            createPreparation: () => vi.fn(),
            revoke: mocks.revokeDisplay,
            close: mocks.closeDisplay
          })
        }
      )
      const selected = await effects.select('display', new AbortController().signal, {
        webContentsId: 42,
        rendererGeneration: 7
      })
      if (selected.canceled) throw new Error('expected a display target')
      effects.release(selected.target, true)
      expect(effects.revokeActiveDisplay({
        webContentsId: 42,
        rendererGeneration: 8
      })).toBe(false)
      expect(mocks.revokeDisplay).not.toHaveBeenCalled()
      expect(mocks.closeDisplay).not.toHaveBeenCalled()

      expect(effects.revokeActiveDisplay({
        webContentsId: 42,
        rendererGeneration: 7
      })).toBe(true)
      expect(mocks.revokeDisplay).toHaveBeenCalledTimes(1)
      expect(mocks.closeDisplay).not.toHaveBeenCalled()

      mocks.showSaveDialog.mockResolvedValue({
        canceled: false,
        filePath: join(root, 'still-available.json')
      })
      await expect(effects.select(
        'export',
        new AbortController().signal
      )).resolves.toMatchObject({ canceled: false })
      await effects.close()
      expect(mocks.closeDisplay).toHaveBeenCalledTimes(1)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('treats dialog cancellation as no target authority', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-cancel-v2-')
    try {
      mocks.showSaveDialog.mockResolvedValue({ canceled: true })
      const effects = await ElectronControlledArtifactReleaseEffectsV2.open(join(root, 'journal'), () => null)
      await expect(effects.select('export', new AbortController().signal)).resolves.toEqual({ canceled: true })
      await effects.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('rejects a dialog that returns after cancellation and creates no authority', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-export-v2-cancel-')
    const targetPath = join(root, 'late.json')
    let finishDialog!: () => void
    const dialogGate = new Promise<void>((resolve) => { finishDialog = resolve })
    try {
      mocks.showSaveDialog.mockImplementation(async () => {
        await dialogGate
        return { canceled: false, filePath: targetPath }
      })
      const effects = await ElectronControlledArtifactReleaseEffectsV2.open(join(root, 'journal'), () => null)
      const controller = new AbortController()
      const selection = effects.select('export', controller.signal)
      controller.abort()
      finishDialog()

      await expect(selection).rejects.toThrow('controlled_artifact_release_effect_v2_unavailable')
      await expect(readFile(targetPath)).rejects.toMatchObject({ code: 'ENOENT' })
      await effects.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('revokes an unconsumed target when the generation closes', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-export-v2-close-')
    try {
      mocks.showSaveDialog.mockResolvedValue({ canceled: false, filePath: join(root, 'closed.json') })
      const effects = await ElectronControlledArtifactReleaseEffectsV2.open(join(root, 'journal'), () => null)
      const selected = await effects.select('export', new AbortController().signal)
      if (selected.canceled) throw new Error('expected an export target')
      await effects.close()

      expect(() => selected.target.createPreparation()).toThrow('controlled_artifact_file_effect_v2_failed')
      await expect(effects.select('export', new AbortController().signal)).rejects.toThrow(
        'controlled_artifact_release_effect_v2_unavailable'
      )
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})

async function canonicalTempRoot(prefix: string): Promise<string> {
  return realpath(await mkdtemp(join(tmpdir(), prefix)))
}
