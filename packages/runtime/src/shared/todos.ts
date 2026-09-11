import type {
  ThreadTodoItem,
  ThreadTodoList,
  ThreadTodoSource,
  ThreadTodoStatus
} from '../contracts/threads.js'

type PlanTodoSource = Extract<ThreadTodoSource, { kind: 'plan' }>
type CompletePlanTodoSource = PlanTodoSource & Required<Omit<PlanTodoSource, 'kind'>>

export type ExtractedPlanTodo = ThreadTodoItem & { source: CompletePlanTodoSource }

type ParsedTaskLine = {
  prefix: string
  marker: string
  suffix: string
  content: string
}

export type MergePlanTodosOptions = {
  threadId: string
  existing: ThreadTodoList | null | undefined
  planItems: readonly ExtractedPlanTodo[]
  now: string
  preserveCompleted?: boolean
}

export function normalizeTodoContent(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

export function todoContentHash(value: string): string {
  const normalized = normalizeTodoContent(value).toLowerCase()
  let hash = 0x811c9dc5
  for (let index = 0; index < normalized.length; index += 1) {
    hash ^= normalized.charCodeAt(index)
    hash = Math.imul(hash, 0x01000193)
  }
  return (hash >>> 0).toString(36)
}

export function makePlanTodoId(input: {
  planId: string
  relativePath: string
  ordinal: number
  contentHash: string
}): string {
  const base = `${input.planId}:${input.relativePath}:${input.ordinal}:${input.contentHash}`
  return `todo_plan_${todoContentHash(base)}`
}

export function extractPlanTodos(input: {
  markdown: string
  planId: string
  relativePath: string
  threadId: string
  now: string
}): ExtractedPlanTodo[] {
  const items: ExtractedPlanTodo[] = []
  const lines = input.markdown.split(/\r?\n/)
  let ordinal = 0
  for (const line of lines) {
    const task = parseTaskLine(line)
    if (!task) continue
    const content = normalizeTodoContent(task.content)
    if (!content) continue
    const contentHash = todoContentHash(content)
    const source: CompletePlanTodoSource = {
      kind: 'plan',
      planId: input.planId,
      relativePath: normalizePlanRelativePath(input.relativePath),
      ordinal,
      contentHash
    }
    items.push({
      id: makePlanTodoId(source),
      content,
      status: taskMarkerToStatus(task.marker),
      source,
      createdAt: input.now,
      updatedAt: input.now
    })
    ordinal += 1
  }
  return items
}

export function mergePlanTodos(options: MergePlanTodosOptions): ThreadTodoList {
  const existingItems = options.existing?.items ?? []
  const usedExistingIds = new Set<string>()
  const nextItems: ThreadTodoItem[] = []

  for (const planItem of options.planItems) {
    const existing = findExistingPlanTodo(existingItems, usedExistingIds, planItem)
    if (existing) usedExistingIds.add(existing.id)
    const status =
      existing && options.preserveCompleted && (
        existing.status === 'completed' ||
        existing.status === 'failed' ||
        existing.status === 'canceled'
      )
        ? existing.status
        : existing?.status ?? planItem.status
    const statusReasonCode =
      status === 'failed' || status === 'canceled'
        ? existing?.statusReasonCode
        : undefined
    const note = statusReasonCode ? existing?.note : undefined
    nextItems.push({
      ...planItem,
      id: existing?.id ?? planItem.id,
      status,
      ...(statusReasonCode ? { statusReasonCode } : {}),
      ...(note ? { note } : {}),
      createdAt: existing?.createdAt ?? planItem.createdAt,
      updatedAt:
        existing &&
        existing.content === planItem.content &&
        existing.status === status &&
        existing.statusReasonCode === statusReasonCode &&
        existing.note === note
          ? existing.updatedAt
          : options.now
    })
  }

  for (const item of existingItems) {
    if (usedExistingIds.has(item.id)) continue
    if (item.source?.kind === 'plan') {
      nextItems.push({
        ...item,
        source: undefined,
        updatedAt: options.now
      })
    } else {
      nextItems.push(item)
    }
  }

  return {
    threadId: options.threadId,
    items: nextItems,
    updatedAt: options.now
  }
}

export function patchPlanTodoStatus(
  markdown: string,
  item: Pick<ThreadTodoItem, 'status' | 'source' | 'content'>
): { markdown: string; changed: boolean } {
  const source = item.source
  if (!isCompletePlanTodoSource(source)) return { markdown, changed: false }
  const lines = markdown.split(/\r?\n/)
  const lineEnding = markdown.includes('\r\n') ? '\r\n' : '\n'
  const tasks = lines
    .map((line, lineIndex) => ({ line, lineIndex, task: parseTaskLine(line) }))
    .filter((entry): entry is { line: string; lineIndex: number; task: ParsedTaskLine } =>
      Boolean(entry.task)
    )
    .map((entry, ordinal) => ({
      ...entry,
      ordinal,
      content: normalizeTodoContent(entry.task.content),
      contentHash: todoContentHash(entry.task.content)
    }))

  const target =
    tasks.find((task) => task.ordinal === source.ordinal && task.contentHash === source.contentHash) ??
    tasks.find((task) => task.contentHash === source.contentHash) ??
    tasks.find((task) => task.ordinal === source.ordinal)
  if (!target) return { markdown, changed: false }

  const marker = item.status === 'completed' ? 'x' : ' '
  const currentMarker = target.task.marker
  if (currentMarker.toLowerCase() === marker) return { markdown, changed: false }
  lines[target.lineIndex] = `${target.task.prefix}${marker}${target.task.suffix}${target.task.content}`
  return { markdown: lines.join(lineEnding), changed: true }
}

export function sourceKey(source: ThreadTodoSource): string {
  switch (source.kind) {
    case 'manual':
      return 'manual'
    case 'child':
      return `child:${source.parentThreadId ?? ''}:${source.childThreadId ?? ''}:${source.childRunId ?? ''}:${source.jobId ?? ''}:${source.projectionId ?? ''}`
    case 'plan':
      return `plan:${source.planId ?? ''}:${source.relativePath ?? ''}:${source.ordinal ?? ''}:${source.contentHash ?? ''}`
  }
}

export function normalizePlanRelativePath(relativePath: string): string {
  return relativePath.replaceAll('\\', '/').replace(/\/+/g, '/').replace(/^\.\//, '')
}

function taskMarkerToStatus(marker: string | undefined): ThreadTodoStatus {
  return marker?.toLowerCase() === 'x' ? 'completed' : 'pending'
}

function parseTaskLine(line: string): ParsedTaskLine | null {
  let index = 0
  while (index < line.length && isTaskWhitespace(line[index])) index += 1
  const bullet = line[index]
  if (bullet !== '-' && bullet !== '*' && bullet !== '+') return null
  index += 1
  if (!isTaskWhitespace(line[index])) return null
  while (index < line.length && isTaskWhitespace(line[index])) index += 1
  if (line[index] !== '[') return null

  const markerIndex = index + 1
  const marker = line[markerIndex]
  if (marker !== ' ' && marker !== 'x' && marker !== 'X') return null
  if (line[markerIndex + 1] !== ']') return null

  let contentStart = markerIndex + 2
  if (!isTaskWhitespace(line[contentStart])) return null
  while (contentStart < line.length && isTaskWhitespace(line[contentStart])) contentStart += 1

  const content = trimEndWhitespace(line.slice(contentStart))
  if (!content) return null
  return {
    prefix: line.slice(0, markerIndex),
    marker,
    suffix: line.slice(markerIndex + 1, contentStart),
    content
  }
}

function isTaskWhitespace(char: string | undefined): boolean {
  return char === ' ' || char === '\t'
}

function trimEndWhitespace(value: string): string {
  let end = value.length
  while (end > 0 && isTaskWhitespace(value[end - 1])) end -= 1
  return end === value.length ? value : value.slice(0, end)
}

function findExistingPlanTodo(
  existingItems: readonly ThreadTodoItem[],
  usedExistingIds: ReadonlySet<string>,
  planItem: ExtractedPlanTodo
): ThreadTodoItem | undefined {
  const candidates = existingItems.filter((item) => !usedExistingIds.has(item.id))
  return (
    candidates.find((item) =>
      item.source?.kind === 'plan' &&
      item.source.planId === planItem.source.planId &&
      item.source.relativePath === planItem.source.relativePath &&
      item.source.contentHash === planItem.source.contentHash
    ) ??
    candidates.find((item) =>
      item.source?.kind === 'plan' &&
      item.source.relativePath === planItem.source.relativePath &&
      item.source.contentHash === planItem.source.contentHash
    ) ??
    candidates.find((item) => todoContentHash(item.content) === planItem.source.contentHash) ??
    candidates.find((item) =>
      item.source?.kind === 'plan' &&
      item.source.planId === planItem.source.planId &&
      item.source.relativePath === planItem.source.relativePath &&
      item.source.ordinal === planItem.source.ordinal
    )
  )
}

function isCompletePlanTodoSource(source: ThreadTodoSource | undefined): source is CompletePlanTodoSource {
  return source?.kind === 'plan' &&
    typeof source.planId === 'string' && source.planId.length > 0 &&
    typeof source.relativePath === 'string' && source.relativePath.length > 0 &&
    typeof source.ordinal === 'number' && Number.isInteger(source.ordinal) && source.ordinal >= 0 &&
    typeof source.contentHash === 'string' && source.contentHash.length > 0
}
