/* Selected ContextMeter colors adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { ContextCapacity, ContextCategoryKey } from '../../lib/context-capacity'
import { formatCompactNumber } from '../../hooks/use-thread-usage'
import css from './InputBarDock.module.css'

const CATEGORY_COLORS: Record<ContextCategoryKey, string> = {
  system: 'rgb(173, 178, 184)',
  tools: 'rgb(167, 139, 250)',
  messages: 'rgb(77, 147, 248)'
}
const CATEGORY_ORDER: ContextCategoryKey[] = ['system', 'tools', 'messages']

type Props = { capacity: ContextCapacity }

export function ContextCapacityPopover({ capacity }: Props): ReactElement {
  const { t } = useTranslation()
  const unknown = t('contextCapacityUnknown')
  const tokens = (value: number | null | undefined): string =>
    value == null ? unknown : t('contextCapacityTokens', { tokens: formatCompactNumber(value) })
  const identity = (value: { model?: string; providerId?: string } | null): string =>
    `${value?.providerId || unknown} · ${value?.model || unknown}`

  return (
    <div className="ds-context-capacity min-w-0">
      <div className={css.capacityHeader}>
        <span className="text-[13.5px] font-medium">{t('contextCapacityTitle')}</span>
      </div>
      <dl className="my-2 flex flex-col gap-2 text-[12px]">
        <div className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-2" data-context-reading="selected-window">
          <dt className="text-ds-muted">{t('contextCapacitySelectedWindow')}</dt>
          <dd className="shrink-0 tabular-nums text-ds-ink">{tokens(capacity.windowTokens)}</dd>
          <dd className="col-span-2 break-words text-[10.5px] text-ds-faint">{identity(capacity)}</dd>
        </div>
        <div className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-2" data-context-reading="last-request">
          <dt className="text-ds-muted">{t('contextCapacityLastInput')}</dt>
          <dd className="shrink-0 tabular-nums text-ds-ink">{tokens(capacity.lastRequest?.inputTokens)}</dd>
          <dd className="col-span-2 break-words text-[10.5px] text-ds-faint">{identity(capacity.lastRequest)}</dd>
        </div>
      </dl>
      <div className="flex flex-col border-t border-ds-border-muted pt-1.5">
        {CATEGORY_ORDER.map(key => (
          <div key={key} className={css.capacityRow} data-context-category={key}>
            <span className="h-[9px] w-[9px] rounded-[3px]" style={{ background: CATEGORY_COLORS[key] }} />
            <span className="truncate text-[12.5px] text-ds-muted">{t(`contextCapacityCat_${key}`)}</span>
            <span className="col-span-2 text-right text-[12px] text-ds-faint">{t('contextCapacityUnmeasured')}</span>
          </div>
        ))}
      </div>
      {capacity.lastRequest?.toolDefinitionTokens != null ? (
        <p className="mt-2 text-[10.5px] leading-snug text-ds-faint" data-context-reading="tool-estimate">
          {t('contextCapacityLastToolEstimate', { tokens: formatCompactNumber(capacity.lastRequest.toolDefinitionTokens) })}
        </p>
      ) : null}
      <p className="mt-2 text-[10.5px] leading-snug text-ds-faint">{t('contextCapacitySampleNote')}</p>
    </div>
  )
}
