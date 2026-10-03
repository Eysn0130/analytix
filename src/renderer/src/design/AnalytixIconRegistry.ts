import { createElement, forwardRef } from 'react'
import type { LucideIcon, LucideProps } from 'lucide-react'
import { PanelTop } from 'lucide-react'
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

export const AnalytixDisclosureDownIcon = OpenAiUiIcons.chevronDown
export const AnalytixDisclosureRightIcon = OpenAiUiIcons.chevronRight
export const AnalytixSidebarHideIcon = OpenAiUiIcons.sidebar
export const AnalytixSidebarShowIcon = OpenAiUiIcons.sidebar
export const AnalytixTerminalIcon = OpenAiUiIcons.terminal
export const AnalytixBrowserIcon = OpenAiUiIcons.globe
export const AnalytixEditIcon = OpenAiUiIcons.editPencil
export const AnalytixTaskListIcon = OpenAiUiIcons.tasks
export const AnalytixPinIcon = OpenAiUiIcons.pin
export const AnalytixPinFilledIcon = OpenAiUiIcons.pinFilled
export const AnalytixPlanIcon = OpenAiUiIcons.tasks
export const AnalytixCommentIcon = OpenAiUiIcons.chat
export const AnalytixWorkspaceIcon = OpenAiUiIcons.folderOpen

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
    bug: OpenAiUiIcons.bug,
    comment: AnalytixCommentIcon,
    disclosureDown: AnalytixDisclosureDownIcon,
    disclosureRight: AnalytixDisclosureRightIcon,
    edit: AnalytixEditIcon,
    fork: OpenAiUiIcons.branchAlt,
    idea: OpenAiUiIcons.lightbulb,
    panel: createSystemIcon(PanelTop),
    pin: AnalytixPinIcon,
    pinFilled: AnalytixPinFilledIcon,
    plan: AnalytixPlanIcon,
    refresh: OpenAiUiIcons.reload,
    settings: OpenAiUiIcons.settingsCog,
    sidebarHide: AnalytixSidebarHideIcon,
    sidebarShow: AnalytixSidebarShowIcon,
    sparkles: OpenAiUiIcons.sparkles,
    taskList: AnalytixTaskListIcon,
    terminal: AnalytixTerminalIcon,
    workspace: AnalytixWorkspaceIcon,
    write: AnalytixEditIcon,
    writeNew: OpenAiUiIcons.pageBlank
  } satisfies Record<string, AnalytixRegistryIcon>,
  sizes: AnalytixIconSizes
} as const
