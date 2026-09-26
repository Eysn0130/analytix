import type { BrowserSelectionRequest, BrowserSelectionResponse } from '../../../../packages/runtime/src/contracts/browser-selection'
import type { NativeReference } from '../office/native-reference-store'

export async function validateBrowserReferences(references: readonly NativeReference[], request: (request: BrowserSelectionRequest) => Promise<BrowserSelectionResponse>, current: () => boolean): Promise<boolean> {
  for (const reference of references) {
    if (reference.kind !== 'browser-selection') continue
    if (!current() || reference.scopeId !== reference.browserScope.scopeId || reference.threadId !== reference.browserScope.threadId ||
      reference.workspace !== reference.browserScope.workspace || reference.objectId !== reference.browserScope.documentId || reference.revision !== reference.browserScope.selectionId) return false
    try { if (!(await request({ action: 'validate', scope: reference.browserScope })).ok || !current()) return false } catch { return false }
  }
  return current()
}
