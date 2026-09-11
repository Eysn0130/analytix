import type { ReactElement } from 'react'
import { CircleDot, ExternalLink, Square } from 'lucide-react'
import type { CoreThreadSummarySubagentJson } from '../../agent/analytix-contract'
import { SummaryRow } from './SummaryRow'

const IDENTICON_SCAN_DELAY_MS = 200
const IDENTICON_SIZE = 5
const IDENTICON_HALF_COLUMNS = 3
const IDENTICON_CELL_SIZE = 4
const IDENTICON_HASH_MODULUS = 4_294_967_296
const IDENTICON_HASH_OFFSET = 2_166_136_261
const IDENTICON_HASH_PRIME = 131
const IDENTICON_COLORS = [
  'var(--color-token-charts-yellow, #fbbc04)',
  'var(--color-token-charts-orange, #fb923c)',
  'var(--color-token-charts-red, #ef4444)',
  'var(--color-token-charts-purple, #8b5cf6)',
  'var(--color-token-charts-blue, #2f8cff)'
]
const SUBAGENT_FALLBACK_NAMES = [
  'Lagrange',
  'Nash',
  'Noether',
  'Turing',
  'Euler',
  'Ada',
  'Kepler',
  'Curie',
  'Fermi',
  'Gauss',
  'Hopper',
  'Bohr'
]
const GENERIC_SUBAGENT_LABELS = new Set([
  '',
  'task',
  'delegate_task',
  'subagent',
  'child-run',
  'child run',
  'parallel-child-run'
])

type IdenticonCell = {
  animationDelayMs: number
  column: number
  filled?: boolean
  row: number
}

export function SubagentSummaryRows({
  subagents,
  busyTaskId,
  onInspectChildAgent,
  onKill,
  onOpenDiagnostics,
  onOpenThread
}: {
  subagents: CoreThreadSummarySubagentJson[]
  busyTaskId?: string | null
  onInspectChildAgent?: (agent: CoreThreadSummarySubagentJson) => void
  onKill?: (agent: CoreThreadSummarySubagentJson) => void
  onOpenDiagnostics?: (agent: CoreThreadSummarySubagentJson) => void
  onOpenThread?: (threadId: string, title: string) => void
}): ReactElement {
  if (subagents.length === 0) {
    return <EmptyRows label="暂无子智能体" />
  }
  return (
    <>
      {subagents.map((agent) => {
        const active = agent.status === 'active'
        const label = subagentDisplayLabel(agent)
        const detail = subagentDetail(agent)
        const taskId = subagentTaskId(agent)
        const busy = Boolean(taskId && busyTaskId === taskId)
        const childThreadId = agent.canOpenThread ? agent.childThreadId?.trim() : ''
        const canOpenChildThread = Boolean(childThreadId && onOpenThread)
        const canKill = Boolean(agent.canKill && taskId && onKill)
        const hasDiagnostics = Boolean(onOpenDiagnostics && (taskId || agent.childThreadId || agent.parentThreadId))
        const hasActions = canOpenChildThread || canKill || hasDiagnostics
        return (
          <SummaryRow
            key={agent.key}
            label={<SubagentLabel agent={agent} label={label} active={active || busy} />}
            labelClassName="flex min-w-0 items-center"
            detail={detail}
            title={detail || undefined}
            interactive={Boolean(onInspectChildAgent || canOpenChildThread)}
            onClick={onInspectChildAgent
              ? () => onInspectChildAgent(agent)
              : canOpenChildThread && childThreadId && onOpenThread
                ? () => onOpenThread(childThreadId, label)
                : undefined}
            actions={hasActions ? (
              <>
                {canOpenChildThread && childThreadId && onOpenThread ? (
                  <IconButton label="打开子线程" onClick={() => onOpenThread(childThreadId, label)}>
                    <ExternalLink className="h-3.5 w-3.5" />
                  </IconButton>
                ) : null}
                {hasDiagnostics ? (
                  <IconButton label="打开运行时诊断" onClick={() => onOpenDiagnostics?.(agent)}>
                    <CircleDot className="h-3.5 w-3.5" />
                  </IconButton>
                ) : null}
                {canKill ? (
                  <IconButton label="停止子智能体" onClick={() => onKill?.(agent)}>
                    <Square className="h-3.5 w-3.5" />
                  </IconButton>
                ) : null}
              </>
            ) : undefined}
          />
        )
      })}
    </>
  )
}

export function subagentTaskId(agent: CoreThreadSummarySubagentJson): string | undefined {
  const jobId = agent.taskJobId?.trim() || agent.childRunId?.trim()
  return jobId ? `taskjob:${jobId}` : undefined
}

function SubagentLabel({
  agent,
  label,
  active
}: {
  agent: CoreThreadSummarySubagentJson
  label: string
  active: boolean
}): ReactElement {
  const meta = compactSubagentInlineMeta(agent)
  return (
    <span className="flex min-w-0 items-center gap-2">
      <SubagentGlyph seed={agent.childThreadId ?? agent.childRunId ?? agent.key} active={active} />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {meta ? (
        <span
          className="max-w-56 shrink-0 truncate text-[11px] font-medium leading-4 text-ds-faint"
          aria-label={`执行 ${meta}`}
        >
          {meta}
        </span>
      ) : null}
      {active ? (
        <span className="ds-summary-shimmer-text shrink-0 whitespace-nowrap text-ds-faint">
          运行中
        </span>
      ) : null}
    </span>
  )
}

export function subagentDisplayLabel(agent: CoreThreadSummarySubagentJson): string {
  const title = normalizeSubagentTitleLabel(agent.title, agent.label, disallowed)
  const label = normalizeSubagentTitleLabel(agent.label, agent.title, disallowed)
  return (
    normalizeSubagentDisplayLabel(agent.displayName, disallowed)
    ?? normalizeSubagentDisplayLabel(agent.agentNickname, disallowed)
    ?? title
    ?? label
    ?? generatedSubagentDisplayLabel(agent)
  )
}

const disallowed: Array<string | undefined> = []

function normalizeSubagentTitleLabel(
  value: string | undefined,
  mirror: string | undefined,
  disallowed: Array<string | undefined>
): string | undefined {
  if (sameSubagentDisplaySource(value, mirror)) return undefined
  return normalizeSubagentDisplayLabel(value, disallowed)
}

function sameSubagentDisplaySource(left: string | undefined, right: string | undefined): boolean {
  const a = left?.trim().replace(/\s+/g, ' ')
  const b = right?.trim().replace(/\s+/g, ' ')
  return Boolean(a && b && a === b)
}

function normalizeSubagentDisplayLabel(
  value: string | undefined,
  disallowed: Array<string | undefined> = []
): string | undefined {
  const label = value?.trim().replace(/\s+/g, ' ')
  if (!label) return undefined
  let display = label.replace(/^child agent:\s*/i, '').trim()
  if (display.endsWith(' fork')) display = display.slice(0, -' fork'.length).trim()
  if (disallowed.some((candidate) => sameSubagentDisplaySource(display, candidate))) return undefined
  const normalized = display
    .toLowerCase()
    .trim()
  if (GENERIC_SUBAGENT_LABELS.has(normalized)) return undefined
  return display.length > 48 ? `${display.slice(0, 48)}...` : display
}

function generatedSubagentDisplayLabel(agent: CoreThreadSummarySubagentJson): string {
  const seed = [
    typeof agent.parallelIndex === 'number' && agent.parallelIndex > 0
      ? String(agent.parallelIndex)
      : undefined,
    agent.childRunId,
    agent.childId,
    agent.key,
    agent.id,
    agent.childThreadId
  ].find((value): value is string => Boolean(value?.trim()))
  if (!seed) return 'Subagent'
  const ordinal = seed.match(/(\d+)(?!.*\d)/)?.[1]
  if (ordinal) {
    const value = Number.parseInt(ordinal, 10)
    if (Number.isFinite(value) && value > 0) {
      return SUBAGENT_FALLBACK_NAMES[(value - 1) % SUBAGENT_FALLBACK_NAMES.length]
    }
  }
  return SUBAGENT_FALLBACK_NAMES[hashIdenticon(seed) % SUBAGENT_FALLBACK_NAMES.length]
}

function subagentDetail(agent: CoreThreadSummarySubagentJson): string {
  const lifecycle = subagentLifecycleLabel(agent)
  const cache = typeof agent.cacheHitRate === 'number'
    ? `缓存命中 ${Math.round(agent.cacheHitRate * 100)}%`
    : ''
  const provider = agent.providerId
  const model = agent.model
  const endpointFormat = agent.endpointFormat
  const variant = agent.variant
  const modelSource = agent.modelSource
  const execution = [provider, model, variant, endpointFormat, modelSource]
    .filter(Boolean)
    .join('/')
  const tokens = typeof agent.totalTokens === 'number'
    ? `tokens ${compactNumber(agent.totalTokens)}`
    : ''
  const childThread = agent.childThreadId
    ? `${agent.canOpenThread ? '可打开' : '不可打开'} ${agent.childThreadId}`
    : ''
  return [
    lifecycle,
    cache,
    tokens,
    execution,
    childThread,
    agent.profile ?? agent.model ?? subagentLifecycleLabel(agent)
  ].filter(Boolean).join(' · ')
}

export function subagentLifecycleLabel(agent: CoreThreadSummarySubagentJson): string {
  if (agent.diagnostics?.paused) return '已暂停'
  const raw = agent.rawStatus.trim().toLowerCase()
  if (raw === 'failed') return '失败'
  if (raw === 'killed') return '已停止'
  if (raw === 'interrupted') return '已中断'
  if (raw === 'aborted' || raw === 'canceled') return '已取消'
  if (raw === 'timeout') return '超时'
  if (agent.status === 'active') return '运行中'
  if (agent.status === 'done') return '完成'
  if (agent.status === 'terminal') return '已结束'
  if (agent.status === 'inactive') return '未激活'
  return '状态未知'
}

function compactSubagentInlineMeta(agent: CoreThreadSummarySubagentJson): string | undefined {
  const provider = agent.providerId
  const model = agent.model
  const endpointFormat = agent.endpointFormat
  const variant = agent.variant
  const modelSource = agent.modelSource
  const meta = [provider, model, variant, endpointFormat, modelSource].filter(Boolean)
  return meta.length > 0 ? meta.join(' · ') : undefined
}

function IconButton({
  label,
  onClick,
  children
}: {
  label: string
  onClick: () => void
  children: ReactElement
}): ReactElement {
  return (
    <button
      type="button"
      className="rounded p-1 text-ds-muted hover:bg-ds-hover hover:text-ds-ink disabled:opacity-50"
      aria-label={label}
      title={label}
      onClick={(event) => {
        event.stopPropagation()
        onClick()
      }}
    >
      {children}
    </button>
  )
}

function compactNumber(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`
  return String(value)
}

function EmptyRows({ label }: { label: string }): ReactElement {
  return <div className="py-1 text-base text-ds-faint">{label}</div>
}

export function SubagentGlyph({
  seed,
  active
}: {
  seed: string
  active: boolean
}): ReactElement {
  const identicon = buildIdenticon(seed)
  return (
    <svg
      viewBox="-2 -1 24 24"
      className="h-5 w-5 shrink-0"
      aria-hidden="true"
      fill="none"
      shapeRendering="crispEdges"
      xmlns="http://www.w3.org/2000/svg"
    >
      {identicon.cells.map((cell) => (
        <rect
          key={`${cell.row}:${cell.column}`}
          className={active ? 'ds-agent-identicon-filled-scan' : undefined}
          x={cell.column * IDENTICON_CELL_SIZE}
          y={cell.row * IDENTICON_CELL_SIZE}
          width={IDENTICON_CELL_SIZE}
          height={IDENTICON_CELL_SIZE}
          fill={identicon.color}
          style={active ? { animationDelay: `${cell.animationDelayMs}ms` } : undefined}
        />
      ))}
      {active
        ? identicon.scanCells.map((cell) => cell.filled ? null : (
          <rect
            key={`scan:${cell.row}:${cell.column}`}
            className="ds-agent-identicon-empty-scan"
            x={cell.column * IDENTICON_CELL_SIZE}
            y={cell.row * IDENTICON_CELL_SIZE}
            width={IDENTICON_CELL_SIZE}
            height={IDENTICON_CELL_SIZE}
            fill={identicon.color}
            style={{ animationDelay: `${cell.animationDelayMs}ms` }}
          />
        ))
        : null}
    </svg>
  )
}

function buildIdenticon(seed: string): {
  cells: IdenticonCell[]
  color: string
  scanCells: IdenticonCell[]
} {
  const shapeHash = hashIdenticon(`${seed}:shape`)
  const colorHash = hashIdenticon(`${seed}:color`)
  const cells: IdenticonCell[] = []
  const filled = new Set<string>()

  for (let row = 0; row < IDENTICON_SIZE; row += 1) {
    for (let column = 0; column < IDENTICON_HALF_COLUMNS; column += 1) {
      if (!hasHashBit(shapeHash, row * IDENTICON_HALF_COLUMNS + column)) continue
      cells.push({ animationDelayMs: scanDelay(row), column, row })
      filled.add(cellKey(column, row))
      const mirroredColumn = IDENTICON_SIZE - 1 - column
      if (mirroredColumn !== column) {
        cells.push({ animationDelayMs: scanDelay(row), column: mirroredColumn, row })
        filled.add(cellKey(mirroredColumn, row))
      }
    }
  }

  if (cells.length === 0) {
    const center = Math.floor(IDENTICON_SIZE / 2)
    cells.push({ animationDelayMs: scanDelay(center), column: center, row: center })
    filled.add(cellKey(center, center))
  }

  return {
    cells,
    color: pickIdenticonColor(colorHash),
    scanCells: buildScanCells(filled)
  }
}

function buildScanCells(filled: Set<string>): IdenticonCell[] {
  const cells: IdenticonCell[] = []
  for (let row = 0; row < IDENTICON_SIZE; row += 1) {
    for (let column = 0; column < IDENTICON_SIZE; column += 1) {
      cells.push({
        animationDelayMs: scanDelay(row),
        column,
        filled: filled.has(cellKey(column, row)),
        row
      })
    }
  }
  return cells
}

function hashIdenticon(value: string): number {
  let hash = IDENTICON_HASH_OFFSET
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * IDENTICON_HASH_PRIME + value.charCodeAt(index)) % IDENTICON_HASH_MODULUS
  }
  return hash
}

function hasHashBit(hash: number, bit: number): boolean {
  return Math.floor(hash / (2 ** bit)) % 2 === 1
}

function cellKey(column: number, row: number): string {
  return `${row}:${column}`
}

function scanDelay(row: number): number {
  return row * IDENTICON_SCAN_DELAY_MS
}

function pickIdenticonColor(hash: number): string {
  const index = Math.floor((hash / IDENTICON_HASH_MODULUS) * IDENTICON_COLORS.length)
  return IDENTICON_COLORS[index % IDENTICON_COLORS.length] ?? IDENTICON_COLORS[0]
}
