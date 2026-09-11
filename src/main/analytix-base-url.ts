/**
 * Base URL resolution for the Analytix local HTTP server. The
 * server is always bound to localhost; the GUI reads the port from
 * settings (default 8899).
 */
export function getAnalytixBaseUrl(port: number, host = '127.0.0.1'): string {
  const normalizedHost = normalizeLocalAnalytixHost(host)
  return `http://${formatHostForUrl(normalizedHost)}:${port}`
}

export function normalizeLocalAnalytixHost(host: string): string {
  const normalized = host.trim().toLowerCase()
  if (normalized === 'localhost') return 'localhost'
  if (normalized === '127.0.0.1') return '127.0.0.1'
  if (normalized === '::1' || normalized === '[::1]') return '::1'
  throw new Error(`Analytix local host must be localhost, 127.0.0.1, or ::1; got "${host}".`)
}

function formatHostForUrl(host: string): string {
  return host.includes(':') ? `[${host}]` : host
}
