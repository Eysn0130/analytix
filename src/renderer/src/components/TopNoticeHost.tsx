import { useEffect, type ReactElement, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { TopNotice } from '../store/chat-store-types'
import { useChatStore } from '../store/chat-store'

const DEFAULT_TOP_NOTICE_DURATION_MS = 5_000

const toneStyles = {
  info: {
    dot: 'bg-accent'
  },
  success: {
    dot: 'bg-emerald-500'
  },
  error: {
    dot: 'bg-red-500'
  }
} as const

export function TopNoticeViewport({ children }: { children: ReactNode }): ReactElement {
  return (
    <span className="pointer-events-none fixed inset-0 z-[60] mx-auto my-2 flex max-w-[560px] flex-col items-center justify-start md:pb-5">
      {children}
    </span>
  )
}

export function TopNoticeCard({
  notice,
  confirmLabel,
  onDismiss
}: {
  notice: TopNotice
  confirmLabel: string
  onDismiss: () => void
}): ReactElement {
  const tone = toneStyles[notice.tone]

  return (
    <div className="no-drag ds-no-drag pointer-events-auto w-full p-1 text-center md:w-auto md:text-justify">
      <div
        role={notice.tone === 'error' ? 'alert' : 'status'}
        aria-live={notice.tone === 'error' ? 'assertive' : 'polite'}
        data-testid="top-notice"
        data-tone={notice.tone}
        className="pointer-events-auto inline-flex max-w-full items-center gap-6 rounded-full border border-ds-border bg-ds-card/95 py-2 pl-4 pr-2 text-ds-ink shadow-[0px_8px_16px_-4px_rgba(0,0,0,0.12)] ring-[0.5px] ring-ds-border backdrop-blur-sm"
      >
        <span className={`h-2 w-2 shrink-0 rounded-full ${tone.dot}`} aria-hidden />
        <span className="min-w-0 flex-1 truncate text-sm font-normal leading-5">
          {notice.message}
        </span>
        <button
          type="button"
          onClick={onDismiss}
          className="inline-flex h-10 min-w-[64px] shrink-0 items-center justify-center rounded-full bg-[#0d0d0d] px-4 text-sm font-semibold leading-5 text-white transition hover:bg-black/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/35 dark:bg-ds-ink dark:text-ds-main dark:hover:bg-ds-ink/85"
        >
          {confirmLabel}
        </button>
      </div>
    </div>
  )
}

export function TopNoticeHost(): ReactElement | null {
  const { t } = useTranslation('common')
  const notice = useChatStore((s) => s.topNotice)
  const dismissTopNotice = useChatStore((s) => s.dismissTopNotice)

  useEffect(() => {
    if (!notice) return
    const duration = notice.durationMs ?? DEFAULT_TOP_NOTICE_DURATION_MS
    if (duration <= 0) return
    const timer = window.setTimeout(() => dismissTopNotice(notice.id), duration)
    return () => window.clearTimeout(timer)
  }, [dismissTopNotice, notice])

  if (!notice) return null

  return (
    <TopNoticeViewport>
      <TopNoticeCard
        notice={notice}
        confirmLabel={t('topNoticeConfirm')}
        onDismiss={() => dismissTopNotice(notice.id)}
      />
    </TopNoticeViewport>
  )
}
