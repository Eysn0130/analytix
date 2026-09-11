// Adapted from DeepSeek-Reasonix internal/shellsafe/shellsafe.go.
// Analytix uses this as an approval signal only; execution policy remains owned
// by runtime approvals, sandbox policy, and hooks.

const READ_ONLY_COMMANDS = new Set([
  'cat', 'head', 'tail', 'less', 'more',
  'ls', 'find', 'locate', 'which', 'whereis', 'type',
  'grep', 'egrep', 'fgrep', 'rg',
  'echo', 'printf',
  'pwd', 'cd', 'whoami', 'id', 'uname', 'hostname',
  'date', 'printenv',
  'wc', 'sort', 'uniq', 'cut', 'tr',
  'stat', 'file', 'du', 'df',
  'ps', 'top', 'htop',
  'diff', 'cmp', 'comm',
  'man', 'info', 'help',
  'true', 'false', 'test', '[',
  'basename', 'dirname', 'realpath', 'readlink'
])

const READ_ONLY_PREFIXES: Record<string, ReadonlySet<string>> = {
  git: new Set([
    'log', 'status', 'diff', 'show', 'tag', 'blame', 'grep', 'ls-files', 'ls-tree',
    'rev-parse', 'rev-list', 'describe', 'reflog', 'shortlog', 'whatchanged',
    'cherry', 'cat-file', 'for-each-ref', 'name-rev'
  ]),
  go: new Set(['vet', 'doc', 'list', 'version', 'env']),
  npm: new Set(['ls', 'list', 'view', 'info', 'outdated', 'audit']),
  cargo: new Set(['check', 'doc', 'search']),
  docker: new Set(['ps', 'images', 'inspect', 'logs', 'stats', 'info', 'version']),
  kubectl: new Set(['get', 'describe', 'logs', 'explain', 'api-resources', 'api-versions']),
  node: new Set(['-v', '--version']),
  python: new Set(['--version', '-v']),
  python3: new Set(['--version', '-v'])
}

export type ShellCommandSafety = {
  command: string
  base?: string
  subcommand?: string
  readOnly: boolean
  reason: 'empty' | 'shell_syntax' | 'read_only_command' | 'read_only_prefix' | 'unknown_or_write_capable'
}

export function containsShellSyntax(command: string): boolean {
  return /[;|&<>\n\r`]/.test(command) || command.includes('$(')
}

export function classifyShellCommandSafety(command: string): ShellCommandSafety {
  const trimmed = command.trim()
  if (!trimmed) return { command: trimmed, readOnly: false, reason: 'empty' }
  if (containsShellSyntax(trimmed)) {
    return { command: trimmed, readOnly: false, reason: 'shell_syntax' }
  }
  const parts = trimmed.split(/\s+/).filter(Boolean)
  const base = parts[0]?.toLowerCase()
  if (!base) return { command: trimmed, readOnly: false, reason: 'empty' }
  if (READ_ONLY_COMMANDS.has(base)) {
    return { command: trimmed, base, readOnly: true, reason: 'read_only_command' }
  }
  const subcommand = parts[1]?.toLowerCase()
  if (subcommand && READ_ONLY_PREFIXES[base]?.has(subcommand)) {
    return { command: trimmed, base, subcommand, readOnly: true, reason: 'read_only_prefix' }
  }
  return {
    command: trimmed,
    base,
    ...(subcommand ? { subcommand } : {}),
    readOnly: false,
    reason: 'unknown_or_write_capable'
  }
}

export function bashApprovalSafetySummary(args: Record<string, unknown>): string | null {
  const action = typeof args.action === 'string' ? args.action.trim() : ''
  if (action && action !== 'run') return null
  const command = typeof args.command === 'string' ? args.command : ''
  const safety = classifyShellCommandSafety(command)
  if (safety.reason === 'empty') return null
  if (safety.readOnly) {
    const subject = safety.subcommand ? `${safety.base} ${safety.subcommand}` : safety.base
    return `Shell safety: read-only command (${subject}).`
  }
  if (safety.reason === 'shell_syntax') {
    return 'Shell safety: review required (shell syntax detected).'
  }
  return 'Shell safety: review required (not in read-only command table).'
}
