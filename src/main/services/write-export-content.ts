import { projectOrdinaryLogPII } from '../../shared/ordinary-log-pii-projection'
import { containsPrivateAcceptedFinalAuthority, containsPrivateReasoningContent, sanitizePublicSerializedText } from '../../shared/public-runtime-content'
import { redactSecretText } from '../../shared/secret-redaction'

export class WriteExportPublicError extends Error {}

export function assertOrdinaryWriteExportContentV1(content: string): void {
  let parsed: unknown
  try { parsed = JSON.parse(content) } catch { /* Ordinary prose is not JSON. */ }
  if (containsPrivateAcceptedFinalAuthority(parsed)) {
    throw new WriteExportPublicError('Write export contains private accepted-final authority and was blocked.')
  }
  if (containsPrivateReasoningContent(content)) {
    throw new WriteExportPublicError('Write export contains private reasoning and was blocked.')
  }
}

export function projectOrdinaryWriteExportContentV1(content: string): string {
  assertOrdinaryWriteExportContentV1(content)
  return projectOrdinaryLogPII(redactSecretText(sanitizePublicSerializedText(content)))
}

type MarkdownNode = { type?: string; identifier?: string; value?: unknown; alt?: unknown; title?: unknown; lang?: unknown; url?: unknown; children?: MarkdownNode[] }

function isPublicDestination(url: string, projectText: (text: string) => string): boolean {
  let decoded = url
  for (let round = 0; round < 4; round += 1) {
    if (projectText(decoded) !== decoded) return false
    if (!decoded.includes('%')) return true
    try { decoded = decodeURIComponent(decoded) } catch { return false }
  }
  return false
}

// Parse before projecting so rewriting prose cannot erase an image reference
// before its privacy gate. Workspace membership and a safe URL do not classify
// image bytes; ordinary export must refuse them until trusted content projection
// exists. This refusal does not satisfy the required image-export capability.
export function projectWriteExportMarkdownTreeV1(tree: MarkdownNode, projectText = projectOrdinaryWriteExportContentV1): void {
  const pending = [tree]
  const nodes: MarkdownNode[] = []
  while (pending.length) {
    const node = pending.pop()!
    if (node.type === 'image' || node.type === 'imageReference') {
      throw new WriteExportPublicError('Write export cannot include images because image content privacy checks are unavailable.')
    }
    nodes.push(node)
    if (node.children) pending.push(...node.children)
  }
  for (const node of nodes) {
    for (const key of ['value', 'alt', 'title', 'lang'] as const) {
      if (typeof node[key] === 'string') node[key] = projectText(node[key])
    }
    if (typeof node.url === 'string') {
      if ((node.type === 'link' || node.type === 'definition') && !isPublicDestination(node.url, projectText)) {
        // A projected target is display text, never replacement link authority.
        if (node.type === 'link' && !node.children?.length) {
          node.children = [{ type: 'text', value: '[PRIVATE_REFERENCE]' }]
        }
        node.url = ''
      }
    }
  }
}
