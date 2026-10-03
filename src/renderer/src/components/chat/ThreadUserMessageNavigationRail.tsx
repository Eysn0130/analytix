import { Globe, File, FileText, Image, Code } from "../../design/AnalytixUiIcons";
import type { ReactElement, RefObject, SVGProps } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type {
  ThreadUserMessageNavigationContextIcon,
  ThreadUserMessageNavigationItem
} from '../../thread/projection/thread-row-model'

const MIN_NAVIGATION_ITEMS = 4
const TOOLTIP_OPEN_DELAY_MS = 1200
const TOOLTIP_EXIT_MS = 160
const RAIL_PORTAL_TARGET_SELECTOR = '.ds-chat-stage'

type SelectOptions = {
  behavior?: ScrollBehavior
}

type Props = {
  items: ThreadUserMessageNavigationItem[]
  containerRef: RefObject<HTMLDivElement | null>
  railLabel: string
  noContentLabel: string
  itemAriaLabel: (item: ThreadUserMessageNavigationItem) => string
  onSelectItem: (item: ThreadUserMessageNavigationItem, options?: SelectOptions) => void
}

type PointerScrubState = {
  itemId: string
  pointerId: number
  pointerCaptureTarget: HTMLElement
}

type NavigationContextIconProps = SVGProps<SVGSVGElement>

function TypescriptContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M4.77431 2H11.2257C11.5771 1.99999 11.8803 1.99998 12.13 2.02038C12.3936 2.04192 12.6557 2.08946 12.908 2.21799C13.2843 2.40974 13.5903 2.7157 13.782 3.09202C13.9105 3.34427 13.9581 3.60642 13.9796 3.86998C14 4.11969 14 4.42287 14 4.77429V11.2257C14 11.5771 14 11.8803 13.9796 12.13C13.9581 12.3936 13.9105 12.6557 13.782 12.908C13.5903 13.2843 13.2843 13.5903 12.908 13.782C12.6557 13.9105 12.3936 13.9581 12.13 13.9796C11.8803 14 11.5771 14 11.2257 14H4.77429C4.42287 14 4.11969 14 3.86998 13.9796C3.60642 13.9581 3.34427 13.9105 3.09202 13.782C2.7157 13.5903 2.40974 13.2843 2.21799 12.908C2.08946 12.6557 2.04192 12.3936 2.02038 12.13C1.99998 11.8803 1.99999 11.5771 2 11.2257V4.77428C1.99999 4.42287 1.99998 4.11969 2.02038 3.86998C2.04192 3.60642 2.08946 3.34427 2.21799 3.09202C2.40974 2.7157 2.7157 2.40974 3.09202 2.21799C3.34427 2.08946 3.60642 2.04192 3.86998 2.02038C4.11969 1.99998 4.4229 1.99999 4.77431 2ZM7.68989 8.55H9.00389V7.692H5.36789V8.55H6.68789V12H7.68989V8.55ZM9.66459 10.578L9.04659 11.268C9.40059 11.76 10.1806 12.066 10.8946 12.066C11.8486 12.066 12.6346 11.544 12.6346 10.644C12.6346 9.70036 11.8023 9.52362 11.126 9.37999L11.1166 9.378L11.0955 9.37343C10.5571 9.25643 10.2706 9.19417 10.2706 8.898C10.2706 8.622 10.5346 8.454 10.9126 8.454C11.3686 8.454 11.7226 8.67 11.9866 9.006L12.5926 8.346C12.2746 7.944 11.6866 7.626 10.9426 7.626C10.0246 7.626 9.2926 8.154 9.2926 9C9.2926 9.864 9.9886 10.074 10.6246 10.212C10.679 10.224 10.7314 10.2353 10.7817 10.2462C11.3369 10.366 11.6446 10.4324 11.6446 10.746C11.6446 11.07 11.3506 11.238 10.9366 11.238C10.4746 11.238 10.0006 11.01 9.66459 10.578Z"
        fill="currentColor"
      />
    </svg>
  )
}

function JavascriptContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M4.77431 2H11.2257C11.5771 1.99999 11.8803 1.99998 12.13 2.02038C12.3936 2.04192 12.6557 2.08946 12.908 2.21799C13.2843 2.40974 13.5903 2.7157 13.782 3.09202C13.9105 3.34427 13.9581 3.60642 13.9796 3.86998C14 4.11969 14 4.42287 14 4.77429V11.2257C14 11.5771 14 11.8803 13.9796 12.13C13.9581 12.3936 13.9105 12.6557 13.782 12.908C13.5903 13.2843 13.2843 13.5903 12.908 13.782C12.6557 13.9105 12.3936 13.9581 12.13 13.9796C11.8803 14 11.5771 14 11.2257 14H4.77429C4.42287 14 4.11969 14 3.86998 13.9796C3.60642 13.9581 3.34427 13.9105 3.09202 13.782C2.7157 13.5903 2.40974 13.2843 2.21799 12.908C2.08946 12.6557 2.04192 12.3936 2.02038 12.13C1.99998 11.8803 1.99999 11.5771 2 11.2257V4.77428C1.99999 4.42287 1.99998 4.11969 2.02038 3.86998C2.04192 3.60642 2.08946 3.34427 2.21799 3.09202C2.40974 2.7157 2.7157 2.40974 3.09202 2.21799C3.34427 2.08946 3.60642 2.04192 3.86998 2.02038C4.11969 1.99998 4.4229 1.99999 4.77431 2ZM6.37311 11.13V11.994C6.54111 12.024 6.73311 12.036 6.93111 12.036C7.84311 12.036 8.41911 11.64 8.41911 10.734V7.692H7.41711V10.584C7.41711 11.016 7.22511 11.166 6.80511 11.166C6.64911 11.166 6.54111 11.154 6.37311 11.13ZM9.36805 10.578L8.75005 11.268C9.10405 11.76 9.88405 12.066 10.598 12.066C11.552 12.066 12.338 11.544 12.338 10.644C12.338 9.70036 11.5058 9.52362 10.8294 9.37999L10.82 9.378L10.799 9.37343C10.2606 9.25643 9.97405 9.19417 9.97405 8.898C9.97405 8.622 10.238 8.454 10.616 8.454C11.072 8.454 11.426 8.67 11.69 9.006L12.296 8.346C11.978 7.944 11.39 7.626 10.646 7.626C9.72805 7.626 8.99605 8.154 8.99605 9C8.99605 9.864 9.69205 10.074 10.328 10.212C10.3824 10.224 10.4348 10.2353 10.4852 10.2462C11.0404 10.366 11.348 10.4324 11.348 10.746C11.348 11.07 11.054 11.238 10.64 11.238C10.178 11.238 9.70405 11.01 9.36805 10.578Z"
        fill="currentColor"
      />
    </svg>
  )
}

function ReactContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        d="M8.02348 6.71097C7.31803 6.71097 6.74614 7.28285 6.74614 7.98831C6.74614 8.69377 7.31803 9.26565 8.02348 9.26565C8.72894 9.26565 9.30083 8.69377 9.30083 7.98831C9.30083 7.28285 8.72894 6.71097 8.02348 6.71097Z"
        fill="currentColor"
      />
      <path
        d="M14.667 7.99987C14.667 6.99705 13.7384 6.11753 12.2598 5.53872C12.2796 5.40836 12.2974 5.27942 12.3101 5.15284C12.456 3.71191 12.1088 2.67192 11.333 2.22393C10.4651 1.72275 9.23913 2.08651 7.99953 3.0785C6.75993 2.08651 5.53397 1.72275 4.66602 2.22393C3.89027 2.67192 3.54308 3.71191 3.68892 5.15284C3.70162 5.27942 3.71903 5.40884 3.73926 5.53966C3.616 5.58672 3.4951 5.6366 3.3789 5.68883C2.05886 6.28224 1.33203 7.10388 1.33203 7.99987C1.33203 9.00268 2.26067 9.8822 3.73926 10.461C3.71903 10.5914 3.70162 10.7203 3.68892 10.8469C3.54308 12.2878 3.89027 13.3278 4.66602 13.7758C4.93273 13.9271 5.23495 14.0044 5.5415 13.9998C6.27632 13.9998 7.1344 13.613 7.99953 12.9212C8.86419 13.613 9.72274 13.9998 10.4585 13.9998C10.765 14.0043 11.0672 13.927 11.334 13.7758C12.1097 13.3278 12.4569 12.2878 12.3111 10.8469C12.2984 10.7203 12.2805 10.5914 12.2607 10.461C13.7393 9.88314 14.668 9.00221 14.668 7.99987M10.4529 2.63522C10.65 2.63046 10.8448 2.67821 11.0174 2.77357C11.556 3.08462 11.7978 3.92838 11.6802 5.08837C11.6722 5.16743 11.6628 5.24742 11.6515 5.32789C11.0367 5.14269 10.4072 5.0106 9.76978 4.93307C9.3834 4.41931 8.95373 3.9396 8.48549 3.49921C9.22078 2.93074 9.91138 2.63522 10.4529 2.63522ZM10.1998 9.27044C9.96183 9.68353 9.70275 10.0841 9.42354 10.4704C8.95027 10.5191 8.47482 10.5433 7.99906 10.5429C7.52346 10.5432 7.04816 10.519 6.57505 10.4704C6.29658 10.084 6.03828 9.68352 5.80118 9.27044C5.56326 8.8585 5.34704 8.4344 5.15339 7.99987C5.34704 7.56534 5.56326 7.14124 5.80118 6.72929C6.03791 6.31788 6.29541 5.91878 6.5727 5.53354C7.04667 5.48371 7.52294 5.45889 7.99953 5.45918C8.47513 5.4589 8.95043 5.48309 9.42354 5.53166C9.70177 5.91749 9.96022 6.31721 10.1979 6.72929C10.4356 7.14134 10.6518 7.56543 10.8457 7.99987C10.6518 8.4343 10.4356 8.8584 10.1979 9.27044M11.1693 8.79986C11.3186 9.20711 11.4412 9.62368 11.5363 10.0469C11.1237 10.1756 10.7032 10.2769 10.2774 10.3504C10.4392 10.1064 10.596 9.8524 10.7478 9.58856C10.8979 9.32786 11.0386 9.0648 11.1712 8.80174M7.10617 11.1495C7.39878 11.1674 7.69751 11.1777 8 11.1777C8.30249 11.1777 8.6031 11.1674 8.89618 11.1495C8.6183 11.4824 8.31893 11.7968 8 12.0907C7.68176 11.7969 7.38317 11.4825 7.10617 11.1495ZM5.72215 10.3495C5.29619 10.2763 4.87549 10.1753 4.46279 10.0469C4.55747 9.62435 4.67957 9.20842 4.82832 8.80174C4.9591 9.0648 5.09929 9.32786 5.25171 9.58856C5.40413 9.84926 5.56173 10.1062 5.72215 10.3504M4.82832 7.19752C4.6801 6.79236 4.55832 6.378 4.46373 5.95706C4.87541 5.82857 5.29498 5.72687 5.71979 5.6526C5.5589 5.89589 5.40131 6.14812 5.24936 6.41118C5.09741 6.67423 4.95863 6.934 4.82596 7.19752M8.89336 4.84978C8.60075 4.8319 8.30202 4.82155 7.99718 4.82155C7.69484 4.82155 7.3969 4.83096 7.10335 4.84978C7.38035 4.5168 7.67894 4.2024 7.99718 3.90861C8.31623 4.20232 8.6156 4.51673 8.89336 4.84978ZM10.7469 6.41118C10.5945 6.14702 10.4369 5.89291 10.2741 5.64883C10.701 5.72232 11.1227 5.82387 11.5363 5.95283C11.4414 6.37533 11.3193 6.79125 11.1707 7.19799C11.04 6.93494 10.8988 6.67141 10.7469 6.41118ZM4.32072 5.08884C4.20169 3.92932 4.44491 3.08509 4.98309 2.77404C5.15574 2.67883 5.35052 2.63109 5.54761 2.63569C6.08909 2.63569 6.77922 2.93121 7.51451 3.49968C7.04596 3.94039 6.61598 4.42041 6.22928 4.93449C5.59207 5.01238 4.96258 5.14398 4.34753 5.32789C4.33671 5.24742 4.32683 5.1679 4.3193 5.08884M3.63952 6.26812C3.71197 6.23675 3.78583 6.20538 3.8611 6.174C4.00908 6.79849 4.20973 7.4093 4.46091 7.99987C4.20925 8.59159 4.00844 9.20368 3.86063 9.8295C2.66196 9.33774 1.96712 8.65633 1.96712 7.99987C1.96712 7.37776 2.57869 6.7467 3.63952 6.26812ZM4.98309 13.2257C4.44491 12.9146 4.20169 12.0704 4.32072 10.9109C4.32824 10.8318 4.33812 10.7523 4.34894 10.6714C4.96376 10.8565 5.59331 10.9886 6.23069 11.0662C6.61719 11.5801 7.04685 12.0602 7.51498 12.501C6.4899 13.293 5.55326 13.5542 4.9845 13.2257M11.6798 10.9109C11.7974 12.0709 11.5556 12.9146 11.0169 13.2257C10.4486 13.5551 9.51151 13.293 8.4869 12.501C8.95488 12.0601 9.38438 11.5801 9.77072 11.0662C10.4081 10.9887 11.0377 10.8566 11.6525 10.6714C11.6638 10.7523 11.6732 10.8318 11.6812 10.9109M12.1413 9.82856C11.993 9.20311 11.7921 8.59135 11.5405 7.99987C11.7919 7.40806 11.9928 6.79599 12.1408 6.17024C13.3371 6.662 14.0338 7.3434 14.0338 7.99987C14.0338 8.65633 13.339 9.33774 12.1403 9.8295"
        fill="currentColor"
      />
    </svg>
  )
}

function GlobeContextIcon(props: NavigationContextIconProps): ReactElement {
  return <Globe width={20} height={20} {...props} />
}

function FileContextIcon(props: NavigationContextIconProps): ReactElement {
  return <File width={10} height={10} {...props} />
}

function DocumentContextIcon(props: NavigationContextIconProps): ReactElement {
  return <FileText width={10} height={10} {...props} />
}

function ImageContextIcon(props: NavigationContextIconProps): ReactElement {
  return <Image width={20} height={21} {...props} />
}

function CodeContextIcon(props: NavigationContextIconProps): ReactElement {
  return <Code width={21} height={21} {...props} />
}

function CssContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path d="M5.99984 2.3335L4.6665 13.6668" stroke="currentColor" strokeWidth="1.33333" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M11.3333 2.3335L10 13.6668" stroke="currentColor" strokeWidth="1.33333" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.6665 5.3335H13.3332" stroke="currentColor" strokeWidth="1.33333" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.6665 10.6665H13.3332" stroke="currentColor" strokeWidth="1.33333" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function JsonContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        d="M3.53574 12.0004V10.0004C3.53574 9.7426 3.40119 9.50225 3.17715 9.35195L3.07559 9.29258C2.69921 9.10638 2.13588 8.6867 2.13574 8.00039C2.13574 7.31387 2.69917 6.89361 3.07559 6.70742L3.17715 6.64883C3.40126 6.49859 3.53565 6.25818 3.53574 6.00039V4.00039C3.53574 2.94641 4.42592 2.13477 5.46777 2.13477C5.76159 2.13477 5.9998 2.37299 5.9998 2.6668C5.9998 2.96061 5.76159 3.19883 5.46777 3.19883C4.96403 3.19883 4.5998 3.58237 4.5998 4.00039V6.00039C4.59971 6.68944 4.21449 7.27922 3.66074 7.60039L3.54746 7.66133C3.42742 7.7207 3.32888 7.79335 3.26699 7.86445C3.20767 7.93263 3.1998 7.97695 3.1998 8.00039C3.19986 8.02387 3.20801 8.06771 3.26699 8.13555C3.32889 8.20666 3.42739 8.28008 3.54746 8.33945L3.66074 8.39961C4.21462 8.72081 4.5998 9.31119 4.5998 10.0004V12.0004C4.59994 12.3921 4.91991 12.7536 5.3748 12.7973L5.46777 12.8012L5.5748 12.8121C5.81725 12.8616 5.99968 13.0762 5.9998 13.3332C5.9998 13.5903 5.81728 13.8047 5.5748 13.8543L5.46777 13.8652L5.27402 13.8559C4.31802 13.7627 3.53588 12.9882 3.53574 12.0004ZM11.3997 12.0004V10.0004C11.3997 9.26522 11.8383 8.64304 12.4521 8.33945L12.538 8.29258C12.6192 8.24368 12.6861 8.18888 12.7326 8.13555C12.7915 8.06771 12.7997 8.02388 12.7997 8.00039C12.7997 7.97695 12.7919 7.93262 12.7326 7.86445C12.6862 7.81117 12.6191 7.75707 12.538 7.7082L12.4521 7.66133C11.8383 7.35781 11.3998 6.73551 11.3997 6.00039V4.00039C11.3997 3.60856 11.0797 3.24726 10.6247 3.20352L10.5318 3.19883L10.4247 3.18789C10.1822 3.13834 9.99974 2.92394 9.99974 2.6668C9.99974 2.37298 10.238 2.13477 10.5318 2.13477L10.7255 2.14414C11.6816 2.2373 12.4638 3.01241 12.4638 4.00039V6.00039C12.4639 6.29497 12.6397 6.56684 12.924 6.70742L13.0724 6.78789C13.4324 7.00237 13.8638 7.39958 13.8638 8.00039C13.8637 8.60099 13.4324 8.99765 13.0724 9.21211L12.924 9.29258C12.6396 9.43321 12.4638 9.70571 12.4638 10.0004V12.0004C12.4637 13.0542 11.5735 13.8652 10.5318 13.8652C10.238 13.8652 9.99974 13.627 9.99974 13.3332C9.99988 13.0395 10.238 12.8012 10.5318 12.8012C11.0354 12.8012 11.3996 12.4183 11.3997 12.0004Z"
        fill="currentColor"
      />
    </svg>
  )
}

function NavigationContextIcon({ icon }: { icon: ThreadUserMessageNavigationContextIcon }): ReactElement {
  const className = 'timeline-user-message-navigation-tooltip-chip-icon'
  switch (icon) {
    case 'typescript':
      return <TypescriptContextIcon className={className} aria-hidden="true" />
    case 'javascript':
      return <JavascriptContextIcon className={className} aria-hidden="true" />
    case 'react':
      return <ReactContextIcon className={className} aria-hidden="true" />
    case 'globe':
      return <GlobeContextIcon className={className} aria-hidden="true" />
    case 'code':
      return <CodeContextIcon className={className} aria-hidden="true" />
    case 'css':
      return <CssContextIcon className={className} aria-hidden="true" />
    case 'json':
      return <JsonContextIcon className={className} aria-hidden="true" />
    case 'document':
      return <DocumentContextIcon className={className} aria-hidden="true" />
    case 'image':
      return <ImageContextIcon className={className} aria-hidden="true" />
    case 'file':
      return <FileContextIcon className={className} aria-hidden="true" />
  }
}

function navigationItemTone(item: ThreadUserMessageNavigationItem): string {
  if (item.durationMs !== undefined && item.durationMs < 60_000) return 'is-short'
  if (item.durationMs !== undefined && item.durationMs > 600_000) return 'is-long'
  return 'is-medium'
}

function navigationItemProximityTone(index: number, previewIndex: number): string {
  if (previewIndex < 0) return ''
  const distance = Math.abs(index - previewIndex)
  if (distance === 1) return 'is-near-1'
  if (distance === 2) return 'is-near-2'
  return ''
}

function activeItemInitialSet(items: ThreadUserMessageNavigationItem[]): Set<string> {
  const lastItemId = items.at(-1)?.id
  return new Set(lastItemId ? [lastItemId] : [])
}

function useActiveNavigationItemIds(
  containerRef: RefObject<HTMLDivElement | null>,
  items: ThreadUserMessageNavigationItem[]
): Set<string> {
  const [activeItemIds, setActiveItemIds] = useState<Set<string>>(() => activeItemInitialSet(items))
  const itemSignature = useMemo(() => items.map((item) => item.id).join('\u0000'), [items])

  useEffect(() => {
    const root = containerRef.current
    if (!root || items.length < MIN_NAVIGATION_ITEMS || typeof IntersectionObserver === 'undefined') {
      setActiveItemIds(activeItemInitialSet(items))
      return
    }

    const orderedItemIds = items.map((item) => item.id)
    const itemIds = new Set(orderedItemIds)
    const visibleIds = new Set<string>()
    const observed = new Set<Element>()
    const syncActiveIds = (): void => {
      const firstIndex = orderedItemIds.findIndex((itemId) => visibleIds.has(itemId))
      if (firstIndex === -1) return
      let lastIndex = firstIndex
      for (let index = orderedItemIds.length - 1; index > firstIndex; index -= 1) {
        if (visibleIds.has(orderedItemIds[index])) {
          lastIndex = index
          break
        }
      }
      const nextIds = new Set(orderedItemIds.slice(firstIndex, lastIndex + 1))
      setActiveItemIds((current) => (
        current.size === nextIds.size && [...current].every((itemId) => nextIds.has(itemId))
          ? current
          : nextIds
      ))
    }
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const itemId = (entry.target as HTMLElement).dataset.contentSearchUnitKey
          if (!itemId || !itemIds.has(itemId)) continue
          if (entry.isIntersecting) visibleIds.add(itemId)
          else visibleIds.delete(itemId)
        }
        syncActiveIds()
      },
      {
        root
      }
    )

    const syncObservedNodes = (): void => {
      for (const node of observed) observer.unobserve(node)
      observed.clear()
      visibleIds.clear()
      root.querySelectorAll<HTMLElement>('[data-content-search-unit-key]').forEach((node) => {
        const itemId = node.dataset.contentSearchUnitKey
        if (!itemId || !itemIds.has(itemId)) return
        observed.add(node)
        observer.observe(node)
      })
      syncActiveIds()
    }

    syncObservedNodes()
    const mutationObserver = new MutationObserver(syncObservedNodes)
    mutationObserver.observe(root, { childList: true, subtree: true })

    return () => {
      mutationObserver.disconnect()
      observer.disconnect()
    }
  }, [containerRef, itemSignature, items])

  return activeItemIds
}

function scrollActiveNavigationItemIntoView(root: HTMLElement | null, itemId: string | null): void {
  if (!root || !itemId) return
  const escapedItemId = typeof CSS !== 'undefined' && CSS.escape
    ? CSS.escape(itemId)
    : itemId.replace(/"/g, '\\"')
  const target = root.querySelector<HTMLElement>(
    `[data-thread-user-message-navigation-item-id="${escapedItemId}"]`
  )
  if (!target) return
  if (target.offsetTop < root.scrollTop) {
    root.scrollTop = target.offsetTop
    return
  }
  if (target.offsetTop + target.offsetHeight > root.scrollTop + root.clientHeight) {
    root.scrollTop = target.offsetTop + target.offsetHeight - root.clientHeight + 1
  }
}

function useRailPortalTarget(
  containerRef: RefObject<HTMLDivElement | null>,
  enabled: boolean
): HTMLElement | null {
  const [portalTarget, setPortalTarget] = useState<HTMLElement | null>(null)

  useEffect(() => {
    if (!enabled || typeof document === 'undefined') {
      setPortalTarget(null)
      return
    }

    const root = containerRef.current
    if (!root) return

    setPortalTarget(root.closest<HTMLElement>(RAIL_PORTAL_TARGET_SELECTOR) ?? root.parentElement)
  }, [containerRef, enabled])

  return portalTarget
}

export function ThreadUserMessageNavigationRail({
  items,
  containerRef,
  railLabel,
  noContentLabel,
  itemAriaLabel,
  onSelectItem
}: Props): ReactElement | null {
  const activeItemIds = useActiveNavigationItemIds(containerRef, items)
  const [previewItemId, setPreviewItemId] = useState<string | null>(null)
  const [tooltipItemId, setTooltipItemId] = useState<string | null>(null)
  const [tooltipMounted, setTooltipMounted] = useState(false)
  const [tooltipOpen, setTooltipOpen] = useState(false)
  const railListRef = useRef<HTMLDivElement>(null)
  const openTimerRef = useRef<number | null>(null)
  const closeTimerRef = useRef<number | null>(null)
  const pointerScrubRef = useRef<PointerScrubState | null>(null)
  const pointerSelectedItemIdRef = useRef<string | null>(null)
  const itemById = useMemo(() => new Map(items.map((item) => [item.id, item])), [items])
  const itemSignature = useMemo(() => items.map((item) => item.id).join('\u0000'), [items])
  const portalTarget = useRailPortalTarget(containerRef, items.length >= MIN_NAVIGATION_ITEMS)

  const tooltipItem = tooltipItemId
    ? itemById.get(tooltipItemId) ?? null
    : null
  const railListClassName = [
    'timeline-user-message-navigation-list',
    items.length > 12 ? 'is-scrollable' : ''
  ].filter(Boolean).join(' ')
  const tooltipTitle = tooltipItem
    ? tooltipItem.title || tooltipItem.preview || noContentLabel
    : ''
  const fallbackTooltipPreview = tooltipItem && tooltipItem.preview !== tooltipTitle
    ? tooltipItem.preview
    : ''
  const tooltipPreviewText = tooltipItem
    ? tooltipItem.responsePreview || fallbackTooltipPreview
    : ''
  const tooltipContextChips = tooltipItem?.contextChips ?? []
  const tooltipContextOverflowCount = tooltipItem?.contextChipOverflowCount ?? 0
  const visibleActiveItemId = items.find((item) => activeItemIds.has(item.id))?.id ?? items.at(-1)?.id ?? null
  const previewIndex = previewItemId
    ? items.findIndex((item) => item.id === previewItemId)
    : -1

  const clearOpenTimer = useCallback((): void => {
    if (openTimerRef.current === null) return
    window.clearTimeout(openTimerRef.current)
    openTimerRef.current = null
  }, [])

  const clearCloseTimer = useCallback((): void => {
    if (closeTimerRef.current === null) return
    window.clearTimeout(closeTimerRef.current)
    closeTimerRef.current = null
  }, [])

  const closeTooltip = useCallback((): void => {
    clearOpenTimer()
    setTooltipOpen(false)
    setPreviewItemId(null)
    clearCloseTimer()
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null
      setTooltipMounted(false)
      setTooltipItemId(null)
    }, TOOLTIP_EXIT_MS)
  }, [clearCloseTimer, clearOpenTimer])

  const openTooltip = useCallback((
    itemId: string,
    options: { immediate?: boolean } = {}
  ): void => {
    setPreviewItemId(itemId)
    clearCloseTimer()
    setTooltipMounted(true)
    if (options.immediate) {
      clearOpenTimer()
      setTooltipItemId(itemId)
      setTooltipOpen(true)
      return
    }
    clearOpenTimer()
    openTimerRef.current = window.setTimeout(() => {
      openTimerRef.current = null
      setTooltipItemId(itemId)
      setTooltipOpen(true)
    }, TOOLTIP_OPEN_DELAY_MS)
  }, [clearCloseTimer, clearOpenTimer])

  const select = useCallback((
    item: ThreadUserMessageNavigationItem,
    options?: SelectOptions
  ): void => {
    onSelectItem(item, options)
  }, [onSelectItem])

  useEffect(() => {
    setPreviewItemId(null)
    setTooltipItemId(null)
    setTooltipMounted(false)
    setTooltipOpen(false)
  }, [itemSignature])

  useEffect(() => () => {
    clearOpenTimer()
    clearCloseTimer()
  }, [clearCloseTimer, clearOpenTimer])

  useEffect(() => {
    scrollActiveNavigationItemIntoView(railListRef.current, visibleActiveItemId)
  }, [visibleActiveItemId, tooltipOpen])

  if (items.length < MIN_NAVIGATION_ITEMS) return null

  const itemFromElement = (target: Element | null): { button: HTMLElement; item: ThreadUserMessageNavigationItem } | null => {
    if (!target) return null
    const button = target.closest('[data-thread-user-message-navigation-item-id]') as HTMLElement | null
    if (!button || !railListRef.current?.contains(button)) return null
    const itemId = button.dataset.threadUserMessageNavigationItemId
    const item = itemId ? itemById.get(itemId) : undefined
    return item ? { button, item } : null
  }

  const releasePointerScrub = (pointerId?: number): void => {
    const current = pointerScrubRef.current
    if (!current || (pointerId !== undefined && current.pointerId !== pointerId)) return
    if (current.pointerCaptureTarget.hasPointerCapture?.(current.pointerId)) {
      current.pointerCaptureTarget.releasePointerCapture?.(current.pointerId)
    }
    pointerScrubRef.current = null
  }

  const rail = (
    <nav
      aria-label={railLabel}
      className={`timeline-user-message-navigation-rail${tooltipOpen ? ' is-open' : ''}`}
      onPointerLeave={() => {
        releasePointerScrub()
        closeTooltip()
      }}
      onPointerUp={(event) => releasePointerScrub(event.pointerId)}
      onPointerCancel={(event) => releasePointerScrub(event.pointerId)}
      onBlur={(event) => {
        const nextTarget = event.relatedTarget
        if (!(nextTarget instanceof Node) || !event.currentTarget.contains(nextTarget)) {
          closeTooltip()
        }
      }}
    >
      <div
        ref={railListRef}
        data-thread-user-message-navigation-rail-list="true"
        className={railListClassName}
        onPointerDownCapture={(event) => {
          if (event.button !== 0) return
          const hit = itemFromElement(event.target instanceof Element ? event.target : null)
          if (!hit) return
          pointerScrubRef.current = {
            itemId: hit.item.id,
            pointerCaptureTarget: hit.button,
            pointerId: event.pointerId
          }
          pointerSelectedItemIdRef.current = hit.item.id
          hit.button.setPointerCapture?.(event.pointerId)
          openTooltip(hit.item.id, { immediate: true })
          select(hit.item, { behavior: 'smooth' })
        }}
        onPointerMove={(event) => {
          const current = pointerScrubRef.current
          if (!current || current.pointerId !== event.pointerId) return
          if (event.buttons % 2 === 0) {
            releasePointerScrub(event.pointerId)
            return
          }
          const hit = itemFromElement(document.elementFromPoint(event.clientX, event.clientY))
          if (!hit || hit.item.id === current.itemId) return
          pointerScrubRef.current = { ...current, itemId: hit.item.id }
          pointerSelectedItemIdRef.current = hit.item.id
          openTooltip(hit.item.id, { immediate: true })
          select(hit.item, { behavior: 'auto' })
        }}
        onLostPointerCapture={(event) => releasePointerScrub(event.pointerId)}
        onScroll={() => closeTooltip()}
      >
        {items.map((item, index) => {
          const active = activeItemIds.has(item.id)
          const preview = previewItemId === item.id
          const className = [
            'timeline-user-message-navigation-button',
            navigationItemTone(item),
            navigationItemProximityTone(index, previewIndex),
            active ? 'is-active' : '',
            preview ? 'is-preview' : ''
          ].filter(Boolean).join(' ')

          return (
            <button
              key={item.id}
              type="button"
              className={className}
              data-thread-user-message-navigation-item-id={item.id}
              aria-label={itemAriaLabel(item)}
              aria-current={active ? 'true' : undefined}
              onFocus={() => openTooltip(item.id, { immediate: true })}
              onPointerEnter={() => openTooltip(item.id)}
              onClick={() => {
                if (pointerSelectedItemIdRef.current === item.id) {
                  pointerSelectedItemIdRef.current = null
                  return
                }
                openTooltip(item.id, { immediate: true })
                select(item, { behavior: 'smooth' })
              }}
            >
              <span className="timeline-user-message-navigation-bar" />
            </button>
          )
        })}
      </div>
      {tooltipMounted && tooltipItem ? (
        <div
          className={`timeline-user-message-navigation-tooltip${tooltipOpen ? ' is-open' : ''}`}
          role="tooltip"
          onPointerEnter={() => openTooltip(tooltipItem.id, { immediate: true })}
          onClick={() => select(tooltipItem, { behavior: 'smooth' })}
        >
          <div className="timeline-user-message-navigation-tooltip-card">
            <div className="timeline-user-message-navigation-tooltip-title">
              {tooltipTitle}
            </div>
            {tooltipPreviewText ? (
              <div className="timeline-user-message-navigation-tooltip-preview">
                {tooltipPreviewText}
              </div>
            ) : null}
            {tooltipContextChips.length > 0 ? (
              <div className="timeline-user-message-navigation-tooltip-context">
                {tooltipContextChips.map((chip) => (
                  <span
                    key={`${chip.icon}:${chip.title ?? chip.label}`}
                    className="timeline-user-message-navigation-tooltip-chip"
                    title={chip.title ?? chip.label}
                  >
                    <NavigationContextIcon icon={chip.icon} />
                    <span className="timeline-user-message-navigation-tooltip-chip-label">
                      {chip.label}
                    </span>
                  </span>
                ))}
                {tooltipContextOverflowCount > 0 ? (
                  <span className="timeline-user-message-navigation-tooltip-chip-more">
                    +{tooltipContextOverflowCount}
                  </span>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      ) : null}
    </nav>
  )

  if (typeof document === 'undefined') return rail
  if (!portalTarget) return null
  return createPortal(rail, portalTarget)
}
