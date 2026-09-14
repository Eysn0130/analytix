import { z } from 'zod'

/** Main-owned local transport; neither plugin frames nor model tools receive it. */
export const pluginPackageHostPath = '/v1/local-display/plugin-package-host'
const packageId = z.enum(['analytix-documents', 'analytix-spreadsheets', 'analytix-presentations'])
const digest = z.string().regex(/^[a-f0-9]{64}$/)
const revision = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const operation = z.string().regex(/^[a-z][a-z0-9_.-]{0,63}$/)

export const pluginPackageViewSchema = z.object({
  packageId, packageVersion: z.string().min(1), displayName: z.string().min(1),
  origin: z.literal('development-source'), publishable: z.literal(false),
  materialized: z.boolean(), generationId: z.union([digest, z.literal('')]),
  activationState: z.enum(['unset', 'recorded', 'unavailable']),
  desiredState: z.enum(['enabled', 'disabled']).optional(), activationRevision: revision,
  activationId: digest.optional(), available: z.boolean(), unavailableReason: z.string().optional(),
  operations: z.array(operation).max(18)
}).strict().superRefine((value, ctx) => {
  if ((value.materialized && !digest.safeParse(value.generationId).success) ||
      (value.activationState === 'recorded' && (!value.desiredState || !value.activationId || value.activationRevision === 0)) ||
      (value.activationState === 'unset' && (value.desiredState !== undefined || value.activationId !== undefined || value.activationRevision !== 0)) ||
      (value.available && (!value.materialized || value.activationState !== 'recorded' || value.desiredState !== 'enabled' || value.operations.length === 0)) ||
      new Set(value.operations).size !== value.operations.length) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Inconsistent package state.' })
  }
})

export const pluginPackageHostRequestSchema = z.discriminatedUnion('action', [
  z.object({ action: z.literal('list') }).strict(),
  z.object({ action: z.literal('setDesiredState'), packageId, generationId: digest,
    expectedRevision: revision.max(Number.MAX_SAFE_INTEGER - 1), desiredState: z.enum(['enabled', 'disabled']) }).strict(),
  z.object({ action: z.literal('invoke'), packageId, generationId: digest,
    expectedRevision: revision.positive(), contributionId: z.literal('workspace-editor'), operation,
    input: z.record(z.string(), z.unknown()) }).strict()
])

export const pluginPackageHostResponseSchema = z.union([
  z.object({ ok: z.literal(true), packages: z.array(pluginPackageViewSchema).max(3) }).strict(),
  z.object({ ok: z.literal(true), package: pluginPackageViewSchema }).strict(),
  z.object({ ok: z.literal(true), output: z.unknown() }).strict(),
  z.object({ ok: z.literal(false), code: z.enum([
    'unavailable', 'invalid_request', 'identity_invalid',
    'package_not_found', 'conflict', 'disabled',
    'adapter_unavailable', 'persistence_failure'
  ]), message: z.string(), relist: z.boolean().optional() }).strict()
])

export type PluginPackageView = z.infer<typeof pluginPackageViewSchema>
export type PluginPackageHostRequest = z.infer<typeof pluginPackageHostRequestSchema>
export type PluginPackageHostResponse = z.infer<typeof pluginPackageHostResponseSchema>
