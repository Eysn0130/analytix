import { z } from 'zod'

export const MemoryScope = z.enum(['user', 'workspace', 'project'])
export type MemoryScope = z.infer<typeof MemoryScope>

function isManualMemoryTextAdmissible(content: string): boolean {
  const lower = content.toLowerCase()
  return !/cer1_[a-p]{64}/.test(content) &&
    !/(?:^|[^a-z0-9_:])(?:acct|card):[a-z0-9]+(?:$|[^a-z0-9_:])/i.test(content) &&
    !containsCompleteFinancialAccountCandidate(content) &&
    !/(?:^|[\s"'=(])\/(?:users|volumes|private|var|tmp|home|cases?)\//i.test(content) &&
    !/(?:^|[\s"'=(])[a-z]:\\[^\s"']+/i.test(content) &&
    !(content.includes('{') && [
      '"structuredcontent"', '"rawresult"', '"hostonly"', '"toolcallid"',
      '"subjectref"', '"sourcefileid"', '"sourcerownumber"'
    ].some((marker) => lower.includes(marker)))
}

function containsCompleteFinancialAccountCandidate(content: string): boolean {
  for (const match of content.matchAll(/[0-9](?:[0-9 -]*[0-9])?/g)) {
    for (const source of match[0].split(/[ -]{2,}/)) {
      if (!/^[0-9](?:[ -]?[0-9])*$/.test(source)) continue
      const canonical = source.replace(/[ -]/g, '')
      if (canonical.length >= 8 && canonical.length <= 32) return true
    }
  }
  return false
}

const MemoryContent = z.string().trim().min(1).max(65_536)
  .refine(isManualMemoryTextAdmissible, 'memory content is not admissible')

const MemoryTag = z.string().trim().min(1).max(256)
  .refine(isManualMemoryTextAdmissible, 'memory tag is not admissible')

export const ActiveMemoryRecord = z.object({
  id: z.string().min(1),
  content: MemoryContent,
  scope: MemoryScope,
  workspace: z.string().optional(),
  project: z.string().optional(),
  provenance: z.literal('manual-general'),
  captureMode: z.literal('manual'),
  modelInjection: z.literal(false),
  tags: z.array(MemoryTag).max(64).default([]),
  confidence: z.number().min(0).max(1).default(1),
  createdAt: z.string(),
  updatedAt: z.string(),
  disabledAt: z.string().optional()
}).strict()
export type ActiveMemoryRecord = z.infer<typeof ActiveMemoryRecord>

export const MemoryTombstone = z.object({
  id: z.string().min(1),
  createdAt: z.string(),
  updatedAt: z.string(),
  deletedAt: z.string()
}).strict()
export type MemoryTombstone = z.infer<typeof MemoryTombstone>

export const MemoryRecord = z.union([ActiveMemoryRecord, MemoryTombstone])
export type MemoryRecord = z.infer<typeof MemoryRecord>

export const MemoryCreateRequest = z.object({
  content: MemoryContent,
  scope: MemoryScope.default('workspace'),
  workspace: z.string().optional(),
  project: z.string().optional(),
  tags: z.array(MemoryTag).max(64).default([]),
  confidence: z.number().min(0).max(1).default(1)
}).strict()
export type MemoryCreateRequest = z.input<typeof MemoryCreateRequest>

export const MemoryUpdateRequest = z.object({
  content: MemoryContent.optional(),
  tags: z.array(MemoryTag).max(64).optional(),
  confidence: z.number().min(0).max(1).optional(),
  disabled: z.boolean().optional()
}).strict()
export type MemoryUpdateRequest = z.input<typeof MemoryUpdateRequest>

export const MemoryDiagnostics = z.object({
  enabled: z.boolean(),
  rootDir: z.string(),
  activeCount: z.number().int().nonnegative(),
  tombstoneCount: z.number().int().nonnegative(),
  lastInjectedIds: z.array(z.string()).default([])
    .refine((ids) => ids.length === 0, 'model injection is disabled'),
  admittedProvenance: z.literal('manual-general'),
  captureMode: z.literal('manual'),
  modelInjection: z.literal(false)
}).strict()
export type MemoryDiagnostics = z.infer<typeof MemoryDiagnostics>
