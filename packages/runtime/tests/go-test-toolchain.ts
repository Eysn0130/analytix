import { spawn } from 'node:child_process'
import { access } from 'node:fs/promises'
import { delimiter, join } from 'node:path'

export function goTestEnv(extra: NodeJS.ProcessEnv = {}): NodeJS.ProcessEnv {
  const { GOROOT: _ignored, ...env } = process.env
  return { ...env, ...extra }
}

export async function findGoBinary(): Promise<string> {
  const candidates = uniqueStrings([
    process.env.ANALYTIX_GO_BIN,
    ...goPathCandidates(),
    '/usr/local/go/bin/go',
    '/opt/homebrew/bin/go',
    '/tmp/analytix-go-toolchain/go/bin/go'
  ].filter((value): value is string => Boolean(value)))

  for (const candidate of candidates) {
    if (await goBinaryLooksUsable(candidate)) {
      return candidate
    }
  }
  throw new Error('Go binary not found; set ANALYTIX_GO_BIN or install a complete analytix Go toolchain')
}

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values)]
}

function goPathCandidates(): string[] {
  const binary = process.platform === 'win32' ? 'go.exe' : 'go'
  return (process.env.PATH ?? '')
    .split(delimiter)
    .filter(Boolean)
    .map((entry) => join(entry, binary))
}

async function goBinaryLooksUsable(candidate: string): Promise<boolean> {
  try {
    await access(candidate)
  } catch {
    return false
  }
  const goroot = await goEnvGOROOT(candidate)
  if (!goroot) return false
  try {
    await access(join(goroot, 'src', 'context', 'context.go'))
    return true
  } catch {
    return false
  }
}

async function goEnvGOROOT(candidate: string): Promise<string | null> {
  const result = spawn(candidate, ['env', 'GOROOT'], {
    env: goTestEnv(),
    stdio: ['ignore', 'pipe', 'ignore']
  })
  let stdout = ''
  result.stdout.setEncoding('utf8')
  result.stdout.on('data', (chunk) => {
    stdout += String(chunk)
  })
  const exited = await new Promise<boolean>((resolve) => {
    result.once('error', () => resolve(false))
    result.once('exit', (code) => resolve(code === 0))
  })
  if (!exited) return null
  return stdout.trim() || null
}
