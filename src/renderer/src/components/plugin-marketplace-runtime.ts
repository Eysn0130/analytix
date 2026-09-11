import type {
  CoreRuntimeInfoJson,
  CoreRuntimeToolDiagnosticsJson
} from '../agent/analytix-contract'

export type McpMarketplaceOverlayStatus =
  | 'offline'
  | 'disabled'
  | 'configured'
  | 'connected'
  | 'drift'
  | 'error'

export type McpMarketplaceOverlay = {
  status: McpMarketplaceOverlayStatus
  configuredServers: number
  connectedServers: number
  toolCount: number
  serverIds: string[]
  searchEnabled: boolean
  searchActive: boolean
  searchMode: 'auto' | 'search' | 'direct'
  indexedToolCount: number
  advertisedToolCount: number
  errorCount: number
  driftCount: number
  failureCode?: 'connection_failed'
}

export function buildMcpMarketplaceOverlay(input: {
  runtimeInfo?: CoreRuntimeInfoJson | null
  toolDiagnostics?: CoreRuntimeToolDiagnosticsJson | null
  managedServers?: Array<{ id: string; toolCount?: number }>
}): McpMarketplaceOverlay {
  const capability = input.runtimeInfo?.capabilities?.mcp
  const managedServers = input.managedServers ?? []
  const diagnosticServers = input.toolDiagnostics?.mcpServers ?? []
  const diagnosticServerIds = new Set(diagnosticServers.map((server) => server.id))
  const servers = [
    ...diagnosticServers,
    ...managedServers
      .filter((server) => server.id && !diagnosticServerIds.has(server.id))
      .map((server) => ({
        id: server.id,
        status: 'configured' as const,
        toolCount: server.toolCount ?? 0
      }))
  ]
  const search = input.toolDiagnostics?.mcpSearch ?? capability?.search
  const serverIds = servers.map((server) => server.id)
  const configuredServers = Math.max(capability?.configuredServers ?? 0, servers.length)
  const connectedServers =
    capability?.connectedServers ??
    servers.filter((server) => server.status === 'connected').length
  const toolCount =
    capability?.toolCount ??
    servers.reduce((sum, server) => sum + server.toolCount, 0)
  const errorCount = servers.filter((server) => server.status === 'error').length
  const driftCount = search?.catalogDrift ? 1 : 0
  const status = overlayStatus({
    hasRuntime: Boolean(input.runtimeInfo || input.toolDiagnostics),
    enabled: capability?.enabled,
    configuredServers,
    connectedServers,
    errorCount,
    driftCount
  })
  return {
    status,
    configuredServers,
    connectedServers,
    toolCount,
    serverIds,
    searchEnabled: Boolean(search?.enabled),
    searchActive: Boolean(search?.active),
    searchMode: search?.mode ?? 'auto',
    indexedToolCount: search?.indexedToolCount ?? 0,
    advertisedToolCount: search?.advertisedToolCount ?? 0,
    errorCount,
    driftCount,
    ...(status === 'error' ? { failureCode: 'connection_failed' as const } : {})
  }
}

function overlayStatus(input: {
  hasRuntime: boolean
  enabled?: boolean
  configuredServers: number
  connectedServers: number
  errorCount: number
  driftCount: number
}): McpMarketplaceOverlayStatus {
  if (!input.hasRuntime) return 'offline'
  if (input.enabled === false || input.configuredServers === 0) return 'disabled'
  if (input.driftCount > 0) return 'drift'
  if (input.connectedServers > 0) return 'connected'
  if (input.errorCount > 0) return 'error'
  return 'configured'
}
