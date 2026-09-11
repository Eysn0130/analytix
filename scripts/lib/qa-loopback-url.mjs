// Packaged QA receives the backend address from an untrusted renderer. Never
// forward synthetic fixture paths/bodies to a remote URL or through redirects.
export function qaLoopbackURL(apiBase, route) {
  const base = new URL(apiBase)
  if (base.protocol !== 'http:' || base.hostname !== '127.0.0.1' || !base.port ||
    base.username || base.password || base.pathname !== '/' || base.search || base.hash ||
    typeof route !== 'string' || !route.startsWith('/api/v1/') || route.includes('\\')) {
    throw new Error('QA backend must be an explicit loopback HTTP port with an API v1 route.')
  }
  const target = new URL(route, base)
  if (target.origin !== base.origin || !target.pathname.startsWith('/api/v1/') || target.hash) {
    throw new Error('QA backend route escaped its loopback API boundary.')
  }
  return target
}
