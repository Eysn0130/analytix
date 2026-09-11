import { createHash } from 'node:crypto'
import type { ModelToolSpec } from '../ports/model-client.js'

export type ToolCatalogFingerprint = {
  fingerprint: string
  toolCount: number
  toolNames: string[]
  toolHashes: Record<string, string>
}

export function buildToolCatalogFingerprint(
  tools: readonly ModelToolSpec[]
): ToolCatalogFingerprint {
  const canonicalTools = normalizeToolSpecs(tools)
  return {
    fingerprint: hashObject(canonicalTools),
    toolCount: canonicalTools.length,
    toolNames: canonicalTools.map((tool) => tool.name),
    toolHashes: Object.fromEntries(canonicalTools.map((tool) => [tool.name, hashObject(tool)]))
  }
}

function normalizeToolSpecs(tools: readonly ModelToolSpec[]): Array<{
  name: string
  description: string
  inputSchema: Record<string, unknown>
}> {
  return [...tools]
    .map((tool) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: canonicalizeSchema(tool.inputSchema)
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

export function canonicalizeToolInputSchema(value: unknown): Record<string, unknown> {
  const canonical = canonicalizeSchemaValue('', value)
  return canonical && typeof canonical === 'object' && !Array.isArray(canonical)
    ? canonical as Record<string, unknown>
    : { type: 'object' }
}

function canonicalizeSchema(value: unknown): Record<string, unknown> {
  return canonicalizeToolInputSchema(value)
}

function canonicalizeSchemaValue(parentKey: string, value: unknown): unknown {
  if (Array.isArray(value)) {
    const out = value.map((child) => canonicalizeSchemaValue('', child))
    if (isCanonicalSchemaSetField(parentKey) && out.every(isScalarSchemaValue)) {
      out.sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)))
    }
    return out
  }
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const key of Object.keys(value as Record<string, unknown>).sort()) {
    const child = (value as Record<string, unknown>)[key]
    if (key === 'required') {
      const required = canonicalStringSetArray(child)
      if (required.length > 0) out[key] = required
      continue
    }
    if (key === 'dependentRequired') {
      const dependentRequired = canonicalDependentRequired(child)
      if (Object.keys(dependentRequired).length > 0) out[key] = dependentRequired
      continue
    }
    out[key] = canonicalizeSchemaValue(key, child)
  }
  return out
}

function canonicalDependentRequired(value: unknown): Record<string, string[]> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const out: Record<string, string[]> = {}
  for (const key of Object.keys(value as Record<string, unknown>).sort()) {
    const required = canonicalStringSetArray((value as Record<string, unknown>)[key])
    if (required.length > 0) out[key] = required
  }
  return out
}

function canonicalStringSetArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const child of value) {
    if (typeof child !== 'string' || seen.has(child)) continue
    seen.add(child)
    out.push(child)
  }
  return out.sort((a, b) => a.localeCompare(b))
}

function isCanonicalSchemaSetField(key: string): boolean {
  return ['required', 'enum', 'type'].includes(key.trim().toLowerCase())
}

function isScalarSchemaValue(value: unknown): boolean {
  return value === null || ['string', 'number', 'boolean'].includes(typeof value)
}

function hashObject(value: unknown): string {
  return createHash('sha256').update(JSON.stringify(value)).digest('hex').slice(0, 16)
}
