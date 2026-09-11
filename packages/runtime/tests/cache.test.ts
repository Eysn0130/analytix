import { describe, expect, it } from 'vitest'
import {
  createImmutablePrefix,
  shouldVerifyImmutablePrefix,
  setFewShots,
  setPinnedConstraints,
  setSystemPrompt,
  setTools,
  describeFingerprintDrift,
  verifyImmutablePrefix
} from '../src/cache/immutable-prefix.js'
import { detectVolatilePrefixContent } from '../src/cache/prefix-volatility.js'
import {
  buildToolCatalogFingerprint,
  canonicalizeToolInputSchema
} from '../src/cache/tool-catalog-fingerprint.js'
import {
  captureCachePrefixShape,
  compareCachePrefixShapes
} from '../src/cache/prefix-cache-diagnostics.js'
import { LruCache } from '../src/cache/lru-cache.js'
import { TtlLruCache } from '../src/cache/ttl-lru-cache.js'
import { makeUserItem } from '../src/domain/item.js'

describe('ImmutablePrefix', () => {
  it('produces a stable fingerprint when the prefix does not change', () => {
    const a = createImmutablePrefix({ systemPrompt: 'hi' })
    const b = createImmutablePrefix({ systemPrompt: 'hi' })
    expect(a.fingerprint).toBe(b.fingerprint)
  })

  it('drifts the fingerprint when the system prompt changes', () => {
    const a = createImmutablePrefix({ systemPrompt: 'hi' })
    const b = setSystemPrompt(a, 'hello')
    expect(a.fingerprint).not.toBe(b.fingerprint)
    const drift = describeFingerprintDrift(a, b)
    expect(drift.drift).toBe(true)
    expect(drift.changedFields).toContain('systemPrompt')
  })

  it('drifts the fingerprint when tools change', () => {
    const a = createImmutablePrefix({ tools: [] })
    const b = setTools(a, [{ name: 'echo', description: 'd', inputSchema: {} }])
    expect(describeFingerprintDrift(a, b).changedFields).toContain('tools')
  })

  it('canonicalizes tool schemas so ordering noise does not perturb the prefix', () => {
    const a = createImmutablePrefix({
      tools: [
        {
          name: 'zeta',
          description: 'z',
          inputSchema: { type: 'object', properties: { b: { type: 'string' }, a: { type: 'number' } } }
        },
        { name: 'alpha', description: 'a', inputSchema: { type: 'object' } }
      ]
    })
    const b = createImmutablePrefix({
      tools: [
        { name: 'alpha', description: 'a', inputSchema: { type: 'object' } },
        {
          name: 'zeta',
          description: 'z',
          inputSchema: { properties: { a: { type: 'number' }, b: { type: 'string' } }, type: 'object' }
        }
      ]
    })
    expect(a.fingerprint).toBe(b.fingerprint)
    expect(a.tools.map((tool) => tool.name)).toEqual(['alpha', 'zeta'])
  })

  it('uses one canonical tool schema shape for catalog, prefix, and cache diagnostics', () => {
    const noisySchema = {
      type: 'object',
      required: true,
      properties: {
        b: { enum: ['z', 'a'] },
        a: { type: 'string' }
      },
      dependentRequired: {
        z: ['b', 'a', 'a'],
        ignored: true
      }
    }
    const canonicalSchema = {
      type: 'object',
      properties: {
        a: { type: 'string' },
        b: { enum: ['a', 'z'] }
      },
      dependentRequired: {
        z: ['a', 'b']
      }
    }
    const noisyTools = [{ name: 'probe', description: 'd', inputSchema: noisySchema }]
    const canonicalTools = [{ name: 'probe', description: 'd', inputSchema: canonicalSchema }]

    expect(canonicalizeToolInputSchema(noisySchema)).toEqual(canonicalSchema)
    expect(buildToolCatalogFingerprint(noisyTools).fingerprint)
      .toBe(buildToolCatalogFingerprint(canonicalTools).fingerprint)
    expect(createImmutablePrefix({ tools: noisyTools }).fingerprint)
      .toBe(createImmutablePrefix({ tools: canonicalTools }).fingerprint)
    expect(captureCachePrefixShape({ tools: noisyTools }).toolsHash)
      .toBe(captureCachePrefixShape({ tools: canonicalTools }).toolsHash)
  })

  it('drifts the fingerprint when pinned constraints change', () => {
    const a = createImmutablePrefix()
    const b = setPinnedConstraints(a, ['user: stay concise'])
    expect(describeFingerprintDrift(a, b).changedFields).toContain('pinnedConstraints')
  })

  it('drifts the fingerprint when few-shots change', () => {
    const a = createImmutablePrefix()
    const b = setFewShots(a, [
      makeUserItem({ id: 'fs', turnId: 't', threadId: 'th', text: 'ping' })
    ])
    expect(describeFingerprintDrift(a, b).changedFields).toContain('fewShots')
  })

  it('ignores volatile few-shot ids that are not sent to the model', () => {
    const a = createImmutablePrefix({
      fewShots: [makeUserItem({ id: 'fs_a', turnId: 't1', threadId: 'th1', text: 'ping' })]
    })
    const b = createImmutablePrefix({
      fewShots: [makeUserItem({ id: 'fs_b', turnId: 't2', threadId: 'th2', text: 'ping' })]
    })
    expect(a.fingerprint).toBe(b.fingerprint)
  })

  it('throws when the prefix is mutated without an explicit mutator', () => {
    const prefix = createImmutablePrefix({ systemPrompt: 'hi' })
    prefix.systemPrompt = 'hello'
    expect(() => verifyImmutablePrefix(prefix)).toThrow(/fingerprint drift/)
  })

  it('does not create an intermediate invalid fingerprint during mutate', () => {
    const prefix = createImmutablePrefix({ systemPrompt: 'hi' })
    const next = setSystemPrompt(prefix, 'hello')
    expect(next.fingerprint).not.toBe('')
    expect(() => verifyImmutablePrefix(next)).not.toThrow()
  })

  it('verifies immutable prefixes by default outside production', () => {
    expect(shouldVerifyImmutablePrefix()).toBe(true)
  })

  it('detects volatile cache-prefix tokens with structured parsers', () => {
    const jwt = [
      Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' })).toString('base64url'),
      Buffer.from(JSON.stringify({ sub: 'user_1' })).toString('base64url'),
      'signature'
    ].join('.')
    const prefix = createImmutablePrefix({
      systemPrompt: [
        'request id 550e8400-e29b-41d4-a716-446655440000',
        'built at 2026-06-03T12:34:56Z',
        'hash d41d8cd98f00b204e9800998ecf8427e',
        `token ${jwt}`
      ].join(' ')
    })

    const findings = detectVolatilePrefixContent(prefix)
    expect(findings.map((finding) => finding.kind)).toEqual([
      'uuid',
      'iso8601',
      'hex_hash',
      'jwt'
    ])
  })

  it('does not mistake dashless UUID-shaped hashes for canonical UUIDs', () => {
    const prefix = createImmutablePrefix({
      systemPrompt: 'dashless 550e8400e29b41d4a716446655440000'
    })

    expect(detectVolatilePrefixContent(prefix)).toEqual([
      expect.objectContaining({ kind: 'hex_hash' })
    ])
  })
})

describe('ToolCatalogFingerprint', () => {
  it('stays stable across tool order and schema key order noise', () => {
    const a = buildToolCatalogFingerprint([
      {
        name: 'zeta',
        description: 'z',
        toolKind: 'command_execution',
        inputSchema: { type: 'object', properties: { b: { type: 'string' }, a: { type: 'number' } } }
      },
      {
        name: 'alpha',
        description: 'a',
        inputSchema: { type: 'object' }
      }
    ])
    const b = buildToolCatalogFingerprint([
      {
        name: 'alpha',
        description: 'a',
        toolKind: 'file_change',
        inputSchema: { type: 'object' }
      },
      {
        name: 'zeta',
        description: 'z',
        inputSchema: { properties: { a: { type: 'number' }, b: { type: 'string' } }, type: 'object' }
      }
    ])

    expect(a.fingerprint).toBe(b.fingerprint)
    expect(a.toolNames).toEqual(['alpha', 'zeta'])
    expect(a.toolCount).toBe(2)
  })

  it('changes when a model-bound tool description changes', () => {
    const a = buildToolCatalogFingerprint([
      { name: 'echo', description: 'Echo text.', inputSchema: { type: 'object' } }
    ])
    const b = buildToolCatalogFingerprint([
      { name: 'echo', description: 'Echo text with extra cache-visible detail.', inputSchema: { type: 'object' } }
    ])

    expect(a.fingerprint).not.toBe(b.fingerprint)
  })
})

describe('CachePrefixDiagnostics', () => {
  it('keeps few-shot metadata noise out of prefix diagnostics', () => {
    const a = captureCachePrefixShape({
      systemPrompt: 'stable system',
      prefix: [makeUserItem({ id: 'fs_a', turnId: 't1', threadId: 'th1', text: 'ping' })]
    })
    const b = captureCachePrefixShape({
      systemPrompt: 'stable system',
      prefix: [makeUserItem({ id: 'fs_b', turnId: 't2', threadId: 'th2', text: 'ping' })]
    })

    expect(a.prefixItemsHash).toBe(b.prefixItemsHash)
    expect(a.prefixHash).toBe(b.prefixHash)
  })

  it('keeps tool schema order noise out of prefix diagnostics', () => {
    const a = captureCachePrefixShape({
      systemPrompt: 'stable system',
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools: [
        {
          name: 'zeta',
          description: 'z',
          inputSchema: { type: 'object', properties: { b: { type: 'string' }, a: { type: 'number' } } }
        },
        {
          name: 'alpha',
          description: 'a',
          inputSchema: { type: 'object' }
        }
      ]
    })
    const b = captureCachePrefixShape({
      systemPrompt: 'stable system',
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools: [
        {
          name: 'alpha',
          description: 'a',
          inputSchema: { type: 'object' }
        },
        {
          name: 'zeta',
          description: 'z',
          inputSchema: { properties: { a: { type: 'number' }, b: { type: 'string' } }, type: 'object' }
        }
      ]
    })

    expect(a.prefixHash).toBe(b.prefixHash)
    expect(a.toolsHash).toBe(b.toolsHash)
    expect(a.toolSchemaTokens).toBeGreaterThan(0)
  })

  it('reports provider-safe cache miss reasons without raw prompt text', () => {
    const previous = captureCachePrefixShape({
      systemPrompt: 'stable system',
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools: [
        { name: 'read', description: 'Read files.', inputSchema: { type: 'object' } }
      ]
    })
    const current = captureCachePrefixShape({
      systemPrompt: 'stable system',
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools: [
        { name: 'read', description: 'Read files.', inputSchema: { type: 'object' } },
        { name: 'grep', description: 'Search files.', inputSchema: { type: 'object' } }
      ]
    })

    const diagnostics = compareCachePrefixShapes(previous, current, {
      promptTokens: 100,
      completionTokens: 10,
      totalTokens: 110,
      cachedTokens: 80,
      cacheHitTokens: 80,
      cacheMissTokens: 20,
      cacheHitRate: 0.8,
      turns: 1
    })

    expect(diagnostics.prefixChanged).toBe(true)
    expect(diagnostics.prefixChangeReasons).toEqual(['tools'])
    expect(diagnostics.cacheHitTokens).toBe(80)
    expect(diagnostics.cacheMissTokens).toBe(20)
    expect(JSON.stringify(diagnostics)).not.toContain('stable system')
    expect(JSON.stringify(diagnostics)).not.toContain('Search files.')
  })

  it('reports prompt route and tool count without adding prompt text to diagnostics', () => {
    const previous = captureCachePrefixShape({
      systemPrompt: 'stable system',
      route: 'direct_answer',
      toolCount: 0,
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools: []
    })
    const current = captureCachePrefixShape({
      systemPrompt: 'stable system',
      route: 'agent',
      toolCount: 1,
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools: [
        { name: 'read', description: 'Read files.', inputSchema: { type: 'object' } }
      ]
    })

    const diagnostics = compareCachePrefixShapes(previous, current, {
      promptTokens: 100,
      completionTokens: 10,
      totalTokens: 110,
      cachedTokens: 80,
      cacheHitTokens: 80,
      cacheMissTokens: 20,
      cacheHitRate: 0.8,
      turns: 1
    }, {
      firstTokenLatencyMs: 12.8,
      durationMs: 34.2
    })

    expect(diagnostics.route).toBe('agent')
    expect(diagnostics.toolCount).toBe(1)
    expect(diagnostics.firstTokenLatencyMs).toBe(12)
    expect(diagnostics.durationMs).toBe(34)
    expect(diagnostics.prefixChangeReasons).toEqual(['tools'])
    expect(JSON.stringify(diagnostics)).not.toContain('stable system')
    expect(JSON.stringify(diagnostics)).not.toContain('Read files.')
  })

  it('keeps dynamic tool source lifecycle out of the stable prefix while reporting diagnostics', () => {
    const tools = [
      { name: 'research_lookup', description: 'Lookup research notes.', inputSchema: { type: 'object' } }
    ]
    const previous = captureCachePrefixShape({
      systemPrompt: 'stable system',
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools,
      toolSources: [
        {
          id: 'mcp:research-a',
          kind: 'mcp',
          status: 'connected',
          catalogFingerprint: 'source_a'
        }
      ]
    })
    const current = captureCachePrefixShape({
      systemPrompt: 'stable system',
      model: 'deepseek-v4-pro',
      provider: 'compat',
      endpointFormat: 'chat_completions',
      tools,
      toolSources: [
        {
          id: 'mcp:research-b',
          kind: 'mcp',
          status: 'connected',
          catalogFingerprint: 'source_b'
        }
      ]
    })

    expect(current.prefixHash).toBe(previous.prefixHash)
    expect(current.toolsHash).toBe(previous.toolsHash)
    expect(current.toolSourcesHash).not.toBe(previous.toolSourcesHash)

    const diagnostics = compareCachePrefixShapes(previous, current, {
      promptTokens: 100,
      completionTokens: 10,
      totalTokens: 110,
      cacheHitRate: null,
      turns: 1
    })

    expect(diagnostics).toMatchObject({
      prefixChanged: false,
      prefixChangeReasons: [],
      toolSourceChanged: true,
      toolSourceChangeReasons: ['tool_sources'],
      toolSourceIds: ['mcp:research-b'],
      cacheTelemetrySupported: false
    })
    expect(JSON.stringify(diagnostics)).not.toContain('stable system')
    expect(JSON.stringify(diagnostics)).not.toContain('Lookup research notes.')
  })
})

describe('LruCache', () => {
  it('promotes entries on get and evicts in LRU order', () => {
    const cache = new LruCache<string, number>(2)
    cache.set('a', 1)
    cache.set('b', 2)
    expect(cache.get('a')).toBe(1)
    cache.set('c', 3)
    expect(cache.has('b')).toBe(false)
    expect(cache.get('a')).toBe(1)
    expect(cache.get('c')).toBe(3)
  })

  it('returns the evicted entry from set', () => {
    const cache = new LruCache<string, number>(1)
    expect(cache.set('a', 1)).toBeUndefined()
    expect(cache.set('b', 2)).toBe(1)
  })

  it('clears and deletes entries', () => {
    const cache = new LruCache<string, number>(2)
    cache.set('a', 1)
    expect(cache.delete('a')).toBe(true)
    expect(cache.delete('a')).toBe(false)
    cache.set('a', 1)
    cache.clear()
    expect(cache.size).toBe(0)
  })

  it('rejects an invalid limit', () => {
    expect(() => new LruCache<string, number>(0)).toThrow()
  })
})

describe('TtlLruCache', () => {
  it('expires entries after the ttl window', () => {
    let now = 0
    const cache = new TtlLruCache<string, number>({ limit: 2, ttlMs: 100, now: () => now })
    cache.set('a', 1)
    expect(cache.get('a')).toBe(1)
    now += 101
    expect(cache.get('a')).toBeUndefined()
  })

  it('sweeps expired entries', () => {
    let now = 0
    const cache = new TtlLruCache<string, number>({ limit: 4, ttlMs: 50, now: () => now })
    cache.set('a', 1)
    cache.set('b', 2)
    now = 100
    expect(cache.sweep()).toBe(2)
  })
})
