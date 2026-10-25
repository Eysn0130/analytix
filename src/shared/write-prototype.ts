import { z } from 'zod'
import { toolResultLocalPresentFileRequestSchemaV1 } from '../../packages/runtime/src/contracts/tool-result-local-display'
import { isExplicitWriteResourceUrl } from './write-markdown-resource'

export const WRITE_PROTOTYPE_MAX_TEXT_CHARS = 6_000

/**
 * Default requirements for interactive prototype generation. Users can
 * override them via write.selectionAssist.prototypePrompt; the SDD assistant
 * turn prompt (sdd-prototype-prompt.ts) embeds them either way.
 */
export const WRITE_PROTOTYPE_DEFAULT_PROMPT = [
  'Build a single-file interactive HTML prototype for the product requirement.',
  'All CSS and JavaScript must be inline in the document; never reference local files.',
  'Make the prototype clickable and stateful where the requirement implies interaction (tabs, forms, lists, dialogs).',
  'Use a clean modern flat design with a light background, and keep every piece of interface text in the same language as the requirement.',
  'The page must render correctly inside a 480px-tall embedded frame and scale to wider viewports.'
].join(' ')

/** Whether an image src points at a local HTML document to embed inline. */
export function isHtmlEmbedSrc(src: string | undefined): boolean {
  if (!src) return false
  const value = src.trim()
  if (!value || isExplicitWriteResourceUrl(value) || value.startsWith('#')) return false
  if (/[?#]/.test(value)) return false
  return /\.html?$/i.test(value)
}


// Desktop HTML authorization is either an ordinary path or a sealed Present
// selector. The latter never accepts a path, workspace or document body.
const prototypePath = z.string().trim().min(1).max(4096)
export const writePrototypeAuthorizePayloadSchema = z.union([
  z.object({ path: prototypePath, workspaceRoot: prototypePath,
    mode: z.enum(['write-prototype', 'workspace-html']).optional() }).strict(),
  z.object({ protectedPresent: toolResultLocalPresentFileRequestSchemaV1,
    mode: z.literal('workspace-html') }).strict()
])
export type WritePrototypeAuthorizePayload = z.infer<typeof writePrototypeAuthorizePayloadSchema>
export const writePrototypeAuthorizeResultSchema = z.union([
  z.object({ ok: z.literal(true), absolutePath: prototypePath,
    fileUrl: z.string().max(16384), grantId: z.string().regex(/^[a-f0-9]{48}$/),
    partition: z.string().max(128) }).strict().refine(value => {
      try {
        const url = new URL(value.fileUrl)
        return url.protocol === 'analytix-prototype:' && url.hostname === value.grantId &&
          !url.username && !url.password && !url.port && !url.search && !url.hash &&
          value.partition === 'analytix-proto-' + value.grantId
      } catch { return false }
    }),
  z.object({ ok: z.literal(false), message: z.string().min(1).max(4096) }).strict()
])
export type WritePrototypeAuthorizeResult = z.infer<typeof writePrototypeAuthorizeResultSchema>
