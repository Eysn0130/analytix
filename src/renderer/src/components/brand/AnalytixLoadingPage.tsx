import type { CSSProperties, ReactElement } from 'react'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'

type LoadingLogoStyle = CSSProperties & {
  '--ax-loading-logo-mask': string
}

const surfaceClassBySurface = {
  main: 'bg-ds-main',
  sidebar: 'bg-ds-sidebar',
  surface: 'ds-surface-strong',
  transparent: 'bg-transparent'
} as const

export function AnalytixLoadingLogo({
  className = '',
  size = 50
}: {
  className?: string
  size?: number
}): ReactElement {
  const style: LoadingLogoStyle = {
    '--ax-loading-logo-mask': `url(${AnalytixIconRegistry.brand.symbolLoaderMask})`,
    width: size,
    height: size
  }

  return (
    <span
      aria-hidden="true"
      className={['ax-loading-logo', className].filter(Boolean).join(' ')}
      style={style}
    >
      <span className="ax-loading-logo__base" />
      <span className="ax-loading-logo__overlay" />
    </span>
  )
}

export function AnalytixLoadingPage({
  className = '',
  dragSafeArea,
  fillParent = false,
  label = 'Loading Analytix',
  overlay = false,
  showLogo = true,
  size = 50,
  surface = 'main'
}: {
  className?: string
  dragSafeArea?: boolean
  fillParent?: boolean
  label?: string
  overlay?: boolean
  showLogo?: boolean
  size?: number
  surface?: 'main' | 'sidebar' | 'surface' | 'transparent'
}): ReactElement {
  const showDragSafeArea = dragSafeArea ?? (!overlay && !fillParent)
  const placementClass = overlay
    ? 'absolute inset-0 z-10'
    : fillParent
      ? 'h-full min-h-0 w-full'
      : 'h-full min-h-0 w-full'
  const surfaceClass = overlay ? 'ax-loading-page--overlay-surface' : surfaceClassBySurface[surface]

  return (
    <div
      role="status"
      aria-label={label}
      aria-live="polite"
      className={['ax-loading-page', placementClass, surfaceClass, className].filter(Boolean).join(' ')}
    >
      {showDragSafeArea ? <div aria-hidden className="ax-loading-page__drag-safe-area" /> : null}
      {showLogo ? <AnalytixLoadingLogo size={size} /> : null}
    </div>
  )
}
