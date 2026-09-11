// Logs are an ordinary, non-controlled projection. They may preserve useful
// diagnostic context, but never a complete financial account, identity
// number, phone number, IP address, or device identifier. This projection is
// intentionally lossy; exact values belong only in an authorized controlled
// artifact.

import { containsRestrictedEvidence } from './restricted-evidence-projection'
import { redactSecretText } from './secret-redaction'

const FORMAT_CHARACTERS = /\p{Cf}/gu
const IDENTIFIER_SEPARATOR = String.raw`[\s\-‐‑‒–—―_/\\.·•]*`
const CONTEXTUAL_IDENTIFIER_SEPARATOR = String.raw`[\s\-‐‑‒–—―_/\\.·•*]*`

const MAC_ADDRESS = /(^|[^0-9A-Fa-f])(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}(?=$|[^0-9A-Fa-f])/g
const IPV4_ADDRESS = /(^|[^0-9])(?:\d{1,3}\.){3}\d{1,3}(?=$|[^0-9])/g
const EMAIL_ADDRESS = /(^|[^A-Za-z0-9.!#$%&'*+/=?^_`{|}~-])(?:[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+(?:\.[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+)*)@(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}(?=$|[^A-Za-z0-9.-])/g
const IDENTITY_NUMBER = new RegExp(
  `(^|[^0-9A-Za-z])(\\p{N}(?:${IDENTIFIER_SEPARATOR}\\p{N}){16}${IDENTIFIER_SEPARATOR}[0-9Xx])(?=$|[^0-9A-Za-z])`,
  'gu'
)
const PHONE_NUMBER = new RegExp(
  `(^|[^0-9])(1(?:${IDENTIFIER_SEPARATOR}\\p{N}){10})(?=$|[^0-9])`,
  'gu'
)
const LONG_FINANCIAL_IDENTIFIER = new RegExp(
  `(^|[^\\p{N}])(\\p{N}(?:${IDENTIFIER_SEPARATOR}\\p{N}){11,})(?=$|[^\\p{N}])`,
  'gu'
)
const CONTEXTUAL_FINANCIAL_IDENTIFIER = new RegExp(
  `((?:(?:银行)?(?:账号|帐号|账户|卡号|账卡号)|(?:bank\\s*)?(?:account|acct|card)(?:[\\s_-]*(?:id|no|number))?)\\s*[:：=]?\\s*)(\\p{N}(?:${CONTEXTUAL_IDENTIFIER_SEPARATOR}\\p{N}){7,})`,
  'giu'
)

const MAX_PUBLIC_PII_DEPTH = 32
const MAX_PUBLIC_PII_NODES = 100_000
const PUBLIC_TEXT_KEYS = new Set([
  'text', 'displaytext', 'prompt', 'question', 'questions', 'summary', 'message', 'title', 'goal',
  'todos', 'description', 'label', 'header', 'reviewtext', 'quote', 'reason', 'error', 'detail',
  'details', 'path', 'relativepath', 'filename', 'name', 'remark', 'memo', 'note', 'content',
  'output', 'result', 'data', 'value', 'target', 'arguments', 'answer', 'answers', 'objective',
  'step', 'blockedreason', 'command', 'instructions', 'paths', 'findings', 'diffsummary',
  'outputpreview', 'suggestion', 'artifactpath', 'worktreepath', 'workspace', 'sourceref', '摘要', '备注'
])
const TYPED_PII_KEYS = new Set([
  'account', 'accountid', 'accountno', 'accountnumber', 'acct', 'acctno', 'card', 'cardid',
  'cardno', 'cardnumber', 'identity', 'identityid', 'identityno', 'identitynumber', 'idno',
  'phone', 'phonenumber', 'mobile', 'mobilenumber', 'telephone', 'tel', 'email', 'emailaddress',
  'mail', 'mac', 'macaddress', 'ip', 'ipaddress', 'deviceid', 'deviceidentifier',
  'devicefingerprint', 'imei', 'address', 'homeaddress', 'postaladdress', 'person', 'personname',
  'holdername', 'accountname', '账号', '账户', '卡号', '银行卡号', '身份证', '身份证号',
  '证件号', '电话', '手机', '手机号', '邮箱', '设备标识', '设备指纹', '住址', '地址',
  '开户地址', '姓名', '户名', '持卡人', '开户人'
])

const SAFE_TYPED_PII_PLACEHOLDER = /^(?:<redacted>|<redacted-identifier(?::[^>]{1,32})?>|\[(?:ACCOUNT|IDENTITY|PHONE|EMAIL|IP|DEVICE|PERSON|ADDRESS|PII)\])$/i
const PUBLIC_OPAQUE_DIGEST_KEYS = new Set(['contenthash'])
const PUBLIC_SCHEMA_DIGEST_KEYS = new Set([
  'rejectedtoolnormalizednamesha256',
  'advertisedtoolmanifesthash',
  'advertisednamesetsortedhash',
  'providerrequesttoolmanifesthash',
  'runtoolstepmanifesthash'
])
const CANONICAL_OPAQUE_DIGEST = /^(?=[a-f0-9]{64}$)(?=.*[a-f])[a-f0-9]{64}$/
const CANONICAL_SCHEMA_DIGEST = /^[a-f0-9]{64}$/

const CASE_ENTITY_REFERENCE_V1 = /cer1_[a-p]{64}/
const PRIVATE_SOURCE_ROW_REFERENCE_V1 = /srow1_[0-9a-f]{64}/
const CASE_FACT_ASSERTION_PHRASES = [
  '实际控制', '实际控制人', '控制关系', '关联关系', '人员关系',
  '亲属', '配偶', '夫妻', '父子', '父女', '母子', '母女', '兄弟', '姐妹',
  '银行账号', '银行卡号', '账户余额', '交易金额', '转账金额', '收款金额', '付款金额',
  '投标报价', '报价金额', '串通投标', '围标', '行贿', '利益输送', '具备立案条件',
  'mac地址', '设备标识', '设备指纹'
]
const MONETARY_CASE_CUES = ['金额', '余额', '转账', '收款', '付款', '支付', '收入', '支出', '流水', '报价', '价款', '涉案']

type PublicPIITraversalState = {
  active: WeakSet<object>
  remainingNodes: number
}

/**
 * Mirrors the Go ordinary-result public-sink guard for canonical case-entity
 * and private source-row references, including their JSON Unicode-escaped
 * ASCII spellings. Opaque/cyclic/over-budget values fail closed.
 */
export function containsInternalCaseEntityReference(value: unknown): boolean {
  const active = new WeakSet<object>()
  let remaining = MAX_PUBLIC_PII_NODES
  const visit = (current: unknown, depth: number): boolean => {
    if (depth > MAX_PUBLIC_PII_DEPTH || remaining <= 0) return true
    remaining -= 1
    if (typeof current === 'string') {
      const decoded = decodeASCIIJSONUnicodeEscapes(current)
      return CASE_ENTITY_REFERENCE_V1.test(current) || PRIVATE_SOURCE_ROW_REFERENCE_V1.test(current) ||
        CASE_ENTITY_REFERENCE_V1.test(decoded) || PRIVATE_SOURCE_ROW_REFERENCE_V1.test(decoded)
    }
    if (!current || typeof current !== 'object') return false
    if (active.has(current)) return true
    active.add(current)
    try {
      if (Array.isArray(current)) return current.some((entry) => visit(entry, depth + 1))
      return Object.entries(current as Record<string, unknown>).some(([key, entry]) =>
        visit(key, depth + 1) || visit(entry, depth + 1)
      )
    } finally {
      active.delete(current)
    }
  }
  return visit(value, 0)
}

/** Mirrors the Go ordinary-result post-projection containment guard. */
export function containsProtectedCaseFactCandidate(text: string): boolean {
  if (containsOrdinaryPublicPII(text)) return true
  const normalized = normalizeCaseFactText(text)
  if (CASE_FACT_ASSERTION_PHRASES.some((phrase) => normalized.includes(phrase))) return true
  if (!/[0-9]/.test(normalized)) return false
  if (containsCurrencyAmount(normalized) || /人民币|美元|欧元|英镑/.test(normalized)) return true
  return MONETARY_CASE_CUES.some((phrase) => normalized.includes(phrase))
}

function decodeASCIIJSONUnicodeEscapes(text: string): string {
  return text.replace(/\\u([0-9a-fA-F]{4})/g, (source, hex: string) => {
    const code = Number.parseInt(hex, 16)
    return code <= 0x7f ? String.fromCharCode(code) : source
  })
}

function normalizeCaseFactText(text: string): string {
  let normalized = ''
  for (const character of text.toLowerCase()) {
    const codePoint = character.codePointAt(0) ?? 0
    if (codePoint >= 0xff10 && codePoint <= 0xff19) {
      normalized += String.fromCharCode('0'.charCodeAt(0) + codePoint - 0xff10)
      continue
    }
    if (character === '\u200b' || character === '\u200c' || character === '\u200d' ||
        character === '\u2060' || character === '\ufeff') continue
    if (character === '，') normalized += ','
    else if (character === '。') normalized += '.'
    else if (character === '：') normalized += ':'
    else if (character === '－' || character === '—' || character === '–') normalized += '-'
    else normalized += character
  }
  return normalized
}

function containsCurrencyAmount(text: string): boolean {
  for (let index = 0; index < text.length; index += 1) {
    const currency = text[index]
    if (!'$¥￥€£'.includes(currency)) continue
    let digits = 0
    let separator = false
    for (let cursor = index + 1; cursor < text.length; cursor += 1) {
      const next = text[cursor]
      if (/\s/.test(next) && digits === 0) continue
      if (/[0-9]/.test(next)) {
        digits += 1
        continue
      }
      if (next === ',' || next === '.') {
        separator = true
        continue
      }
      break
    }
    if (digits >= 2 || (digits > 0 && separator) || (currency !== '$' && digits > 0)) return true
  }
  return false
}

export function projectOrdinaryLogPII(text: string): string {
  const original = String(text)
  const detectionText = original.normalize('NFKC').replace(FORMAT_CHARACTERS, '')
  let projected = detectionText
  projected = projected.replace(MAC_ADDRESS, (_match, prefix: string) => `${prefix}[DEVICE]`)
  projected = projected.replace(IPV4_ADDRESS, (_match, prefix: string) => `${prefix}[IP]`)
  projected = projected.replace(EMAIL_ADDRESS, (_match, prefix: string) => `${prefix}[EMAIL]`)
  projected = projected.replace(IDENTITY_NUMBER, (_match, prefix: string) => `${prefix}[IDENTITY]`)
  projected = projected.replace(PHONE_NUMBER, (_match, prefix: string) => `${prefix}[PHONE]`)
  projected = projected.replace(
    CONTEXTUAL_FINANCIAL_IDENTIFIER,
    (_match, prefix: string) => `${prefix}[ACCOUNT]`
  )
  projected = projected.replace(
    LONG_FINANCIAL_IDENTIFIER,
    (_match, prefix: string) => `${prefix}[ACCOUNT]`
  )
  return projected === detectionText ? original : projected
}

/**
 * Canonical lossy projection for ordinary UI, SSE-adjacent renderer state,
 * logs, history, transcripts, and non-controlled exports. Exact restricted
 * identifiers remain available only through an authorized controlled
 * artifact; callers must never send this projected value back as analysis
 * input in place of the user's original text.
 */
export function projectOrdinaryPublicText(text: string): string {
  const source = String(text)
  if (containsRestrictedEvidence(source) || containsInternalCaseEntityReference(source)) return ''
  return projectOrdinaryLogPII(redactSecretText(source))
}

export function containsOrdinaryPublicPII(value: unknown): boolean {
  if (typeof value === 'string') return projectOrdinaryLogPII(value) !== value
  return containsPublicPII(value, false, newPublicPIITraversalState(), 0)
}

function containsPublicPII(
  value: unknown,
  inheritedText: boolean,
  state: PublicPIITraversalState,
  depth: number
): boolean {
  if (!consumePublicPIINode(state, depth)) return true
  if (typeof value === 'string') {
    return inheritedText && projectOrdinaryLogPII(value) !== value
  }
  if (!value || typeof value !== 'object') {
    return inheritedText && value !== null && value !== undefined &&
      projectOrdinaryLogPII(String(value)) !== String(value)
  }
  if (state.active.has(value)) return true
  state.active.add(value)
  try {
    if (Array.isArray(value)) {
      for (const entry of value) {
        if (containsPublicPII(entry, inheritedText, state, depth + 1)) return true
      }
      return false
    }
    for (const key in value as Record<string, unknown>) {
      if (!Object.prototype.hasOwnProperty.call(value, key)) continue
      const entry = (value as Record<string, unknown>)[key]
      const normalized = normalizePublicKey(key)
      if (PUBLIC_OPAQUE_DIGEST_KEYS.has(normalized) &&
          typeof entry === 'string' && CANONICAL_OPAQUE_DIGEST.test(entry)) {
        continue
      }
      if (PUBLIC_SCHEMA_DIGEST_KEYS.has(normalized) &&
          typeof entry === 'string' && CANONICAL_SCHEMA_DIGEST.test(entry)) {
        continue
      }
      if (TYPED_PII_KEYS.has(normalized) && containsUnmaskedTypedPII(entry, state, depth + 1)) {
        return true
      }
      if (containsPublicPII(
        entry,
        inheritedText || PUBLIC_TEXT_KEYS.has(normalized) || TYPED_PII_KEYS.has(normalized),
        state,
        depth + 1
      )) {
        return true
      }
    }
    return false
  } finally {
    state.active.delete(value)
  }
}

function normalizePublicKey(value: string): string {
  return value.toLowerCase().trim().replace(/[^\p{L}\p{N}]/gu, '')
}

function containsUnmaskedTypedPII(
  value: unknown,
  state: PublicPIITraversalState,
  depth: number
): boolean {
  if (!consumePublicPIINode(state, depth)) return true
  if (typeof value === 'string') {
    const normalized = value.trim()
    return normalized !== '' && !SAFE_TYPED_PII_PLACEHOLDER.test(normalized)
  }
  if (value === null || value === undefined) return false
  if (Array.isArray(value)) {
    for (const entry of value) {
      if (containsUnmaskedTypedPII(entry, state, depth + 1)) return true
    }
    return false
  }
  if (typeof value === 'object') {
    for (const key in value as Record<string, unknown>) {
      if (Object.prototype.hasOwnProperty.call(value, key)) return true
    }
    return false
  }
  return true
}

function newPublicPIITraversalState(): PublicPIITraversalState {
  return {
    active: new WeakSet<object>(),
    remainingNodes: MAX_PUBLIC_PII_NODES
  }
}

function consumePublicPIINode(state: PublicPIITraversalState, depth: number): boolean {
  if (depth > MAX_PUBLIC_PII_DEPTH || state.remainingNodes <= 0) return false
  state.remainingNodes -= 1
  return true
}
