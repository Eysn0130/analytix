import { createElement, forwardRef, type ComponentType, type SVGProps } from 'react'
import type { LucideProps } from 'lucide-react'
import Sidebar from './svg/Sidebar'
import ComposeEditSquare from './svg/ComposeEditSquare'
import Search from './svg/Search'
import PlusComposer from './svg/PlusComposer'
import MicLgDictate from './svg/MicLgDictate'

// Public MIT candidates visually compared with ChatGPT, not verified web source identity.
// Preserve upstream filled-outline geometry; legacy Lucide stroke props must not repaint it.
function adaptOfficialIcon(Source: ComponentType<SVGProps<SVGSVGElement>>, name: string) {
  const Icon = forwardRef<SVGSVGElement, LucideProps>((props, ref) => {
    const svgProps = { ...props }
    delete svgProps.size
    delete svgProps.strokeWidth
    delete svgProps.absoluteStrokeWidth
    delete svgProps.fill
    delete svgProps.stroke
    return createElement(Source, {
      ...svgProps, ref,
      width: props.width ?? props.size ?? 20,
      height: props.height ?? props.size ?? 20,
      fill: 'currentColor', stroke: 'none'
    })
  })
  Icon.displayName = `AnalytixOfficial${name}`
  return Icon
}

export const OpenAiUiIcons = {
  sidebar: adaptOfficialIcon(Sidebar, 'Sidebar'),
  newChat: adaptOfficialIcon(ComposeEditSquare, 'NewChat'),
  search: adaptOfficialIcon(Search, 'Search'),
  plus: adaptOfficialIcon(PlusComposer, 'Plus'),
  mic: adaptOfficialIcon(MicLgDictate, 'Mic')
} as const
