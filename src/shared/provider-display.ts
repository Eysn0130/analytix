/** Presentation only. Never use these labels as routing or capability evidence. */
export type ProviderEndpointKind = 'official-deepseek' | 'local' | 'remote' | 'unknown'

export function providerEndpointKind(endpoint: string): ProviderEndpointKind {
  if (isOfficialDeepSeekEndpoint(endpoint)) return 'official-deepseek'
  try {
    const url = new URL(endpoint)
    if (!['http:', 'https:'].includes(url.protocol)) return 'unknown'
    const host = url.hostname.toLowerCase()
    // Loopback is a location, never evidence that a server is a Mock.
    if (host === 'localhost' || host === '[::1]' || /^127\.\d+\.\d+\.\d+$/.test(host)) return 'local'
    return 'remote'
  } catch {
    return 'unknown'
  }
}

export function modelLabelFromCatalog(labels: Record<string, string> | undefined, modelId: string): string {
  if (!labels || !Object.prototype.hasOwnProperty.call(labels, modelId)) return modelId
  const label = labels[modelId]
  return typeof label === 'string' && label.trim() ? label : modelId
}

function isOfficialDeepSeekEndpoint(endpoint: string): boolean {
  try {
    const url = new URL(endpoint)
    return url.origin === 'https://api.deepseek.com' && !url.username && !url.password &&
      !url.search && !url.hash && /^\/(?:v1\/?)?$/.test(url.pathname)
  } catch {
    return false
  }
}

export function providerDisplayName(id: string, name: string): string {
  const label = name.trim() || id.trim()
  if (label.toLowerCase() === 'deepseek') return 'DeepSeek'
  return label
}

export function providerModelDisplayName(modelId: string): string {
  const id = modelId.trim()
  // Official pricing/model documentation, checked 2026-09-14:
  // https://api-docs.deepseek.com/quick_start/pricing/
  // Catalog names stay consistent across official, proxy and local endpoints.
  // This is a presentation alias, not proof of the server implementation.
  if (['deepseek-flash', 'deepseek-v4-flash', 'deepseek-v4-flash-vision-exp'].includes(id)) {
    return 'V4.1 Flash'
  }
  if (id === 'deepseek-v4-pro') return 'V4 Pro'
  return id
}
