/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolDetailItem, ToolDetailsModel } from '../models/details-card-model'
import { DshCheckOutlineRegular, DshChevronRightOutlineRegular, DshCloseOutlineRegular, DshPlayOutlineRegular } from '../../DshToolIcons'
import css from './ToolDetails.module.css'
function DetailItem({ item }: { item: ToolDetailItem }) {
  const status = item.status
  return <li className={css.item} data-change={item.change?.value}>
    {item.title !== undefined ? <div className={css.heading}>
      {status ? <span className={css.status} data-status={status.value} role="img" aria-label={item.change?.label ?? status.label}>
        {item.change?.value === 'added' ? '+' : item.change?.value === 'removed' ? '−'
          : status.value === 'completed' ? <DshCheckOutlineRegular size={14} />
            : status.value === 'in_progress' ? <DshPlayOutlineRegular size={14} />
              : status.value === 'failed' ? <DshCloseOutlineRegular size={14} />
                : status.value === 'canceled' ? '−' : <span className={css.pending} />}
      </span> : null}
      <span className={css.text}>{item.title}</span>
      {item.badge ? <span className={css.badge} data-tone={item.badge.tone}>{item.badge.label}</span> : null}
      {status ? <span className={css.statusText}>
        {item.previousStatus !== undefined ? <><span className={css.previous}>{item.previousStatus}</span><span> → </span></> : null}
        <span>{status.label}</span>
        {item.change?.value === 'updated' && item.previousStatus === undefined ? <span> · {item.change.label}</span> : null}
      </span> : null}
    </div> : null}
    {item.subtitle !== undefined ? <div className={css.subtitle}>{item.subtitle}</div> : null}
    {item.description !== undefined ? <p className={css.description}>{item.description}</p> : null}
    {item.fields.length > 0 ? <dl className={css.fields}>
      {item.fields.map((field, index) => <div className={css.field} key={index}><dt>{field.label}</dt><dd>{field.value}</dd></div>)}
    </dl> : null}
  </li>
}

/** Read-only typed presentation inside its owning public or protected leaf. */
export function ToolDetails({ model, hasInspect = false }: { model: ToolDetailsModel; hasInspect?: boolean }) {
  return <div className={css.root} data-tool-details data-inspect={hasInspect || undefined} data-caption={model.caption !== undefined || undefined}>
    {model.caption !== undefined ? <div className={css.caption}>{model.caption}</div> : null}
    {model.items.length === 0 ? <p className={css.empty}>{model.empty}</p> : <ul className={css.list}>
      {model.items.map((item, index) => <DetailItem key={item.id ?? index} item={item} />)}
    </ul>}
    {model.unchanged ? <details className={css.unchanged}>
      <summary><DshChevronRightOutlineRegular /><span>{model.unchanged.label}</span></summary>
      <ul className={css.list}>{model.unchanged.items.map((item, index) => <DetailItem key={item.id ?? index} item={item} />)}</ul>
    </details> : null}
  </div>
}
