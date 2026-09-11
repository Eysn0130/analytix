import { dialog, shell, type BrowserWindow } from 'electron'
import {
  ControlledArtifactDisplayStoreV1,
  prepareControlledArtifactExportV1
} from './file-effects'
import type {
  ControlledArtifactAccessActionV1,
  ControlledArtifactReleaseEffectV1
} from './host'

const RELEASE_EFFECT_ERROR = 'controlled_artifact_release_effect_unavailable'

export type PreparedElectronControlledArtifactReleaseV1 = {
  canceled: boolean
  release?: ControlledArtifactReleaseEffectV1
}

/**
 * Electron-main-only selection and delivery effects. No method returns a host
 * path, artifact body, access token, or full identifier to preload/renderer.
 */
export class ElectronControlledArtifactReleaseEffectsV1 {
  private closed = false

  private constructor(
    private readonly displays: ControlledArtifactDisplayStoreV1,
    private readonly getMainWindow: () => BrowserWindow | null
  ) {}

  static async open(
    displayRoot: string,
    getMainWindow: () => BrowserWindow | null
  ): Promise<ElectronControlledArtifactReleaseEffectsV1> {
    if (typeof getMainWindow !== 'function') throw fixedReleaseEffectError()
    try {
      const displays = await ControlledArtifactDisplayStoreV1.open(displayRoot, async (path) => shell.openPath(path))
      return new ElectronControlledArtifactReleaseEffectsV1(displays, getMainWindow)
    } catch {
      throw fixedReleaseEffectError()
    }
  }

  async prepare(action: ControlledArtifactAccessActionV1): Promise<PreparedElectronControlledArtifactReleaseV1> {
    if (this.closed) throw fixedReleaseEffectError()
    if (action === 'display') {
      return { canceled: false, release: this.displays.createReleaseEffect() }
    }
    if (action !== 'export') throw fixedReleaseEffectError()
    try {
      const options: Electron.SaveDialogOptions = {
        title: 'Export verified controlled case evidence',
        buttonLabel: 'Export verified evidence',
        filters: [{ name: 'Analytix controlled evidence', extensions: ['json'] }],
        properties: ['createDirectory', 'showOverwriteConfirmation']
      }
      const window = this.getMainWindow()
      const selected = window && !window.isDestroyed()
        ? await dialog.showSaveDialog(window, options)
        : await dialog.showSaveDialog(options)
      if (selected.canceled || !selected.filePath) return { canceled: true }
      const prepared = await prepareControlledArtifactExportV1(selected.filePath)
      return { canceled: false, release: prepared.release }
    } catch {
      throw fixedReleaseEffectError()
    }
  }

  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true
    try {
      await this.displays.close()
    } catch {
      throw fixedReleaseEffectError()
    }
  }
}

function fixedReleaseEffectError(): Error {
  return new Error(RELEASE_EFFECT_ERROR)
}
