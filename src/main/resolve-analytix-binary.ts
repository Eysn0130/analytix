import { existsSync } from 'node:fs'
import { join } from 'node:path'

/**
 * Resolve the Analytix launcher. `packages/runtime` now ships only a
 * small Node.js compatibility shim for `analytix serve`; the actual agent
 * runtime is the Go `runtime-server`.
 *
 * Resolution order:
 * 1. Bundled `packages/runtime/dist/cli/serve-entry.js` (built by the root
 *    `build:runtime` script before dev, build, install, and packaging).
 *
 * User-supplied runtime executable paths are retired with the TypeScript
 * agent runtime. The resolver deliberately ignores `userBinaryPath` so
 * production, rollback, and diagnostic paths cannot switch away from the
 * bundled Go runtime boundary.
 */
export type AnalytixBinaryResolution = {
  kind: 'node-script'
  command: string
  args: string[]
  dataDir: string
}

const DIST_ENTRY_CANDIDATES = [
  'packages/runtime/dist/cli/serve-entry.js',
  'packages/runtime/dist/cli/serve.js'
]

function exists(path: string): boolean {
  try {
    return existsSync(path)
  } catch {
    return false
  }
}

export function resolveAnalytixExecutable(
  appRoot: string,
  userBinaryPath: string
): AnalytixBinaryResolution {
  void userBinaryPath
  for (const candidate of DIST_ENTRY_CANDIDATES) {
    const full = join(appRoot, candidate)
    if (exists(full)) {
      return {
        kind: 'node-script',
        command: process.execPath,
        args: [full],
        dataDir: ''
      }
    }
  }
  return {
    kind: 'node-script',
    command: process.execPath,
    args: [join(appRoot, DIST_ENTRY_CANDIDATES[0])],
    dataDir: ''
  }
}

/**
 * Build the full `analytix serve` argv from resolved binary info
 * and Analytix runtime settings. The function is pure: no I/O, no
 * side effects, easy to test.
 */
export function buildAnalytixServeArgs(input: {
  resolution: AnalytixBinaryResolution
  host: string
  port: number
  dataDir: string
  baseUrl?: string
  modelProxyUrl?: string
  mcpProxyUrl?: string
  endpointFormat?: string
  model: string
  approvalPolicy: string
  sandboxMode: string
  tokenEconomyMode: boolean
  insecure: boolean
}): string[] {
  return [
    ...input.resolution.args,
    '--host',
    input.host,
    '--port',
    String(input.port),
    '--data-dir',
    input.dataDir,
    ...(input.baseUrl ? ['--base-url', input.baseUrl] : []),
    ...(input.modelProxyUrl ? ['--model-proxy-url', input.modelProxyUrl] : []),
    ...(input.mcpProxyUrl ? ['--mcp-proxy-url', input.mcpProxyUrl] : []),
    ...(input.endpointFormat ? ['--endpoint-format', input.endpointFormat] : []),
    '--model',
    input.model,
    '--approval-policy',
    input.approvalPolicy,
    '--sandbox-mode',
    input.sandboxMode,
    '--token-economy-mode',
    input.tokenEconomyMode ? 'true' : 'false',
    ...(input.insecure ? ['--insecure'] : [])
  ]
}
