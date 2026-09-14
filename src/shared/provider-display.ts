/** Presentation only. Never use these labels as routing or capability evidence. */
function isOfficialDeepSeekEndpoint(endpoint: string): boolean {
  try {
    const url = new URL(endpoint)
    return url.origin === 'https://api.deepseek.com' && !url.username && !url.password &&
      !url.search && !url.hash && /^\/(?:v1\/?)?$/.test(url.pathname)
  } catch {
    return false
  }
}

export function providerDisplayName(id: string, name: string, endpoint: string): string {
  const label = name.trim() || id.trim()
  if (label.toLowerCase() === 'deepseek' && isOfficialDeepSeekEndpoint(endpoint)) return 'DeepSeek'
  return label
}

export function providerModelDisplayName(modelId: string, endpoint: string): string {
  const id = modelId.trim()
  if (!isOfficialDeepSeekEndpoint(endpoint)) return id
  // Official pricing/model documentation, checked 2026-09-14:
  // https://api-docs.deepseek.com/quick_start/pricing/
  // These legacy API aliases are currently served by V4.1 Flash.
  if (['deepseek-flash', 'deepseek-v4-flash', 'deepseek-v4-flash-vision-exp'].includes(id)) {
    return 'V4.1 Flash'
  }
  if (id === 'deepseek-v4-pro') return 'V4 Pro'
  return id
}
