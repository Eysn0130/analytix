import type { ReactElement } from 'react'
import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { SettingsCard } from './settings-controls'

type LlmDebugRound = {
  id: number
  durationMs: number
  status: 'completed' | 'failed'
  request?: { sha256: string; bytes: number }
  response?: { sha256: string; bytes: number }
  toolCallCount?: number
  usage?: Record<string, number>
  stopReason?: string
  errorCode?: 'provider_request_failed'
}

const preClass =
  'mt-1 max-h-[360px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-ds-subtle px-3 py-2 font-mono text-[11.5px] leading-5 text-ds-ink'

function pretty(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function RoundCard({ round, t }: { round: LlmDebugRound; t: (key: string) => string }): ReactElement {
  const [open, setOpen] = useState(false)
  const failed = round.status === 'failed'
  const status = round.errorCode ? `⚠ ${round.errorCode}` : (round.stopReason ?? round.status)
  return (
    <div className="rounded-xl border border-ds-border bg-ds-card">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-3 px-3 py-2.5 text-left"
      >
        <span className="font-mono text-[12px] text-ds-faint">#{round.id}</span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium text-ds-ink">LLM round</span>
          <span className="block truncate text-[11.5px] text-ds-faint">
            {round.durationMs}ms{status ? ` · ${status}` : ''}
          </span>
        </span>
        <span className={`shrink-0 text-[11px] ${failed ? 'text-ds-danger' : 'text-ds-muted'}`}>
          {open ? '▲' : '▼'}
        </span>
      </button>
      {open ? (
        <div className="border-t border-ds-border px-3 py-3">
          <div className="text-[12px] text-ds-faint">Provider identity withheld at the public boundary.</div>

          <div className="mt-3">
            <div className="text-[12px] font-semibold text-ds-muted">{t('llmDebugRequest')}</div>
            <pre className={preClass}>{round.request ? pretty(round.request) : '—'}</pre>
          </div>

          <div className="mt-3">
            <div className="text-[12px] font-semibold text-ds-muted">{t('llmDebugOutput')}</div>
            {round.response ? <pre className={preClass}>{pretty(round.response)}</pre> : null}
            {(round.toolCallCount ?? 0) > 0 ? (
              <>
                <div className="mt-2 text-[11.5px] text-ds-faint">{t('llmDebugToolCallCount')}</div>
                <pre className={preClass}>{round.toolCallCount}</pre>
              </>
            ) : null}
            {round.usage ? (
              <>
                <div className="mt-2 text-[11.5px] text-ds-faint">{t('llmDebugUsage')}</div>
                <pre className={preClass}>{pretty(round.usage)}</pre>
              </>
            ) : null}
            {round.errorCode ? <pre className={`${preClass} text-ds-danger`}>{round.errorCode}</pre> : null}
            {!round.response && !round.usage && !round.errorCode ? (
              <pre className={preClass}>—</pre>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  )
}

export function LlmDebugSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const { t } = ctx as { t: (key: string) => string }
  const [rounds, setRounds] = useState<LlmDebugRound[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (): Promise<void> => {
    setLoading(true)
    setError(null)
    try {
      const result = await rendererRuntimeClient.runtimeRequest('/v1/debug/llm-rounds', 'GET')
      if (!result.ok) {
        setError(`HTTP ${result.status}`)
        return
      }
      const parsed = JSON.parse(result.body) as { rounds?: LlmDebugRound[] }
      setRounds(parsed.rounds ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <SettingsCard title={t('sectionLlmDebug')}>
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-[12.5px] leading-5 text-ds-faint">{t('llmDebugDesc')}</p>
          <button
            type="button"
            disabled={loading}
            onClick={() => void load()}
            className="inline-flex shrink-0 items-center gap-2 rounded-full bg-accent/12 px-4 py-2 text-[12.5px] font-medium text-accent transition hover:bg-accent/18 disabled:opacity-60"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} strokeWidth={1.8} />
            {t('refresh')}
          </button>
        </div>

        {error ? <p className="text-[12px] text-ds-danger">{error}</p> : null}

        {rounds.length === 0 ? (
          <p className="rounded-xl bg-ds-subtle px-3 py-6 text-center text-[12.5px] text-ds-faint">
            {t('llmDebugEmpty')}
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            {rounds.map((round) => (
              <RoundCard key={round.id} round={round} t={t} />
            ))}
          </div>
        )}
      </div>
    </SettingsCard>
  )
}
