'use strict'

const DEFAULT_MAX_DEPTH = 16
const DEFAULT_MAX_TOKENS = 2048
const DEFAULT_MAX_STRING_BYTES = 64 << 10
const DEFAULT_MAX_NUMBER_BYTES = 128

function parseStrictJsonObject(input, options) {
  if (!Buffer.isBuffer(input) || !options || !Number.isSafeInteger(options.maxBytes) ||
      options.maxBytes <= 0 || input.length === 0 || input.length > options.maxBytes) {
    throw new Error('strict JSON size is invalid')
  }
  let source = ''
  try {
    // Keep a leading UTF-8 BOM visible to the grammar. TextDecoder strips it
    // by default, which would make two byte-distinct protocol frames decode
    // to the same JSON value.
    source = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(input)
  } catch {
    throw new Error('strict JSON is not UTF-8')
  }
  const parser = new StrictJsonParser(source, options)
  const value = parser.parse()
  if (!isJsonObject(value)) throw new Error('strict JSON must be an object')
  return value
}

function isJsonObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

class StrictJsonParser {
  constructor(source, options) {
    this.source = source
    this.index = 0
    this.tokens = 0
    this.maxDepth = positiveLimit(options.maxDepth, DEFAULT_MAX_DEPTH)
    this.maxTokens = positiveLimit(options.maxTokens, DEFAULT_MAX_TOKENS)
    this.maxStringBytes = positiveLimit(options.maxStringBytes, DEFAULT_MAX_STRING_BYTES)
    this.maxNumberBytes = positiveLimit(options.maxNumberBytes, DEFAULT_MAX_NUMBER_BYTES)
    this.integerOnly = options.integerOnly === true
  }

  parse() {
    this.skipWhitespace()
    const value = this.parseValue(0)
    this.skipWhitespace()
    if (this.index !== this.source.length) throw new Error('strict JSON has trailing content')
    return value
  }

  parseValue(depth) {
    if (depth > this.maxDepth) throw new Error('strict JSON depth exceeded')
    this.takeToken()
    switch (this.source[this.index]) {
      case '{': return this.parseObject(depth + 1)
      case '[': return this.parseArray(depth + 1)
      case '"': return this.parseString()
      case 't': return this.parseLiteral('true', true)
      case 'f': return this.parseLiteral('false', false)
      case 'n': return this.parseLiteral('null', null)
      default: return this.parseNumber()
    }
  }

  parseObject(depth) {
    this.index += 1
    const result = Object.create(null)
    const keys = new Set()
    this.skipWhitespace()
    if (this.source[this.index] === '}') {
      this.index += 1
      this.takeToken()
      return result
    }
    for (;;) {
      this.takeToken()
      if (this.source[this.index] !== '"') throw new Error('strict JSON object key is invalid')
      const key = this.parseString()
      if (keys.has(key)) throw new Error('strict JSON contains a duplicate key')
      keys.add(key)
      this.skipWhitespace()
      if (this.source[this.index] !== ':') throw new Error('strict JSON object separator is invalid')
      this.index += 1
      this.skipWhitespace()
      result[key] = this.parseValue(depth)
      this.skipWhitespace()
      const separator = this.source[this.index]
      if (separator === '}') {
        this.index += 1
        this.takeToken()
        return result
      }
      if (separator !== ',') throw new Error('strict JSON object is not closed')
      this.index += 1
      this.skipWhitespace()
    }
  }

  parseArray(depth) {
    this.index += 1
    const result = []
    this.skipWhitespace()
    if (this.source[this.index] === ']') {
      this.index += 1
      this.takeToken()
      return result
    }
    for (;;) {
      result.push(this.parseValue(depth))
      this.skipWhitespace()
      const separator = this.source[this.index]
      if (separator === ']') {
        this.index += 1
        this.takeToken()
        return result
      }
      if (separator !== ',') throw new Error('strict JSON array is not closed')
      this.index += 1
      this.skipWhitespace()
    }
  }

  parseString() {
    const start = this.index
    this.index += 1
    let escaped = false
    for (; this.index < this.source.length; this.index += 1) {
      const code = this.source.charCodeAt(this.index)
      if (code < 0x20) throw new Error('strict JSON string contains a control character')
      const character = this.source[this.index]
      if (escaped) {
        if (!'"\\/bfnrtu'.includes(character)) throw new Error('strict JSON escape is invalid')
        if (character === 'u') {
          const value = this.source.slice(this.index + 1, this.index + 5)
          if (!/^[0-9a-fA-F]{4}$/.test(value)) throw new Error('strict JSON unicode escape is invalid')
          this.index += 4
        }
        escaped = false
        continue
      }
      if (character === '\\') {
        escaped = true
        continue
      }
      if (character === '"') {
        const raw = this.source.slice(start, this.index + 1)
        this.index += 1
        const value = JSON.parse(raw)
        if (Buffer.byteLength(value, 'utf8') > this.maxStringBytes || !hasValidSurrogatePairs(value)) {
          throw new Error('strict JSON string is invalid')
        }
        return value
      }
    }
    throw new Error('strict JSON string is not closed')
  }

  parseLiteral(source, value) {
    if (this.source.slice(this.index, this.index + source.length) !== source) {
      throw new Error('strict JSON literal is invalid')
    }
    this.index += source.length
    return value
  }

  parseNumber() {
    const remaining = this.source.slice(this.index)
    const match = /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/.exec(remaining)
    if (!match || Buffer.byteLength(match[0], 'utf8') > this.maxNumberBytes) {
      throw new Error('strict JSON number is invalid')
    }
    if (this.integerOnly && /[.eE]/.test(match[0])) {
      throw new Error('strict JSON number must be an integer')
    }
    const terminator = remaining[match[0].length]
    if (terminator !== undefined && !/[\s,}\]]/.test(terminator)) {
      throw new Error('strict JSON number terminator is invalid')
    }
    const value = Number(match[0])
    if (!Number.isFinite(value)) throw new Error('strict JSON number is out of range')
    this.index += match[0].length
    return value
  }

  skipWhitespace() {
    while (this.index < this.source.length && /[\t\n\r ]/.test(this.source[this.index])) this.index += 1
  }

  takeToken() {
    this.tokens += 1
    if (this.tokens > this.maxTokens) throw new Error('strict JSON token limit exceeded')
  }
}

function positiveLimit(value, fallback) {
  return Number.isSafeInteger(value) && value > 0 ? value : fallback
}

function hasValidSurrogatePairs(value) {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index)
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(index + 1)
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false
      index += 1
      continue
    }
    if (code >= 0xdc00 && code <= 0xdfff) return false
  }
  return true
}

module.exports = { parseStrictJsonObject }
