import { z } from 'zod'
import { HOOK_PHASES, type HookPhase } from './hook-phases.js'

/**
 * Command hook entry as written in `config.json` under the top-level
 * `hooks` key. Only command hooks are configurable from JSON; function
 * hooks are reserved for embedders that assemble the runtime in code.
 */
export const HookCommandConfigSchema = z
  .object({
    phase: z.enum(HOOK_PHASES),
    /** Glob matched against the tool name (`*` wildcard, `|` alternation). Tool phases only. */
    matcher: z.string().min(1).optional(),
    /** Exact tool-name list; matches when either this or `matcher` matches. Tool phases only. */
    toolNames: z.array(z.string().min(1)).optional(),
    /** Shell command. Receives the invocation as JSON on stdin. */
    command: z.string().min(1),
    /** Working directory; defaults to the active workspace. */
    cwd: z.string().min(1).optional(),
    timeoutMs: z.number().int().positive().optional()
  })
  .strict()

export const HooksConfigSchema = z.array(HookCommandConfigSchema)

export type HookCommandConfig = z.infer<typeof HookCommandConfigSchema>
export type HooksConfig = z.infer<typeof HooksConfigSchema>
export type ResolvedCommandHook = {
  phase: HookPhase
  matcher?: string
  toolNames?: readonly string[]
  timeoutMs?: number
  command: string
  cwd?: string
}

/** Map validated config entries onto runnable command hooks. */
export function resolveConfiguredHooks(config: HooksConfig | undefined): ResolvedCommandHook[] {
  return (config ?? []).map((entry) => ({
    phase: entry.phase,
    ...(entry.matcher ? { matcher: entry.matcher } : {}),
    ...(entry.toolNames ? { toolNames: entry.toolNames } : {}),
    ...(entry.timeoutMs ? { timeoutMs: entry.timeoutMs } : {}),
    command: entry.command,
    ...(entry.cwd ? { cwd: entry.cwd } : {})
  }))
}
