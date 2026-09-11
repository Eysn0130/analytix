const CREDENTIAL_NAME = String.raw`(?:authorization|proxy[-_ ]?authorization|x[-_ ]?api[-_ ]?key|api[-_ ]?key|apikey|access[-_ ]?token|refresh[-_ ]?token|id[-_ ]?token|token|aws[-_ ]?access[-_ ]?key[-_ ]?id|aws[-_ ]?secret[-_ ]?access[-_ ]?key|aws[-_ ]?(?:security|session)[-_ ]?token|github[-_ ]?token|slack[-_ ]?token|session[-_ ]?token|client[-_ ]?secret|password|passwd|pwd|secret|credential|cookie|[A-Za-z][A-Za-z0-9_. -]{0,95}(?:token|secret|password|credential|api[-_. ]?key))`
const DOUBLE_QUOTED_CREDENTIAL = String.raw`"(?:\\.|[^"\\\r\n])*"`
const SINGLE_QUOTED_CREDENTIAL = String.raw`'(?:\\.|[^'\\\r\n])*'`
const UNQUOTED_CREDENTIAL = String.raw`[^\s,;}\]]+`
const CREDENTIAL_VALUE = String.raw`(?:(?:bearer|basic)\s+(?:${DOUBLE_QUOTED_CREDENTIAL}|${SINGLE_QUOTED_CREDENTIAL}|${UNQUOTED_CREDENTIAL})|${DOUBLE_QUOTED_CREDENTIAL}|${SINGLE_QUOTED_CREDENTIAL}|${UNQUOTED_CREDENTIAL})`
const CREDENTIAL_ASSIGNMENT = new RegExp(
  String.raw`(^|[^A-Za-z0-9_-])["']?(${CREDENTIAL_NAME})["']?\s*[:=]\s*(${CREDENTIAL_VALUE})`,
  'gi'
)
const CREDENTIAL_FLAG = new RegExp(
  String.raw`(^|[^A-Za-z0-9_-])--(${CREDENTIAL_NAME})(?:=|\s+)(${CREDENTIAL_VALUE})`,
  'gi'
)
const AUTH_SCHEME = new RegExp(
  String.raw`\b(bearer|basic)\s+(${DOUBLE_QUOTED_CREDENTIAL}|${SINGLE_QUOTED_CREDENTIAL}|[A-Za-z0-9._~+/=-]+)`,
  'gi'
)
const KNOWN_TOKEN = /\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr][_-][A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{8,}|xox[baprsocd]-[-A-Za-z0-9.]{8,}|xapp-[-A-Za-z0-9.]{8,}|xoxe(?:\.xoxp)?-[-A-Za-z0-9.]{8,})\b/g
const AWS_ACCESS_KEY = /\b(?:AKIA|ASIA)[A-Z0-9]{16}\b/g
const PRIVATE_KEY_BLOCK = /-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----/g
const PRIVATE_KEY_REMAINDER = /-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*$/g
const URL_CANDIDATE = /\b[a-z][a-z0-9+.-]*:\/\/[^\s"'<>]+/gi
const MAX_SECRET_VALUE_DEPTH = 32
const MAX_SECRET_VALUE_NODES = 100_000

const SAFE_CREDENTIAL_METADATA_KEYS = new Set([
  'hasapikey',
  'hascredential',
  'haspassword',
  'hassecret',
  'hastoken'
])
const CREDENTIAL_KEYS = new Set([
  'authorization',
  'proxyauthorization',
  'xapikey',
  'apikey',
  'accesstoken',
  'refreshtoken',
  'idtoken',
  'token',
  'clientsecret',
  'password',
  'passwd',
  'pwd',
  'secret',
  'credential',
  'cookie',
  'awsaccesskeyid',
  'awssecretaccesskey',
  'awssecuritytoken',
  'awssessiontoken',
  'sessiontoken',
  'githubtoken',
  'slacktoken'
])

type SecretTraversalState = {
  active: WeakSet<object>
  remainingNodes: number
}

export const REDACTED_SECRET = '<redacted>'

export function redactSecrets<T>(value: T): T {
  return redact(value, '', newSecretTraversalState(), 0) as T
}

function redact(
  value: unknown,
  key: string,
  state: SecretTraversalState,
  depth: number
): unknown {
  if (!consumeSecretNode(state, depth)) return REDACTED_SECRET
  if (typeof value === 'string') {
    if (credentialRequiresRedaction(key, value)) return REDACTED_SECRET
    return redactSecretText(value)
  }
  if (!value || typeof value !== 'object') {
    if (credentialRequiresRedaction(key, value)) return REDACTED_SECRET
    return value
  }
  if (state.active.has(value)) return REDACTED_SECRET
  state.active.add(value)
  try {
    if (Array.isArray(value)) {
      const projected: unknown[] = []
      for (const item of value) {
        if (state.remainingNodes <= 0) {
          projected.push(REDACTED_SECRET)
          break
        }
        projected.push(redact(item, key, state, depth + 1))
      }
      return projected
    }
    const out: Record<string, unknown> = {}
    for (const childKey in value as Record<string, unknown>) {
      if (!Object.prototype.hasOwnProperty.call(value, childKey)) continue
      if (state.remainingNodes <= 0) {
        defineSafeProperty(out, REDACTED_SECRET, REDACTED_SECRET)
        break
      }
      const childValue = (value as Record<string, unknown>)[childKey]
      const projectedKey = redactSecretText(childKey)
      const projectedValue = credentialRequiresRedaction(childKey, childValue)
        ? REDACTED_SECRET
        : redact(childValue, childKey, state, depth + 1)
      defineSafeProperty(out, projectedKey, projectedValue)
    }
    return out
  } finally {
    state.active.delete(value)
  }
}

export function containsSecretMaterial(value: unknown): boolean {
  return containsSecret(value, '', newSecretTraversalState(), 0)
}

function containsSecret(
  value: unknown,
  key: string,
  state: SecretTraversalState,
  depth: number
): boolean {
  if (!consumeSecretNode(state, depth)) return true
  if (typeof value === 'string') {
    return credentialRequiresRedaction(key, value) || redactSecretText(value) !== value
  }
  if (!value || typeof value !== 'object') {
    return credentialRequiresRedaction(key, value)
  }
  if (state.active.has(value)) return true
  state.active.add(value)
  try {
    if (Array.isArray(value)) {
      for (const child of value) {
        if (containsSecret(child, '', state, depth + 1)) return true
      }
      return false
    }
    for (const childKey in value as Record<string, unknown>) {
      if (!Object.prototype.hasOwnProperty.call(value, childKey)) continue
      const childValue = (value as Record<string, unknown>)[childKey]
      if (redactSecretText(childKey) !== childKey ||
          credentialRequiresRedaction(childKey, childValue) ||
          containsSecret(childValue, childKey, state, depth + 1)) {
        return true
      }
    }
    return false
  } finally {
    state.active.delete(value)
  }
}

export function redactSecretText(value: string): string {
  if (value.trim() === '') return value
  const explicitSecrets = collectExplicitSecrets(value).sort((left, right) => right.length - left.length)
  let projected = value.replace(URL_CANDIDATE, redactUrl)
  projected = projected.replace(PRIVATE_KEY_BLOCK, REDACTED_SECRET)
  projected = projected.replace(PRIVATE_KEY_REMAINDER, REDACTED_SECRET)
  projected = projected.replace(
    CREDENTIAL_ASSIGNMENT,
    (match, prefix: string, key: string, rawSecret: string) =>
      !credentialLiteralRequiresRedaction(key, rawSecret)
        ? match
        : `${prefix}${key}=${REDACTED_SECRET}`
  )
  projected = projected.replace(
    CREDENTIAL_FLAG,
    (match, prefix: string, key: string, rawSecret: string) =>
      !credentialLiteralRequiresRedaction(key, rawSecret)
        ? match
        : `${prefix}--${key}=${REDACTED_SECRET}`
  )
  projected = projected.replace(AUTH_SCHEME, (match, scheme: string, rawSecret: string) =>
    normalizedCredentialLiteral(rawSecret) === REDACTED_SECRET
      ? match
      : `${scheme} ${REDACTED_SECRET}`
  )
  projected = projected.replace(KNOWN_TOKEN, REDACTED_SECRET)
  projected = projected.replace(AWS_ACCESS_KEY, REDACTED_SECRET)
  for (const secret of explicitSecrets) {
    if (secret.length < 4 || secret === REDACTED_SECRET) continue
    projected = projected.split(secret).join(REDACTED_SECRET)
  }
  return projected
}

function collectExplicitSecrets(value: string): string[] {
  const secrets = new Set<string>()
  const add = (candidate: string | undefined): void => {
    const normalized = normalizedCredentialLiteral(String(candidate ?? ''))
    if (normalized.length >= 4 && normalized !== REDACTED_SECRET) secrets.add(normalized)
  }
  for (const match of value.matchAll(clonePattern(CREDENTIAL_ASSIGNMENT))) {
    if (credentialLiteralRequiresRedaction(match[2] ?? '', match[3] ?? '')) add(match[3])
  }
  for (const match of value.matchAll(clonePattern(CREDENTIAL_FLAG))) {
    if (credentialLiteralRequiresRedaction(match[2] ?? '', match[3] ?? '')) add(match[3])
  }
  for (const match of value.matchAll(clonePattern(AUTH_SCHEME))) add(match[2])
  for (const match of value.matchAll(clonePattern(KNOWN_TOKEN))) add(match[0])
  for (const match of value.matchAll(clonePattern(AWS_ACCESS_KEY))) add(match[0])
  for (const match of value.matchAll(clonePattern(URL_CANDIDATE))) {
    try {
      const parsed = new URL(match[0])
      add(safeDecodeURIComponent(parsed.username))
      add(safeDecodeURIComponent(parsed.password))
      for (const [queryKey, queryValue] of parsed.searchParams.entries()) {
        if (credentialRequiresRedaction(queryKey, queryValue, true)) add(queryValue)
      }
    } catch {
      add(match[0])
    }
  }
  return [...secrets]
}

function redactUrl(raw: string): string {
  try {
    const parsed = new URL(raw)
    let sensitive = false
    if (parsed.username || parsed.password) {
      parsed.username = REDACTED_SECRET
      parsed.password = REDACTED_SECRET
      sensitive = true
    }
    for (const key of [...parsed.searchParams.keys()]) {
      const queryValue = parsed.searchParams.get(key) ?? ''
      if (!credentialRequiresRedaction(key, queryValue, true)) continue
      parsed.searchParams.set(key, REDACTED_SECRET)
      sensitive = true
    }
    if (containsInlineCredential(parsed.hash.slice(1))) {
      parsed.hash = REDACTED_SECRET
      sensitive = true
    }
    return sensitive ? parsed.toString() : raw
  } catch {
    return REDACTED_SECRET
  }
}

function containsInlineCredential(value: string): boolean {
  return clonePattern(CREDENTIAL_ASSIGNMENT, 'i').test(value) ||
    clonePattern(AUTH_SCHEME, 'i').test(value)
}

function isCredentialKey(key: string): boolean {
  const normalized = key.toLowerCase().trim().replace(/[_\-.\s]/g, '')
  if (CREDENTIAL_KEYS.has(normalized)) return true
  return /(token|secret|password|credential|apikey)$/.test(normalized)
}

function credentialRequiresRedaction(
  key: string,
  value: unknown,
  allowSerializedBoolean = false
): boolean {
  if (!isCredentialKey(key) || !credentialValuePresent(value)) return false
  const normalizedKey = key.toLowerCase().trim().replace(/[_\-.\s]/g, '')
  if (!SAFE_CREDENTIAL_METADATA_KEYS.has(normalizedKey)) return true
  if (typeof value === 'boolean') return false
  return allowSerializedBoolean && typeof value === 'string' && /^(?:true|false)$/i.test(value.trim())
    ? false
    : true
}

function credentialLiteralRequiresRedaction(key: string, rawValue: string): boolean {
  const normalized = normalizedCredentialLiteral(rawValue)
  if (normalized === REDACTED_SECRET) return false
  return credentialRequiresRedaction(key, normalized, true)
}

function credentialValuePresent(value: unknown): boolean {
  if (value === null || value === undefined) return false
  if (typeof value === 'string') {
    const normalized = value.trim()
    return normalized !== '' && normalized !== REDACTED_SECRET
  }
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'object') {
    for (const key in value as Record<string, unknown>) {
      if (Object.prototype.hasOwnProperty.call(value, key)) return true
    }
    return false
  }
  return true
}

function normalizedCredentialLiteral(value: string): string {
  let normalized = value.trim()
  if ((normalized.startsWith('"') && normalized.endsWith('"')) ||
      (normalized.startsWith("'") && normalized.endsWith("'"))) {
    normalized = normalized.slice(1, -1)
  }
  normalized = normalized.replace(/^(?:bearer|basic)\s+/i, '').trim()
  if ((normalized.startsWith('"') && normalized.endsWith('"')) ||
      (normalized.startsWith("'") && normalized.endsWith("'"))) {
    normalized = normalized.slice(1, -1)
  }
  return normalized
}

function clonePattern(pattern: RegExp, flags = pattern.flags): RegExp {
  return new RegExp(pattern.source, flags)
}

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function newSecretTraversalState(): SecretTraversalState {
  return {
    active: new WeakSet<object>(),
    remainingNodes: MAX_SECRET_VALUE_NODES
  }
}

function consumeSecretNode(state: SecretTraversalState, depth: number): boolean {
  if (depth > MAX_SECRET_VALUE_DEPTH || state.remainingNodes <= 0) return false
  state.remainingNodes -= 1
  return true
}

function defineSafeProperty(target: Record<string, unknown>, key: string, value: unknown): void {
  Object.defineProperty(target, key, {
    value,
    enumerable: true,
    configurable: true,
    writable: true
  })
}
