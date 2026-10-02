import { createElement, forwardRef } from 'react'
import type { LucideIcon, LucideProps } from 'lucide-react'
import {
  Bug,
  ChevronDown,
  ChevronRight,
  FolderOpen,
  Globe,
  ListChecks,
  ListTodo,
  MessageSquare,
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
import { OpenAiUiIcons } from './openai-icons/OpenAiUiIcons'
import appIcon512 from '../../../asset/brand/analytix-app-icon-512.png'
import splashImage from '../../../asset/brand/analytix-splash.png'
import symbolColor from '../../../asset/brand/analytix-symbol-color.png'
import symbolLoaderMask from '../../../asset/brand/analytix-symbol-loader-mask.png'
import symbolMonoBlack from '../../../asset/brand/analytix-symbol-mono-black.png'
import symbolMonoWhite from '../../../asset/brand/analytix-symbol-mono-white.png'
import symbolReversed from '../../../asset/brand/analytix-symbol-reversed.png'

type AnalytixSvgIcon = LucideIcon
type AnalytixRegistryIcon = LucideIcon | AnalytixSvgIcon

export type AnalytixIconSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

export const AnalytixIconSizes: Record<AnalytixIconSize, number> = {
  xs: 14,
  sm: 16,
  md: 18,
  lg: 20,
  xl: 24
}

// Unmatched controls keep the existing licensed Lucide glyphs; this is not a web-match claim.
export function createSystemIcon(icon: LucideIcon, filled = false): AnalytixSvgIcon {
  const SystemIcon = forwardRef<SVGSVGElement, LucideProps>((props, ref) => createElement(icon, {
    size: 20,
    ...props,
    ref,
    strokeWidth: 1.75,
    fill: filled ? 'currentColor' : props.fill ?? 'none'
  }))
  SystemIcon.displayName = `Analytix${icon.displayName ?? 'Icon'}`
  return SystemIcon
}

export const AnalytixDisclosureDownIcon = createSystemIcon(ChevronDown)
export const AnalytixDisclosureRightIcon = createSystemIcon(ChevronRight)
export const AnalytixSidebarHideIcon = OpenAiUiIcons.sidebar
export const AnalytixSidebarShowIcon = OpenAiUiIcons.sidebar
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
    bug: createSystemIcon(Bug),
    comment: AnalytixCommentIcon,
    disclosureDown: AnalytixDisclosureDownIcon,
    disclosureRight: AnalytixDisclosureRightIcon,
    edit: AnalytixEditIcon,
    fork: createSystemIcon(GitFork),
    idea: createSystemIcon(Lightbulb),
    panel: createSystemIcon(PanelTop),
    pin: AnalytixPinIcon,
    pinFilled: AnalytixPinFilledIcon,
    plan: AnalytixPlanIcon,
    refresh: createSystemIcon(RefreshCw),
    settings: createSystemIcon(Settings),
    sidebarHide: AnalytixSidebarHideIcon,
    sidebarShow: AnalytixSidebarShowIcon,
    sparkles: createSystemIcon(Sparkles),
    taskList: AnalytixTaskListIcon,
    terminal: AnalytixTerminalIcon,
    workspace: AnalytixWorkspaceIcon,
    write: AnalytixEditIcon,
    writeNew: createSystemIcon(FilePlus2)
  } satisfies Record<string, AnalytixRegistryIcon>,
  sizes: AnalytixIconSizes
} as const
