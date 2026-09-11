#!/usr/bin/env node

import { readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'

export const REQUIRED_METADATA_KEYS = [
  'Cache-impact',
  'Cache-guard',
  'System-prompt-review',
  'Provider-compat-impact',
  'Speed-impact',
  'Speed-guard'
]

const CACHE_IMPACTS = new Set(['none', 'low', 'medium', 'high'])

const SENSITIVE_RULES = [
  {
    id: 'system_prompt',
    label: 'Provider-visible system prompt or mode instruction',
    pattern: /(?:system[-_]?prompt|mode[-_]?instruction|goal[-_]?instruction|plan[-_]?instruction|runtime-factory|agent-loop|AGENTS\.md|重构升级方案\.md)/i
  },
  {
    id: 'memory_prefix',
    label: 'Memory prefix, skill index, output style, or reasoning language',
    pattern: /(?:memory|skill|output[-_]?style|reasoning[-_]?language|skills?\/|skill-runtime|frontmatter)/i
  },
  {
    id: 'builtin_tool_schema',
    label: 'Built-in tool name, description, schema, readOnly flag, order, or alias',
    pattern: /(?:local-tool-host|capability-registry|contracts\/capabilities|runtime_(?:server|components)\.go|internal\/tool|tools?\/|tool[-_]?schema|tool[-_]?catalog)/i
  },
  {
    id: 'mcp_tool_registration',
    label: 'MCP/tool registration, lazy catalog, schema cache, or SpecFingerprint',
    pattern: /(?:\/mcp\/|mcp-|mcp_|plugin|SpecFingerprint|schema[-_]?cache|manager\.go)/i
  },
  {
    id: 'provider_request',
    label: 'Provider request serialization, headers, stream parser, or usage parser',
    pattern: /(?:internal\/provider|compat-model-client|model-client|provider-connection|upstream-models|openai-compat-url|usage(?:\.go|-service|\.ts)|pricing)/i
  },
  {
    id: 'history_repair',
    label: 'History repair, compaction, log rewrite, or tool-result pruning',
    pattern: /(?:history|compaction|compact|model-history-repair|history-healing|thread-service|SanitizeToolPairing|NormalizeMessages|NormalizeSessionMessages)/i
  },
  {
    id: 'cache_review_gate',
    label: 'Cache-first review gate, speed/cache validation, or prefix diagnostics',
    pattern: /(?:cache-first-review|speed-cache-gate|product-regression|runtime-go-performance-check|prefix-cache|cache-diagnostics|PrefixShape)/i
  },
  {
    id: 'attachments_vision',
    label: 'Attachments, vision, references, snippets, or fallback injection',
    pattern: /(?:attachment|vision|workspace-file|file-reference|clipboard|image|pdf|resolveAttachments)/i
  },
  {
    id: 'runtime_event_mapping',
    label: 'Runtime event mapping, SSE, renderer streaming, or timeline projection',
    pattern: /(?:runtime-sse|analytix-mapper|analytix-runtime|streaming|MessageTimeline|events\.ts|contracts\/events|recordRuntime|tool_progress|assistant_text_delta|assistant_reasoning_delta)/i
  },
  {
    id: 'settings_provider_migration',
    label: 'Settings/provider migration, baseUrl, endpointFormat, model list, or probe',
    pattern: /(?:app-settings|settings-store|provider-connection|endpointFormat|baseUrl|model list|model-list|provider profile|providers?\[\])/i
  },
  {
    id: 'tool_naming_migration',
    label: 'Kun/Reasonix tool naming migration',
    pattern: /(?:read_file|write_file|edit_file|delegate_task|parallel_tasks|tool[-_]?alias|tool[-_]?migration)/i
  }
]

export function changedFileImpacts(files) {
  const impacts = []
  for (const file of files) {
    const normalized = String(file || '').trim()
    if (!normalized) continue
    for (const rule of SENSITIVE_RULES) {
      if (rule.pattern.test(normalized)) {
        impacts.push({ file: normalized, id: rule.id, label: rule.label })
      }
    }
  }
  return impacts
}

export function parseReviewMetadata(text) {
  const metadata = new Map()
  for (const line of String(text || '').split(/\r?\n/)) {
    const match = line.match(/^\s*([A-Za-z-]+):\s*(.*?)\s*$/)
    if (match) {
      metadata.set(match[1], match[2])
    }
  }
  return metadata
}

export function validateCacheFirstReview({ files = [], metadataText = '' } = {}) {
  const impacts = changedFileImpacts(files)
  const metadata = parseReviewMetadata(metadataText)
  const failures = []
  if (impacts.length === 0) {
    return { ok: true, impacts, failures, metadata: Object.fromEntries(metadata) }
  }

  for (const key of REQUIRED_METADATA_KEYS) {
    if (!metadata.has(key) || String(metadata.get(key) || '').trim() === '') {
      failures.push(`missing required cache-first metadata: ${key}`)
    }
  }

  const impact = String(metadata.get('Cache-impact') || '').trim().toLowerCase()
  if (impact && !CACHE_IMPACTS.has(impact)) {
    failures.push(`Cache-impact must be one of ${Array.from(CACHE_IMPACTS).join(', ')}`)
  }
  if ((impact === 'medium' || impact === 'high') && metadataValueLooksEmpty(metadata.get('Cache-guard'))) {
    failures.push('Cache-impact medium/high requires a concrete Cache-guard')
  }
  if (impacts.some((item) => item.id === 'system_prompt') && metadataValueLooksEmpty(metadata.get('System-prompt-review'))) {
    failures.push('system prompt or mode instruction changes require System-prompt-review')
  }
  if (impacts.some((item) => item.id === 'provider_request') && metadataValueLooksEmpty(metadata.get('Provider-compat-impact'))) {
    failures.push('provider request changes require Provider-compat-impact')
  }
  if (impacts.some((item) => item.id === 'runtime_event_mapping') && metadataValueLooksEmpty(metadata.get('Speed-guard'))) {
    failures.push('runtime event/SSE/renderer changes require Speed-guard')
  }

  return {
    ok: failures.length === 0,
    impacts,
    failures,
    metadata: Object.fromEntries(metadata)
  }
}

function metadataValueLooksEmpty(value) {
  const normalized = String(value || '').trim().toLowerCase()
  return normalized === '' || normalized === 'none' || normalized === 'n/a' || normalized === 'na'
}

export function parseGitPorcelainChangedFiles(text) {
  const parts = String(text || '').split('\0').filter(Boolean)
  const files = []
  for (let index = 0; index < parts.length; index += 1) {
    const entry = parts[index]
    const status = entry.slice(0, 2)
    const file = entry.slice(3)
    if (!file) continue
    files.push(file)
    if (status.includes('R') || status.includes('C')) {
      index += 1
    }
  }
  return Array.from(new Set(files))
}

function readChangedFilesFromGit() {
  const result = spawnSync('git', ['status', '--porcelain=v1', '-z', '--untracked-files=all'], { encoding: 'utf8' })
  if (result.status !== 0) {
    throw new Error(result.stderr || result.stdout || 'git status --porcelain failed')
  }
  return parseGitPorcelainChangedFiles(result.stdout)
}

export function parseArgs(argv) {
  const args = { files: undefined, metadataText: '', json: false }
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index]
    if (arg === '--json') {
      args.json = true
    } else if (arg === '--files') {
      const files = []
      while (index + 1 < argv.length && !String(argv[index + 1]).startsWith('--')) {
        files.push(...String(argv[++index]).split(',').map((item) => item.trim()).filter(Boolean))
      }
      args.files = files
    } else if (arg === '--metadata') {
      args.metadataText = String(argv[++index] || '')
    } else if (arg === '--metadata-file') {
      args.metadataText = readFileSync(String(argv[++index] || ''), 'utf8')
    }
  }
  return args
}

function main() {
  const args = parseArgs(process.argv.slice(2))
  const files = args.files ?? readChangedFilesFromGit()
  const result = validateCacheFirstReview({ files, metadataText: args.metadataText })
  if (args.json) {
    console.log(JSON.stringify(result, null, 2))
  } else if (!result.ok) {
    console.error('Cache-first review gate failed:')
    for (const failure of result.failures) {
      console.error(`- ${failure}`)
    }
    console.error('Sensitive files:')
    for (const impact of result.impacts) {
      console.error(`- ${impact.file}: ${impact.label}`)
    }
  }
  process.exit(result.ok ? 0 : 1)
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main()
}
