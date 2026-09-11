const DEFAULT_ACCOUNT_ID = 'default'

function isBlockedObjectKey(value) {
  return value === '__proto__' || value === 'prototype' || value === 'constructor'
}

export function normalizeAccountId(value) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return DEFAULT_ACCOUNT_ID
  const lowered = trimmed.toLowerCase()
  const normalized = isValidAccountId(lowered)
    ? lowered
    : sanitizeAccountId(lowered)
  return normalized && !isBlockedObjectKey(normalized) ? normalized : DEFAULT_ACCOUNT_ID
}

export function normalizeOptionalAccountId(value) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return undefined
  const normalized = normalizeAccountId(trimmed)
  return normalized === DEFAULT_ACCOUNT_ID && trimmed.toLowerCase() !== DEFAULT_ACCOUNT_ID
    ? undefined
    : normalized
}

function isValidAccountId(value) {
  if (!isAccountIdStart(value[0]) || value.length > 64) return false
  for (const char of value) {
    if (!isAccountIdChar(char)) return false
  }
  return true
}

function sanitizeAccountId(value) {
  let out = ''
  for (const char of value) {
    if (isAccountIdChar(char)) {
      out += char
    } else if (out && out[out.length - 1] !== '-') {
      out += '-'
    }
  }
  return trimBoundaryHyphens(out).slice(0, 64)
}

function trimBoundaryHyphens(value) {
  let start = 0
  let end = value.length
  while (start < end && value[start] === '-') start += 1
  while (end > start && value[end - 1] === '-') end -= 1
  return value.slice(start, end)
}

function isAccountIdStart(char) {
  return isAsciiLetter(char) || isAsciiDigit(char)
}

function isAccountIdChar(char) {
  return isAccountIdStart(char) || char === '_' || char === '-'
}

function isAsciiLetter(char) {
  if (!char) return false
  const code = char.charCodeAt(0)
  return code >= 97 && code <= 122
}

function isAsciiDigit(char) {
  if (!char) return false
  const code = char.charCodeAt(0)
  return code >= 48 && code <= 57
}

export { DEFAULT_ACCOUNT_ID }
