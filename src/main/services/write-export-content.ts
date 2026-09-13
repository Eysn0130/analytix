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

// Project prose after parsing. Image destinations remain private structure until
// the existing format-specific workspace checks and byte embedding complete.
// Rewriting an entire Markdown string can erase image syntax before those gates.
export function projectWriteExportMarkdownTreeV1(tree: MarkdownNode, projectText = projectOrdinaryWriteExportContentV1): void {
  const imageReferences = new Set<string>()
  const pending = [tree]
  const nodes: MarkdownNode[] = []
  while (pending.length) {
    const node = pending.pop()!
    nodes.push(node)
    if (node.type === 'imageReference' && node.identifier) imageReferences.add(node.identifier)
    if (node.children) pending.push(...node.children)
  }
  for (const node of nodes) {
    for (const key of ['value', 'alt', 'title', 'lang'] as const) {
      if (typeof node[key] === 'string') node[key] = projectText(node[key])
    }
    if (typeof node.url === 'string') {
      const imageTarget = node.type === 'image' || (node.type === 'definition' && !!node.identifier && imageReferences.has(node.identifier))
      if (!imageTarget && (node.type === 'link' || node.type === 'definition') && !isPublicDestination(node.url, projectText)) {
        // A projected target is display text, never replacement link authority.
        if (node.type === 'link' && !node.children?.length) {
          node.children = [{ type: 'text', value: '[PRIVATE_REFERENCE]' }]
        }
        node.url = ''
      } else if (imageTarget && /^(?:https?:|data:)/iu.test(node.url) && !isPublicDestination(node.url, projectText)) {
        throw new WriteExportPublicError('Write export image URL contains private content and was blocked.')
      }
    }
  }
}
