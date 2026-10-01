export function runBoundedDiagnosticChild(options: {
  command: string
  args: string[]
  cwd: string
  env: NodeJS.ProcessEnv
  timeoutMs: number
  maxBuffer: number
  allowedResidualCommand: (command: string) => boolean
}): Promise<{ status: number | null; signal: string | null; stdout: string; stderr: string; cleanup: null }>
