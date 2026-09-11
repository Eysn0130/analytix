import { createHash } from 'node:crypto'
import { mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ControlledArtifactReleaseEffectInputV1 } from './host'

const mocks = vi.hoisted(() => ({
  showSaveDialog: vi.fn(),
  openPath: vi.fn()
}))

vi.mock('electron', () => ({
  dialog: { showSaveDialog: mocks.showSaveDialog },
  shell: { openPath: mocks.openPath }
}))

import { ElectronControlledArtifactReleaseEffectsV1 } from './release-effects'

const FULL_ACCOUNT = '6222020202020202020'

beforeEach(() => {
  mocks.showSaveDialog.mockReset()
  mocks.openPath.mockReset()
  mocks.openPath.mockResolvedValue('')
})

describe('Electron controlled artifact release effects', () => {
  it('keeps the save target main-only and writes exact verified bytes once', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-export-')
    const target = join(root, 'verified.json')
    const window = { isDestroyed: () => false }
    try {
      const effects = await ElectronControlledArtifactReleaseEffectsV1.open(
        join(root, 'display'),
        () => window as never
      )
      mocks.showSaveDialog.mockResolvedValue({ canceled: false, filePath: target })
      const prepared = await effects.prepare('export')

      expect(Object.keys(prepared).sort()).toEqual(['canceled', 'release'])
      expect(JSON.stringify(prepared)).toBe('{"canceled":false}')
      expect(mocks.showSaveDialog).toHaveBeenCalledWith(window, expect.objectContaining({
        properties: ['createDirectory', 'showOverwriteConfirmation']
      }))
      await prepared.release?.(effectInput('export'))
      await expect(readFile(target, 'utf8')).resolves.toContain(FULL_ACCOUNT)
      await effects.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('publishes a protected display copy only through the OS viewer', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-display-')
    let viewerBody = ''
    try {
      mocks.openPath.mockImplementation(async (path: string) => {
        viewerBody = await readFile(path, 'utf8')
        return ''
      })
      const effects = await ElectronControlledArtifactReleaseEffectsV1.open(join(root, 'display'), () => null)
      const prepared = await effects.prepare('display')
      expect(JSON.stringify(prepared)).toBe('{"canceled":false}')
      await prepared.release?.(effectInput('display'))
      expect(viewerBody).toContain(FULL_ACCOUNT)
      expect(mocks.showSaveDialog).not.toHaveBeenCalled()
      await effects.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('treats cancel as no authority and rejects overwrite without reading artifact bytes', async () => {
    const root = await canonicalTempRoot('analytix-electron-controlled-cancel-')
    try {
      const effects = await ElectronControlledArtifactReleaseEffectsV1.open(join(root, 'display'), () => null)
      mocks.showSaveDialog.mockResolvedValueOnce({ canceled: true })
      await expect(effects.prepare('export')).resolves.toEqual({ canceled: true })

      const existing = join(root, 'existing.json')
      await writeFile(existing, 'victim', { mode: 0o600 })
      mocks.showSaveDialog.mockResolvedValueOnce({ canceled: false, filePath: existing })
      await expect(effects.prepare('export')).rejects.toThrow('controlled_artifact_release_effect_unavailable')
      await expect(readFile(existing, 'utf8')).resolves.toBe('victim')
      await effects.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})

function effectInput(action: 'display' | 'export'): ControlledArtifactReleaseEffectInputV1 {
  const body = Buffer.from(JSON.stringify({ bankAccount: FULL_ACCOUNT }), 'utf8')
  return {
    action,
    body,
    receipt: {
      accessAction: action,
      accessId: '1'.repeat(64),
      artifactByteLength: body.length,
      artifactSha256: createHash('sha256').update(body).digest('hex')
    }
  } as ControlledArtifactReleaseEffectInputV1
}

async function canonicalTempRoot(prefix: string): Promise<string> {
  return realpath(await mkdtemp(join(tmpdir(), prefix)))
}
