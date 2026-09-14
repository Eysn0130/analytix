import { generatedArtifactResponseSchema } from '../../../../packages/runtime/src/contracts/generated-artifact'
import { useChatStore } from '../store/chat-store'
import { useWriteWorkspaceStore } from '../write/write-workspace-store'
import { useNativeOfficeStore } from './native-office-store'

let latestOpen = 0

/** Resolve an opaque receipt through Core only when the user opens its object. */
export async function openGeneratedArtifact(
  artifactId: string,
  expectedScope?: { threadId: string; workspace: string }
): Promise<boolean> {
  const { activeThreadId: threadId, workspaceRoot: workspace } = useChatStore.getState()
  if (!threadId || !workspace) return false
  if (expectedScope && (expectedScope.threadId !== threadId || expectedScope.workspace !== workspace)) return false
  const sequence = ++latestOpen
  const isCurrent = () => sequence === latestOpen &&
    useChatStore.getState().activeThreadId === threadId &&
    useChatStore.getState().workspaceRoot === workspace
  try {
    const response = generatedArtifactResponseSchema.safeParse(
      await window.analytix.objects.resolveArtifact({ threadId, artifactId })
    )
    if (!isCurrent() || !response.success || !response.data.ok) return false
    const artifact = response.data.artifact
    if (artifact.threadId !== threadId || artifact.artifactId !== artifactId || artifact.workspace !== workspace) return false
    if (!(await useWriteWorkspaceStore.getState().flushSave(workspace)) || !isCurrent()) return false
    if (!(await useNativeOfficeStore.getState().select(workspace, artifact.path, isCurrent)) || !isCurrent()) return false
    await useChatStore.getState().openWrite()
    return isCurrent()
  } catch {
    return false
  }
}
