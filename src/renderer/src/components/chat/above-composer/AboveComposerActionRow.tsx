import { type ReactElement, type ReactNode } from 'react'

type Props = {
  icon?: ReactNode
  title: ReactNode
  meta?: ReactNode
  actions?: ReactNode
  trailing?: ReactNode
  titleClassName?: string
  className?: string
}

export function AboveComposerActionRow({
  icon,
  title,
  meta,
  actions,
  trailing,
  titleClassName = '',
  className = ''
}: Props): ReactElement {
  return (
    <div className={`group flex min-w-0 items-center justify-between gap-2 py-0.5 text-sm ${className}`}>
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        {icon ? <span className="flex h-4 shrink-0 items-center justify-center">{icon}</span> : null}
        <div className={`min-w-0 flex-1 leading-4 ${titleClassName}`}>
          {title}
          {meta ? <span className="ml-1 text-ds-faint">{meta}</span> : null}
        </div>
      </div>
      {trailing || actions ? (
        <div className="flex shrink-0 items-center gap-1">
          {trailing}
          {actions}
        </div>
      ) : null}
    </div>
  )
}
