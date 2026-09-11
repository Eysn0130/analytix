import { PENDING_AUTO_THREAD_TITLE, type ThreadRecord } from '../contracts/threads.js'
import { makeUserItem } from '../domain/item.js'
import type { ModelClient } from '../ports/model-client.js'
import type { ThreadService } from './thread-service.js'

const DEFAULT_TITLE_TIMEOUT_MS = 8_000
const DEFAULT_TITLE_MAX_TOKENS = 32
const MAX_TITLE_PROMPT_CHARS = 2_000
const MAX_TITLE_CHARS = 48
const AUTO_TITLE_PLACEHOLDERS = new Set([
  PENDING_AUTO_THREAD_TITLE,
  'New chat',
  'New Thread',
  '新会话'
])

export type ThreadTitleServiceDeps = {
  threadService: Pick<ThreadService, 'get' | 'update'>
  model: ModelClient
  timeoutMs?: number
}

export class ThreadTitleService {
  private readonly deps: ThreadTitleServiceDeps
  private readonly inFlight = new Set<string>()

  constructor(deps: ThreadTitleServiceDeps) {
    this.deps = deps
  }

  async generateForFirstUserPrompt(input: {
    threadId: string
    prompt: string
    model?: string
  }): Promise<void> {
    const prompt = normalizePrompt(input.prompt)
    if (!prompt) return
    if (this.inFlight.has(input.threadId)) return

    const initial = await this.deps.threadService.get(input.threadId)
    if (!initial || !shouldAutoGenerateTitle(initial)) return

    this.inFlight.add(input.threadId)
    try {
      const modelTitle = await this.generateModelTitle({
        thread: initial,
        prompt,
        model: input.model?.trim() || initial.model
      })
      const title = modelTitle || deriveFallbackTitle(prompt)
      if (!title) return

      const latest = await this.deps.threadService.get(input.threadId)
      if (!latest || !shouldAutoGenerateTitle(latest)) return
      await this.deps.threadService.update(input.threadId, { title })
    } finally {
      this.inFlight.delete(input.threadId)
    }
  }

  private async generateModelTitle(input: {
    thread: ThreadRecord
    prompt: string
    model: string
  }): Promise<string | null> {
    const controller = new AbortController()
    const timeout = setTimeout(() => controller.abort(), this.deps.timeoutMs ?? DEFAULT_TITLE_TIMEOUT_MS)
    let text = ''
    try {
      for await (const chunk of this.deps.model.stream({
        threadId: input.thread.id,
        turnId: `title_${input.thread.id}`,
        model: input.model,
        systemPrompt: [
          'Generate a concise title for this conversation.',
          'Return only the title, without quotes, markdown, punctuation decoration, or explanation.',
          'Use the same language as the user prompt when possible.'
        ].join(' '),
        prefix: [],
        history: [
          makeUserItem({
            id: `item_title_${input.thread.id}`,
            turnId: `title_${input.thread.id}`,
            threadId: input.thread.id,
            text: clipPrompt(input.prompt)
          })
        ],
        tools: [],
        stream: false,
        maxTokens: DEFAULT_TITLE_MAX_TOKENS,
        temperature: 0.2,
        abortSignal: controller.signal
      })) {
        if (chunk.kind === 'assistant_text_delta') {
          text += chunk.text
          continue
        }
        if (chunk.kind === 'error') return null
        if (chunk.kind === 'completed') break
      }
    } catch {
      return null
    } finally {
      clearTimeout(timeout)
    }
    return cleanGeneratedTitle(text)
  }
}

export function shouldAutoGenerateTitle(thread: Pick<ThreadRecord, 'title' | 'turns'>): boolean {
  if (thread.turns.length > 1) return false
  const title = thread.title.trim()
  return title.length === 0 || AUTO_TITLE_PLACEHOLDERS.has(title) || title === PENDING_AUTO_THREAD_TITLE
}

export function cleanGeneratedTitle(value: string): string | null {
  let title = normalizeTitleLine(value)
    .replace(/^["'“”‘’]+|["'“”‘’]+$/g, '')
    .replace(/^[《「『【]+|[》」』】]+$/g, '')
    .replace(/^(?:title|标题)\s*[:：]\s*/i, '')
    .trim()
  const newline = title.search(/\r?\n/)
  if (newline > 0) title = title.slice(0, newline).trim()
  title = stripTrailingPunctuation(shortenTitle(title))
  return title || null
}

export function deriveFallbackTitle(prompt: string): string | null {
  const lines = prompt
    .split(/\r?\n/)
    .filter((line) => !/^\s*(```|~~~)/.test(line))
    .map((line) => normalizeTitleLine(line))
    .filter((line) => line.length > 0)

  const firstLine = lines[0] ?? normalizeTitleLine(prompt)
  if (!firstLine) return null
  const sentenceBreak = firstLine.search(/[。！？.!?]/)
  const core = sentenceBreak >= 4 ? firstLine.slice(0, sentenceBreak) : firstLine
  return stripTrailingPunctuation(shortenTitle(core)) || null
}

function normalizePrompt(prompt: string): string {
  return prompt.replace(/\s+/g, ' ').trim()
}

function clipPrompt(prompt: string): string {
  return prompt.length <= MAX_TITLE_PROMPT_CHARS ? prompt : prompt.slice(0, MAX_TITLE_PROMPT_CHARS)
}

function normalizeTitleLine(line: string): string {
  return line
    .replace(/^#{1,6}\s+/, '')
    .replace(/^>\s+/, '')
    .replace(/^[-*+]\s+/, '')
    .replace(/^\d+[.)]\s+/, '')
    .replace(/`+/g, '')
    .replace(/\[(.*?)\]\((.*?)\)/g, '$1')
    .replace(/\s+/g, ' ')
    .trim()
}

function stripTrailingPunctuation(text: string): string {
  return text.replace(/[\s,.;:!?，。；：！？、'"`()[\]{}]+$/g, '').trim()
}

function shortenTitle(text: string): string {
  if (text.length <= MAX_TITLE_CHARS) return text
  const sliced = text.slice(0, MAX_TITLE_CHARS)
  const lastSpace = sliced.lastIndexOf(' ')
  const compact = lastSpace >= 18 ? sliced.slice(0, lastSpace) : sliced
  return compact.trim()
}
