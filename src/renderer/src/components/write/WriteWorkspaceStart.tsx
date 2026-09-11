import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import analytixWordmark from '../../../../asset/brand/analytix-logo-transparent.png'
import analytixWordmarkReversed from '../../../../asset/brand/analytix-logo-transparent-reversed.png'
import { AnalytixBrandMark } from '../brand/AnalytixBrandMark'

type WriteWorkspaceStartProps = {
  onAskAssistant: () => void
  onCreateDraft: () => void
  onPickWorkspace: () => void
  onRefreshWorkspace: () => void
  workspaceName: string
  workspacePathLabel: string
}

export function WriteWorkspaceStart(props: WriteWorkspaceStartProps): ReactElement {
  const { t } = useTranslation('common')

  return (
    <div
      className="ds-page-scroll-edge ds-home-transition flex h-full min-h-[420px] items-center justify-center overflow-auto px-5 py-8"
      aria-label={`${t('writeStudio')} ${props.workspaceName}`}
    >
      <section className="write-empty-report-window ds-home-transition-stage relative w-full max-w-[560px] overflow-hidden rounded-[22px] border border-slate-200/80 bg-white/92 text-left shadow-[0_28px_84px_rgba(35,64,112,0.11)] ring-1 ring-white/80 dark:border-white/10 dark:bg-[#111827]/96 dark:ring-white/5">
        <div className="write-empty-report-titlebar flex h-16 items-center gap-3 border-b border-slate-200/75 bg-white/80 px-5 dark:border-white/10 dark:bg-white/[0.035]">
          <div className="flex shrink-0 items-center gap-2" aria-hidden="true">
            <span className="h-2.5 w-2.5 rounded-full bg-[#ff6b68]" />
            <span className="h-2.5 w-2.5 rounded-full bg-[#f8c44f]" />
            <span className="h-2.5 w-2.5 rounded-full bg-[#57d487]" />
          </div>
          <span className="write-empty-report-wordmark relative ml-3 block h-6 w-[104px]" aria-hidden="true">
            <img
              className="write-empty-report-wordmark-image write-empty-report-wordmark-light"
              src={analytixWordmark}
              alt=""
              draggable={false}
              decoding="async"
            />
            <img
              className="write-empty-report-wordmark-image write-empty-report-wordmark-dark"
              src={analytixWordmarkReversed}
              alt=""
              draggable={false}
              decoding="async"
            />
          </span>
        </div>

        <div className="relative bg-[linear-gradient(180deg,rgba(248,251,255,0.86),rgba(255,255,255,0.96))] px-5 py-6 dark:bg-[linear-gradient(180deg,rgba(15,23,42,0.9),rgba(15,23,42,0.98))]">
          <article className="write-empty-a4-report relative mx-auto flex aspect-[1/1.414] w-full max-w-[390px] flex-col overflow-hidden rounded-[3px] border border-slate-200/75 bg-white px-8 py-8 shadow-[0_24px_62px_rgba(35,64,112,0.12)] dark:border-white/10 dark:bg-[#f8fbff]">
            <div className="write-empty-typing-layer absolute inset-x-8 top-[39%] space-y-2.5" aria-hidden="true">
              <span className="write-empty-type-line h-2.5 w-[76%]" />
              <span className="write-empty-type-line h-2.5 w-[90%]" />
              <span className="write-empty-type-line h-2.5 w-[84%]" />
              <span className="write-empty-type-line h-2.5 w-[64%]" />
              <span className="write-empty-type-line mt-5 h-2.5 w-[72%] is-accent" />
            </div>

            <div className="relative z-10 flex items-center justify-between gap-3 border-b border-slate-200/70 pb-4" aria-hidden="true">
              <div className="space-y-2">
                <span className="block h-2.5 w-20 rounded-full bg-slate-900/12" />
                <span className="block h-2 w-28 rounded-full bg-slate-900/[0.06]" />
              </div>
              <div className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full bg-rose-400/70" />
                <span className="h-2 w-2 rounded-full bg-amber-400/70" />
                <span className="h-2 w-2 rounded-full bg-emerald-400/70" />
              </div>
            </div>

            <div className="relative z-10 mt-8 flex flex-1 flex-col items-center text-center">
              <AnalytixBrandMark className="write-empty-report-mark" />
              <h2 className="mt-8 max-w-[13ch] text-[22px] font-semibold leading-[1.18] tracking-[0] text-[#203354] [text-wrap:balance]">
                {t('writeStartTitle')}
              </h2>
              <p className="mt-3 max-w-[25ch] text-[13.5px] leading-6 text-[#5e7194]">
                {t('writeStartSub')}
              </p>
            </div>
          </article>
        </div>
      </section>
    </div>
  )
}
