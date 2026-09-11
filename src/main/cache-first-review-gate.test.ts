import { describe, expect, it } from 'vitest'
// @ts-expect-error The cache-first gate is a JavaScript CLI; this test asserts its exported contract.
import * as gateModule from '../../scripts/cache-first-review-gate.mjs'

type CacheFirstGate = {
  changedFileImpacts(files: string[]): Array<{ id: string }>
  parseArgs(argv: string[]): {
    files?: string[]
    metadataText: string
    json: boolean
  }
  parseGitPorcelainChangedFiles(text: string): string[]
  validateCacheFirstReview(input: {
    files: string[]
    metadataText: string
  }): {
    ok: boolean
    impacts: Array<{ id: string }>
    failures: string[]
  }
}

const {
  changedFileImpacts,
  parseArgs,
  parseGitPorcelainChangedFiles,
  validateCacheFirstReview
} = gateModule as CacheFirstGate

const completeMetadata = [
  'Cache-impact: medium',
  'Cache-guard: go test ./internal/provider -run DeepSeek -count=1',
  'System-prompt-review: none required',
  'Provider-compat-impact: DeepSeek / OpenAI-compatible / Anthropic/messages checked',
  'Speed-impact: first event and SSE bridge unaffected',
  'Speed-guard: npm test -- src/main/runtime-sse-ipc.test.ts'
].join('\n')

describe('cache-first review gate', () => {
  it('does not require metadata for files outside cache-sensitive surfaces', () => {
    const result = validateCacheFirstReview({
      files: ['docs/release-notes.md'],
      metadataText: ''
    })

    expect(result.ok).toBe(true)
    expect(result.impacts).toHaveLength(0)
  })

  it('detects provider request and runtime event sensitive files', () => {
    const impacts = changedFileImpacts([
      'packages/runtime-go/internal/provider/provider.go',
      'src/main/runtime-sse-ipc.ts',
      'packages/runtime/src/contracts/events.ts',
      'src/renderer/src/agent/analytix-contract.ts',
      'packages/runtime/src/contracts/capabilities.ts',
      'scripts/runtime-go-product-regression.mjs',
      'scripts/runtime-go-speed-cache-gate.mjs'
    ])

    expect(impacts.map((item: { id: string }) => item.id)).toEqual(expect.arrayContaining([
      'provider_request',
      'runtime_event_mapping',
      'builtin_tool_schema',
      'cache_review_gate'
    ]))
  })

  it('parses comma-separated and space-separated --files arguments', () => {
    expect(parseArgs([
      '--files',
      'packages/runtime-go/internal/provider/provider.go,src/main/runtime-sse-ipc.ts',
      'src/renderer/src/agent/analytix-mapper.ts',
      '--metadata',
      completeMetadata
    ])).toEqual({
      files: [
        'packages/runtime-go/internal/provider/provider.go',
        'src/main/runtime-sse-ipc.ts',
        'src/renderer/src/agent/analytix-mapper.ts'
      ],
      metadataText: completeMetadata,
      json: false
    })
  })

  it('includes untracked and renamed files from git porcelain status', () => {
    const files = parseGitPorcelainChangedFiles([
      ' M src/main/runtime-sse-ipc.ts',
      '?? scripts/runtime-go-local-validation.mjs',
      'R  src/main/runtime-new.ts',
      'src/main/runtime-old.ts',
      '?? packages/runtime-go/internal/provider/provider_test.go',
      ''
    ].join('\0'))

    expect(files).toEqual([
      'src/main/runtime-sse-ipc.ts',
      'scripts/runtime-go-local-validation.mjs',
      'src/main/runtime-new.ts',
      'packages/runtime-go/internal/provider/provider_test.go'
    ])
    expect(changedFileImpacts(files).map((item: { id: string }) => item.id)).toEqual(expect.arrayContaining([
      'runtime_event_mapping',
      'provider_request'
    ]))
  })

  it('requires the full cache-first metadata block for sensitive files', () => {
    const result = validateCacheFirstReview({
      files: ['packages/runtime-go/internal/provider/provider.go'],
      metadataText: 'Cache-impact: medium'
    })

    expect(result.ok).toBe(false)
    expect(result.failures).toEqual(expect.arrayContaining([
      'missing required cache-first metadata: Cache-guard',
      'missing required cache-first metadata: Provider-compat-impact'
    ]))
  })

  it('accepts provider and SSE changes with concrete cache and speed guards', () => {
    const result = validateCacheFirstReview({
      files: [
        'packages/runtime-go/internal/provider/provider.go',
        'src/main/runtime-sse-ipc.ts'
      ],
      metadataText: completeMetadata
    })

    expect(result.ok).toBe(true)
  })

  it('rejects medium and high cache impact without a concrete Cache-guard', () => {
    const result = validateCacheFirstReview({
      files: ['packages/runtime-go/internal/server/runtime_components.go'],
      metadataText: completeMetadata.replace(
        'Cache-guard: go test ./internal/provider -run DeepSeek -count=1',
        'Cache-guard: none'
      )
    })

    expect(result.ok).toBe(false)
    expect(result.failures).toContain('Cache-impact medium/high requires a concrete Cache-guard')
  })

  it('requires a system prompt review for system prompt sensitive files', () => {
    const result = validateCacheFirstReview({
      files: ['packages/runtime/src/loop-test-support/agent-loop.ts'],
      metadataText: completeMetadata.replace('System-prompt-review: none required', 'System-prompt-review: none')
    })

    expect(result.ok).toBe(false)
    expect(result.failures).toContain('system prompt or mode instruction changes require System-prompt-review')
  })
})
