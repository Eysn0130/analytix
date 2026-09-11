export type ClawCommand =
  | { kind: 'clear' }
  | { kind: 'help' }
  | { kind: 'showModel' }
  | { kind: 'model'; model: string }
  | { kind: 'showProvider' }
  | { kind: 'provider'; providerId: string }

export function parseClawCommand(text: string): ClawCommand | null {
  const raw = text.trim().replace(/^／/, '/')
  const lower = raw.toLowerCase()
  if (/^[/-](?:clear|reset|new|清空|重置|新会话|新话题)$/.test(lower)) {
    return { kind: 'clear' }
  }
  if (/^[/-](?:help|帮助|命令|\?)$/.test(lower)) {
    return { kind: 'help' }
  }
  const match = raw.match(/^[/-](?:model|模型)(?:\s+(.+))?$/i)
  if (match) {
    const value = (match[1] ?? '').trim()
    return value ? { kind: 'model', model: value } : { kind: 'showModel' }
  }
  const providerMatch = raw.match(/^[/-](?:provider|供应商|提供商)(?:\s+(.+))?$/i)
  if (!providerMatch) return null
  const providerId = (providerMatch[1] ?? '').trim()
  return providerId ? { kind: 'provider', providerId } : { kind: 'showProvider' }
}
