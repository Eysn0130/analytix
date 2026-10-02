import { createElement, type ReactElement, type SVGProps } from 'react'
import type { LucideIcon } from 'lucide-react'
import {
  Bug,
  ChevronDown,
  ChevronRight,
  FolderOpen,
  Globe,
  ListChecks,
  ListTodo,
  MessageSquare,
  PanelLeftClose,
  PanelLeftOpen,
  Pencil,
  Pin,
  SquareTerminal,
  FilePlus2,
  GitFork,
  Lightbulb,
  PanelTop,
  RefreshCw,
  Settings,
  Sparkles
} from 'lucide-react'
import appIcon512 from '../../../asset/brand/analytix-app-icon-512.png'
import splashImage from '../../../asset/brand/analytix-splash.png'
import symbolColor from '../../../asset/brand/analytix-symbol-color.png'
import symbolLoaderMask from '../../../asset/brand/analytix-symbol-loader-mask.png'
import symbolMonoBlack from '../../../asset/brand/analytix-symbol-mono-black.png'
import symbolMonoWhite from '../../../asset/brand/analytix-symbol-mono-white.png'
import symbolReversed from '../../../asset/brand/analytix-symbol-reversed.png'

type AnalytixSvgIcon = (props: SVGProps<SVGSVGElement>) => ReactElement
type AnalytixRegistryIcon = LucideIcon | AnalytixSvgIcon

export type AnalytixIconSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

export const AnalytixIconSizes: Record<AnalytixIconSize, number> = {
  xs: 14,
  sm: 16,
  md: 18,
  lg: 20,
  xl: 24
}

// Use the existing ISC/MIT Lucide dependency; no private desktop SVG paths are bundled.
function createSystemIcon(icon: LucideIcon, filled = false): AnalytixSvgIcon {
  return function AnalytixIcon(props: SVGProps<SVGSVGElement>): ReactElement {
    return createElement(icon, {
      width: 20,
      height: 20,
      strokeWidth: 1.75,
      ...props,
      fill: filled ? 'currentColor' : props.fill ?? 'none'
    })
  }
}

export const AnalytixDisclosureDownIcon = createSystemIcon(ChevronDown)
export const AnalytixDisclosureRightIcon = createSystemIcon(ChevronRight)
export const AnalytixSidebarHideIcon = createSystemIcon(PanelLeftClose)
export const AnalytixSidebarShowIcon = createSystemIcon(PanelLeftOpen)
export const AnalytixTerminalIcon = createSystemIcon(SquareTerminal)
export const AnalytixBrowserIcon = createSystemIcon(Globe)
export const AnalytixEditIcon = createSystemIcon(Pencil)
export const AnalytixTaskListIcon = createSystemIcon(ListTodo)
export const AnalytixPinIcon = createSystemIcon(Pin)
export const AnalytixPinFilledIcon = createSystemIcon(Pin, true)
export const AnalytixPlanIcon = createSystemIcon(ListChecks)
export const AnalytixCommentIcon = createSystemIcon(MessageSquare)
export const AnalytixWorkspaceIcon = createSystemIcon(FolderOpen)

export const AnalytixIconRegistry = {
  brand: {
    appIcon512,
    splashImage,
    symbolColor,
    symbolLoaderMask,
    symbolReversed,
    symbolMonoBlack,
    symbolMonoWhite
  },
  icons: {
    browser: AnalytixBrowserIcon,
    bug: Bug,
    comment: AnalytixCommentIcon,
    disclosureDown: AnalytixDisclosureDownIcon,
    disclosureRight: AnalytixDisclosureRightIcon,
    edit: AnalytixEditIcon,
    fork: GitFork,
    idea: Lightbulb,
    panel: PanelTop,
    pin: AnalytixPinIcon,
    pinFilled: AnalytixPinFilledIcon,
    plan: AnalytixPlanIcon,
    refresh: RefreshCw,
    settings: Settings,
    sidebarHide: AnalytixSidebarHideIcon,
    sidebarShow: AnalytixSidebarShowIcon,
    sparkles: Sparkles,
    taskList: AnalytixTaskListIcon,
    terminal: AnalytixTerminalIcon,
    workspace: AnalytixWorkspaceIcon,
    write: AnalytixEditIcon,
    writeNew: FilePlus2
  } satisfies Record<string, AnalytixRegistryIcon>,
  sizes: AnalytixIconSizes
} as const
