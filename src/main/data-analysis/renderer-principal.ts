import type { BrowserWindow } from 'electron'
import type { DataAnalysisBackendManager } from './backend-manager'

export function bindDataAnalysisRendererPrincipal(
  manager: DataAnalysisBackendManager,
  window: BrowserWindow
): () => void {
  const contents = window.webContents
  const register = (): void => {
    if (window.isDestroyed() || contents.isDestroyed()) return
    manager.registerRenderer(contents, contents.mainFrame)
  }
  contents.on('did-finish-load', register)
  return () => {
    try {
      contents.removeListener('did-finish-load', register)
    } catch {
      // Destroyed WebContents cannot emit another registration event.
    }
  }
}
