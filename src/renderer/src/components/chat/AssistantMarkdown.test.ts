// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { clearFileReferenceValidationCache } from '../../lib/file-reference-validation'
import { sharedMarkdownFinalizationScheduler } from '../../thread/streaming/markdown-finalization-queue'
import { PersistedThreadTraceSink } from '../../thread/tracing/thread-performance-trace'
import {
  AssistantMarkdown,
  getMarkdownFinalizationPolicy,
  nextPlainTextVisibleLength,
  shouldDeferMarkdownFinalizationForPacedText,
  visiblePlainTextForTypewriter
} from './AssistantMarkdown'

describe('getMarkdownFinalizationPolicy', () => {
  it('allows small markdown to finalize through the normal queue', () => {
    expect(getMarkdownFinalizationPolicy('Short **answer**.')).toMatchObject({
      mode: 'immediate',
      chars: 17,
      codeFences: 0
    })
  })

  it('defers long markdown instead of rich-rendering it immediately', () => {
    const text = Array.from({ length: 220 }, (_, index) => `line ${index + 1}`).join('\n')

    expect(getMarkdownFinalizationPolicy(text)).toMatchObject({
      mode: 'defer',
      estimatedLines: 220
    })
  })

  it('defers very heavy code without discarding markdown semantics', () => {
    const code = 'x'.repeat(17000)
    const text = [
      'Analysis:',
      '',
      '```ts',
      code,
      '```'
    ].join('\n')

    expect(getMarkdownFinalizationPolicy(text)).toMatchObject({
      mode: 'defer',
      codeBlockChars: 17001
    })
  })
})

describe('visiblePlainTextForTypewriter', () => {
  it('does not cut a surrogate-pair emoji in half', () => {
    expect(visiblePlainTextForTypewriter('A🙂B', 2)).toBe('A🙂')
  })

  it('keeps combining marks attached to their base character', () => {
    const text = `Cafe\u0301 done`

    expect(visiblePlainTextForTypewriter(text, 4)).toBe('Cafe\u0301')
  })

  it('does not cut a zwj emoji sequence in half', () => {
    expect(visiblePlainTextForTypewriter('A👨‍👩‍👧‍👦B', 2)).toBe('A👨‍👩‍👧‍👦')
  })

  it('snaps backwards on replacement without negative animation', () => {
    expect(nextPlainTextVisibleLength(20, 5)).toBe(5)
  })

  it('catches up large streaming backlogs without a long typewriter queue', () => {
    expect(nextPlainTextVisibleLength(0, 10000)).toBeGreaterThanOrEqual(1000)
  })

  it('keeps the plain paced surface until a completed stream catches up', () => {
    expect(shouldDeferMarkdownFinalizationForPacedText({
      streaming: false,
      hadStreaming: true,
      pacedCaughtUp: false
    })).toBe(true)
    expect(shouldDeferMarkdownFinalizationForPacedText({
      streaming: false,
      hadStreaming: true,
      pacedCaughtUp: true
    })).toBe(false)
  })
})

const heavyCases = [
  ['characters', 'x'.repeat(24001)],
  ['lines', Array.from({ length: 701 }, () => 'line').join('\n')],
  ['five closed fences', Array.from({ length: 5 }, () => '```text\nx\n```').join('\n')],
  ['aggregate code', '```text\n' + 'x'.repeat(16000) + '\n```']
] as const

describe('cost controls settlement timing without a terminal plain-text mode', () => {
  it.each([
    ['characters at limit', 'x'.repeat(24000)],
    ['lines at limit', Array.from({ length: 700 }, () => 'line').join('\n')],
    ['eight delimiters', Array.from({ length: 4 }, () => '```text\nx\n```').join('\n')],
    ['aggregate code at limit', '```text\n' + 'x'.repeat(15999) + '\n```']
  ])('keeps %s eligible for deferred rich rendering', (_name, text) => {
    expect(getMarkdownFinalizationPolicy(text).mode).toBe('defer')
  })
  it.each(heavyCases)('keeps %s eligible for deferred rich rendering', (_name, text) => {
    expect(getMarkdownFinalizationPolicy(text).mode).toBe('defer')
  })
})

const codeBodies = Array.from({ length: 5 }, (_, index) => `code body ${index + 1}`)
const longCode = 'x'.repeat(17000)
const longProse = 'x'.repeat(24001)
const tableRows = Array.from({ length: 701 }, (_, index) => `| Row ${index + 1} | [Link ${index + 1}](https://example.test/${index + 1}) |`)
const table = ['| Name | Link |', '| --- | --- |', ...tableRows].join('\n')
const fiveCodeBlocks = codeBodies.map(code => `\`\`\`text\n${code}\n\`\`\``).join('\n\n')
const reply = (body: string) => `## Complete reply\n\n[Late reference][completion]\n\n${body}\n\n**TAIL_SENTINEL** [Destination](https://example.test/destination) $x^2$\n\n[completion]: https://example.test/final`
const historyCases = [
  { name: 'over 24k characters', body: longProse, code: [], rows: 0 },
  { name: 'over 700 lines of table and links', body: table, code: [], rows: 701 },
  { name: 'five closed code blocks', body: fiveCodeBlocks, code: codeBodies, rows: 0 },
  { name: 'over 16k code characters', body: `\`\`\`text\n${longCode}\n\`\`\``, code: [longCode], rows: 0 }
]

describe('deferred markdown through the actual wrapper and renderer', () => {
  let root: Root
  let container: HTMLDivElement
  let frames: Map<number, FrameRequestCallback>
  let idles: Map<number, () => void>
  let serial: number
  let previousChat: ReturnType<typeof useChatStore.getState>
  let enqueue: MockInstance<typeof sharedMarkdownFinalizationScheduler.enqueue>
  const finalized = vi.fn()

  const render = async (text: string, streaming = false) => act(async () => root.render(createElement(AssistantMarkdown, {
    text, streaming, rowId: 'public-reply', className: 'ds-markdown', onFinalized: finalized
  })))
  async function frame(): Promise<void> {
    const batch = [...frames.values()]
    frames.clear()
    await act(async () => { batch.forEach(callback => callback(0)) })
  }
  async function idle(): Promise<void> {
    const batch = [...idles.values()]
    idles.clear()
    await act(async () => { batch.forEach(callback => callback()) })
  }
  async function enqueueDeferred() {
    await act(async () => { vi.advanceTimersByTime(240) })
    await idle()
    expect(sharedMarkdownFinalizationScheduler.size).toBe(1)
    return enqueue.mock.calls.at(-1)![0]
  }
  async function settle(): Promise<void> {
    await enqueueDeferred()
    await idle()
    expect(sharedMarkdownFinalizationScheduler.size).toBe(0)
    await frame()
    expect(finalized).toHaveBeenCalled()
  }

  beforeEach(async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    await i18n.changeLanguage('en')
    await import('./DshAssistant')
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    frames = new Map(); idles = new Map(); serial = 0
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++serial, callback); return serial })
    vi.stubGlobal('cancelAnimationFrame', (handle: number) => frames.delete(handle))
    vi.stubGlobal('requestIdleCallback', (callback: () => void) => { idles.set(++serial, callback); return serial })
    vi.stubGlobal('cancelIdleCallback', (handle: number) => idles.delete(handle))
    vi.stubGlobal('analytix', { app: { openExternal: vi.fn().mockResolvedValue(true) }, files: { resolve: vi.fn().mockResolvedValue({ ok: false }) } })
    previousChat = useChatStore.getState()
    useChatStore.setState({ workspaceRoot: '/synthetic/workspace' })
    clearFileReferenceValidationCache()
    sharedMarkdownFinalizationScheduler.dispose()
    enqueue = vi.spyOn(sharedMarkdownFinalizationScheduler, 'enqueue')
    finalized.mockClear()
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    sharedMarkdownFinalizationScheduler.dispose()
    useChatStore.setState(previousChat)
    clearFileReferenceValidationCache()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
    await i18n.changeLanguage('en')
  })

  it.each(historyCases)('restores complete settled semantics on history first mount: $name', async ({ body, code, rows }) => {
    await render(reply(body))
    expect(container.querySelector('h2')?.textContent).toBe('Complete reply')
    expect(container.querySelector('strong')?.textContent).toBe('TAIL_SENTINEL')
    expect(container.querySelector('.katex')).toBeNull()
    expect(enqueue).not.toHaveBeenCalled()
    expect(finalized).not.toHaveBeenCalled()
    await settle()
    expect(container.querySelector('.katex')).not.toBeNull()
    expect(container.querySelector('[role="note"]')).toBeNull()
    expect(container.querySelector('a[href="https://example.test/destination"]')?.textContent).toContain('Destination')
    expect(container.querySelector('a[href="https://example.test/final"]')?.textContent).toContain('Late reference')
    expect([...container.querySelectorAll('pre code')].map(node => node.textContent)).toEqual(code)
    expect(container.querySelectorAll('tbody tr')).toHaveLength(rows)
    if (body === longProse) expect(container.textContent).toContain(longProse)
    if (rows) {
      expect(container.querySelectorAll('tbody a')).toHaveLength(rows)
      expect(container.querySelector('a[href="https://example.test/701"]')?.textContent).toContain('Link 701')
      expect(container.querySelector<HTMLButtonElement>('button[aria-label="Copy table"]')?.disabled).toBe(false)
    }
  })

  it('paces a long stream and self-heals frozen reference links only after queued settlement', async () => {
    const prefix = '## Complete reply\n\n[Late reference][completion]\n\nFirst block\n\nSecond block\n\nThird block'
    await render(prefix, true)
    for (let count = 0; frames.size && count < 100; count++) await frame()
    const text = reply('First block\n\nSecond block\n\nThird block\n\n' + fiveCodeBlocks + '\n\n' + longProse)
    await render(text, true)
    await render(text)
    await act(async () => { vi.advanceTimersByTime(240) })
    expect(enqueue).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('TAIL_SENTINEL')
    for (let count = 0; frames.size && count < 200; count++) await frame()
    expect(container.querySelector('strong')?.textContent).toBe('TAIL_SENTINEL')
    expect([...container.querySelectorAll('pre code')].map(node => node.textContent)).toEqual(codeBodies)
    expect(container.querySelector('a[href="https://example.test/final"]')).toBeNull()
    expect(container.querySelector('.katex')).toBeNull()
    expect(finalized).not.toHaveBeenCalled()
    await settle()
    expect(container.querySelector('a[href="https://example.test/final"]')?.textContent).toContain('Late reference')
    expect(container.querySelectorAll('h2')).toHaveLength(1)
    expect(container.querySelectorAll('strong')).toHaveLength(1)
    expect(container.textContent).toContain(longProse)
    expect(container.querySelector('.katex')).not.toBeNull()
  })

  it('keeps raw HTML/SVG, unsafe URLs and unvalidated files inert in a long settled reply', async () => {
    await render(reply(longProse + '\n\n<svg data-untrusted="raw"><script>RAW_SENTINEL</script></svg>\n\n[unsafe](javascript:alert%281%29) [denied](/synthetic/denied.ts#L3)\n\n![unsafe image](data:image/svg+xml,unsafe) ![safe image](https://example.test/image.png)\n\n$\\href{javascript:alert(1)}{unsafe}$'))
    await settle()
    expect(container.querySelector('script,svg[data-untrusted],a[href^="javascript:"],a[href^="data:"]')).toBeNull()
    expect(container.textContent).toContain('<svg data-untrusted="raw"><script>RAW_SENTINEL</script></svg>')
    expect(window.analytix.files.resolve).toHaveBeenCalledWith(expect.objectContaining({ path: '/synthetic/denied.ts', line: 3 }))
    expect(container.querySelector('.ds-file-reference-link')).toBeNull()
    expect(container.querySelectorAll('img')).toHaveLength(1)
    expect(container.querySelector('img')?.getAttribute('src')).toBe('https://example.test/image.png')
    expect(container.querySelector('img')?.getAttribute('referrerpolicy')).toBe('no-referrer')
    expect(window.analytix.app.openExternal).not.toHaveBeenCalled()
  })

  it('cancels a replaced queued reply and ignores its late finalizer', async () => {
    const text = reply(longProse)
    await render(text)
    const oldJob = await enqueueDeferred()
    const replacement = reply('**NEW_ONLY**\n\n' + 'y'.repeat(24001))
    await render(replacement)
    expect(sharedMarkdownFinalizationScheduler.size).toBe(0)
    await act(async () => oldJob.onFinalize({ rowId: oldJob.rowId, text: oldJob.text }))
    expect(container.querySelector('.katex')).toBeNull()
    expect(finalized).not.toHaveBeenCalled()
    await settle()
    expect(container.textContent).toContain('NEW_ONLY')
    expect(container.textContent).not.toContain(longProse)
  })

  it('rejects a late same-text finalizer after streaming restarts and settles again', async () => {
    const text = reply(longProse)
    await render(text)
    const oldJob = await enqueueDeferred()
    await render(text, true)
    await render(text)
    await act(async () => oldJob.onFinalize({ rowId: oldJob.rowId, text: oldJob.text }))
    await frame()
    expect(container.querySelector('.katex')).toBeNull()
    expect(finalized).not.toHaveBeenCalled()
    expect(sharedMarkdownFinalizationScheduler.size).toBe(0)
    await settle()
  })

  it.each([false, true])('cancels deferred work on unmount, already queued=%s', async queued => {
    const trace = vi.spyOn(PersistedThreadTraceSink.prototype, 'record')
    await render(reply(longProse))
    const job = queued ? await enqueueDeferred() : null
    await act(async () => root.render(null))
    await act(async () => {
      vi.advanceTimersByTime(240)
      job?.onFinalize({ rowId: job.rowId, text: job.text })
    })
    await idle(); await frame()
    expect(sharedMarkdownFinalizationScheduler.size).toBe(0)
    expect(container.textContent).toBe('')
    expect(finalized).not.toHaveBeenCalled()
    expect(trace).not.toHaveBeenCalled()
    if (!queued) expect(enqueue).not.toHaveBeenCalled()
  })
})
