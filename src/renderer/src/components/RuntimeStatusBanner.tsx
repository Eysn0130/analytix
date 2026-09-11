import type { ReactElement } from 'react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, Loader2, X } from 'lucide-react'
import { useChatStore } from '../store/chat-store'

/**
 * Slim banner for transient runtime supervisor states (auto-restart in
 * progress, crash recovery, settings rollback). Terminal failures are
 * routed into the main error banner instead, which carries the full
 * diagnostics UI.
 */
export function RuntimeStatusBanner(): ReactElement | null {
  const { t } = useTranslation('common')
  const status = useChatStore((s) => s.runtimeStatus)
  const [dismissedAt, setDismissedAt] = useState<string | null>(null)
  if (!status) return null
  const recoveredWithRollback = status.state === 'running' && status.rolledBack === true
  const transient = status.state === 'restarting' || status.state === 'crashed'
  if (!transient && !recoveredWithRollback) return null
  if (dismissedAt === status.at) return null
  const label = recoveredWithRollback
    ? t('runtimeStatusRolledBack')
    : status.state === 'restarting'
      ? typeof status.attempt === 'number'
        ? t('runtimeStatusRestartingAttempt', {
            attempt: status.attempt,
            max: status.maxAttempts ?? 3
          })
        : t('runtimeStatusRestarting')
      : t('runtimeStatusCrashed')
  return (
    <div className="ds-no-drag shrink-0 border-b border-amber-200/70 bg-[rgba(255,248,235,0.82)] backdrop-blur-lg dark:border-amber-800/50 dark:bg-amber-950/35">
      <div className="flex w-full min-w-0 items-center gap-2 px-4 py-1.5">
        {recoveredWithRollback ? (
          <AlertTriangle
            className="h-3.5 w-3.5 shrink-0 text-amber-700 dark:text-amber-300"
            strokeWidth={2}
          />
        ) : (
          <Loader2
            className="h-3.5 w-3.5 shrink-0 animate-spin text-amber-700 dark:text-amber-300"
            strokeWidth={2}
          />
        )}
        <p
          className="min-w-0 flex-1 truncate text-[12.5px] leading-5 text-amber-950 dark:text-amber-100"
          title={status.message ?? label}
        >
          {label}
        </p>
        {recoveredWithRollback ? (
          <button
            type="button"
            aria-label={t('runtimeStatusDismiss')}
            className="inline-flex shrink-0 items-center rounded-md p-1 text-amber-900/70 transition hover:bg-amber-100/70 dark:text-amber-100/80 dark:hover:bg-amber-900/40"
            onClick={() => setDismissedAt(status.at)}
          >
            <X className="h-3.5 w-3.5" strokeWidth={2} />
          </button>
        ) : null}
      </div>
    </div>
  )
}
