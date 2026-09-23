import { z } from 'zod'

export const PROVIDER_REGISTRY_SCHEMA_VERSION_V1 = 1 as const
export const PROVIDER_REGISTRY_MAX_SECRET_BYTES_V1 = 1 << 20
export const PROVIDER_REGISTRY_MAX_REQUEST_BYTES_V1 = 2 << 20
export const PROVIDER_REGISTRY_MAX_RESPONSE_BYTES_V1 = 16 << 20
export const PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1 = '18446744073709551615' as const
export const PROVIDER_REGISTRY_PORTABLE_MANIFEST_MAX_BYTES_V1 = 1 << 20

const providerIdentifierPattern = /^[a-z0-9][a-z0-9._-]{0,95}$/
const exactProviderRoutePrefix = 'provider:'
const opaqueIncarnationPattern = /^inc_[A-Za-z0-9_-]{43}$/
const purposePattern = /^[a-z0-9][a-z0-9._:/-]{0,95}$/
const maxUint64 = BigInt(PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1)

function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength
}

function boundedUniqueStrings(maximumItems: number) {
  return z.array(z.string().min(1).refine(
    (value) => value === value.trim() && !/[\u0000\r\n\t]/.test(value) && utf8ByteLength(value) <= 256,
    'provider registry value is invalid'
  )).max(maximumItems).superRefine((values, context) => {
    if (new Set(values).size !== values.length) {
      context.addIssue({ code: 'custom', message: 'provider registry values must be unique' })
    }
  })
}

function isCanonicalBase64(value: string): boolean {
  if (value.length === 0 || value.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(value)) {
    return false
  }
  const firstPadding = value.indexOf('=')
  const padding = firstPadding < 0 ? 0 : value.length - firstPadding
  if (padding > 2 || (firstPadding >= 0 && firstPadding < value.length - 2)) return false
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  if (padding === 2 && (alphabet.indexOf(value[value.length - 3] ?? '') & 0x0f) !== 0) return false
  if (padding === 1 && (alphabet.indexOf(value[value.length - 2] ?? '') & 0x03) !== 0) return false
  const decodedBytes = value.length / 4 * 3 - padding
  return decodedBytes > 0 && decodedBytes <= PROVIDER_REGISTRY_MAX_SECRET_BYTES_V1
}

const redactedCredentialPlaceholders = new Set([
  'Kioq',
  'KioqKioqKio=',
  'cmVkYWN0ZWQ=',
  'UkVEQUNURUQ=',
  'W3JlZGFjdGVkXQ==',
  'PFJFREFDVEVEPg=='
])

export const providerRegistryDecimalSchemaV1 = z.string().refine(
  (value) => /^(0|[1-9][0-9]*)$/.test(value) && BigInt(value) <= maxUint64,
  'provider registry decimal must be canonical uint64'
)

const nonZeroProviderRegistryDecimalSchemaV1 = providerRegistryDecimalSchemaV1.refine(
  (value) => value !== '0',
  'provider registry decimal must be non-zero'
)

export const providerRegistryIncarnationSchemaV1 = z.string().regex(opaqueIncarnationPattern)
export const providerRegistryProviderIdSchemaV1 = z.string().regex(providerIdentifierPattern)
export const providerRegistryCredentialPurposeSchemaV1 = z.string().regex(purposePattern)

const endpointSchema = z.string().max(2048).refine((value) => {
  if (value !== value.trim() || /[\u0000\r\n\t]/.test(value)) return false
  const sourcePrefix = /^(?:http|https):\/\//i.exec(value)?.[0]
  const fragmentDelimiter = value.indexOf('#')
  if (sourcePrefix === undefined || value.includes('?') ||
    (fragmentDelimiter >= 0 && fragmentDelimiter !== value.length - 1)) return false
  const authorityEnd = value.indexOf('/', sourcePrefix.length)
  const rawAuthority = value.slice(sourcePrefix.length, authorityEnd < 0 ? value.length : authorityEnd)
  if (rawAuthority.includes('@') || rawAuthority.includes('\\')) return false
  try {
    const parsed = new URL(value)
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
      parsed.host.length > 0 && parsed.username === '' && parsed.password === '' &&
      parsed.search === '' && parsed.hash === ''
  } catch {
    return false
  }
}, 'provider registry endpoint is invalid')

function portableMetadataString(value: string, maximumBytes = 4096): boolean {
  return value === value.trim() && !/[\\\u0000\r\n\t]/.test(value) &&
    !value.startsWith('/') && !value.startsWith('~/') &&
    !value.includes('../') && !value.includes('..\\') &&
    !value.includes('\u2028') && !value.includes('\u2029') &&
    utf8ByteLength(value) <= maximumBytes
}

function portableOAuthMetadataString(value: string, maximumBytes = 256): boolean {
  return portableMetadataString(value, maximumBytes)
}

function canonicalPortableEndpoint(value: string): boolean {
  if (value !== value.trim() || /[\\\u0000\r\n\t]/.test(value) ||
    value.includes('\u2028') || value.includes('\u2029')) return false
  const source = /^(https?):\/\/([^/?#]*)([^?#]*)$/.exec(value)
  if (!source || source[2] === '' || source[2].includes('@') || source[2].includes('\\')) return false
  if (source[3].includes('%')) return false
  try {
    const parsed = new URL(value)
    if ((parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
      parsed.host.length === 0 || parsed.username !== '' || parsed.password !== '' ||
      parsed.search !== '' || parsed.hash !== '' || parsed.pathname.includes('%')) return false
  } catch {
    return false
  }
  return `${source[1].toLowerCase()}://${source[2].toLowerCase()}${source[3]}` === value &&
    utf8ByteLength(value) <= 2048
}

const portableManifestIdentifierSchema = z.string().regex(providerIdentifierPattern)
const portableManifestIntentSchema = z.enum(['reentry_required', 'protected_recovery_available'])
const portableManifestModelsSchema = z.array(z.string().min(1).refine(
  (value) => portableMetadataString(value, 256),
  'portable manifest model metadata is invalid'
)).max(512).superRefine((values, context) => {
  if (new Set(values).size !== values.length) {
    context.addIssue({ code: 'custom', message: 'portable manifest model metadata must be unique' })
  }
  if (values.some((value, index) => index > 0 && values[index - 1]! >= value)) {
    context.addIssue({ code: 'custom', message: 'portable manifest model metadata must be sorted' })
  }
})
const portableManifestRoutesSchema = z.array(portableManifestIdentifierSchema).max(128).superRefine((values, context) => {
  if (new Set(values).size !== values.length) {
    context.addIssue({ code: 'custom', message: 'portable manifest routes must be unique' })
  }
})
const portableManifestEndpointSchema = z.string().min(1).refine(
  canonicalPortableEndpoint,
  'portable manifest endpoint must be normalized and contain no query, fragment, or userinfo'
)
const portableManifestOAuthEndpointSchema = z.string().min(1).max(2048).refine(
  (value) => isStrictOAuthAuthorityEndpoint(value) && canonicalPortableEndpoint(value),
  'portable manifest OAuth endpoint is invalid'
)
const portableManifestOAuthBindingSchemaV1 = z.object({
  schemaVersion: z.literal(1),
  issuer: portableManifestOAuthEndpointSchema,
  authorizationEndpoint: portableManifestOAuthEndpointSchema,
  tokenEndpoint: portableManifestOAuthEndpointSchema,
  revocationEndpoint: portableManifestOAuthEndpointSchema.optional(),
  clientId: z.string().min(1).max(256).refine((value) => portableOAuthMetadataString(value, 256)),
  scopes: z.array(z.string().min(1).refine((value) => portableOAuthMetadataString(value, 256)))
    .min(1).max(32).superRefine((values, context) => {
      if (new Set(values).size !== values.length) {
        context.addIssue({ code: 'custom', message: 'portable manifest OAuth scopes must be unique' })
      }
    }),
  redirectModeVersion: z.literal(1)
}).strict()
const portableManifestObservationSchemaV1 = z.object({
  schemaVersion: z.literal(1),
  endpoint: portableManifestOAuthEndpointSchema,
  method: z.literal('GET'),
  projection: z.literal('normalized-quota-v1')
}).strict()
const portableManifestProviderSchemaV1 = z.object({
  correlation: portableManifestIdentifierSchema,
  kind: portableManifestIdentifierSchema,
  endpoint: portableManifestEndpointSchema,
  proxy: portableManifestEndpointSchema.optional(),
  models: portableManifestModelsSchema,
  mediaModels: portableManifestModelsSchema,
  selectedModel: z.string().min(1).refine((value) => portableMetadataString(value, 4096)).optional(),
  selectedMediaModel: z.string().min(1).refine((value) => portableMetadataString(value, 4096)).optional(),
  oauthBinding: portableManifestOAuthBindingSchemaV1.optional(),
  accountObservation: portableManifestObservationSchemaV1.optional(),
  routes: portableManifestRoutesSchema,
  intent: portableManifestIntentSchema
}).strict().superRefine((provider, context) => {
  if (provider.kind === 'analytix-private-account') {
    context.addIssue({ code: 'custom', path: ['kind'], message: 'private accounts are not Providers' })
  }
  if (provider.selectedModel !== undefined && !provider.models.includes(provider.selectedModel)) {
    context.addIssue({ code: 'custom', path: ['selectedModel'], message: 'selected model is not declared' })
  }
  if (provider.selectedMediaModel !== undefined && !provider.mediaModels.includes(provider.selectedMediaModel)) {
    context.addIssue({ code: 'custom', path: ['selectedMediaModel'], message: 'selected media model is not declared' })
  }
})
const portableManifestAccountSchemaV1 = z.object({
  correlation: portableManifestIdentifierSchema,
  owner: z.enum(['mcp', 'extension']),
  provider: z.string().min(1).max(128).refine((value) =>
    portableMetadataString(value, 128) && !value.includes('/') && !value.includes('\\')
  ),
  endpoint: portableManifestEndpointSchema,
  purpose: z.enum(['mcp-oauth-access-token', 'extension-provider-account-token']),
  intent: portableManifestIntentSchema
}).strict().superRefine((account, context) => {
  if ((account.owner === 'mcp' && account.purpose !== 'mcp-oauth-access-token') ||
    (account.owner === 'extension' && account.purpose !== 'extension-provider-account-token')) {
    context.addIssue({ code: 'custom', path: ['purpose'], message: 'portable manifest account purpose is not owner-bound' })
  }
})

function portableProviderIdentity(provider: {
  kind: string
  endpoint: string
}): string {
  return `${provider.kind}\u0000${provider.endpoint}`
}

function portableAccountIdentity(account: {
  owner: string
  provider: string
  purpose: string
}): string {
  return `${account.owner}\u0000${account.provider}\u0000${account.purpose}`
}

export const providerRegistryPortableManifestV1Schema = z.object({
  schema: z.literal('analytix.provider-portable-manifest/v1'),
  providers: z.array(portableManifestProviderSchemaV1).max(256),
  accounts: z.array(portableManifestAccountSchemaV1).max(128)
}).strict().superRefine((manifest, context) => {
  const correlations = new Set<string>()
  const providerCorrelations = new Set<string>()
  const providerIdentities = new Set<string>()
  const accountIdentities = new Set<string>()
  if (manifest.providers.length + manifest.accounts.length > 256) {
    context.addIssue({ code: 'custom', path: ['accounts'], message: 'portable manifest entry count exceeds Registry capacity' })
  }
  for (const [index, provider] of manifest.providers.entries()) {
    if (provider.correlation !== `provider-${index}`) {
      context.addIssue({ code: 'custom', path: ['providers', index, 'correlation'], message: 'portable manifest Provider correlations must follow sorted indexes' })
    }
    if (correlations.has(provider.correlation)) {
      context.addIssue({ code: 'custom', path: ['providers', index, 'correlation'], message: 'portable manifest correlations must be unique' })
    }
    correlations.add(provider.correlation)
    providerCorrelations.add(provider.correlation)
    const identity = portableProviderIdentity(provider)
    if (providerIdentities.has(identity)) {
      context.addIssue({ code: 'custom', path: ['providers', index], message: 'portable manifest Provider identity must be unique' })
    }
    providerIdentities.add(identity)
    if (index > 0 && portableProviderIdentity(manifest.providers[index - 1]!) >= identity) {
      context.addIssue({ code: 'custom', path: ['providers'], message: 'portable manifest Providers must be sorted by stable identity' })
    }
    for (const [routeIndex, route] of provider.routes.entries()) {
      if (!providerCorrelations.has(route) && !manifest.providers.some((candidate) => candidate.correlation === route)) {
        context.addIssue({ code: 'custom', path: ['providers', index, 'routes', routeIndex], message: 'portable manifest route reference is unknown' })
      }
    }
  }
  for (const [index, account] of manifest.accounts.entries()) {
    if (account.correlation !== `account-${index}`) {
      context.addIssue({ code: 'custom', path: ['accounts', index, 'correlation'], message: 'portable manifest account correlations must follow sorted indexes' })
    }
    if (correlations.has(account.correlation)) {
      context.addIssue({ code: 'custom', path: ['accounts', index, 'correlation'], message: 'portable manifest correlations must be unique' })
    }
    correlations.add(account.correlation)
    const identity = portableAccountIdentity(account)
    if (accountIdentities.has(identity)) {
      context.addIssue({ code: 'custom', path: ['accounts', index], message: 'portable manifest account identity must be unique' })
    }
    accountIdentities.add(identity)
    if (index > 0 && portableAccountIdentity(manifest.accounts[index - 1]!) >= identity) {
      context.addIssue({ code: 'custom', path: ['accounts'], message: 'portable manifest accounts must be sorted by stable identity' })
    }
  }
})

export type ProviderRegistryPortableManifestV1 = z.infer<typeof providerRegistryPortableManifestV1Schema>

function portableManifestCanonicalValue(manifest: ProviderRegistryPortableManifestV1): Record<string, unknown> {
	const canonicalOAuthBinding = (binding: ProviderRegistryPortableManifestV1['providers'][number]['oauthBinding']): unknown => {
		if (binding === undefined) return undefined
		const value: Record<string, unknown> = {
			schemaVersion: binding.schemaVersion,
			issuer: binding.issuer,
			authorizationEndpoint: binding.authorizationEndpoint,
			tokenEndpoint: binding.tokenEndpoint
		}
		if (binding.revocationEndpoint !== undefined) value.revocationEndpoint = binding.revocationEndpoint
		value.clientId = binding.clientId
		value.scopes = binding.scopes
		value.redirectModeVersion = binding.redirectModeVersion
		return value
	}
	const canonicalObservation = (observation: ProviderRegistryPortableManifestV1['providers'][number]['accountObservation']): unknown => {
		if (observation === undefined) return undefined
		return {
			schemaVersion: observation.schemaVersion,
			endpoint: observation.endpoint,
			method: observation.method,
			projection: observation.projection
		}
	}
	const providers = manifest.providers.map((provider) => {
    const value: Record<string, unknown> = {
      correlation: provider.correlation,
      kind: provider.kind,
      endpoint: provider.endpoint
    }
    if (provider.proxy !== undefined) value.proxy = provider.proxy
    value.models = provider.models
    value.mediaModels = provider.mediaModels
    if (provider.selectedModel !== undefined) value.selectedModel = provider.selectedModel
    if (provider.selectedMediaModel !== undefined) value.selectedMediaModel = provider.selectedMediaModel
		if (provider.oauthBinding !== undefined) value.oauthBinding = canonicalOAuthBinding(provider.oauthBinding)
		if (provider.accountObservation !== undefined) value.accountObservation = canonicalObservation(provider.accountObservation)
    value.routes = provider.routes
    value.intent = provider.intent
    return value
  })
  const accounts = manifest.accounts.map((account) => ({
    correlation: account.correlation,
    owner: account.owner,
    provider: account.provider,
    endpoint: account.endpoint,
    purpose: account.purpose,
    intent: account.intent
  }))
  return {
    schema: manifest.schema,
    providers,
    accounts
  }
}

function portableManifestStringEnd(input: string, start: number): number | null {
  if (input[start] !== '"') return null
  let escaped = false
  for (let index = start + 1; index < input.length; index++) {
    const current = input[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (current === '\\') {
      escaped = true
      continue
    }
    if (current === '"') return index + 1
  }
  return null
}

function portableManifestHasUniqueObjectKeys(input: string): boolean {
  let offset = 0
  const skipSpace = (): void => {
    while (/\s/.test(input[offset] ?? '')) offset++
  }
  const scan = (depth: number): boolean => {
    if (depth > 32) return false
    skipSpace()
    const current = input[offset]
    if (current === '"') {
      const end = portableManifestStringEnd(input, offset)
      if (end === null) return false
      try {
        JSON.parse(input.slice(offset, end))
      } catch {
        return false
      }
      offset = end
      return true
    }
    if (current === '{') {
      offset++
      skipSpace()
      const keys = new Set<string>()
      if (input[offset] === '}') {
        offset++
        return true
      }
      while (true) {
        skipSpace()
        const keyStart = offset
        const keyEnd = portableManifestStringEnd(input, keyStart)
        if (keyEnd === null) return false
        let key: unknown
        try {
          key = JSON.parse(input.slice(keyStart, keyEnd))
        } catch {
          return false
        }
        if (typeof key !== 'string' || keys.has(key)) return false
        keys.add(key)
        offset = keyEnd
        skipSpace()
        if (input[offset] !== ':') return false
        offset++
        if (!scan(depth + 1)) return false
        skipSpace()
        if (input[offset] === '}') {
          offset++
          return true
        }
        if (input[offset] !== ',') return false
        offset++
      }
    }
    if (current === '[') {
      offset++
      skipSpace()
      if (input[offset] === ']') {
        offset++
        return true
      }
      while (true) {
        if (!scan(depth + 1)) return false
        skipSpace()
        if (input[offset] === ']') {
          offset++
          return true
        }
        if (input[offset] !== ',') return false
        offset++
      }
    }
    const start = offset
    while (offset < input.length) {
      const delimiter = input[offset]
      if (delimiter === ',' || delimiter === ']' || delimiter === '}' || /\s/.test(delimiter ?? '')) break
      offset++
    }
    if (start === offset) return false
    try {
      JSON.parse(input.slice(start, offset))
      return true
    } catch {
      return false
    }
  }
  if (!scan(0)) return false
  skipSpace()
  return offset === input.length
}

export function parseProviderRegistryPortableManifestV1(manifestJson: string): ProviderRegistryPortableManifestV1 | null {
  if (utf8ByteLength(manifestJson) > PROVIDER_REGISTRY_PORTABLE_MANIFEST_MAX_BYTES_V1 ||
    !portableManifestHasUniqueObjectKeys(manifestJson)) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(manifestJson)
  } catch {
    return null
  }
  const result = providerRegistryPortableManifestV1Schema.safeParse(parsed)
  if (!result.success || JSON.stringify(portableManifestCanonicalValue(result.data)) !== manifestJson) return null
  return result.data
}

const optionalEndpointSchema = z.union([z.literal(''), endpointSchema])
const modelsSchema = boundedUniqueStrings(512)

function isStrictOAuthAuthorityEndpoint(value: string): boolean {
  if (value !== value.trim() || /[\u0000\r\n\t\\]/.test(value)) return false
  try {
    const parsed = new URL(value)
    const hostname = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '')
    const loopback = hostname === '127.0.0.1' || hostname === '::1'
    const exactLoopbackHttp = /^http:\/\/(?:127\.0\.0\.1(?::[0-9]+)?|\[::1\](?::[0-9]+)?)(?:\/|$)/.test(value)
    const privateAddress = hostname === 'localhost' || hostname.endsWith('.localhost') ||
      /^10\./.test(hostname) || /^192\.168\./.test(hostname) ||
      /^172\.(?:1[6-9]|2[0-9]|3[01])\./.test(hostname) || /^169\.254\./.test(hostname) ||
      /^(?:fc|fd|fe8|fe9|fea|feb)/i.test(hostname)
    return !privateAddress && parsed.host.length > 0 && !parsed.username && !parsed.password &&
      !parsed.search && !parsed.hash && !parsed.pathname.includes('%') &&
      (parsed.protocol === 'https:' || (parsed.protocol === 'http:' && loopback && exactLoopbackHttp))
  } catch {
    return false
  }
}

export const providerRegistryOAuthBindingSchemaV1 = z.object({
  schemaVersion: z.literal(1),
  issuer: z.string().min(1).max(2048).refine(isStrictOAuthAuthorityEndpoint),
  authorizationEndpoint: z.string().min(1).max(2048).refine(isStrictOAuthAuthorityEndpoint),
  tokenEndpoint: z.string().min(1).max(2048).refine(isStrictOAuthAuthorityEndpoint),
  revocationEndpoint: z.string().min(1).max(2048).refine(isStrictOAuthAuthorityEndpoint).optional(),
  clientId: z.string().min(1).max(256).refine((value) =>
    value === value.trim() && !/[\u0000\r\n\t]/.test(value)
  ),
  scopes: boundedUniqueStrings(32).refine((values) => values.length > 0),
  redirectModeVersion: z.literal(1)
}).strict()

export const providerRegistryAccountObservationBindingSchemaV1 = z.object({
  schemaVersion: z.literal(1),
  endpoint: z.string().min(1).max(2048).refine(isStrictOAuthAuthorityEndpoint),
  method: z.literal('GET'),
  projection: z.literal('normalized-quota-v1')
}).strict()

function providerRouteProviderId(policyProviderId: string, route: string): string | null {
  if (route === 'primary') return policyProviderId
  if (route.startsWith(exactProviderRoutePrefix)) {
    const exactProviderId = route.slice(exactProviderRoutePrefix.length)
    return providerIdentifierPattern.test(exactProviderId) ? exactProviderId : null
  }
  return providerIdentifierPattern.test(route) ? route : null
}

const routesSchema = boundedUniqueStrings(128).superRefine((routes, context) => {
  for (const [index, route] of routes.entries()) {
    if (providerRouteProviderId('route-policy', route) === null) {
      context.addIssue({
        code: 'custom',
        path: [index],
        message: 'provider registry route must be a local provider identifier'
      })
    }
  }
})

function validateProviderRouteIdentities(
  provider: { id: string; selectedRoutes: readonly string[] },
  context: z.RefinementCtx
): void {
  const seen = new Set<string>()
  for (const [index, route] of provider.selectedRoutes.entries()) {
    const providerId = providerRouteProviderId(provider.id, route)
    if (providerId === null) continue
    if (seen.has(providerId)) {
      context.addIssue({
        code: 'custom',
        path: ['selectedRoutes', index],
        message: 'provider registry route identities must be unique'
      })
    }
    seen.add(providerId)
  }
}

export const providerRegistryProviderInputSchemaV1 = z.object({
  id: providerRegistryProviderIdSchemaV1,
  kind: providerRegistryProviderIdSchemaV1,
  endpoint: endpointSchema,
  proxy: optionalEndpointSchema,
  models: modelsSchema,
  mediaModels: modelsSchema,
  selectedModel: z.string().max(256),
  selectedMediaModel: z.string().max(256),
  selectedRoutes: routesSchema,
  oauthBinding: providerRegistryOAuthBindingSchemaV1.optional(),
  accountObservation: providerRegistryAccountObservationBindingSchemaV1.optional()
}).strict().superRefine((provider, context) => {
  validateProviderRouteIdentities(provider, context)
  if (provider.selectedModel !== '' && !provider.models.includes(provider.selectedModel)) {
    context.addIssue({ code: 'custom', path: ['selectedModel'], message: 'selected model is not declared' })
  }
  if (provider.selectedMediaModel !== '' && !provider.mediaModels.includes(provider.selectedMediaModel)) {
    context.addIssue({ code: 'custom', path: ['selectedMediaModel'], message: 'selected media model is not declared' })
  }
})

const expectedStateShape = {
  registryRevision: providerRegistryDecimalSchemaV1,
  registryIncarnation: providerRegistryIncarnationSchemaV1,
  providerRevision: providerRegistryDecimalSchemaV1,
  providerGeneration: providerRegistryDecimalSchemaV1,
  providerIncarnation: z.union([z.literal(''), providerRegistryIncarnationSchemaV1]),
  providerCredentialPurpose: z.union([z.literal(''), providerRegistryCredentialPurposeSchemaV1])
}

export const providerRegistryConnectExpectedStateSchemaV1 = z.object(expectedStateShape).strict().refine(
  (expected) => expected.providerRevision === '0' && expected.providerGeneration === '0' &&
    expected.providerIncarnation === '' && expected.providerCredentialPurpose === '',
  'connect must not claim existing provider state'
)

export const providerRegistryExpectedStateSchemaV1 = z.object({
  ...expectedStateShape,
  registryRevision: providerRegistryDecimalSchemaV1.refine(
    (value) => value !== PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1,
    'registry revision cannot advance'
  ),
  providerRevision: nonZeroProviderRegistryDecimalSchemaV1,
  providerGeneration: nonZeroProviderRegistryDecimalSchemaV1,
  providerIncarnation: providerRegistryIncarnationSchemaV1
}).strict()

export const providerRegistryCredentialSetSchemaV1 = z.object({
  kind: z.literal('set'),
  purpose: providerRegistryCredentialPurposeSchemaV1,
  valueBase64: z.string().refine(
    (value) => isCanonicalBase64(value) && !redactedCredentialPlaceholders.has(value),
    'credential must be non-empty canonical base64 within the write limit'
  )
}).strict()

export const providerRegistryCredentialKeepSchemaV1 = z.object({
  kind: z.literal('keep')
}).strict()

export const providerRegistryCredentialUnsetSchemaV1 = z.object({
  kind: z.literal('unset')
}).strict()

const schemaVersionShape = { schemaVersion: z.literal(PROVIDER_REGISTRY_SCHEMA_VERSION_V1) }
const providerIdShape = { providerId: providerRegistryProviderIdSchemaV1 }

export const providerRegistryListRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('list')
}).strict()

export const providerRegistryGetRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('get'),
  ...providerIdShape
}).strict()

export const providerRegistryConnectRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('connect'),
  deferSelection: z.boolean().optional(),
  expected: providerRegistryConnectExpectedStateSchemaV1,
  provider: providerRegistryProviderInputSchemaV1,
  credential: providerRegistryCredentialSetSchemaV1
}).strict()

export const providerRegistryUpdateRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('update'),
  ...providerIdShape,
  expected: providerRegistryExpectedStateSchemaV1,
  provider: providerRegistryProviderInputSchemaV1,
  credential: z.discriminatedUnion('kind', [
    providerRegistryCredentialKeepSchemaV1,
    providerRegistryCredentialUnsetSchemaV1,
    providerRegistryCredentialSetSchemaV1
  ])
}).strict().refine(
  (request) => request.providerId === request.provider.id,
  'provider id must match the update target'
)

const expectedOperationRequestShape = {
  ...schemaVersionShape,
  ...providerIdShape,
  expected: providerRegistryExpectedStateSchemaV1
}

export const providerRegistrySelectRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('select')
}).strict()

export const providerRegistryDisconnectRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('disconnect')
}).strict().refine(
  (request) => request.expected.providerCredentialPurpose !== '',
  'disconnect requires the committed credential purpose'
)

export const providerRegistryExplicitDeleteRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('explicit-delete')
}).strict()

export const providerRegistryCredentialReplaceRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('credential-replace'),
  credential: providerRegistryCredentialSetSchemaV1
}).strict()

export const providerRegistryProbeRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('probe')
}).strict().refine(
  (request) => request.expected.providerCredentialPurpose !== '',
  'probe requires the committed credential purpose'
)

export const providerRegistryCredentialCheckRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('credential-check')
}).strict().refine(
  (request) => request.expected.providerCredentialPurpose !== '',
  'credential check requires the committed credential purpose'
)

export const providerRegistryDiscoverModelsRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('discover-models')
}).strict().refine(
  (request) => request.expected.providerCredentialPurpose !== '',
  'model discovery requires the committed credential purpose'
)

export const providerRegistryObserveAccountRequestSchemaV1 = z.object({
  ...expectedOperationRequestShape,
  operation: z.literal('observe-account')
}).strict().refine(
  (request) => request.expected.providerCredentialPurpose !== '',
  'account observation requires the committed credential purpose'
)

export const providerRegistryRecoverRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('recover')
}).strict()

export const providerRegistryPortableManifestExportRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('export-portable-manifest')
}).strict()

export const providerRegistryPortableManifestImportRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('import-portable-manifest'),
  manifestJson: z.string().min(1).refine(
    (value) => utf8ByteLength(value) <= PROVIDER_REGISTRY_PORTABLE_MANIFEST_MAX_BYTES_V1,
    'portable manifest is too large'
  ).refine(
    (value) => parseProviderRegistryPortableManifestV1(value) !== null,
    'portable manifest is not canonical'
  )
}).strict()

export const providerRegistryRequestSchemaV1 = z.discriminatedUnion('operation', [
  providerRegistryListRequestSchemaV1,
  providerRegistryGetRequestSchemaV1,
  providerRegistryConnectRequestSchemaV1,
  providerRegistryUpdateRequestSchemaV1,
  providerRegistrySelectRequestSchemaV1,
  providerRegistryDisconnectRequestSchemaV1,
  providerRegistryExplicitDeleteRequestSchemaV1,
  providerRegistryCredentialReplaceRequestSchemaV1,
  providerRegistryProbeRequestSchemaV1,
  providerRegistryCredentialCheckRequestSchemaV1,
  providerRegistryDiscoverModelsRequestSchemaV1,
  providerRegistryObserveAccountRequestSchemaV1,
  providerRegistryRecoverRequestSchemaV1,
  providerRegistryPortableManifestExportRequestSchemaV1,
  providerRegistryPortableManifestImportRequestSchemaV1
])

export const providerRegistryPublicProviderSchemaV1 = z.object({
  id: providerRegistryProviderIdSchemaV1,
  kind: providerRegistryProviderIdSchemaV1,
  endpoint: endpointSchema,
  proxy: endpointSchema.optional(),
  models: modelsSchema,
  mediaModels: modelsSchema,
  selectedModel: z.string().max(256).optional(),
  selectedMediaModel: z.string().max(256).optional(),
  selectedRoutes: routesSchema,
  credentialConfigured: z.boolean(),
  credentialPurpose: providerRegistryCredentialPurposeSchemaV1.optional(),
  revision: nonZeroProviderRegistryDecimalSchemaV1,
  generation: nonZeroProviderRegistryDecimalSchemaV1,
  incarnation: providerRegistryIncarnationSchemaV1,
  tombstone: z.boolean(),
  oauthBinding: providerRegistryOAuthBindingSchemaV1.optional(),
  accountObservation: providerRegistryAccountObservationBindingSchemaV1.optional()
}).strict().superRefine((provider, context) => {
  validateProviderRouteIdentities(provider, context)
  if ((provider.credentialPurpose !== undefined) !== provider.credentialConfigured) {
    context.addIssue({ code: 'custom', path: ['credentialPurpose'], message: 'credential projection is inconsistent' })
  }
  if (provider.selectedModel !== undefined && !provider.models.includes(provider.selectedModel)) {
    context.addIssue({ code: 'custom', path: ['selectedModel'], message: 'selected model is not declared' })
  }
  if (provider.selectedMediaModel !== undefined && !provider.mediaModels.includes(provider.selectedMediaModel)) {
    context.addIssue({ code: 'custom', path: ['selectedMediaModel'], message: 'selected media model is not declared' })
  }
  if (provider.tombstone && (provider.credentialConfigured || provider.selectedRoutes.length !== 0)) {
    context.addIssue({ code: 'custom', message: 'tombstoned provider projection is inconsistent' })
  }
})

const publicResponseBase = {
  ...schemaVersionShape,
  registryRevision: providerRegistryDecimalSchemaV1,
  registryIncarnation: providerRegistryIncarnationSchemaV1
}

function validateProviderCollection(
  response: {
    selectedProviderId?: string
    providers: readonly z.infer<typeof providerRegistryPublicProviderSchemaV1>[]
  },
  context: z.RefinementCtx
): void {
  const providerIds = response.providers.map((provider) => provider.id)
  if (new Set(providerIds).size !== providerIds.length) {
    context.addIssue({ code: 'custom', path: ['providers'], message: 'provider ids must be unique' })
  }
  if (providerIds.some((providerId, index) => index > 0 && providerIds[index - 1]! > providerId)) {
    context.addIssue({ code: 'custom', path: ['providers'], message: 'provider projections must be sorted' })
  }
  if (response.selectedProviderId !== undefined) {
    const selected = response.providers.find((provider) => provider.id === response.selectedProviderId)
    if (!selected || selected.tombstone) {
      context.addIssue({ code: 'custom', path: ['selectedProviderId'], message: 'selected provider is not active' })
    }
  }
}

export const providerRegistrySnapshotResponseSchemaV1 = z.object({
  ...publicResponseBase,
  selectedProviderId: providerRegistryProviderIdSchemaV1.optional(),
  providers: z.array(providerRegistryPublicProviderSchemaV1).max(256)
}).strict().superRefine((response, context) => {
  validateProviderCollection(response, context)
})

export const providerRegistryProviderResponseSchemaV1 = z.object({
  ...publicResponseBase,
  provider: providerRegistryPublicProviderSchemaV1
}).strict()

export const providerRegistryDeletedResponseSchemaV1 = z.object({
  ...publicResponseBase,
  deletedProviderId: providerRegistryProviderIdSchemaV1
}).strict()

export const providerRegistryRecoveredResponseSchemaV1 = z.object({
  ...publicResponseBase,
  selectedProviderId: providerRegistryProviderIdSchemaV1.optional(),
  providers: z.array(providerRegistryPublicProviderSchemaV1).max(256),
  recovered: z.literal(true)
}).strict().superRefine((response, context) => {
  validateProviderCollection(response, context)
})

export const providerRegistryProbeStatusSchemaV1 = z.enum([
  'reachable',
  'auth_failed',
  'timeout',
  'redirect_blocked',
  'provider_error',
  'unavailable',
  'invalid_response'
])

export const providerRegistryProbeResponseSchemaV1 = z.object({
  ...schemaVersionShape,
  registryRevision: providerRegistryDecimalSchemaV1,
  registryIncarnation: providerRegistryIncarnationSchemaV1,
  providerId: providerRegistryProviderIdSchemaV1,
  providerRevision: nonZeroProviderRegistryDecimalSchemaV1,
  providerGeneration: nonZeroProviderRegistryDecimalSchemaV1,
  providerIncarnation: providerRegistryIncarnationSchemaV1,
  status: providerRegistryProbeStatusSchemaV1,
  code: z.number().int().min(100).max(599).optional(),
  modelCount: z.number().int().min(0).max(512),
  latencyMs: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
}).strict().superRefine((response, context) => {
  const successfulCode = response.code !== undefined && response.code >= 200 && response.code < 300
  const redirectCode = response.code !== undefined && response.code >= 300 && response.code < 400
  const authCode = response.code === 401 || response.code === 403
  const zeroModels = response.modelCount === 0
  let coherent = false
  switch (response.status) {
    case 'reachable':
      coherent = successfulCode
      break
    case 'auth_failed':
      coherent = authCode && zeroModels
      break
    case 'redirect_blocked':
      coherent = redirectCode && zeroModels
      break
    case 'provider_error':
      coherent = response.code !== undefined && !successfulCode && !redirectCode && !authCode && zeroModels
      break
    case 'timeout':
    case 'unavailable':
      coherent = response.code === undefined && zeroModels
      break
    case 'invalid_response':
      coherent = (response.code === undefined || successfulCode) && zeroModels
      break
  }
  if (!coherent) {
    context.addIssue({
      code: 'custom',
      path: ['status'],
      message: 'probe status, code, and model count must form a coherent closed tuple'
    })
  }
})

export const providerRegistryCredentialCheckResponseSchemaV1 = z.object({
  ...schemaVersionShape,
  registryRevision: providerRegistryDecimalSchemaV1,
  registryIncarnation: providerRegistryIncarnationSchemaV1,
  providerId: providerRegistryProviderIdSchemaV1,
  providerRevision: nonZeroProviderRegistryDecimalSchemaV1,
  providerGeneration: nonZeroProviderRegistryDecimalSchemaV1,
  providerIncarnation: providerRegistryIncarnationSchemaV1,
  credentialAvailable: z.literal(true)
}).strict()

export const providerRegistryAccountObservationStatusSchemaV1 = z.enum([
  'available',
  'unavailable',
  'auth_failed',
  'timeout',
  'redirect_blocked',
  'provider_error',
  'invalid_response'
])

const boundedObservationValueSchemaV1 = z.number().finite().min(0).max(1e15)

export const providerRegistryAccountObservationResponseSchemaV1 = z.object({
  ...schemaVersionShape,
  registryRevision: providerRegistryDecimalSchemaV1,
  registryIncarnation: providerRegistryIncarnationSchemaV1,
  providerId: providerRegistryProviderIdSchemaV1,
  providerRevision: nonZeroProviderRegistryDecimalSchemaV1,
  providerGeneration: nonZeroProviderRegistryDecimalSchemaV1,
  providerIncarnation: providerRegistryIncarnationSchemaV1,
  providerCredentialPurpose: providerRegistryCredentialPurposeSchemaV1,
  status: providerRegistryAccountObservationStatusSchemaV1,
  observedAt: z.string().datetime({ offset: true }),
  expiresAt: z.string().datetime({ offset: true }),
  quota: boundedObservationValueSchemaV1.optional(),
  usage: boundedObservationValueSchemaV1.optional(),
  remaining: boundedObservationValueSchemaV1.optional()
}).strict().superRefine((response, context) => {
  const values = [response.quota, response.usage, response.remaining]
  if ((response.status === 'available') !== values.some((value) => value !== undefined)) {
    context.addIssue({ code: 'custom', path: ['status'], message: 'account observation tuple is inconsistent' })
  }
  const observedAtMs = Date.parse(response.observedAt)
  const expiresAtMs = Date.parse(response.expiresAt)
  if (expiresAtMs < observedAtMs || expiresAtMs > observedAtMs + 60 * 60 * 1000) {
    context.addIssue({ code: 'custom', path: ['expiresAt'], message: 'account observation expiry is invalid' })
  }
})

export const providerRegistryPortableManifestExportResponseSchemaV1 = z.object({
  ...schemaVersionShape,
  manifestJson: z.string().min(1).refine(
    (value) => parseProviderRegistryPortableManifestV1(value) !== null,
    'portable manifest response is not canonical'
  )
}).strict()

export const providerRegistryPortableManifestImportEntryResponseSchemaV1 = z.object({
  correlation: portableManifestIdentifierSchema,
  destinationProviderId: providerRegistryProviderIdSchemaV1,
  status: z.literal('reentry_required')
}).strict()

export const providerRegistryPortableManifestImportResponseSchemaV1 = z.object({
  ...schemaVersionShape,
  providerCount: z.number().int().min(0).max(256),
  accountCount: z.number().int().min(0).max(128),
  reentryRequired: z.number().int().min(0).max(256),
  entries: z.array(providerRegistryPortableManifestImportEntryResponseSchemaV1).max(256)
}).strict().superRefine((response, context) => {
  const expectedCount = response.providerCount + response.accountCount
  if (response.reentryRequired !== expectedCount || response.entries.length !== expectedCount) {
    context.addIssue({ code: 'custom', path: ['entries'], message: 'portable manifest import counts are inconsistent' })
  }
  const correlations = new Set<string>()
  const destinationProviderIds = new Set<string>()
  for (const [index, entry] of response.entries.entries()) {
    const expectedCorrelation = index < response.providerCount
      ? `provider-${index}`
      : `account-${index - response.providerCount}`
    if (entry.correlation !== expectedCorrelation) {
      context.addIssue({ code: 'custom', path: ['entries', index, 'correlation'], message: 'portable manifest import entry order is invalid' })
    }
    if (correlations.has(entry.correlation)) {
      context.addIssue({ code: 'custom', path: ['entries', index, 'correlation'], message: 'portable manifest import correlations must be unique' })
    }
    correlations.add(entry.correlation)
    if (destinationProviderIds.has(entry.destinationProviderId)) {
      context.addIssue({ code: 'custom', path: ['entries', index, 'destinationProviderId'], message: 'portable manifest destination Provider IDs must be unique' })
    }
    destinationProviderIds.add(entry.destinationProviderId)
  }
})

export const providerRegistrySuccessSchemaV1 = z.union([
  providerRegistrySnapshotResponseSchemaV1,
  providerRegistryProviderResponseSchemaV1,
  providerRegistryDeletedResponseSchemaV1,
  providerRegistryRecoveredResponseSchemaV1,
  providerRegistryProbeResponseSchemaV1,
  providerRegistryCredentialCheckResponseSchemaV1,
  providerRegistryAccountObservationResponseSchemaV1,
  providerRegistryPortableManifestExportResponseSchemaV1,
  providerRegistryPortableManifestImportResponseSchemaV1
])

export const PROVIDER_REGISTRY_FAILURE_MESSAGES_V1 = {
  invalid_request: 'The provider registry request was rejected.',
  method_not_allowed: 'The HTTP method is not allowed for this provider registry operation.',
  not_found: 'The requested provider was not found.',
  conflict: 'The provider registry state has changed.',
  persistence_failure: 'The provider registry is temporarily unavailable.',
  credential_unavailable: 'Secure credential storage is temporarily unavailable. Check system security access and retry. Existing settings have been kept.',
  verification_failure: 'The provider registry operation could not be verified.',
  request_too_large: 'The provider registry request is too large.',
  unauthorized: 'Provider registry authentication is required.',
  runtime_unavailable: 'The provider registry is unavailable.',
  invalid_response: 'The provider registry returned an invalid response.'
} as const

export const providerRegistryFailureCodeSchemaV1 = z.enum(Object.keys(
  PROVIDER_REGISTRY_FAILURE_MESSAGES_V1
) as [keyof typeof PROVIDER_REGISTRY_FAILURE_MESSAGES_V1, ...(keyof typeof PROVIDER_REGISTRY_FAILURE_MESSAGES_V1)[]])

export const providerRegistryFailureSchemaV1 = z.object({
  schemaVersion: z.literal(PROVIDER_REGISTRY_SCHEMA_VERSION_V1),
  error: z.object({
    code: providerRegistryFailureCodeSchemaV1,
    message: z.string()
  }).strict()
}).strict().refine(
  (failure) => failure.error.message === PROVIDER_REGISTRY_FAILURE_MESSAGES_V1[failure.error.code],
  'provider registry failure message is not canonical'
)

export const providerRegistryResultSchemaV1 = z.union([
  providerRegistrySuccessSchemaV1,
  providerRegistryFailureSchemaV1
])

const accountScopeComponentSchemaV1 = (maximumBytes: number) => z.string().min(1).refine(
  (value) => value === value.trim() && !/[\u0000\r\n\t]/.test(value) && utf8ByteLength(value) <= maximumBytes,
  'account credential scope component is invalid'
)

export const accountCredentialPurposeSchemaV1 = z.enum([
  'provider-oauth-token-bundle',
  'provider-oauth-authorization-state',
  'mcp-oauth-access-token',
  'mcp-oauth-authorization-state',
  'extension-provider-account-token',
  'extension-oauth-authorization-state',
  'transport-telegram-bot-token',
  'transport-weixin-session-key',
  'transport-weixin-context-token',
  'transport-feishu-app-secret'
])

export const accountCredentialScopeSchemaV1 = z.object({
  owner: z.enum(['provider', 'mcp', 'extension', 'transport']),
  provider: accountScopeComponentSchemaV1(128),
  accountId: accountScopeComponentSchemaV1(256),
  channelId: z.union([z.literal(''), accountScopeComponentSchemaV1(256)]).optional(),
  purpose: accountCredentialPurposeSchemaV1
}).strict().superRefine((scope, context) => {
  const channelBound = scope.channelId !== undefined && scope.channelId !== ''
  const coherent =
    (scope.owner === 'provider' && (
      (scope.purpose === 'provider-oauth-token-bundle' && !channelBound && scope.accountId === scope.provider) ||
      (scope.purpose === 'provider-oauth-authorization-state' && channelBound)
    )) ||
    (scope.owner === 'mcp' && channelBound && scope.purpose.startsWith('mcp-')) ||
    (scope.owner === 'extension' && channelBound &&
      (scope.purpose === 'extension-provider-account-token' ||
        scope.purpose === 'extension-oauth-authorization-state')) ||
    (scope.owner === 'transport' && channelBound && scope.purpose.startsWith(`transport-${scope.provider}-`))
  if (!coherent) {
    context.addIssue({ code: 'custom', path: ['purpose'], message: 'account credential purpose is not bound to its owner and provider' })
  }
})

const boundedAccountSecretSchemaV1 = z.string().min(1).refine(
  (value) => !/[\u0000\r\n]/.test(value) && utf8ByteLength(value) <= PROVIDER_REGISTRY_MAX_SECRET_BYTES_V1,
  'account credential draft is invalid'
)

const oauthProtectedBindingSchemaV1 = z.object({
  owner: z.enum(['provider', 'mcp', 'extension']),
  issuer: z.string().min(1).max(2048),
  authorizationEndpoint: z.string().min(1).max(2048),
  tokenEndpoint: z.string().min(1).max(2048),
  revocationEndpoint: z.string().min(1).max(2048).optional(),
  clientId: z.string().min(1).max(256),
  provider: z.string().min(1).max(128),
  accountId: z.string().min(1).max(256),
  channelId: z.string().min(1).max(256).optional(),
  redirectUri: z.string().min(1).max(2048),
  scopes: z.array(z.string().min(1).max(256)).min(1).max(32),
  bindingKey: z.string().regex(/^oauthb_[A-Za-z0-9_-]{43}$/),
  ownerFingerprint: z.string().regex(/^[a-f0-9]{64}$/),
  ownerRevision: providerRegistryDecimalSchemaV1,
  ownerGeneration: providerRegistryDecimalSchemaV1,
  ownerIncarnation: z.string().min(1).max(256),
  profileBinding: z.string().regex(/^[a-f0-9]{64}$/)
}).strict()

export const accountCredentialDraftSchemaV1 = z.discriminatedUnion('kind', [
  z.object({
    kind: z.literal('telegram'),
    botToken: boundedAccountSecretSchemaV1,
    allowedChatIds: z.string().max(4096)
  }).strict(),
  z.object({ kind: z.literal('weixin'), sessionKey: boundedAccountSecretSchemaV1 }).strict(),
  z.object({
    kind: z.literal('weixin-context'),
    contextToken: boundedAccountSecretSchemaV1,
    parentGeneration: providerRegistryDecimalSchemaV1,
    parentIncarnation: providerRegistryIncarnationSchemaV1,
    expiresAtMs: z.number().int().positive()
  }).strict(),
  z.object({ kind: z.literal('feishu'), appSecret: boundedAccountSecretSchemaV1 }).strict(),
  z.object({ kind: z.literal('oauth-token'), token: boundedAccountSecretSchemaV1 }).strict(),
  z.object({ kind: z.literal('mcp-oauth-token'), token: boundedAccountSecretSchemaV1 }).strict(),
  z.object({ kind: z.literal('extension-oauth-token'), token: boundedAccountSecretSchemaV1 }).strict(),
  z.object({
    kind: z.literal('provider-oauth-bundle'),
    accessToken: boundedAccountSecretSchemaV1,
    refreshToken: boundedAccountSecretSchemaV1.optional(),
    idToken: boundedAccountSecretSchemaV1.optional(),
    subscriptionToken: boundedAccountSecretSchemaV1.optional(),
    tokenType: z.literal('Bearer'),
    expiresAtMs: z.number().int().positive().optional(),
    oauthBinding: oauthProtectedBindingSchemaV1
  }).strict(),
  z.object({
    kind: z.literal('mcp-oauth-bundle'),
    accessToken: boundedAccountSecretSchemaV1,
    refreshToken: boundedAccountSecretSchemaV1.optional(),
    tokenType: z.literal('Bearer'),
    expiresAtMs: z.number().int().positive().optional(),
    oauthBinding: oauthProtectedBindingSchemaV1
  }).strict(),
  z.object({
    kind: z.literal('extension-account-token'),
    token: boundedAccountSecretSchemaV1,
    bindingFingerprint: z.string().regex(/^[a-f0-9]{64}$/)
  }).strict(),
  z.object({
    kind: z.literal('extension-oauth-bundle'),
    accessToken: boundedAccountSecretSchemaV1,
    refreshToken: boundedAccountSecretSchemaV1.optional(),
    tokenType: z.literal('Bearer'),
    expiresAtMs: z.number().int().positive().optional(),
    oauthBinding: oauthProtectedBindingSchemaV1
  }).strict()
])

export const accountCredentialAnyExpectedStateSchemaV1 = z.union([
  providerRegistryConnectExpectedStateSchemaV1,
  providerRegistryExpectedStateSchemaV1
])

export const accountCredentialStatusRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('status'),
  scope: accountCredentialScopeSchemaV1
}).strict()

export const accountCredentialPutRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.literal('put'),
  scope: accountCredentialScopeSchemaV1,
  expected: accountCredentialAnyExpectedStateSchemaV1,
  credential: accountCredentialDraftSchemaV1
}).strict().superRefine((request, context) => {
  const coherent =
    (request.credential.kind === 'telegram' && request.scope.purpose === 'transport-telegram-bot-token') ||
    (request.credential.kind === 'weixin' && request.scope.purpose === 'transport-weixin-session-key') ||
    (request.credential.kind === 'weixin-context' && request.scope.purpose === 'transport-weixin-context-token') ||
    (request.credential.kind === 'feishu' && request.scope.purpose === 'transport-feishu-app-secret') ||
    (request.credential.kind === 'oauth-token' && request.scope.owner === 'provider' &&
      request.scope.purpose === 'provider-oauth-authorization-state') ||
    (request.credential.kind === 'mcp-oauth-token' && request.scope.owner === 'mcp' &&
      (request.scope.purpose === 'mcp-oauth-access-token' || request.scope.purpose === 'mcp-oauth-authorization-state')) ||
    (request.credential.kind === 'extension-oauth-token' && request.scope.owner === 'extension' &&
      request.scope.purpose === 'extension-oauth-authorization-state') ||
    (request.credential.kind === 'provider-oauth-bundle' && request.scope.owner === 'provider' &&
      request.scope.purpose === 'provider-oauth-token-bundle') ||
    (request.credential.kind === 'mcp-oauth-bundle' && request.scope.owner === 'mcp' &&
      request.scope.purpose === 'mcp-oauth-access-token') ||
    (request.credential.kind === 'extension-oauth-bundle' && request.scope.owner === 'extension' &&
      request.scope.purpose === 'extension-provider-account-token') ||
    (request.credential.kind === 'extension-account-token' && request.scope.owner === 'extension')
  if (!coherent) {
    context.addIssue({ code: 'custom', path: ['credential', 'kind'], message: 'account credential draft is not bound to its scope' })
  }
})

export const accountCredentialMutationRequestSchemaV1 = z.object({
  ...schemaVersionShape,
  operation: z.enum(['revoke', 'disconnect', 'delete']),
  scope: accountCredentialScopeSchemaV1,
  expected: accountCredentialAnyExpectedStateSchemaV1
}).strict()

export const accountCredentialRequestSchemaV1 = z.union([
  accountCredentialStatusRequestSchemaV1,
  accountCredentialPutRequestSchemaV1,
  accountCredentialMutationRequestSchemaV1
])

export const accountCredentialStatusSchemaV1 = z.enum(['absent', 'ready', 'revoked', 'disconnected', 'unusable'])

export const accountCredentialStateSchemaV1 = z.object({
  ...schemaVersionShape,
  scope: accountCredentialScopeSchemaV1,
  status: accountCredentialStatusSchemaV1,
  registryRevision: providerRegistryDecimalSchemaV1,
  registryIncarnation: providerRegistryIncarnationSchemaV1,
  providerRevision: providerRegistryDecimalSchemaV1,
  providerGeneration: providerRegistryDecimalSchemaV1,
  providerIncarnation: z.union([z.literal(''), providerRegistryIncarnationSchemaV1]),
  credentialPurpose: providerRegistryCredentialPurposeSchemaV1.optional()
}).strict().superRefine((state, context) => {
  const absent = state.status === 'absent'
  const providerAbsent = state.providerRevision === '0' && state.providerGeneration === '0' && state.providerIncarnation === ''
  if (absent !== providerAbsent) {
    context.addIssue({ code: 'custom', path: ['status'], message: 'account credential state fence is inconsistent' })
  }
})

export const accountCredentialResultSchemaV1 = z.union([
  accountCredentialStateSchemaV1,
  providerRegistryFailureSchemaV1
])

export type ProviderRegistryProviderInputV1 = z.infer<typeof providerRegistryProviderInputSchemaV1>
export type ProviderRegistryOAuthBindingV1 = z.infer<typeof providerRegistryOAuthBindingSchemaV1>
export type ProviderRegistryAccountObservationBindingV1 = z.infer<typeof providerRegistryAccountObservationBindingSchemaV1>
export type ProviderRegistryExpectedStateV1 = z.infer<typeof providerRegistryExpectedStateSchemaV1>
export type ProviderRegistryCredentialSetV1 = z.infer<typeof providerRegistryCredentialSetSchemaV1>
export type ProviderRegistryRequestV1 = z.infer<typeof providerRegistryRequestSchemaV1>
export type ProviderRegistryPublicProviderV1 = z.infer<typeof providerRegistryPublicProviderSchemaV1>
export type ProviderRegistryProbeResponseV1 = z.infer<typeof providerRegistryProbeResponseSchemaV1>
export type ProviderRegistryAccountObservationResponseV1 = z.infer<typeof providerRegistryAccountObservationResponseSchemaV1>
export type ProviderRegistrySuccessV1 = z.infer<typeof providerRegistrySuccessSchemaV1>
export type ProviderRegistryPortableManifestExportResponseV1 = z.infer<typeof providerRegistryPortableManifestExportResponseSchemaV1>
export type ProviderRegistryPortableManifestImportEntryResponseV1 = z.infer<typeof providerRegistryPortableManifestImportEntryResponseSchemaV1>
export type ProviderRegistryPortableManifestImportResponseV1 = z.infer<typeof providerRegistryPortableManifestImportResponseSchemaV1>
export type ProviderRegistryFailureCodeV1 = keyof typeof PROVIDER_REGISTRY_FAILURE_MESSAGES_V1
export type ProviderRegistryFailureV1 = z.infer<typeof providerRegistryFailureSchemaV1>
export type ProviderRegistryResultV1 = z.infer<typeof providerRegistryResultSchemaV1>
export type AccountCredentialPurposeV1 = z.infer<typeof accountCredentialPurposeSchemaV1>
export type AccountCredentialScopeV1 = z.infer<typeof accountCredentialScopeSchemaV1>
export type AccountCredentialDraftV1 = z.infer<typeof accountCredentialDraftSchemaV1>
export type AccountCredentialRequestV1 = z.infer<typeof accountCredentialRequestSchemaV1>
export type AccountCredentialStateV1 = z.infer<typeof accountCredentialStateSchemaV1>
export type AccountCredentialResultV1 = z.infer<typeof accountCredentialResultSchemaV1>
