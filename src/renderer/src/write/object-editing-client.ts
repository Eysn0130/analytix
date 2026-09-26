import i18n from '../i18n'
import type { ObjectEditingResponse } from '../../../../packages/runtime/src/contracts/object-editing'

export function objectEditingFailure(response: ObjectEditingResponse): string {
  if (response.ok) return i18n.t('writeObjectUnexpected')
  if (response.code === 'conflict') return i18n.t('writeObjectConflict')
  if (response.code === 'session_invalid') return i18n.t('writeObjectSessionInvalid')
  if (response.code === 'too_large') return i18n.t('writeObjectTooLarge')
  if (response.code === 'forbidden') return i18n.t('writeObjectForbidden')
  if (response.receipt?.status === 'unknown' || response.receipt?.status === 'pending') return i18n.t('writeObjectUnknown')
  return i18n.t('writeObjectUnavailable')
}

export async function openTextObject(workspace: string, path: string) {
  const result = await window.analytix.objects.request({ action: 'open', workspace, path })
  if (!result.ok && result.code === 'unsupported_platform') {
    // Explicit Core platform readiness only. Storage/identity/permission errors
    // never fall back. Retain the existing manual editor on platforms whose
    // atomic replacement implementation has not yet been admitted.
    const legacy = await window.analytix.files.read({ workspaceRoot: workspace, path })
    if (!legacy.ok) throw new Error(legacy.message)
    return { ...legacy, sessionId: '', objectId: '', revision: '', legacy: true }
  }
  if (!result.ok || !('document' in result)) throw new Error(objectEditingFailure(result))
  return { ...result.document, legacy: false, truncated: false }
}
