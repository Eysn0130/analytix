import {
  lstatSync,
  realpathSync,
  statSync,
  type Stats
} from 'node:fs'
import {
  delimiter,
  dirname,
  isAbsolute,
  join,
  normalize,
  parse,
  relative,
  resolve,
  sep
} from 'node:path'

const DARWIN_SANDBOX_EXECUTABLE = '/usr/bin/sandbox-exec'
const DARWIN_DNS_SOCKET = '/private/var/run/mDNSResponder'
const MAX_PROTECTED_ROOTS = 32
const MAX_PATH_BYTES = 16 * 1024

const TERMINAL_ENV_ALLOWLIST = [
  'HOME',
  'USER',
  'LOGNAME',
  'SHELL',
  'PATH',
  'LANG',
  'LC_ALL',
  'LC_CTYPE',
  'TZ',
  'TERMINFO',
  'TERMINFO_DIRS',
  'TMPDIR',
  'TEMP',
  'TMP'
] as const

export const TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE =
  'The terminal is unavailable because process containment could not be established.'

export type TerminalProcessLaunch = Readonly<{
  file: string
  args: readonly string[]
  cwd: string
  env: NodeJS.ProcessEnv
}>

export type PrepareTerminalProcessLaunchInput = Readonly<{
  shellFile: string
  shellArgs: readonly string[]
  cwd: string
  protectedRoots: readonly string[]
  env?: NodeJS.ProcessEnv
  platform?: NodeJS.Platform
  sandboxExecutable?: string
}>

/**
 * Produces the one process boundary used by the Electron PTY. The profile is
 * intentionally equivalent to the Go process sandbox: ordinary workspace,
 * child-process, and remote-network work remains available, while protected
 * host roots and local deputy transports are denied.
 *
 * The returned argv contains private enforcement paths and must never be
 * logged or projected to the renderer.
 */
export function prepareTerminalProcessLaunch(
  input: PrepareTerminalProcessLaunchInput
): TerminalProcessLaunch {
  try {
    const platform = input.platform ?? process.platform
    if (platform !== 'darwin') throw fixedContainmentError()
    const sandboxExecutable = input.sandboxExecutable ?? DARWIN_SANDBOX_EXECUTABLE
    if (!trustedDarwinSandboxExecutable(sandboxExecutable)) {
      throw fixedContainmentError()
    }
    const protectedRoots = normalizeProtectedRoots(input.protectedRoots)
    if (protectedRoots.length === 0) throw fixedContainmentError()
    const cwd = canonicalExistingDirectory(input.cwd)
    const shellFile = canonicalExecutable(input.shellFile)
    if (
      protectedRoots.some((root) =>
        pathWithinRoot(root, cwd) || pathWithinRoot(root, shellFile)
      )
    ) {
      throw fixedContainmentError()
    }
    const shellArgs = input.shellArgs.map(validArgument)
    const profile = buildDarwinTerminalSandboxProfile(protectedRoots)
    const environment = buildTerminalEnvironment(input.env ?? process.env, protectedRoots)
    environment.SHELL = shellFile
    environment.HISTFILE = '/dev/null'
    return Object.freeze({
      file: sandboxExecutable,
      args: Object.freeze(['-p', profile, shellFile, ...shellArgs]),
      cwd,
      env: Object.freeze(environment)
    })
  } catch {
    throw fixedContainmentError()
  }
}

export function defaultDarwinTerminalProtectedRoots(homeDirectory: string): string[] {
  const home = validAbsolutePath(homeDirectory)
  return [
    join(home, 'Music'),
    join(home, 'Pictures'),
    join(home, 'Movies'),
    join(home, 'Library')
  ]
}

export function buildDarwinTerminalSandboxProfile(
  protectedRoots: readonly string[]
): string {
  const roots = normalizeProtectedRoots(protectedRoots)
  if (roots.length === 0) throw fixedContainmentError()
  let profile = '(version 1)\n'
  profile += '(allow default)\n'
  profile += '(deny process-info*)\n'
  profile += '(allow process-info* (target self))\n'
  profile += '(deny appleevent-send)\n'
  profile += '(deny job-creation lsopen system-socket)\n'
  profile += '(deny mach-bootstrap mach-lookup mach-register mach-per-user-lookup mach-cross-domain-lookup)\n'
  profile += '(deny mach-task-name mach-task-read mach-task-special-port* mach-priv-host-port)\n'
  profile += '(deny ipc-posix* ipc-sysv*)\n'
  profile += '(deny network-outbound (remote unix-socket))\n'
  profile += '(deny network-outbound (remote ip "localhost:*"))\n'
  profile += `(allow network-outbound (remote unix-socket (path-literal ${quoteSeatbeltString(DARWIN_DNS_SOCKET)})))\n`
  for (const root of roots) {
    profile += `(deny file-read* file-write* process-exec (subpath ${quoteSeatbeltString(root)}))\n`
  }
  return profile
}

export function buildTerminalEnvironment(
  source: NodeJS.ProcessEnv,
  protectedRoots: readonly string[]
): NodeJS.ProcessEnv {
  const roots = normalizeProtectedRoots(protectedRoots)
  const environment: NodeJS.ProcessEnv = {}
  for (const name of TERMINAL_ENV_ALLOWLIST) {
    const value = source[name]
    if (!validEnvironmentValue(value)) continue
    if (name === 'PATH' || name === 'TERMINFO_DIRS') {
      const paths = value
        .split(delimiter)
        .filter((candidate) => safeEnvironmentPath(candidate, roots))
      if (paths.length > 0) environment[name] = paths.join(delimiter)
      continue
    }
    if (
      ['HOME', 'SHELL', 'TERMINFO', 'TMPDIR', 'TEMP', 'TMP'].includes(name) &&
      !safeEnvironmentPath(value, roots)
    ) {
      continue
    }
    environment[name] = value
  }
  if (!environment.PATH) environment.PATH = '/usr/bin:/bin:/usr/sbin:/sbin'
  environment.TERM = 'xterm-256color'
  environment.COLORTERM = 'truecolor'
  return environment
}

function safeEnvironmentPath(value: string, protectedRoots: readonly string[]): boolean {
  if (!isAbsolute(value)) return false
  try {
    const canonical = canonicalPotentialPath(value)
    return !protectedRoots.some((root) => pathWithinRoot(root, canonical))
  } catch {
    return false
  }
}

export function normalizeProtectedRoots(values: readonly string[]): string[] {
  if (!Array.isArray(values) || values.length === 0 || values.length > MAX_PROTECTED_ROOTS) {
    throw fixedContainmentError()
  }
  const roots = Array.from(new Set(values.map(canonicalPotentialPath))).sort()
  return roots.filter((candidate, index) =>
    !roots.some((root, otherIndex) =>
      otherIndex !== index && pathWithinRoot(root, candidate)
    )
  )
}

export function pathWithinRoot(root: string, candidate: string): boolean {
  const rel = relative(root, candidate)
  return rel === '' ||
    (rel !== '..' && !rel.startsWith(`..${sep}`) &&
      !isAbsolute(rel))
}

function canonicalPotentialPath(value: string): string {
  const exact = validAbsolutePath(value)
  let existing = exact
  const suffix: string[] = []
  while (true) {
    try {
      const existingRealPath = realpathSync.native(existing)
      return resolve(existingRealPath, ...suffix)
    } catch {
      const parent = dirname(existing)
      if (parent === existing) throw fixedContainmentError()
      suffix.unshift(existing.slice(parent.length + (parent.endsWith('/') ? 0 : 1)))
      existing = parent
    }
  }
}

function canonicalExistingDirectory(value: string): string {
  const exact = validAbsolutePath(value)
  const canonical = realpathSync.native(exact)
  const state = statSync(canonical)
  if (!state.isDirectory()) throw fixedContainmentError()
  return canonical
}

function canonicalExecutable(value: string): string {
  const exact = validAbsolutePath(value)
  const canonical = realpathSync.native(exact)
  const state = statSync(canonical)
  if (!state.isFile() || (state.mode & 0o111) === 0) throw fixedContainmentError()
  return canonical
}

function trustedDarwinSandboxExecutable(value: string): boolean {
  if (value !== DARWIN_SANDBOX_EXECUTABLE) return false
  let state: Stats
  try {
    state = lstatSync(value)
  } catch {
    return false
  }
  return state.isFile() &&
    !state.isSymbolicLink() &&
    state.uid === 0 &&
    (state.mode & 0o111) !== 0 &&
    realpathSync.native(value) === value
}

function validAbsolutePath(value: string): string {
  if (
    typeof value !== 'string' ||
    value === '' ||
    value !== value.trim() ||
    Buffer.byteLength(value, 'utf8') > MAX_PATH_BYTES ||
    !isAbsolute(value) ||
    normalize(value) !== value ||
    resolve(value) !== value ||
    value === parse(value).root ||
    hasUnsupportedControl(value)
  ) {
    throw fixedContainmentError()
  }
  return value
}

function validArgument(value: string): string {
  if (typeof value !== 'string' || value.includes('\0')) throw fixedContainmentError()
  return value
}

function validEnvironmentValue(value: string | undefined): value is string {
  return typeof value === 'string' &&
    Buffer.byteLength(value, 'utf8') <= MAX_PATH_BYTES &&
    !value.includes('\0') &&
    !value.includes('\n') &&
    !value.includes('\r')
}

function hasUnsupportedControl(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (code < 0x20 || code === 0x7f) return true
  }
  return false
}

function quoteSeatbeltString(value: string): string {
  return `"${value.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"`
}

function fixedContainmentError(): Error {
  return new Error(TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE)
}
