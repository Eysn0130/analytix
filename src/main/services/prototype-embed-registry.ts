import { stat } from 'node:fs/promises'
import { resolveTargetPathWithinWorkspace } from './workspace-paths'
import { writePathToFileUrl } from '../../shared/write-markdown-resource'

/**
 * Authorization gate for embedded prototype webviews.
 *
 * `will-attach-webview` rejects every webview src except dev-preview URLs, so
 * file:// prototypes must be allow-listed first: the renderer asks to
 * authorize a concrete document path, the main process validates it
 * (workspace containment with symlink canonicalization, a `proto` directory
 * segment, an .html extension) and records the resulting file URL. The guard
 * then admits exactly those URLs for the rest of the session.
 */

const MAX_AUTHORIZED_PROTOTYPES = 256

const authorizedFileUrls = new Set<string>()

export type AuthorizePrototypeResult =
  | { ok: true; absolutePath: string; fileUrl: string }
  | { ok: false; message: string }

function hasPrototypeDirSegment(path: string): boolean {
  return path.replaceAll('\\', '/').split('/').includes('proto')
}

export async function authorizePrototypePath(
  path: string,
  workspaceRoot: string
): Promise<AuthorizePrototypeResult> {
  if (!/\.html?$/i.test(path.trim())) {
    return { ok: false, message: 'only .html prototypes can be embedded' }
  }
  let absolutePath: string
  try {
    // Canonicalizes (symlinks resolved) and throws when the target escapes
    // the workspace root.
    absolutePath = await resolveTargetPathWithinWorkspace(path, workspaceRoot)
  } catch (error) {
    return { ok: false, message: error instanceof Error ? error.message : String(error) }
  }
  if (!hasPrototypeDirSegment(absolutePath)) {
    return { ok: false, message: 'prototypes must live in a proto directory' }
  }
  try {
    const info = await stat(absolutePath)
    if (!info.isFile()) return { ok: false, message: 'prototype file not found' }
  } catch {
    return { ok: false, message: 'prototype file not found' }
  }
  const fileUrl = writePathToFileUrl(absolutePath)
  if (!authorizedFileUrls.has(fileUrl) && authorizedFileUrls.size >= MAX_AUTHORIZED_PROTOTYPES) {
    const oldest = authorizedFileUrls.values().next().value
    if (oldest) authorizedFileUrls.delete(oldest)
  }
  authorizedFileUrls.add(fileUrl)
  return { ok: true, absolutePath, fileUrl }
}

export function isAuthorizedPrototypeFileUrl(src: string): boolean {
  if (!src.startsWith('file://')) return false
  return authorizedFileUrls.has(src.split(/[?#]/, 1)[0])
}

export function clearAuthorizedPrototypes(): void {
  authorizedFileUrls.clear()
}
