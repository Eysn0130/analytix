import {
  pluginPackageHostPath, pluginPackageHostRequestSchema, pluginPackageHostResponseSchema,
  type PluginPackageHostResponse
} from '../../../packages/runtime/src/contracts/plugin-package-host'

type Transport = (path: string, body: string) => Promise<{ ok: boolean; status: number; body: string }>

export function createPluginPackageHostHandler(transport: Transport) {
  return async (payload: unknown): Promise<PluginPackageHostResponse> => {
    const parsed = pluginPackageHostRequestSchema.safeParse(payload)
    if (!parsed.success) return { ok: false, code: 'invalid_request', message: 'Invalid plugin request.' }
    const request = parsed.data
    try {
      const body = JSON.stringify(request)
      if (Buffer.byteLength(body) > 270_336) throw new Error('Oversized request')
      const response = await transport(pluginPackageHostPath, body)
      if (Buffer.byteLength(response.body) > 2_105_344) throw new Error('Oversized response')
      const result = pluginPackageHostResponseSchema.safeParse(JSON.parse(response.body))
      if (result.success && result.data.ok === response.ok) {
        const value = result.data
        if (!value.ok) return value
        if (request.action === 'list' && 'packages' in value &&
            new Set(value.packages.map((item) => item.packageId)).size === value.packages.length) return value
        if (request.action === 'setDesiredState' && 'package' in value &&
            value.package.packageId === request.packageId && value.package.generationId === request.generationId &&
            value.package.activationRevision === request.expectedRevision + 1 && value.package.desiredState === request.desiredState) return value
        if (request.action === 'invoke' && 'output' in value) return value
      }
    } catch { /* A lost mutation response must be reconciled by reading Core. */ }
    return { ok: false, code: 'unavailable',
      message: 'Plugin result could not be confirmed. Refresh its current state.', relist: request.action !== 'list' }
  }
}
