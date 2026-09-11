import { Children, type ReactElement, type ReactNode } from 'react'

type Props = {
  children: ReactNode
  className?: string
}

export function AboveComposerPanelStack({ children, className = '' }: Props): ReactElement | null {
  const rows = Children.toArray(children).filter(Boolean)
  if (rows.length === 0) return null

  return (
    <div
      data-above-composer-portal
      className={`order-2 flex min-w-0 flex-col px-0 ${className}`}
    >
      {rows}
    </div>
  )
}
