import { dialog, type BrowserWindow } from 'electron'
import { freezeControlledArtifactExportTargetV2 } from './file-effects-v2'
import { ControlledArtifactExportJournalV2 } from './export-journal-v2'
import {
  ControlledArtifactDisplayTargetV2,
  type ControlledArtifactDisplayTargetV2Options
} from './display-effects-v2'
import type {
  ControlledArtifactAccessActionV2,
  ControlledArtifactInvocationContextV2,
  ControlledArtifactReleasePreparationV2
} from './host-v2'

const RELEASE_EFFECT_ERROR = 'controlled_artifact_release_effect_v2_unavailable'

export type ControlledArtifactReleaseTargetV2 = Readonly<{
  targetIdentityDigest: () => string
  bindInvocationContext?: (context: ControlledArtifactInvocationContextV2) => void
  createPreparation: () => ControlledArtifactReleasePreparationV2
  revoke: () => void
  close: () => void
}>

export type ElectronControlledArtifactReleaseEffectsV2Options = Readonly<{
  isRendererCurrent?: (webContentsId: number, rendererGeneration: number) => boolean
  createDisplayTarget?: (
    options: ControlledArtifactDisplayTargetV2Options
  ) => ControlledArtifactReleaseTargetV2
}>

export type ElectronControlledArtifactReleaseSelectionV2 =
  | { canceled: true }
  | { canceled: false; target: ControlledArtifactReleaseTargetV2 }

/**
 * Electron-main-only target selection. Export uses a frozen filesystem target;
 * display uses an isolated internal viewer bound to the initiating renderer
 * generation. Neither capability is returned through preload or the renderer.
 */
export class ElectronControlledArtifactReleaseEffectsV2 {
  private closed = false
  private readonly targets = new Set<ControlledArtifactReleaseTargetV2>()
  private readonly displayRenderers = new Map<ControlledArtifactReleaseTargetV2, Readonly<{
    webContentsId: number
    rendererGeneration: number
  }>>()
  private displayTarget: ControlledArtifactReleaseTargetV2 | null = null
  private displayRenderer: Readonly<{
    webContentsId: number
    rendererGeneration: number
  }> | null = null

  private constructor(
    private readonly journal: ControlledArtifactExportJournalV2,
    private readonly getMainWindow: () => BrowserWindow | null,
    private readonly options: ElectronControlledArtifactReleaseEffectsV2Options
  ) {
    if (typeof getMainWindow !== 'function') throw fixedReleaseEffectError()
  }

  static async open(
    journalRoot: string,
    getMainWindow: () => BrowserWindow | null,
    options: ElectronControlledArtifactReleaseEffectsV2Options = {}
  ): Promise<ElectronControlledArtifactReleaseEffectsV2> {
    if (typeof getMainWindow !== 'function' || !options ||
      (options.isRendererCurrent !== undefined &&
        typeof options.isRendererCurrent !== 'function') ||
      (options.createDisplayTarget !== undefined &&
        typeof options.createDisplayTarget !== 'function')) {
      throw fixedReleaseEffectError()
    }
    try {
      const journal = await ControlledArtifactExportJournalV2.open(journalRoot)
      return new ElectronControlledArtifactReleaseEffectsV2(journal, getMainWindow, options)
    } catch {
      throw fixedReleaseEffectError()
    }
  }

  async select(
    action: ControlledArtifactAccessActionV2,
    signal: AbortSignal,
    renderer?: Readonly<{ webContentsId: number; rendererGeneration: number }>
  ): Promise<ElectronControlledArtifactReleaseSelectionV2> {
    if (this.closed || !(signal instanceof AbortSignal) || signal.aborted) {
      throw fixedReleaseEffectError()
    }
    if (action === 'display') {
      const isRendererCurrent = this.options.isRendererCurrent
      if (!renderer || !Number.isSafeInteger(renderer.webContentsId) ||
        renderer.webContentsId <= 0 || !Number.isSafeInteger(renderer.rendererGeneration) ||
        renderer.rendererGeneration <= 0 || typeof isRendererCurrent !== 'function' ||
        !rendererIsCurrentV2(
          isRendererCurrent,
          renderer.webContentsId,
          renderer.rendererGeneration
        )) {
        throw fixedReleaseEffectError()
      }
      try {
        const create = this.options.createDisplayTarget ??
          ((targetOptions: ControlledArtifactDisplayTargetV2Options) =>
            new ControlledArtifactDisplayTargetV2(targetOptions))
        const target = create({
          webContentsId: renderer.webContentsId,
          rendererGeneration: renderer.rendererGeneration,
          isRendererCurrent
        })
        if (this.closed || signal.aborted ||
          !rendererIsCurrentV2(
            isRendererCurrent,
            renderer.webContentsId,
            renderer.rendererGeneration
          )) {
          target.close()
          throw fixedReleaseEffectError()
        }
        this.displayRenderers.set(target, Object.freeze({
          webContentsId: renderer.webContentsId,
          rendererGeneration: renderer.rendererGeneration
        }))
        this.targets.add(target)
        return { canceled: false, target }
      } catch {
        throw fixedReleaseEffectError()
      }
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
      if (this.closed || signal.aborted) throw fixedReleaseEffectError()
      if (selected.canceled || !selected.filePath) return { canceled: true }
      const target = await freezeControlledArtifactExportTargetV2(selected.filePath, this.journal)
      if (this.closed || signal.aborted) {
        target.revoke()
        throw fixedReleaseEffectError()
      }
      this.targets.add(target)
      return { canceled: false, target }
    } catch {
      throw fixedReleaseEffectError()
    }
  }

  release(target: ControlledArtifactReleaseTargetV2, keepOpen = false): void {
    if (!this.targets.has(target)) return
    if (keepOpen) {
      const renderer = this.displayRenderers.get(target)
      if (!renderer) return
      const previous = this.displayTarget
      this.displayTarget = target
      this.displayRenderer = renderer
      if (previous && previous !== target) {
        this.targets.delete(previous)
        this.displayRenderers.delete(previous)
        closeTargetV2(previous)
      }
      return
    }
    this.targets.delete(target)
    this.displayRenderers.delete(target)
    if (this.displayTarget === target) {
      this.displayTarget = null
      this.displayRenderer = null
    }
    target.close()
  }

  revokeActiveDisplay(renderer?: Readonly<{
    webContentsId: number
    rendererGeneration: number
  }>): boolean {
    const target = this.displayTarget
    const boundRenderer = this.displayRenderer
    if (!target || !boundRenderer || (renderer &&
      (renderer.webContentsId !== boundRenderer.webContentsId ||
        renderer.rendererGeneration !== boundRenderer.rendererGeneration))) {
      return false
    }
    try {
      target.revoke()
    } catch {
      closeTargetV2(target)
    }
    return true
  }

  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true
    for (const target of this.targets) closeTargetV2(target)
    this.targets.clear()
    this.displayRenderers.clear()
    this.displayTarget = null
    this.displayRenderer = null
    try {
      await this.journal.close()
    } catch {
      throw fixedReleaseEffectError()
    }
  }
}

function closeTargetV2(target: ControlledArtifactReleaseTargetV2): void {
  try {
    target.close()
    return
  } catch {
    // Fall through to the narrower revocation operation.
  }
  try {
    target.revoke()
  } catch {
    // The caller retains no target capability after this final fail-closed attempt.
  }
}

function fixedReleaseEffectError(): Error {
  return new Error(RELEASE_EFFECT_ERROR)
}

function rendererIsCurrentV2(
  isCurrent: (webContentsId: number, rendererGeneration: number) => boolean,
  webContentsId: number,
  rendererGeneration: number
): boolean {
  try {
    return isCurrent(webContentsId, rendererGeneration) === true
  } catch {
    return false
  }
}
