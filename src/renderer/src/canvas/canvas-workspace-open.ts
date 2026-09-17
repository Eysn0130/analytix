import { previewWorkspaceFile } from '../lib/workspace-file-preview'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../store/workspace-tabs-store'

/** Reuse the existing file route. The preview hint grants no file or plugin access. */
export function openCanvasWorkspaceObject(workspace: string, path: string): void {
  useWorkspaceTabsStore.getState().openTab({
    id: workspaceObjectTabId(workspace, path), kind: 'file', mode: 'file', preview: 'canvas',
    workspaceRoot: workspace, path, title: path.replaceAll('\\', '/').split('/').at(-1) || path
  })
  previewWorkspaceFile({ workspaceRoot: workspace, path })
}
