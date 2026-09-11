import type { ReactElement, ReactNode } from 'react'

export type MarketplaceNotice = {
  tone: 'success' | 'error' | 'info'
  message: string
}

export function TabButton({
  active,
  onClick,
  count,
  children
}: {
  active: boolean
  tone?: 'default' | 'skill'
  onClick: () => void
  count?: number
  children: ReactNode
}): ReactElement {
  const activeClass = 'bg-black/[0.12] text-[#0d0d0d] dark:bg-ds-hover dark:text-ds-ink'

  return (
    <button
      type="button"
      onClick={onClick}
      className={`relative inline-flex h-9 items-center rounded-[14px] px-4 text-[16px] font-normal leading-5 transition ${
        active ? activeClass : 'text-[#0d0d0d] hover:bg-black/[0.1] dark:text-ds-ink dark:hover:bg-ds-hover'
      }`}
    >
      {children}
      {typeof count === 'number' ? (
        <span className="absolute -right-1 -top-1 inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-[#0d0d0d] px-1 text-[11px] font-medium leading-none text-white shadow-sm dark:bg-ds-ink dark:text-ds-main">
          {count}
        </span>
      ) : null}
    </button>
  )
}
