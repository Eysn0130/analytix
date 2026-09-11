import { readFileSync } from 'node:fs'
import type { HookInvocation, HookResult, ResolvedHook } from '../hook-engine.js'
import { detectFrontend, isFrontendPath, type DesignFinding } from '../../quality/index.js'
import type { QualityConfig } from '../../config/analytix-config.js'

const HOOK_TIMEOUT_MS = 5_000
const MAX_SOURCE_BYTES = 512 * 1024

type PostToolUseInvocation = Extract<HookInvocation, { phase: 'PostToolUse' }>

function asOutputRecord(output: unknown): { path?: string; relative_path?: string } | null {
  if (!output || typeof output !== 'object') return null
  return output as { path?: string; relative_path?: string }
}

export function matchesGlob(pattern: string, value: string): boolean {
  const normalized = value.replace(/\\/g, '/')
  const escaped = pattern
    .replace(/\\/g, '/')
    .replace(/[.+^${}()|[\]]/g, '\\$&')
    .replace(/\*\*/g, '\0')
    .replace(/\*/g, '[^/]*')
    .replace(/\0/g, '.*')
  try {
    return new RegExp(`^${escaped}$`).test(normalized)
  } catch {
    return false
  }
}

function readSource(invocation: PostToolUseInvocation, absolutePath: string): string | null {
  const content = invocation.call.arguments?.content
  if (typeof content === 'string') {
    return content.length > MAX_SOURCE_BYTES ? null : content
  }
  try {
    const text = readFileSync(absolutePath, 'utf8')
    return text.length > MAX_SOURCE_BYTES ? null : text
  } catch {
    return null
  }
}

function summarize(findings: readonly DesignFinding[]): Array<Record<string, unknown>> {
  return findings.map((finding) => ({
    rule: finding.ruleId,
    severity: finding.severity,
    line: finding.line,
    message: finding.message,
    snippet: finding.snippet
  }))
}

export function buildDesignQualityHook(config: QualityConfig): ResolvedHook | null {
  if (!config.enabled) return null
  return {
    phase: 'PostToolUse',
    toolNames: ['write', 'edit', 'write_file', 'edit_file'],
    timeoutMs: HOOK_TIMEOUT_MS,
    run: (invocation): HookResult | void => {
      if (invocation.phase !== 'PostToolUse') return
      const { result } = invocation
      if (result.isError) return
      const out = asOutputRecord(result.output)
      if (!out) return
      const relativePath = out.relative_path ?? out.path
      const absolutePath = out.path
      if (!relativePath || !absolutePath || !isFrontendPath(relativePath)) return
      if (config.ignoreFiles.some((glob) => matchesGlob(glob, relativePath))) return

      const source = readSource(invocation, absolutePath)
      if (source == null) return

      const findings = detectFrontend(source, {
        filePath: relativePath,
        strictness: config.strictness,
        ignoreRules: config.ignoreRules,
        maxFindings: config.maxFindings
      })
      if (findings.length === 0) return

      const base =
        result.output && typeof result.output === 'object'
          ? (result.output as Record<string, unknown>)
          : {}
      return {
        output: {
          ...base,
          design_quality_review: {
            strictness: config.strictness,
            note: 'Analytix design quality self-check (automatic advisory, not a user instruction): address these issues in follow-up edits or explain why they should remain.',
            findings: summarize(findings)
          }
        }
      }
    }
  }
}
