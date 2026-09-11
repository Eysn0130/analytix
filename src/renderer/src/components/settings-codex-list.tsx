import type { ReactElement, ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'

export type CodexSelectOption = {
  value: string
  label: string
}

export function CodexSettingsPage({
  title,
  subtitle,
  eyebrow,
  actions,
  children
}: {
  title: string
  subtitle?: ReactNode
  eyebrow?: ReactNode
  actions?: ReactNode
  children: ReactNode
}): ReactElement {
  return (
    <div className="w-full pb-16">
      <header className="mb-8">
        {eyebrow ? <div className="mb-6">{eyebrow}</div> : null}
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <h1 className="text-2xl font-semibold leading-tight tracking-tight text-ds-ink">
              {title}
            </h1>
            {subtitle ? (
              <p className="mt-1 text-[14px] leading-6 text-ds-muted">
                {subtitle}
              </p>
            ) : null}
          </div>
          {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2 sm:justify-end">{actions}</div> : null}
        </div>
      </header>
      <div className="space-y-6">{children}</div>
    </div>
  )
}

export function CodexSettingsSection({
  title,
  subtitle,
  action,
  children
}: {
  title: string
  subtitle?: ReactNode
  action?: ReactNode
  children: ReactNode
}): ReactElement {
  return (
    <section>
      <div className="mb-3 flex min-h-[28px] items-end justify-between gap-4">
        <div className="min-w-0">
          <h2 className="text-[16px] font-semibold leading-6 tracking-tight text-ds-ink">{title}</h2>
          {subtitle ? <p className="mt-0.5 text-[13px] leading-5 text-ds-muted">{subtitle}</p> : null}
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      {children}
    </section>
  )
}

export function CodexSettingsPanel({ children }: { children: ReactNode }): ReactElement {
  return (
    <div className="overflow-hidden rounded-2xl border border-ds-border bg-ds-card shadow-sm shadow-black/[0.03]">
      <div className="divide-y divide-ds-border-muted">{children}</div>
    </div>
  )
}

export function CodexSettingsRow({
  title,
  description,
  icon,
  control,
  className = ''
}: {
  title: ReactNode
  description?: ReactNode
  icon?: ReactNode
  control?: ReactNode
  className?: string
}): ReactElement {
  return (
    <div className={`flex min-h-[68px] items-center gap-3 px-3 py-4 sm:px-5 ${className}`}>
      {icon ? <div className="flex h-10 w-10 shrink-0 items-center justify-center">{icon}</div> : null}
      <div className="min-w-0 flex-1">
        <div className="text-[14px] font-semibold leading-5 tracking-normal text-ds-ink">{title}</div>
        {description ? <div className="mt-1 text-[13px] leading-5 text-ds-muted">{description}</div> : null}
      </div>
      {control ? <div className="ml-2 shrink-0">{control}</div> : null}
    </div>
  )
}

export function CodexSelect({
  value,
  options,
  onChange,
  widthClassName = 'w-[220px]',
  disabled = false,
  title
}: {
  value: string
  options: CodexSelectOption[]
  onChange: (value: string) => void
  widthClassName?: string
  disabled?: boolean
  title?: string
}): ReactElement {
  return (
    <label className={`relative block ${widthClassName}`} title={title}>
      <select
        className="h-10 w-full appearance-none rounded-xl border-0 bg-ds-subtle px-3.5 pr-9 text-[14px] font-semibold text-ds-ink outline-none transition hover:bg-ds-hover focus:ring-2 focus:ring-accent/25 disabled:cursor-not-allowed disabled:opacity-60"
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ds-muted" strokeWidth={2} />
    </label>
  )
}

export function CodexToolbarButton({
  children,
  onClick,
  disabled = false,
  ariaLabel,
  title,
  tone = 'default'
}: {
  children: ReactNode
  onClick?: () => void
  disabled?: boolean
  ariaLabel?: string
  title?: string
  tone?: 'default' | 'danger'
}): ReactElement {
  const toneClass = tone === 'danger'
    ? 'bg-red-500/10 text-red-600 hover:bg-red-500/15 dark:text-red-300'
    : 'bg-ds-subtle text-ds-ink hover:bg-ds-hover'
  return (
    <button
      type="button"
      aria-label={ariaLabel}
      title={title}
      disabled={disabled}
      onClick={onClick}
      className={`inline-flex h-10 items-center justify-center gap-1.5 rounded-xl px-3.5 text-[14px] font-semibold leading-none transition disabled:cursor-not-allowed disabled:opacity-55 ${toneClass}`}
    >
      {children}
    </button>
  )
}

export function CodexToggle({
  checked,
  onChange,
  label,
  disabled = false
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  disabled?: boolean
}): ReactElement {
  return (
    <button
      type="button"
      role="switch"
      aria-label={label}
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-7 w-[48px] shrink-0 items-center rounded-full transition ${
        checked ? 'bg-[#2f9bff]' : 'bg-ds-border-muted'
      } disabled:cursor-not-allowed disabled:opacity-60`}
    >
      <span
        className={`h-[22px] w-[22px] rounded-full bg-white shadow-sm transition-transform ${
          checked ? 'translate-x-[23px]' : 'translate-x-[3px]'
        }`}
      />
    </button>
  )
}

export function CodexEmptyState({ children }: { children: ReactNode }): ReactElement {
  return (
    <div className="flex min-h-[56px] items-center justify-center rounded-2xl border border-ds-border bg-ds-card px-4 text-[14px] font-semibold text-ds-muted">
      {children}
    </div>
  )
}
