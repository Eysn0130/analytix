import type { ReactElement } from 'react'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'

export function AnalytixBrandMark({
  className = '',
  imageClassName = ''
}: {
  className?: string
  imageClassName?: string
}): ReactElement {
  return (
    <span className={['ax-brand-mark', className].filter(Boolean).join(' ')} aria-hidden="true">
      <img
        className={['ax-brand-mark-image ax-brand-mark-image-light', imageClassName].filter(Boolean).join(' ')}
        src={AnalytixIconRegistry.brand.symbolColor}
        alt=""
        draggable={false}
        decoding="async"
      />
      <img
        className={['ax-brand-mark-image ax-brand-mark-image-dark', imageClassName].filter(Boolean).join(' ')}
        src={AnalytixIconRegistry.brand.symbolReversed}
        alt=""
        draggable={false}
        decoding="async"
      />
    </span>
  )
}
