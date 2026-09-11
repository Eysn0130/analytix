import { type ReactElement, type ReactNode } from 'react'

type Props = {
  children: ReactNode
  className?: string
}

export function AboveComposerPanelRow({ children, className = '' }: Props): ReactElement {
  return (
    <div
      className={[
        'relative mx-auto w-[90%] min-w-0 overflow-clip border-x border-t border-ds-border bg-ds-card/78 text-ds-ink backdrop-blur-xl',
        'first:rounded-t-2xl',
        className
      ].filter(Boolean).join(' ')}
    >
      {children}
    </div>
  )
}
