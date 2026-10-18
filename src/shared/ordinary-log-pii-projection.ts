// Logs are an ordinary, non-controlled projection. They may preserve useful
// diagnostic context, but never a complete financial account, identity
// number, phone number, IP address, or device identifier. This projection is
// intentionally lossy; exact values belong only in an authorized controlled
// artifact.

import { containsRestrictedEvidence } from './restricted-evidence-projection'
import { redactSecretText } from './secret-redaction'

const FORMAT_CHARACTERS = /\p{Cf}/gu
const FORMAT_CHARACTER = /\p{Cf}/u
const DECIMAL_DIGIT = /\p{Nd}/u
const WHITE_SPACE = /\p{White_Space}/u

const MAC_ADDRESS = /(^|[^0-9A-Fa-f])(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}(?=$|[^0-9A-Fa-f])/g
const IPV4_ADDRESS = /(^|[^0-9])(?:\d{1,3}\.){3}\d{1,3}(?=$|[^0-9])/g
const EMAIL_ADDRESS = /(^|[^A-Za-z0-9.!#$%&'*+/=?^_`{|}~-])(?:[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+(?:\.[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+)*)@(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}(?=$|[^A-Za-z0-9.-])/g

// Only free-form display text uses this locator projection. Keep ordinary
// relative code paths and HTTP(S) URL tokens intact; structured path fields
// used for execution are not rewritten by the PII classification helpers.
// A closed quote owns the whole locator on that line, including spaces and
// brackets. Unquoted locators stop at whitespace but permit filename brackets.
const PRIVATE_SOURCE_LOCATOR = /https?:\/\/[^\s"'`<>]+|(["'`])(?:file:\/\/|\/(?:Users|Volumes|private|var|tmp|home|cases?)\/|[a-z]:[\\/]|~(?:[a-z0-9_.-]+)?[\\/]|\\\\|\/\/)[^\r\n]*?\1|(^|[\s"'`=(:：[{,，;；])(?:file:\/\/|\/(?:Users|Volumes|private|var|tmp|home|cases?)\/|[a-z]:[\\/]|~(?:[a-z0-9_.-]+)?[\\/]|\\\\|\/\/)[^\s"'`<>{},;，。；！？]+/giu
const PRIVATE_DIFF_HEADER = /^((?:---|\+\+\+)[\t ]+)"?[ab]\/(?:file:\/\/|\/(?:Users|Volumes|private|var|tmp|home|cases?)\/|[a-z]:[\\/]|~(?:[a-z0-9_.-]+)?[\\/]|\\\\|\/\/)[^\r\n]*/gimu

function projectPrivateSourceLocatorsRaw(text: string): string {
  return text.replace(PRIVATE_DIFF_HEADER, '$1[PRIVATE_PATH]').replace(PRIVATE_SOURCE_LOCATOR, (match, quote: string | undefined, prefix: string | undefined) =>
    quote !== undefined ? `${quote}[PRIVATE_PATH]${quote}` :
      prefix === undefined ? match : `${prefix}[PRIVATE_PATH]`
  )
}

// Escape pairs and ordinary URI characters must be disjoint: otherwise a
// missing closing parenthesis can exponentially re-partition backslashes.
const COMPOSER_MENTION = /([@$])\[((?:\\.|[^\]\\])*)\]\(((?:\\.|[^)\s\\])*)\)/gu

function safeMentionId(encoded: string): boolean {
  let id = encoded
  // Inspect encoded identifiers as well as their display spelling. Nested
  // encodings are untrusted and receive a bounded, fail-closed inspection.
  for (let round = 0; round < 4; round += 1) {
    if (!id.trim() || projectPrivateSourceLocatorsRaw(id) !== id ||
      projectPIIIdentifiers(id) !== id || redactSecretText(id) !== id ||
      containsInternalCaseEntityReference(id) || containsRestrictedEvidence(id)) return false
    if (!id.includes('%')) return true
    try { id = decodeURIComponent(id) } catch { return false }
  }
  return false
}

function projectPrivateSourceLocators(text: string): string {
  let cursor = 0
  let output = ''
  for (const match of text.matchAll(COMPOSER_MENTION)) {
    const [token, marker, rawLabel, rawURI] = match
    const uri = rawURI.replace(/\\(.)/gu, '$1')
    const prefix = marker === '@' ? 'plugin://' : 'skill://'
    if (!uri.startsWith(prefix)) continue
    output += projectPrivateSourceLocatorsRaw(text.slice(cursor, match.index))
    const label = rawLabel.replace(/\\(.)/gu, '$1')
    const projectedLabel = projectPIIIdentifiers(projectPrivateSourceLocatorsRaw(redactSecretText(label)))
    const escapedLabel = projectedLabel.replaceAll('\\', '\\\\').replaceAll('[', '\\[').replaceAll(']', '\\]')
    output += safeMentionId(uri.slice(prefix.length))
      ? `${marker}[${escapedLabel}](${rawURI})`
      : `${projectedLabel} [PRIVATE_REFERENCE]`
    cursor = match.index + token.length
  }
  return output + projectPrivateSourceLocatorsRaw(text.slice(cursor))
}

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
const HOST_USER_INPUT_ID = /^input_[a-f0-9]{64}$/

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
const UNBOUND_CASE_ASSERTION_PHRASES = [
  '实际控制', '控制关系', '关联关系', '人员关系', '亲属', '配偶', '夫妻',
  '是父子', '为父子', '系父子',
  '是父女', '为父女', '系父女',
  '是母子', '为母子', '系母子',
  '是母女', '为母女', '系母女',
  '是兄弟', '为兄弟', '系兄弟',
  '是姐妹', '为姐妹', '系姐妹',
  '串通投标', '围标',
  '行贿', '利益输送', '具备立案条件', '当前案件', '案件账户', '案件账号', '涉案'
]
const UNBOUND_MONETARY_ACTION_CUES = ['支付', '转账', '收款', '付款', '汇入', '汇出', '取得']
const ACQUIRED_YEAR_FINANCIAL_CUES = ['收益', '收入', '利润', '利息', '补助', '货款', '价款']
const UNBOUND_RELATIONSHIP_ASSERTION = /([\p{Script=Han}]{1,4})[与和]([\p{Script=Han}]{1,4})存在(?:父子|父女|母子|母女|兄弟|姐妹)关系/gu

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

/** Mirrors the Go unbound ordinary-turn/result risk signal. Case authority is host-owned. */
export function containsUnboundCaseRiskV1(text: string): boolean {
  if (containsOrdinaryPublicPII(text)) return true
  const normalized = normalizeCaseFactText(text)
  if (UNBOUND_CASE_ASSERTION_PHRASES.some((phrase) => normalized.includes(phrase))) return true
  if (containsUnboundRelationshipAssertionV1(normalized)) return true
  if (containsLineWrappedMonetaryActionV1(text)) return true
  return text.split(/[。！？!?；;\n]/u).some((rawClause) => {
    const clause = normalizeCaseFactText(rawClause)
    const acquiredAt = clause.indexOf('取得')
    if (acquiredAt >= 0) {
      const tail = clause.slice(acquiredAt + '取得'.length)
      if (containsCurrencyAmount(tail) || acquiredYearHasUnitAmountV1(tail)) return true
    }
    return UNBOUND_MONETARY_ACTION_CUES.some((cue) => actionHasAdjacentAmount(clause, cue))
  })
}

function containsLineWrappedMonetaryActionV1(text: string): boolean {
  // Join only an action at line end to its immediately following amount.
  const lines = text.split('\n')
  for (let index = 0; index + 1 < lines.length; index += 1) {
    const left = normalizeCaseFactText(lines[index].trim())
    const right = normalizeCaseFactText(lines[index + 1].trim())
    for (const cue of UNBOUND_MONETARY_ACTION_CUES) {
      if ((left.endsWith(cue) || left.endsWith(`${cue}了`)) &&
          actionHasAdjacentAmount(`${cue}${right}`, cue)) return true
    }
  }
  return false
}

function acquiredYearHasUnitAmountV1(tail: string): boolean {
  // A year may be a filename; require a nearby financial noun and bound the scan.
  const trimmed = tail.trim()
  const year = trimmed.match(/^(?:了\s*)?[0-9]{4}\s*年/u)
  if (!year) return false
  const afterYear = trimmed.slice(year[0].length)
  let window = ''
  let windowRunes = 0
  for (const character of afterYear) {
    if (windowRunes >= 64) break
    window += character
    windowRunes += 1
  }
  let offset = 0
  let runeIndex = 0
  for (const character of window) {
    if (runeIndex > 32) break
    if (/^[0-9]$/u.test(character) &&
        amountHasCurrencyUnit(window.slice(offset))) {
      const nearAmount = Array.from(window.slice(0, offset)).slice(-8).join('')
      if (ACQUIRED_YEAR_FINANCIAL_CUES.some((cue) => nearAmount.includes(cue))) return true
    }
    offset += character.length
    runeIndex += 1
  }
  return false
}

function containsUnboundRelationshipAssertionV1(normalized: string): boolean {
  for (const match of normalized.matchAll(UNBOUND_RELATIONSHIP_ASSERTION)) {
    if (!technicalRelationshipSubjectV1(match[1]) && !technicalRelationshipSubjectV1(match[2])) return true
  }
  return false
}

function technicalRelationshipSubjectV1(subject: string): boolean {
  return ['元素', '节点', '组件', '模块', '对象'].some((term) => subject.includes(term))
}

function actionHasAdjacentAmount(clause: string, cue: string): boolean {
  let offset = 0
  while (offset < clause.length) {
    const index = clause.indexOf(cue, offset)
    if (index < 0) return false
    offset = index + cue.length
    const rawTail = clause.slice(offset).trimStart().replace(/^了/u, '').trimStart()
    const currencyPrefix = /^(?:人民币|￥|¥|\$|€|£)/u.test(rawTail)
    const tail = rawTail.replace(/^(?:人民币|￥|¥|\$|€|£)/u, '').trimStart()
    if (/^[0-9]/u.test(tail) && (currencyPrefix || amountHasCurrencyUnit(tail))) return true
    // A bounded payee may follow directly or after 给/向; a bare count stays ordinary.
    const payeeAndAmount = tail.replace(/^(?:给|向)/u, '').trim()
    const digitIndex = payeeAndAmount.search(/[0-9]/u)
    if (digitIndex > 0 && Array.from(payeeAndAmount.slice(0, digitIndex)).length <= 16) {
      const beforeAmount = payeeAndAmount.slice(0, digitIndex).trim()
      if (/[￥¥$€£]$/u.test(beforeAmount) || amountHasCurrencyUnit(payeeAndAmount.slice(digitIndex))) return true
    }
  }
  return false
}

function amountHasCurrencyUnit(text: string): boolean {
  if (!/^[0-9]/u.test(text)) return false
  const afterAmount = text.replace(/^[0-9][0-9,.]*/u, '').trimStart().replace(/^[十百千万亿]+/u, '')
  return /^(?:元(?!件)|人民币|美元|欧元|英镑)/u.test(afterAmount)
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
  return projectPIIIdentifiers(projectPrivateSourceLocators(String(text)))
}

type IdentifierSpan = { start: number; end: number; placeholder: string }
type IdentifierRune = { rune: string; start: number; end: number }

function decimalDigit(rune: string): string | undefined {
  if (rune >= '0' && rune <= '9') return rune
  if (rune >= '\uff10' && rune <= '\uff19') return String(rune.charCodeAt(0) - 0xff10)
  // Core maps other decimal scripts to zero for classification, not coercion
  // into an apparently valid identity, phone or timestamp.
  return DECIMAL_DIGIT.test(rune) ? '0' : undefined
}

function identifierSeparator(rune: string): boolean {
  return rune !== '\r' && rune !== '\n' && WHITE_SPACE.test(rune) ||
    '-/_.*·•‐‑‒–—―−﹘﹣－'.includes(rune)
}

function validCalendarDate(value: string): boolean {
  const year = Number(value.slice(0, 4)), month = Number(value.slice(4, 6)), day = Number(value.slice(6, 8))
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0)
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]
  return value.length === 8 && month >= 1 && month <= 12 && day >= 1 && day <= days[month - 1]
}

function validChineseIdentity(value: string): boolean {
  if (value.slice(0, 6) === '000000') return false
  if (value.length === 15) {
    const shortYear = Number(value.slice(6, 8))
    return validCalendarDate(`${shortYear >= 69 ? '19' : '20'}${value.slice(6, 12)}`)
  }
  if (value.length !== 18 || !validCalendarDate(value.slice(6, 14))) return false
  const weights = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2]
  const sum = weights.reduce((total, weight, index) => total + Number(value[index]) * weight, 0)
  return value[17] === '10X98765432'[sum % 11]
}

function calendarTimestamp(digits: string): boolean {
  return (digits.length === 12 || digits.length === 14) && validCalendarDate(digits.slice(0, 8)) &&
    Number(digits.slice(8, 10)) <= 23 && Number(digits.slice(10, 12)) <= 59 &&
    (digits.length === 12 || Number(digits.slice(12, 14)) <= 59)
}

function explicitAccountLabelBefore(units: readonly IdentifierRune[], start: number): boolean {
  const inline = (rune: string) => rune !== '\r' && rune !== '\n' &&
    (WHITE_SPACE.test(rune) || FORMAT_CHARACTER.test(rune))
  let cursor = start
  while (cursor > 0 && inline(units[cursor - 1].rune)) cursor -= 1
  if (cursor === 0 || !':：='.includes(units[cursor - 1].rune)) return false
  cursor -= 1
  while (cursor > 0 && inline(units[cursor - 1].rune)) cursor -= 1
  // Core's independent labels use original runes and ASCII case folding.
  // Only the whitespace around the delimiter is unbounded; the semantic cue
  // window below stays at 32 runes, and an unrelated later measure stays safe.
  const label = units.slice(Math.max(0, cursor - 14), cursor)
    .map(({ rune }) => rune >= 'A' && rune <= 'Z' ? rune.toLowerCase() : rune).join('')
  return ['账号', '帐号', '账户', '卡号', 'account number', 'card number', 'account', 'card']
    .some(candidate => label.endsWith(candidate))
}

function lastNumericCue(context: string, cues: readonly string[]): number {
  let latest = -1
  for (const cue of cues) {
    for (let at = context.indexOf(cue); at >= 0; at = context.indexOf(cue, at + cue.length)) {
      if (/^[a-z]+$/.test(cue) && (/[A-Za-z0-9_]/.test(context[at - 1] ?? '') ||
          /[A-Za-z0-9_]/.test(context[at + cue.length] ?? ''))) continue
      latest = Math.max(latest, at)
    }
  }
  return latest
}

function classifyNumericIdentifier(
  units: readonly IdentifierRune[], start: number, digits: string, groups: readonly number[], separated: boolean, trailingX: boolean
): string | undefined {
  if (validChineseIdentity(digits + (trailingX ? 'X' : ''))) return '[IDENTITY]'
  if (digits.length === 11 && /^1[3-9]/.test(digits)) return '[PHONE]'
  if (digits.length >= 8 && explicitAccountLabelBefore(units, start)) return '[ACCOUNT]'
  // The window is cut in original runes before removing Cf or folding fullwidth
  // ASCII. Normalizing the entire string first changes Core's 32-rune authority.
  const context = units.slice(Math.max(0, start - 32), start).map(({ rune }) => {
    if (FORMAT_CHARACTER.test(rune)) return ''
    const code = rune.charCodeAt(0)
    const folded = code >= 0xff01 && code <= 0xff5e ? String.fromCodePoint(code - 0xff01 + 33) :
      rune === '\u3000' ? ' ' : rune
    // Core uses Unicode simple lowercase: U+0130 maps to one rune, rather
    // than JavaScript's expanding mapping that would introduce a cue boundary.
    return folded === '\u0130' ? 'i' : folded.toLowerCase()
  }).join('')
  if ([14, 15, 16].includes(digits.length) && ['设备', '终端', 'imei', 'device', 'fingerprint', 'mac'].some(cue => context.includes(cue))) return '[DEVICE]'
  const account = lastNumericCue(context, ['账号', '帐号', '账户', '卡号', '银行卡', 'account', 'acct', 'card'])
  const measure = lastNumericCue(context, ['金额', '余额', '合计', '总计', '数量', '笔数', '计数', '时间戳', '时间', '时点', '日期',
    'amount', 'balance', 'total', 'count', 'timestamp', 'time', 'date'])
  if (digits.length < 12) return account >= 0 && account > measure && digits.length >= 8 ? '[ACCOUNT]' : undefined
  if (digits.length > 32) return '[ACCOUNT]'
  if (measure >= 0 && account <= measure) return undefined
  const epoch = digits.length === 13 && Number(digits) >= 946684800000 && Number(digits) <= 4102444800000
  if (account < 0 && !separated && (calendarTimestamp(digits) || epoch)) return undefined
  if (separated && groups.length > 1 && groups.some(size => size < 3 || size > 6) && account < 0) return undefined
  return '[ACCOUNT]'
}

function numericIdentifierSpans(units: readonly IdentifierRune[]): IdentifierSpan[] {
  const spans: IdentifierSpan[] = []
  for (let start = 0; start < units.length;) {
    if (decimalDigit(units[start].rune) === undefined || start > 0 && decimalDigit(units[start - 1].rune) !== undefined) {
      start += 1
      continue
    }
    const digits: string[] = [], groups: number[] = []
    let end = start, groupSize = 0, separated = false, trailingX = false
    for (; end < units.length; end += 1) {
      const rune = units[end].rune, digit = decimalDigit(rune)
      if (digit !== undefined) { digits.push(digit); groupSize += 1; continue }
      if ('xXＸｘ'.includes(rune) && digits.length === 17) { trailingX = true; end += 1; break }
      if (identifierSeparator(rune)) {
        if (groupSize > 0) { groups.push(groupSize); groupSize = 0 }
        separated = true
        continue
      }
      if (FORMAT_CHARACTER.test(rune)) continue
      break
    }
    if (groupSize > 0) groups.push(groupSize)
    let trimmedEnd = end
    while (trimmedEnd > start && (identifierSeparator(units[trimmedEnd - 1].rune) || FORMAT_CHARACTER.test(units[trimmedEnd - 1].rune))) trimmedEnd -= 1
    const placeholder = classifyNumericIdentifier(units, start, digits.join(''), groups, separated, trailingX)
    if (placeholder) spans.push({ start: units[start].start, end: units[trimmedEnd - 1].end, placeholder })
    start = end
  }
  return spans
}

function projectIdentifierPass(source: string): string {
  let offset = 0, detectionText = ''
  const starts: number[] = [], ends: number[] = []
  // Stable positions are collected before inserting any placeholders. The
  // other identifier patterns retain their normalized detection view; output
  // outside a matched source span keeps its original bytes.
  const units = Array.from(source, rune => {
    const unit = { rune, start: offset, end: offset + rune.length }
    offset = unit.end
    const normalized = rune.normalize('NFKC').replace(FORMAT_CHARACTERS, '')
    detectionText += normalized
    for (let index = 0; index < normalized.length; index += 1) { starts.push(unit.start); ends.push(unit.end) }
    return unit
  })
  const spans: IdentifierSpan[] = []
  for (const [pattern, placeholder] of [[MAC_ADDRESS, '[DEVICE]'], [IPV4_ADDRESS, '[IP]'], [EMAIL_ADDRESS, '[EMAIL]']] as const) {
    for (const match of detectionText.matchAll(pattern)) {
      spans.push({ start: starts[match.index + match[1].length], end: ends[match.index + match[0].length - 1], placeholder })
    }
  }
  spans.push(...numericIdentifierSpans(units))
  spans.sort((left, right) => left.start - right.start || right.end - left.end)
  const merged: IdentifierSpan[] = []
  for (const span of spans) {
    const prior = merged.at(-1)
    if (prior && span.start < prior.end) prior.end = Math.max(prior.end, span.end)
    else merged.push({ ...span })
  }
  let cursor = 0, projected = ''
  for (const span of merged) { projected += source.slice(cursor, span.start) + span.placeholder; cursor = span.end }
  return projected + source.slice(cursor)
}

// Detection also inspects identifiers inside locators. Match Core's bounded
// fixed point: an earlier mask can expose a contextual identifier in the next
// pass, but each pass classifies all original positions before replacing them.
function projectPIIIdentifiers(text: string): string {
  let current = String(text)
  for (let pass = 0; pass < 8; pass += 1) {
    const next = projectIdentifierPass(current)
    if (next === current) return current
    current = next
  }
  return '[ACCOUNT]'
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
  if (typeof value === 'string') return projectPIIIdentifiers(value) !== value
  return containsPublicPII(value, false, newPublicPIITraversalState(), 0)
}

function hostUserInputQuestionsId(
  record: Record<string, unknown>,
  key: string,
  entry: unknown,
  inheritedText: boolean
): string | undefined {
  if (key !== 'questions' || inheritedText || !Array.isArray(entry) ||
      typeof record.inputId !== 'string' || !HOST_USER_INPUT_ID.test(record.inputId)) return undefined
  if (record.kind === 'user_input' && record.role === 'system' &&
      (record.status === 'pending' || record.status === 'submitted' ||
        record.status === 'cancelled' || record.status === 'continued')) return record.inputId
  if (record.kind === 'user_input_requested' && record.status === 'pending') return record.inputId
  if (record.kind === 'user_input_resolved' &&
      (record.status === 'submitted' || record.status === 'cancelled' ||
        record.status === 'continued')) return record.inputId
  return undefined
}

function containsPublicPII(
  value: unknown,
  inheritedText: boolean,
  state: PublicPIITraversalState,
  depth: number,
  hostInputId?: string,
  expectedQuestionId?: string
): boolean {
  if (!consumePublicPIINode(state, depth)) return true
  if (typeof value === 'string') {
    return inheritedText && projectPIIIdentifiers(value) !== value
  }
  if (!value || typeof value !== 'object') {
    return inheritedText && value !== null && value !== undefined &&
      projectPIIIdentifiers(String(value)) !== String(value)
  }
  if (state.active.has(value)) return true
  state.active.add(value)
  try {
    if (Array.isArray(value)) {
      for (const [index, entry] of value.entries()) {
        if (containsPublicPII(entry, inheritedText, state, depth + 1, undefined,
          hostInputId === undefined ? undefined : `${hostInputId}_${index + 1}`)) return true
      }
      return false
    }
    const record = value as Record<string, unknown>
    for (const key in record) {
      if (!Object.prototype.hasOwnProperty.call(record, key)) continue
      const entry = record[key]
      const normalized = normalizePublicKey(key)
      // The Go host derives these correlation IDs from its gate digest. They
      // are structural only in a closed user-input record outside prose; all
      // question/option text and any mismatched ID still receive the ordinary check.
      if (normalized === 'id' && expectedQuestionId !== undefined &&
          entry === expectedQuestionId) continue
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
        depth + 1,
        normalized === 'options' && expectedQuestionId !== undefined &&
          record.id === expectedQuestionId && typeof record.question === 'string' && Array.isArray(entry)
          ? `${expectedQuestionId}_option`
          : hostUserInputQuestionsId(record, key, entry, inheritedText)
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
