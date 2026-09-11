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
  return (
    <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path d="M10 2.125C14.3492 2.125 17.875 5.65076 17.875 10C17.875 14.3492 14.3492 17.875 10 17.875C5.65076 17.875 2.125 14.3492 2.125 10C2.125 5.65076 5.65076 2.125 10 2.125ZM7.88672 10.625C7.94334 12.3161 8.22547 13.8134 8.63965 14.9053C8.87263 15.5194 9.1351 15.9733 9.39453 16.2627C9.65437 16.5524 9.86039 16.625 10 16.625C10.1396 16.625 10.3456 16.5524 10.6055 16.2627C10.8649 15.9733 11.1274 15.5194 11.3604 14.9053C11.7745 13.8134 12.0567 12.3161 12.1133 10.625H7.88672ZM3.40527 10.625C3.65313 13.2734 5.45957 15.4667 7.89844 16.2822C7.7409 15.997 7.5977 15.6834 7.4707 15.3486C6.99415 14.0923 6.69362 12.439 6.63672 10.625H3.40527ZM13.3633 10.625C13.3064 12.439 13.0059 14.0923 12.5293 15.3486C12.4022 15.6836 12.2582 15.9969 12.1006 16.2822C14.5399 15.467 16.3468 13.2737 16.5947 10.625H13.3633ZM12.1006 3.7168C12.2584 4.00235 12.4021 4.31613 12.5293 4.65137C13.0059 5.90775 13.3064 7.56102 13.3633 9.375H16.5947C16.3468 6.72615 14.54 4.53199 12.1006 3.7168ZM10 3.375C9.86039 3.375 9.65437 3.44756 9.39453 3.7373C9.1351 4.02672 8.87263 4.48057 8.63965 5.09473C8.22547 6.18664 7.94334 7.68388 7.88672 9.375H12.1133C12.0567 7.68388 11.7745 6.18664 11.3604 5.09473C11.1274 4.48057 10.8649 4.02672 10.6055 3.7373C10.3456 3.44756 10.1396 3.375 10 3.375ZM7.89844 3.7168C5.45942 4.53222 3.65314 6.72647 3.40527 9.375H6.63672C6.69362 7.56102 6.99415 5.90775 7.4707 4.65137C7.59781 4.31629 7.74073 4.00224 7.89844 3.7168Z" />
    </svg>
  )
}

function FileContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="10" height="10" viewBox="0 0 10 10" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M5.62988 1.12599C5.9198 1.12599 6.11903 1.12407 6.31006 1.16993L6.42969 1.20362C6.54783 1.24203 6.66141 1.29433 6.76758 1.35939L6.8291 1.39943C6.97079 1.49824 7.09992 1.62972 7.2793 1.80909L7.77442 2.30421L7.91651 2.44728C8.04775 2.58071 8.14716 2.69039 8.22412 2.81593L8.28516 2.92482C8.34146 3.0354 8.38453 3.1525 8.41358 3.27345L8.42871 3.34571C8.459 3.51573 8.45752 3.70002 8.45752 3.95362V6.542C8.45752 6.88641 8.45784 7.16509 8.43945 7.39015C8.42307 7.59041 8.39048 7.77088 8.31836 7.93898L8.28516 8.01027C8.15244 8.27068 7.95039 8.48862 7.70264 8.64064L7.59326 8.70167C7.40494 8.7976 7.20206 8.83726 6.97315 8.85597C6.74803 8.87436 6.46954 8.87452 6.125 8.87452H3.875C3.53046 8.87452 3.25197 8.87436 3.02686 8.85597C2.82659 8.8396 2.64613 8.80745 2.47803 8.73536L2.40674 8.70167C2.14617 8.56891 1.92793 8.36707 1.77588 8.11915L1.71484 8.01027C1.61894 7.822 1.57927 7.61897 1.56055 7.39015C1.54216 7.16509 1.54248 6.88641 1.54248 6.542V3.45851C1.54248 3.11403 1.54217 2.83546 1.56055 2.61036C1.57925 2.38151 1.61898 2.17852 1.71484 1.99025C1.86655 1.6925 2.109 1.45007 2.40674 1.29835C2.59504 1.20245 2.79796 1.16276 3.02686 1.14405C3.25198 1.12566 3.53045 1.12599 3.875 1.12599H5.62988ZM3.875 1.79103C3.51948 1.79103 3.27281 1.79147 3.08106 1.80714C2.89321 1.82249 2.7875 1.85087 2.7085 1.89112C2.5359 1.97909 2.39557 2.1194 2.30762 2.292C2.26739 2.37099 2.23898 2.4768 2.22363 2.66456C2.20798 2.8563 2.20752 3.10308 2.20752 3.45851V6.542C2.20752 6.89736 2.20797 7.14424 2.22363 7.33595C2.23899 7.52361 2.26738 7.62955 2.30762 7.70851L2.34277 7.7715C2.43093 7.91522 2.55744 8.03242 2.7085 8.10939L2.77344 8.13722C2.84532 8.16295 2.94008 8.18185 3.08106 8.19337C3.27281 8.20904 3.51949 8.20948 3.875 8.20948H6.125C6.48051 8.20948 6.72719 8.20904 6.91895 8.19337C7.10673 8.17803 7.21251 8.14961 7.29151 8.10939L7.35449 8.07374C7.49817 7.98564 7.6154 7.85948 7.69238 7.70851L7.7207 7.64308C7.74635 7.57129 7.76487 7.47652 7.77637 7.33595C7.79203 7.14424 7.79248 6.89736 7.79248 6.542V4.27882L6.67529 4.1548C6.12859 4.09405 5.69878 3.65917 5.64404 3.11183L5.51172 1.79103H3.875ZM6.30567 3.04591C6.32918 3.281 6.51374 3.46752 6.74854 3.49366L7.78809 3.60939C7.78635 3.56879 7.7843 3.53557 7.78076 3.50636L7.76709 3.42872C7.75025 3.35858 7.72504 3.29071 7.69238 3.22657L7.65723 3.16359C7.63125 3.1212 7.59976 3.0807 7.54639 3.02247L7.3042 2.77443L6.80908 2.27931C6.63847 2.1087 6.55244 2.02455 6.48438 1.9712L6.41992 1.92628C6.35836 1.88856 6.29263 1.85821 6.22412 1.83595L6.18359 1.82423L6.30567 3.04591Z"
        fill="currentColor"
      />
    </svg>
  )
}

function DocumentContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path d="M1.4585 6.54161V3.45812C1.4585 3.115 1.45807 2.83254 1.47681 2.60322C1.49594 2.36903 1.53694 2.15357 1.63997 1.95136L1.70427 1.83662C1.86439 1.57557 2.09396 1.36282 2.36833 1.22301L2.44482 1.1872C2.62471 1.11004 2.81525 1.07659 3.02018 1.05984C3.24951 1.04111 3.53196 1.04153 3.87508 1.04153H5.63005C5.91595 1.04153 6.12695 1.03878 6.32992 1.08751L6.45606 1.12332C6.58043 1.16376 6.69992 1.21881 6.81169 1.2873L6.8772 1.33002C7.02732 1.43474 7.16218 1.57269 7.33903 1.74954L7.83382 2.24433L7.97624 2.38715C8.10903 2.52211 8.21429 2.63828 8.29606 2.77168L8.36035 2.88601C8.41974 3.00259 8.46523 3.12594 8.49585 3.25345L8.51172 3.33035C8.54382 3.51054 8.54183 3.70323 8.54183 3.95332V6.54161C8.54183 6.88473 8.54226 7.16719 8.52352 7.39651C8.50677 7.60145 8.47332 7.79198 8.39616 7.97187L8.36035 8.04837C8.22054 8.32273 8.00779 8.55231 7.74674 8.71243L7.632 8.77672C7.42979 8.87975 7.21433 8.92075 6.98014 8.93989C6.75082 8.95863 6.46837 8.9582 6.12524 8.9582H3.87508C3.53196 8.9582 3.24951 8.95863 3.02018 8.93989C2.81525 8.92314 2.62471 8.88969 2.44482 8.81253L2.36833 8.77672C2.09396 8.63691 1.86439 8.42416 1.70427 8.16311L1.63997 8.04837C1.53694 7.84616 1.49594 7.6307 1.47681 7.39651C1.45807 7.16719 1.4585 6.88473 1.4585 6.54161ZM5.41683 5.41653C5.64695 5.41653 5.8335 5.60308 5.8335 5.8332C5.8335 6.06332 5.64695 6.24987 5.41683 6.24987H3.75016C3.52004 6.24987 3.3335 6.06332 3.3335 5.8332C3.3335 5.60308 3.52004 5.41653 3.75016 5.41653H5.41683ZM6.25016 3.74987C6.48028 3.74987 6.66683 3.93641 6.66683 4.16653C6.66683 4.39665 6.48028 4.5832 6.25016 4.5832H3.75016C3.52004 4.5832 3.3335 4.39665 3.3335 4.16653C3.3335 3.93641 3.52004 3.74987 3.75016 3.74987H6.25016ZM2.29183 6.54161C2.29183 6.89844 2.29198 7.14104 2.30729 7.32856C2.32222 7.51123 2.34937 7.60478 2.38257 7.66995L2.41471 7.72732C2.49477 7.85785 2.60956 7.96422 2.74675 8.03413L2.80208 8.05773C2.8644 8.08002 2.95122 8.09822 3.08814 8.1094C3.27565 8.12472 3.51825 8.12487 3.87508 8.12487H6.12524C6.48207 8.12487 6.72468 8.12472 6.91219 8.1094C7.09486 8.09448 7.18841 8.06733 7.25358 8.03413L7.31095 8.00198C7.44148 7.92192 7.54785 7.80713 7.61776 7.66995L7.64136 7.61461C7.66365 7.55229 7.68185 7.46548 7.69303 7.32856C7.70835 7.14104 7.7085 6.89844 7.7085 6.54161V3.95332C7.7085 3.70924 7.70684 3.59482 7.69751 3.51712L7.6853 3.44794C7.67 3.38427 7.64741 3.32265 7.61776 3.26443L7.58561 3.20706C7.55147 3.15138 7.50616 3.09869 7.38867 2.97879L7.24463 2.83352L6.74984 2.33873C6.57722 2.16612 6.49521 2.08597 6.43368 2.03763L6.3763 1.99775C6.32043 1.96351 6.26066 1.93577 6.19849 1.91556L6.13542 1.89806C6.05066 1.87771 5.95562 1.87487 5.63005 1.87487H3.87508C3.51825 1.87487 3.27565 1.87501 3.08814 1.89033C2.95122 1.90151 2.8644 1.91971 2.80208 1.942L2.74675 1.9656C2.60956 2.03551 2.49477 2.14189 2.41471 2.27241L2.38257 2.32978C2.34937 2.39495 2.32222 2.4885 2.30729 2.67117C2.29198 2.85869 2.29183 3.10129 2.29183 3.45812V6.54161Z" />
    </svg>
  )
}

function ImageContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="20" height="21" viewBox="0 0 20 21" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        d="M16.0012 7.78796C16.0012 7.07693 16.0013 6.58359 15.97 6.20007C15.9469 5.91812 15.9091 5.72861 15.8577 5.58484L15.802 5.45496C15.6481 5.15285 15.4137 4.89982 15.1262 4.72351L14.9993 4.6532C14.8413 4.57274 14.6297 4.51592 14.2542 4.48523C13.8707 4.45391 13.3771 4.453 12.6663 4.453H7.33325C6.62221 4.453 6.12888 4.4539 5.74536 4.48523C5.46351 4.50826 5.27388 4.54512 5.13013 4.59656L5.00024 4.6532C4.69799 4.80722 4.44511 5.04134 4.2688 5.32898L4.19849 5.45496C4.11798 5.61296 4.06122 5.82437 4.03052 6.20007C3.99918 6.58359 3.99829 7.07693 3.99829 7.78796V12.1815L5.01782 11.1629L5.19458 11.0028C6.1104 10.2557 7.46195 10.3092 8.31567 11.1629L13.6038 16.451C13.8548 16.4469 14.0675 16.4399 14.2542 16.4247C14.6295 16.394 14.8413 16.3371 14.9993 16.2567L15.1262 16.1854C15.4136 16.0091 15.6481 15.756 15.802 15.454L15.8577 15.3241C15.9091 15.1803 15.9469 14.9906 15.97 14.7088C16.0013 14.3254 16.0012 13.8318 16.0012 13.121V7.78796ZM7.37525 12.1034C7.00846 11.7366 6.42786 11.714 6.03442 12.035L5.95825 12.1034L4.0022 14.0594C4.00634 14.3101 4.0153 14.5224 4.03052 14.7088C4.0612 15.0844 4.11803 15.296 4.19849 15.454L4.2688 15.5809C4.44511 15.8683 4.69813 16.1028 5.00024 16.2567L5.13013 16.3124C5.2739 16.3638 5.46341 16.4016 5.74536 16.4247C6.12888 16.456 6.62222 16.4559 7.33325 16.4559H11.7268L7.37525 12.1034ZM13.0852 8.37097C13.085 7.81792 12.6363 7.37 12.0833 7.37C11.5302 7.37 11.0815 7.81792 11.0813 8.37097C11.0813 8.92418 11.53 9.37293 12.0833 9.37293C12.6365 9.37293 13.0852 8.92418 13.0852 8.37097ZM14.4153 8.37097C14.4153 9.65872 13.371 10.703 12.0833 10.703C10.7955 10.703 9.75122 9.65872 9.75122 8.37097C9.7514 7.08338 10.7956 6.03992 12.0833 6.03992C13.3709 6.03992 14.4151 7.08338 14.4153 8.37097ZM17.3313 13.121C17.3313 13.81 17.3319 14.367 17.2952 14.8172C17.2624 15.2182 17.1974 15.5794 17.053 15.9159L16.9866 16.0585C16.7211 16.5794 16.3172 17.0151 15.8215 17.3192L15.6038 17.4413C15.227 17.6332 14.8206 17.7124 14.3625 17.7499C13.9123 17.7866 13.3553 17.786 12.6663 17.786H7.33325C6.64416 17.786 6.0872 17.7866 5.63696 17.7499C5.23628 17.7171 4.87563 17.6519 4.53931 17.5077L4.39673 17.4413C3.87561 17.1757 3.43911 16.772 3.13501 16.2762L3.01294 16.0585C2.82097 15.6817 2.74177 15.2752 2.70435 14.8172C2.66758 14.367 2.66821 13.8099 2.66821 13.121V7.78796C2.66821 7.09887 2.66756 6.54192 2.70435 6.09168C2.74176 5.63388 2.82113 5.22806 3.01294 4.85144L3.13501 4.63269C3.4391 4.13698 3.87569 3.73313 4.39673 3.46765L4.53931 3.40125C4.8756 3.25701 5.23632 3.1918 5.63696 3.15906C6.0872 3.12227 6.64416 3.12293 7.33325 3.12293H12.6663C13.3553 3.12293 13.9123 3.12229 14.3625 3.15906C14.8206 3.19648 15.227 3.27568 15.6038 3.46765L15.8215 3.58972C16.3174 3.89382 16.7211 4.33032 16.9866 4.85144L17.053 4.99402C17.1973 5.33034 17.2624 5.69099 17.2952 6.09168C17.332 6.54192 17.3313 7.09887 17.3313 7.78796V13.121Z"
        fill="currentColor"
      />
    </svg>
  )
}

function CodeContextIcon(props: NavigationContextIconProps): ReactElement {
  return (
    <svg width="21" height="21" viewBox="0 0 21 21" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        d="M11.9025 5.3302C12.0658 5.06755 12.3961 4.94629 12.6975 5.05774C13.0419 5.1853 13.2176 5.56881 13.09 5.91321L9.75703 14.9132L9.69745 15.0333C9.53415 15.296 9.20387 15.4172 8.90253 15.3058C8.55813 15.1782 8.3824 14.7947 8.50995 14.4503L11.843 5.45032L11.9025 5.3302ZM5.21894 5.35853C5.3974 5.03773 5.8023 4.92241 6.12324 5.10071C6.44404 5.27917 6.55935 5.68407 6.38105 6.00501L4.05976 10.1818L6.38105 14.3585L6.43476 14.4825C6.52764 14.7774 6.4039 15.1067 6.12324 15.2628C5.84224 15.4189 5.49646 15.3503 5.29511 15.1154L5.21894 15.005L2.71894 10.505C2.60736 10.3042 2.60736 10.0594 2.71894 9.85853L5.21894 5.35853ZM15.4768 5.10071C15.7578 4.9446 16.1035 5.01323 16.3049 5.24817L16.381 5.35853L18.881 9.85853C18.9926 10.0594 18.9926 10.3042 18.881 10.505L16.381 15.005C16.2026 15.3258 15.7977 15.4411 15.4768 15.2628C15.156 15.0844 15.0406 14.6795 15.2189 14.3585L17.5393 10.1818L15.2189 6.00501L15.1652 5.88099C15.0723 5.58611 15.1961 5.25684 15.4768 5.10071Z"
        fill="currentColor"
      />
    </svg>
  )
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
