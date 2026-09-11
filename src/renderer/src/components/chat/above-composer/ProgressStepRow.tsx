import { Check, CheckCircle2, Circle, Loader2, X, XCircle } from 'lucide-react'
import { type ReactElement, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ThreadHandoffStepStatus } from '@shared/thread-handoff'

type Props = {
  children: ReactNode
  compact?: boolean
  status: ThreadHandoffStepStatus
}

function srStatusKey(status: ThreadHandoffStepStatus): string {
  switch (status) {
    case 'running':
      return 'progressStepStatusRunning'
    case 'done':
      return 'progressStepStatusDone'
    case 'failed':
      return 'progressStepStatusFailed'
    case 'pending':
      return 'progressStepStatusPending'
  }
}

function ProgressStepIcon({
  compact,
  status
}: {
  compact: boolean
  status: ThreadHandoffStepStatus
}): ReactElement {
  if (compact) {
    const iconClass = status === 'failed' ? 'h-3.5 w-3.5 text-ds-danger' : 'h-3.5 w-3.5'
    const icon =
      status === 'running' ? <Loader2 className={`${iconClass} animate-spin`} /> :
      status === 'done' ? <CheckCircle2 className={iconClass} /> :
      status === 'failed' ? <XCircle className={iconClass} /> :
      <Circle className={iconClass} />
    return (
      <span aria-hidden className="flex h-4 w-4 shrink-0 items-center justify-center text-ds-muted">
        {icon}
      </span>
    )
  }

  switch (status) {
    case 'running':
      return (
        <span aria-hidden className="relative h-4 w-4 shrink-0">
          <span className="absolute inset-0 animate-spin rounded-full border-2 border-transparent border-r-ds-ink border-t-ds-ink motion-reduce:animate-none" />
        </span>
      )
    case 'done':
      return (
        <span aria-hidden className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border-2 border-emerald-500/40 bg-emerald-500/15">
          <Check className="h-2.5 w-2.5 text-emerald-600 dark:text-emerald-400" strokeWidth={2.4} />
        </span>
      )
    case 'failed':
      return (
        <span aria-hidden className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border-2 border-ds-danger/40 bg-ds-danger/15">
          <X className="h-2.5 w-2.5 text-ds-danger" strokeWidth={2.4} />
        </span>
      )
    case 'pending':
      return (
        <span aria-hidden className="h-4 w-4 shrink-0 rounded-full border-2 border-current text-ds-faint" />
      )
  }
}

export function ProgressStepRow({ children, compact = false, status }: Props): ReactElement {
  const { t } = useTranslation('common')
  const textClass = compact
    ? 'text-[13px] leading-5 text-ds-muted'
    : [
        'text-base leading-6',
        status === 'running' ? 'font-medium text-ds-ink' : '',
        status === 'done' ? 'text-ds-ink' : '',
        status === 'failed' ? 'text-ds-danger' : '',
        status === 'pending' ? 'text-ds-faint' : ''
      ].filter(Boolean).join(' ')

  return (
    <div className={`flex items-center ${compact ? 'gap-2' : 'gap-3'}`}>
      <ProgressStepIcon compact={compact} status={status} />
      <div className={textClass}>
        <span className="sr-only">{t(srStatusKey(status))}</span>
        {children}
      </div>
    </div>
  )
}
