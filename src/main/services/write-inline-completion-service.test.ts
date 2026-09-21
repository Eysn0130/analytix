import { mkdtemp, open, readdir, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsV1
} from '../../shared/app-settings'
import type { WriteInlineCompletionRequest } from '../../shared/write-inline-completion'
import { clearWriteRetrievalCache } from './write-retrieval-service'
vi.mock('node:fs/promises', async (importOriginal) => {
  const actual = await importOriginal<typeof import('node:fs/promises')>()
  return { ...actual, open: vi.fn(actual.open), readdir: vi.fn(actual.readdir) }
})
import {
  buildWriteInlineCompletionPrompt,
  clearWriteInlineCompletionDebugEntries,
  listWriteInlineCompletionDebugEntries,
  parseWriteInlineAction,
  requestWriteInlineCompletion
} from './write-inline-completion-service'
function createSettings(patch: Partial<AppSettingsV1['write']['inlineCompletion']> = {}): AppSettingsV1 {
  const write = defaultWriteSettings()
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/workspace',
    log: {
      enabled: true,
      retentionDays: 2
    },
    notifications: {
      turnComplete: true
    },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: {
      ...write,
      inlineCompletion: {
        ...write.inlineCompletion,
        ...patch
      }
    },
    schedule: defaultScheduleSettings(),
    guiUpdate: {
      channel: 'stable'
    },
    codePromptPrefix: '',
    disabledSkillIds: [],
    claw: defaultClawSettings()
  }
}

function createRequest(): WriteInlineCompletionRequest {
  return {
    prefix: '# Draft\n\nThis is',
    suffix: ' a test.',
    currentFilePath: '/tmp/workspace/draft.md',
    cursor: {
      line: 3,
      column: 7
    },
    context: {
      language: 'markdown',
      currentLinePrefix: 'This is',
      currentLineSuffix: ' a test.',
      previousLine: '',
      previousNonEmptyLine: '# Draft',
      nextLine: '',
      indentation: '',
      signals: {
        list: false,
        quote: false,
        heading: false,
        table: false,
        atLineEnd: false,
        endsWithSentencePunctuation: false,
        previousLineEndsWithSentencePunctuation: false,
        prefersNewLineCompletion: false,
        paragraphBreakOpportunity: false
      }
    },
    policy: {
      name: 'precision-inline-v2',
      instruction: 'Return only inserted text.',
      acceptanceCriteria: ['Keep it short.'],
      rejectionCriteria: ['Do not ramble.']
    },
    preview: {
      local: 'This is',
      documentTail: '# Draft This is'
    },
    model: 'deepseek-v4-flash'
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
  clearWriteRetrievalCache()
  clearWriteInlineCompletionDebugEntries()
})

describe('requestWriteInlineCompletion', () => {
  it.each(['create', 'poll'])('rejects ownership lost during %s while releasing only its own temporary thread', async (phase) => {
    let current = true
    const runtime = vi.fn(async (path: string, method?: string) => {
      if (path === '/v1/threads' && method === 'POST') {
        if (phase === 'create') current = false
        return { ok: true, status: 201, body: JSON.stringify({ id: 'thr_owned' }) }
      }
      if (method === 'POST') return { ok: true, status: 202, body: JSON.stringify({ turnId: 'turn_owned' }) }
      if (method === 'DELETE') return { ok: true, status: 204, body: '' }
      current = false
      return { ok: true, status: 200, body: JSON.stringify({ turns: [{ id: 'turn_owned', status: 'completed',
        items: [{ kind: 'assistant_text', text: 'LATE_RESULT' }] }] }) }
    })
    const result = await requestWriteInlineCompletion(createSettings({ retrievalEnabled: false }), createRequest(), runtime, { isCurrent: () => current })
    expect(result.ok).toBe(false)
    expect(JSON.stringify(result)).not.toContain('LATE_RESULT')
    expect(runtime).toHaveBeenLastCalledWith('/v1/threads/thr_owned', 'DELETE')
    if (phase === 'create') expect(runtime).toHaveBeenCalledTimes(2)
  })

  it('does not request the API when inline completion is disabled', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    const result = await requestWriteInlineCompletion(createSettings({ enabled: false }), createRequest())

    expect(result).toEqual({ ok: false, message: 'Inline completion is disabled.' })
    expect(fetchMock).not.toHaveBeenCalled()
    const debugEntries = listWriteInlineCompletionDebugEntries()
    expect(debugEntries).toHaveLength(1)
    expect(debugEntries[0]).toMatchObject({
      ok: false,
      errorCode: 'preflight_failed',
      completion: { sha256: expect.stringMatching(/^[a-f0-9]{64}$/), bytes: 0 },
      responseChars: 0
    })
  })

  it('fails closed without a Go runtime execution boundary', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const settings = createSettings()

    const result = await requestWriteInlineCompletion(settings, createRequest())

    expect(result).toEqual({ ok: false, message: 'Inline completion runtime is unavailable.' })
    expect(fetchMock).not.toHaveBeenCalled()
    const debugEntries = listWriteInlineCompletionDebugEntries()
    expect(debugEntries).toHaveLength(1)
    expect(debugEntries[0]).toMatchObject({
      ok: false,
      errorCode: 'preflight_failed',
      mode: 'short',
      suffix: { sha256: expect.stringMatching(/^[a-f0-9]{64}$/), bytes: 8 },
      responseChars: 0
    })
    expect(JSON.stringify(debugEntries[0])).not.toContain('Analytix inline completion')
    expect(JSON.stringify(debugEntries[0])).not.toContain('# Draft')
  })

  it.each([true, false])('sends key-free runtime intent with retrievalEnabled=%s and never performs direct provider fetch', async (retrievalEnabled) => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const runtimeRequest = vi.fn(async (path: string, method?: string, body?: string) => {
      if (path === '/v1/threads' && method === 'POST') {
        return { ok: true, status: 201, body: JSON.stringify({ id: 'thr_inline' }) }
      }
      if (path === '/v1/threads/thr_inline/turns' && method === 'POST') {
        return { ok: true, status: 202, body: JSON.stringify({ turnId: 'turn_inline' }) }
      }
      if (path === '/v1/threads/thr_inline' && method === 'GET') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            turns: [{
              id: 'turn_inline',
              status: 'completed',
              items: [{ kind: 'assistant_text', text: '<<<SHORT\n continuation\n>>>' }]
            }]
          })
        }
      }
      if (path === '/v1/threads/thr_inline' && method === 'DELETE') {
        return { ok: true, status: 204, body: '' }
      }
      throw new Error(`unexpected runtime request ${method ?? 'GET'} ${path} ${body ?? ''}`)
    })

    const settings = createSettings({ retrievalEnabled })
    settings.runtime.baseUrl = 'https://copied-route.invalid/v1'
    settings.provider.proxy = { enabled: true, url: 'socks5://copied-proxy.invalid:1080' }
    const workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-inline-retrieval-'))
    const referencePath = join(workspaceRoot, 'reference.md')
    const reference = '# BM25 retrieval\n\nBM25 retrieval SYNTHETIC_CONTEXT_CANARY provides relevant background for this synthetic draft completion.'
    await writeFile(referencePath, reference, 'utf8')
    const scan = vi.fn(async () => [{ path: 'reference.md', kind: 'text' as const, revision: '0'.repeat(64), content: Buffer.from(reference).toString('base64') }])
    const request = {
      ...createRequest(), workspaceRoot, currentFilePath: join(workspaceRoot, 'draft.md'),
      prefix: '# Draft\n\nBM25 retrieval', preview: { local: 'BM25 retrieval', documentTail: 'BM25 retrieval' }
    }
    const result = await requestWriteInlineCompletion(settings, request, runtimeRequest, { source: { threadId: 'test-thread', key: workspaceRoot, workspaceRoot, current: async () => true, scan } })

    expect(result).toMatchObject({
      ok: true,
      completion: ' continuation',
      action: { kind: 'short', text: ' continuation' }
    })
    expect(fetchMock).not.toHaveBeenCalled()
    const serializedCalls = JSON.stringify(runtimeRequest.mock.calls)
    expect(serializedCalls).not.toMatch(/copied-route|copied-proxy|apiKey|credentialRef|Authorization/)
    if (retrievalEnabled) {
      expect(scan).toHaveBeenCalledExactlyOnceWith(false)
      expect(readdir).not.toHaveBeenCalled()
      expect(open).not.toHaveBeenCalled()
      expect(serializedCalls).toContain('SYNTHETIC_CONTEXT_CANARY')
    } else {
      expect(scan).not.toHaveBeenCalled()
      expect(readdir).not.toHaveBeenCalled()
      expect(open).not.toHaveBeenCalled()
      expect(serializedCalls).not.toContain('SYNTHETIC_CONTEXT_CANARY')
    }
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_inline/turns',
      'POST',
      expect.stringContaining('"maxModelSteps":1')
    )
    expect(runtimeRequest).toHaveBeenCalledWith('/v1/threads/thr_inline', 'DELETE')
  })

  it('builds the unified action prompt without retrieval snippets when none are supplied', () => {
    const request = createRequest()

    const prompt = buildWriteInlineCompletionPrompt(request, null)
    expect(prompt).toContain('Analytix inline completion')
    expect(prompt).toContain('<<<PREFIX')
    expect(prompt).toContain('<<<SUFFIX')
    expect(prompt).not.toContain('<<<SHORT')
    expect(prompt).not.toContain('Reference snippets from the same writing workspace')
    expect(prompt.endsWith(request.prefix)).toBe(true)
  })
})

describe('parseWriteInlineAction', () => {
  it('parses TextIDE-style marked short, long, and edit blocks', () => {
    expect(parseWriteInlineAction('<<<SHORT\n next words\n>>>')).toEqual({
      kind: 'short',
      text: ' next words'
    })
    expect(parseWriteInlineAction('<<<LONG\n\nA fuller continuation.\n>>>')).toEqual({
      kind: 'long',
      text: '\nA fuller continuation.'
    })
    expect(parseWriteInlineAction('<<<EDIT\nWrite mode\n>>>', {
      editTarget: {
        from: 9,
        to: 21,
        original: 'Project GUI',
        scopeKind: 'selection'
      }
    })).toEqual({
      kind: 'edit',
      replacement: 'Write mode',
      from: 9,
      to: 21,
      original: 'Project GUI',
      scopeKind: 'selection'
    })
  })

  it('suppresses echoed boundary-marker prompts', () => {
    expect(parseWriteInlineAction('<<<PREFIX\nThis is\n>>>\n<<<SUFFIX\n a test.\n>>>')).toEqual({
      kind: 'short',
      text: ''
    })
  })

  it('parses JSON action payloads', () => {
    expect(parseWriteInlineAction(JSON.stringify({ kind: 'long', text: 'Continue the paragraph.' }))).toEqual({
      kind: 'long',
      text: 'Continue the paragraph.'
    })
    expect(parseWriteInlineAction(JSON.stringify({ action: 'edit', replacement: 'Rewrite locally.' }), {
      editTarget: {
        from: 3,
        to: 11,
        original: 'Old text',
        scopeKind: 'paragraph'
      }
    })).toEqual({
      kind: 'edit',
      replacement: 'Rewrite locally.',
      from: 3,
      to: 11,
      original: 'Old text',
      scopeKind: 'paragraph'
    })
  })

  it('parses XML-style action wrappers', () => {
    expect(parseWriteInlineAction('<short>next words</short>')).toEqual({
      kind: 'short',
      text: 'next words'
    })
    expect(parseWriteInlineAction('<long>Two sentences.\nMaybe three.</long>')).toEqual({
      kind: 'long',
      text: 'Two sentences.\nMaybe three.'
    })
    expect(parseWriteInlineAction('<edit>Replace this scope</edit>', {
      editTarget: {
        from: 12,
        to: 20,
        original: 'old value',
        scopeKind: 'selection'
      }
    })).toEqual({
      kind: 'edit',
      replacement: 'Replace this scope',
      from: 12,
      to: 20,
      original: 'old value',
      scopeKind: 'selection'
    })
  })

  it('parses labeled plain-text fallbacks', () => {
    expect(parseWriteInlineAction('completion: next sentence')).toEqual({
      kind: 'short',
      text: 'next sentence'
    })
    expect(parseWriteInlineAction('long: A fuller continuation.')).toEqual({
      kind: 'long',
      text: 'A fuller continuation.'
    })
    expect(parseWriteInlineAction('edit: Rewrite this block', {
      editTarget: {
        from: 1,
        to: 4,
        original: 'old',
        scopeKind: 'paragraph'
      }
    })).toEqual({
      kind: 'edit',
      replacement: 'Rewrite this block',
      from: 1,
      to: 4,
      original: 'old',
      scopeKind: 'paragraph'
    })
  })

  it('falls back to the requested mode for unstructured plain text', () => {
    expect(parseWriteInlineAction('Raw continuation text')).toEqual({
      kind: 'short',
      text: 'Raw continuation text'
    })
    expect(parseWriteInlineAction('Raw long continuation', { fallbackKind: 'long' })).toEqual({
      kind: 'long',
      text: 'Raw long continuation'
    })
    expect(parseWriteInlineAction('Raw edit replacement', {
      fallbackKind: 'edit',
      editTarget: {
        from: 8,
        to: 15,
        original: 'old text',
        scopeKind: 'selection'
      }
    })).toEqual({
      kind: 'edit',
      replacement: 'Raw edit replacement',
      from: 8,
      to: 15,
      original: 'old text',
      scopeKind: 'selection'
    })
  })

  it('returns an empty completion for a malformed marker skeleton instead of leaking markers', () => {
    // Regression: a degenerate single-line skeleton used to fall through to the
    // plain-text fallback and render ">>> <<<LONG >>> <<<EDIT" as ghost text.
    expect(parseWriteInlineAction('>>> <<<LONG >>> <<<EDIT')).toEqual({
      kind: 'short',
      text: ''
    })
    expect(parseWriteInlineAction('<<<SHORT >>> <<<LONG >>> <<<EDIT >>>', { fallbackKind: 'long' })).toEqual({
      kind: 'long',
      text: ''
    })
  })

  it('returns an empty completion when the model parrots the protocol template', () => {
    const template = [
      '<<<SHORT',
      'short text to insert at the cursor',
      '>>>',
      '<<<LONG',
      'longer continuation to insert at the cursor',
      '>>>',
      '<<<EDIT',
      'replacement text for the editable local scope',
      '>>>'
    ].join('\n')
    expect(parseWriteInlineAction(template)).toEqual({ kind: 'short', text: '' })
  })

  it('parses same-line marked blocks', () => {
    expect(parseWriteInlineAction('<<<SHORT next words >>>')).toEqual({
      kind: 'short',
      text: 'next words'
    })
  })

  it('prefers the first non-empty block when an earlier block is empty', () => {
    expect(parseWriteInlineAction('<<<SHORT\n>>>\n<<<LONG\nA fuller continuation.\n>>>')).toEqual({
      kind: 'long',
      text: 'A fuller continuation.'
    })
  })

  it('extracts a block that dropped its closing marker without swallowing the next marker', () => {
    expect(parseWriteInlineAction('<<<SHORT\nnext words\n<<<EDIT\nignored\n>>>')).toEqual({
      kind: 'short',
      text: 'next words'
    })
  })

  it('keeps plain text that legitimately contains >>> when no protocol marker is present', () => {
    expect(parseWriteInlineAction('>>> a Python prompt')).toEqual({
      kind: 'short',
      text: '>>> a Python prompt'
    })
  })
})
