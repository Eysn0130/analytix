const DEFAULT_MAX_DEPTH = 16
const DEFAULT_MAX_TOKENS = 2048
const DEFAULT_MAX_STRING_BYTES = 64 << 10
const DEFAULT_MAX_NUMBER_BYTES = 128

export type StrictJsonOptions = {
  maxBytes: number
  maxDepth?: number
  maxTokens?: number
  maxStringBytes?: number
  maxNumberBytes?: number
}

// JSON.parse accepts duplicate object keys and silently keeps the last value.
// Authority protocols cannot use that behavior, so this parser validates the
// complete token stream and rejects duplicates before constructing a value.
export function parseStrictJsonObject(
  input: Buffer,
  options: StrictJsonOptions
): Record<string, unknown> {
  const value = parseStrictJsonValue(input, options)
  if (!isJsonObject(value)) throw new Error('controlled artifact JSON must be an object')
  return value
}

export function parseStrictJsonValue(input: Buffer, options: StrictJsonOptions): unknown {
  if (!Buffer.isBuffer(input) || input.length === 0 || input.length > options.maxBytes) {
    throw new Error('controlled artifact JSON size is invalid')
  }
  let text = ''
  try {
    text = new TextDecoder('utf-8', { fatal: true }).decode(input)
  } catch {
    throw new Error('controlled artifact JSON is not UTF-8')
  }
  const parser = new StrictJsonParser(text, options)
  return parser.parse()
}

function isJsonObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

class StrictJsonParser {
  private index = 0
  private tokens = 0
  private readonly maxDepth: number
  private readonly maxTokens: number
  private readonly maxStringBytes: number
  private readonly maxNumberBytes: number

  constructor(
    private readonly text: string,
    options: StrictJsonOptions
  ) {
    this.maxDepth = positiveLimit(options.maxDepth, DEFAULT_MAX_DEPTH)
    this.maxTokens = positiveLimit(options.maxTokens, DEFAULT_MAX_TOKENS)
    this.maxStringBytes = positiveLimit(options.maxStringBytes, DEFAULT_MAX_STRING_BYTES)
    this.maxNumberBytes = positiveLimit(options.maxNumberBytes, DEFAULT_MAX_NUMBER_BYTES)
  }

  parse(): unknown {
    this.skipWhitespace()
    const value = this.parseValue(0)
    this.skipWhitespace()
    if (this.index !== this.text.length) throw new Error('controlled artifact JSON has trailing content')
    return value
  }

  private parseValue(depth: number): unknown {
    if (depth > this.maxDepth) throw new Error('controlled artifact JSON depth exceeded')
    this.takeToken()
    const current = this.text[this.index]
    switch (current) {
      case '{':
        return this.parseObject(depth + 1)
      case '[':
        return this.parseArray(depth + 1)
      case '"':
        return this.parseString()
      case 't':
        return this.parseLiteral('true', true)
      case 'f':
        return this.parseLiteral('false', false)
      case 'n':
        return this.parseLiteral('null', null)
      default:
        return this.parseNumber()
    }
  }

  private parseObject(depth: number): Record<string, unknown> {
    this.index += 1
    const result: Record<string, unknown> = Object.create(null) as Record<string, unknown>
    const keys = new Set<string>()
    this.skipWhitespace()
    if (this.text[this.index] === '}') {
      this.index += 1
      return result
    }
    for (;;) {
      this.takeToken()
      if (this.text[this.index] !== '"') throw new Error('controlled artifact JSON object key is invalid')
      const key = this.parseString()
      if (keys.has(key)) throw new Error('controlled artifact JSON contains a duplicate key')
      keys.add(key)
      this.skipWhitespace()
      if (this.text[this.index] !== ':') throw new Error('controlled artifact JSON object separator is invalid')
      this.index += 1
      this.skipWhitespace()
      result[key] = this.parseValue(depth)
      this.skipWhitespace()
      const separator = this.text[this.index]
      if (separator === '}') {
        this.index += 1
        return result
      }
      if (separator !== ',') throw new Error('controlled artifact JSON object is not closed')
      this.index += 1
      this.skipWhitespace()
    }
  }

  private parseArray(depth: number): unknown[] {
    this.index += 1
    const result: unknown[] = []
    this.skipWhitespace()
    if (this.text[this.index] === ']') {
      this.index += 1
      return result
    }
    for (;;) {
      result.push(this.parseValue(depth))
      this.skipWhitespace()
      const separator = this.text[this.index]
      if (separator === ']') {
        this.index += 1
        return result
      }
      if (separator !== ',') throw new Error('controlled artifact JSON array is not closed')
      this.index += 1
      this.skipWhitespace()
    }
  }

  private parseString(): string {
    const start = this.index
    this.index += 1
    let escaped = false
    for (; this.index < this.text.length; this.index += 1) {
      const code = this.text.charCodeAt(this.index)
      if (code < 0x20) throw new Error('controlled artifact JSON string contains a control character')
      const character = this.text[this.index]
      if (escaped) {
        if (!'"\\/bfnrtu'.includes(character)) throw new Error('controlled artifact JSON escape is invalid')
        if (character === 'u') {
          const value = this.text.slice(this.index + 1, this.index + 5)
          if (!/^[0-9a-fA-F]{4}$/.test(value)) throw new Error('controlled artifact JSON unicode escape is invalid')
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
        const raw = this.text.slice(start, this.index + 1)
        this.index += 1
        const value = JSON.parse(raw) as string
        if (Buffer.byteLength(value, 'utf8') > this.maxStringBytes || !hasValidSurrogatePairs(value)) {
          throw new Error('controlled artifact JSON string is invalid')
        }
        return value
      }
    }
    throw new Error('controlled artifact JSON string is not closed')
  }

  private parseLiteral(source: string, value: boolean | null): boolean | null {
    if (this.text.slice(this.index, this.index + source.length) !== source) {
      throw new Error('controlled artifact JSON literal is invalid')
    }
    this.index += source.length
    return value
  }

  private parseNumber(): number {
    const remaining = this.text.slice(this.index)
    const match = /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/.exec(remaining)
    if (!match || match[0].length > this.maxNumberBytes) {
      throw new Error('controlled artifact JSON number is invalid')
    }
    const terminator = remaining[match[0].length]
    if (terminator !== undefined && !/[\s,}\]]/.test(terminator)) {
      throw new Error('controlled artifact JSON number terminator is invalid')
    }
    const value = Number(match[0])
    if (!Number.isFinite(value)) throw new Error('controlled artifact JSON number is out of range')
    this.index += match[0].length
    return value
  }

  private skipWhitespace(): void {
    while (this.index < this.text.length && /[\t\n\r ]/.test(this.text[this.index])) this.index += 1
  }

  private takeToken(): void {
    this.tokens += 1
    if (this.tokens > this.maxTokens) throw new Error('controlled artifact JSON token limit exceeded')
  }
}

function positiveLimit(value: number | undefined, fallback: number): number {
  return Number.isSafeInteger(value) && Number(value) > 0 ? Number(value) : fallback
}

function hasValidSurrogatePairs(value: string): boolean {
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
