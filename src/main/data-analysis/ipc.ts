import { ipcMain, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import {
  DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
  type DataAnalysisBackendManager
} from './backend-manager'
import { ensureDataAnalysisWorkspaceCase } from './case-project-registry'

type DataAnalysisRendererPrincipal = {
  webContentsId: number
  rendererGeneration: number
  backendGeneration: number
}

type RegisterDataAnalysisIpcHandlersOptions = {
  manager: DataAnalysisBackendManager
  getMainWindow: () => BrowserWindow | null
  logError?: (category: string, message: string, detail?: unknown) => void
}

function text(value: unknown): string {
  return String(value == null ? '' : value).trim()
}

function requireAuthorizedRenderer(
  manager: DataAnalysisBackendManager,
  event: IpcMainInvokeEvent
): DataAnalysisRendererPrincipal {
  const frame = event.senderFrame
  if (!frame || frame !== event.sender.mainFrame || !manager.validateRenderer(event.sender, frame)) {
    throw new Error('unauthorized data analysis renderer')
  }
  const rendererGeneration = manager.getRendererAuthorityGeneration(event.sender, frame)
  const backendGeneration = manager.getState().generation
  if (
    rendererGeneration === undefined ||
    !Number.isSafeInteger(backendGeneration) ||
    Number(backendGeneration) < 0
  ) {
    throw new Error('unauthorized data analysis renderer')
  }
  return {
    webContentsId: event.sender.id,
    rendererGeneration,
    backendGeneration: Number(backendGeneration)
  }
}

function requireRunningPathPrincipal(
  manager: DataAnalysisBackendManager,
  event: IpcMainInvokeEvent
): DataAnalysisRendererPrincipal {
  const principal = requireAuthorizedRenderer(manager, event)
  const state = manager.getState()
  if (state.phase !== 'running' || state.generation !== principal.backendGeneration) {
    throw new Error('data analysis backend path authority unavailable')
  }
  return principal
}

export function registerDataAnalysisIpcHandlers({
  manager
}: RegisterDataAnalysisIpcHandlersOptions): void {
  manager.onState((state) => {
    for (const contents of manager.getAuthorizedRenderers()) {
      try {
        contents.send('data-analysis:backend-runtime-state', state)
      } catch {
        // A destroyed renderer cannot retain endpoint or bearer authority.
      }
    }
  })

  ipcMain.handle('data-analysis:ensure-backend', async (event) => {
    requireAuthorizedRenderer(manager, event)
    return manager.ensureBackend()
  })
  ipcMain.handle('data-analysis:runtime-info', async (event) => {
    requireAuthorizedRenderer(manager, event)
    return manager.getRuntimeInfo()
  })
  ipcMain.handle('data-analysis:ensure-workspace-case', async (event, payload: unknown) => {
    requireAuthorizedRenderer(manager, event)
    const authority = await manager.ensureBackend()
    if (authority.terminal || authority.authority !== 'go-native' || authority.phase !== 'running') {
      return { ok: false, message: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER }
    }
    const workspaceRoot = text((payload as { workspaceRoot?: unknown } | null)?.workspaceRoot)
    return ensureDataAnalysisWorkspaceCase(manager, workspaceRoot)
  })

  ipcMain.handle('data-analysis:pick-files', async (event) => {
    requireRunningPathPrincipal(manager, event)
    // P2 replaces this quarantine with host-owned immutable source handles.
    return { canceled: true, paths: [], filePaths: [] }
  })

  ipcMain.handle('data-analysis:pick-directory', async (event) => {
    requireRunningPathPrincipal(manager, event)
    // Renderer-visible host paths are not an ingestion authority.
    return { canceled: true, path: '', filePath: '' }
  })

}
