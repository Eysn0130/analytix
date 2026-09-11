function normalizePathForMatch(path: string): string {
  return path.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()
}

// 品牌升级后默认目录在 ~/.analytix 下;老版本/迁移失败的机器上仍可能出现
// ~/.analytix 形式,这里对两套路径都要认,并归一到同一个身份键,
// 避免同一个默认工作区在侧栏里出现两份。
function isDefaultWorkspacePath(normalized: string): boolean {
  return (
    normalized === '~/.analytix/default_workspace'
    || normalized.endsWith('/.analytix/default_workspace')
    || normalized === '~/.analytix/default_workspace'
    || normalized.endsWith('/.analytix/default_workspace')
  )
}

export function workspaceRootIdentityKey(path?: string): string {
  const trimmed = path?.trim() ?? ''
  if (!trimmed) return ''
  const normalized = normalizePathForMatch(trimmed)
  if (isDefaultWorkspacePath(normalized)) {
    return '~/.analytix/default_workspace'
  }
  return normalized
}

export function isInternalTemporaryWorkspace(path?: string): boolean {
  const trimmed = path?.trim() ?? ''
  if (!trimmed) return false
  const normalized = normalizePathForMatch(trimmed)
  return (
    /\/deepseek-tui-updates\/tmp(?:\/|$)/.test(normalized)
    || normalized === '/tmp'
    || normalized.startsWith('/tmp/')
    || normalized === '/private/tmp'
    || normalized.startsWith('/private/tmp/')
    || /^\/var\/folders\/[^/]+\/[^/]+\/t(?:\/|$)/.test(normalized)
    || /^\/private\/var\/folders\/[^/]+\/[^/]+\/t(?:\/|$)/.test(normalized)
    || /\/appdata\/local\/temp(?:\/|$)/.test(normalized)
  )
}

export function isClawWorkspacePath(path?: string): boolean {
  const trimmed = path?.trim() ?? ''
  if (!trimmed) return false
  const normalized = normalizePathForMatch(trimmed)
  return normalized.includes('/.analytix/claw/') || normalized.includes('/.analytix/claw/')
}

export function isInternalDeepSeekGuiWorkspace(path?: string): boolean {
  const trimmed = path?.trim() ?? ''
  if (!trimmed) return false
  const normalized = normalizePathForMatch(trimmed)
  return (
    normalized === '~/.analytix/write_workspace'
    || normalized.endsWith('/.analytix/write_workspace')
    || normalized === '~/.analytix/write_workspace'
    || normalized.endsWith('/.analytix/write_workspace')
  )
}

export function normalizeWorkspaceRoot(path?: string): string {
  const trimmed = path?.trim() ?? ''
  if (!trimmed) return ''
  if (isInternalTemporaryWorkspace(trimmed)) return ''
  return trimmed
}
