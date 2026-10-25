/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ReactNode } from 'react'

type GlyphProps = { size?: number; className?: string }
const FILE_BODY = 'M8.48924 28H19.5108C21.6479 28 22.7165 28 23.5594 27.6509C24.6833 27.1853 25.5762 26.2924 26.0417 25.1685C26.3909 24.3256 26.3909 23.257 26.3909 21.1199V8.79443C26.3909 8.32877 26.3909 8.09593 26.3471 7.87507C26.2887 7.58058 26.173 7.30042 26.0067 7.05048C25.882 6.86303 25.7177 6.69799 25.3893 6.36792L20.0611 1.01354C19.7304 0.681235 19.5651 0.515081 19.3769 0.38885C19.126 0.220541 18.8443 0.103463 18.5481 0.0443412C18.3259 0 18.0915 0 17.6226 0H8.48924C6.35209 0 5.28351 0 4.4406 0.349145C3.31672 0.814671 2.4238 1.70759 1.95828 2.83147C1.60913 3.67438 1.60913 4.74296 1.60913 6.88011V21.1199C1.60913 23.257 1.60913 24.3256 1.95828 25.1685C2.4238 26.2924 3.31672 27.1853 4.4406 27.6509C5.28351 28 6.35209 28 8.48924 28Z'
const FILE_FOLD = 'M26.3909 7.37445L19.0525 0V3.77445C19.0525 4.89271 19.0525 5.45184 19.2352 5.89289C19.4788 6.48096 19.946 6.94818 20.5341 7.19176C20.9751 7.37445 21.5342 7.37445 22.6525 7.37445H26.3909Z'
const LARGE_FILE_MARK_TRANSFORM = 'translate(14 16) scale(1.22) translate(-14 -16)'

/** Selected file artwork shared by Office deliveries and submitted Markdown plans. */
function FileGlyph({ size = 20, className, color, large = false, children }: GlyphProps & { color: string; large?: boolean; children: ReactNode }) {
  return <svg width={size} height={size} className={className} viewBox="0 0 28 28" fill="none"
    xmlns="http://www.w3.org/2000/svg" aria-hidden="true" style={{ color: 'var(--dsh-file-type-icon-color, ' + color + ')' }}>
    <path d={FILE_BODY} fill="currentColor" />
    <path d={FILE_FOLD} fill="var(--dsw-static-neutral-00, rgb(255, 255, 255))" fillOpacity=".7" />
    <g color="var(--dsw-static-neutral-00, rgb(255, 255, 255))" data-file-type-mark transform={large ? LARGE_FILE_MARK_TRANSFORM : undefined}>{children}</g>
  </svg>
}

export function DshMarkdownFileIcon(props: GlyphProps) {
  return <FileGlyph {...props} color="var(--dsw-static-deepseek-500, rgb(65, 118, 230))" large>
    <path d="M8.7588 19.5V14.6H9.8998L11.9298 17.932H11.3278L13.3018 14.6H14.4428L14.4568 19.5H13.1828L13.1688 16.539H13.3858L11.9088 19.017H11.2928L9.7738 16.539H10.0398V19.5H8.7588ZM15.4375 19.5V14.6H17.7545C18.2958 14.6 18.7718 14.7003 19.1825 14.901C19.5932 15.1017 19.9128 15.384 20.1415 15.748C20.3748 16.112 20.4915 16.546 20.4915 17.05C20.4915 17.5493 20.3748 17.9833 20.1415 18.352C19.9128 18.716 19.5932 18.9983 19.1825 19.199C18.7718 19.3997 18.2958 19.5 17.7545 19.5H15.4375ZM16.8235 18.394H17.6985C17.9785 18.394 18.2212 18.3427 18.4265 18.24C18.6365 18.1327 18.7998 17.9787 18.9165 17.778C19.0332 17.5727 19.0915 17.33 19.0915 17.05C19.0915 16.7653 19.0332 16.5227 18.9165 16.322C18.7998 16.1213 18.6365 15.9697 18.4265 15.867C18.2212 15.7597 17.9785 15.706 17.6985 15.706H16.8235V18.394Z" fill="currentColor" />
  </FileGlyph>
}

export function DshOfficeFileIcon({ kind, ...props }: GlyphProps & { kind: 'docx' | 'xlsx' | 'pptx' }) {
  if (kind === 'xlsx') return <FileGlyph {...props} color="var(--dsw-static-green-500, rgb(34, 197, 94))">
    <path
        d="M14 11.5H11.4C10.5599 11.5 10.1399 11.5 9.81901 11.6635C9.53677 11.8073 9.3073 12.0368 9.16349 12.319C9 12.6399 9 13.0599 9 13.9V16.5M14 11.5H16.6C17.4401 11.5 17.8601 11.5 18.181 11.6635C18.4632 11.8073 18.6927 12.0368 18.8365 12.319C19 12.6399 19 13.0599 19 13.9V16.5M14 11.5V21.5M14 21.5H16.6C17.4401 21.5 17.8601 21.5 18.181 21.3365C18.4632 21.1927 18.6927 20.9632 18.8365 20.681C19 20.3601 19 19.9401 19 19.1V16.5M14 21.5H11.4C10.5599 21.5 10.1399 21.5 9.81901 21.3365C9.53677 21.1927 9.3073 20.9632 9.16349 20.681C9 20.3601 9 19.9401 9 19.1V16.5M19 16.5H9"
        stroke="currentColor"
        strokeWidth="1.2"
      />
  </FileGlyph>
  return <FileGlyph {...props} color={kind === 'pptx' ? 'var(--dsw-static-amber-500, rgb(245, 158, 11))' : 'var(--dsw-static-deepseek-450, rgb(86, 134, 254))'} large>
    {kind === 'pptx' ? <path d="M11.0132 20.5V13.5H14.2132C14.8532 13.5 15.4032 13.6033 15.8632 13.81C16.3299 14.0167 16.6899 14.3167 16.9432 14.71C17.1966 15.0967 17.3232 15.5567 17.3232 16.09C17.3232 16.6167 17.1966 17.0733 16.9432 17.46C16.6899 17.8467 16.3299 18.1467 15.8632 18.36C15.4032 18.5667 14.8532 18.67 14.2132 18.67H12.1132L12.9932 17.81V20.5H11.0132ZM12.9932 18.02L12.1132 17.11H14.0932C14.5066 17.11 14.8132 17.02 15.0132 16.84C15.2199 16.66 15.3232 16.41 15.3232 16.09C15.3232 15.7633 15.2199 15.51 15.0132 15.33C14.8132 15.15 14.5066 15.06 14.0932 15.06H12.1132L12.9932 14.15V18.02Z" fill="currentColor" /> : <path d="M10.5118 20.5L8.24179 13.5H10.2818L12.1918 19.56H11.1618L13.1718 13.5H14.9918L16.8918 19.56H15.9018L17.8718 13.5H19.7618L17.4918 20.5H15.3718L13.7518 15.35H14.3218L12.6318 20.5H10.5118Z" fill="currentColor" />}
  </FileGlyph>
}

export function DshDeliverDocRegular({ size = 14, className }: GlyphProps) {
  return <svg width={size} height={size} className={className} viewBox="0 0 16 16" fill="none"
    xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
    <path d="M6.15479 4.91687H9.84543" stroke="currentColor" />
    <path d="M11.8798 9.55347V2.71525C11.8798 2.37416 11.564 2.09766 11.1744 2.09766H4.82577C4.43618 2.09766 4.12036 2.37416 4.12036 2.71525V9.55347" stroke="currentColor" />
    <path d="M2.28735 13.8022V8.84792C2.28735 8.77514 2.36262 8.72673 2.42884 8.75693L13.2936 13.7112C13.3914 13.7558 13.3596 13.9022 13.2521 13.9022H2.38735C2.33213 13.9022 2.28735 13.8575 2.28735 13.8022Z" stroke="currentColor" />
    <path d="M7.46929 10.979L13.5783 8.7416C13.6435 8.7177 13.7126 8.76601 13.7126 8.83551L13.7125 13.8022C13.7125 13.8574 13.6678 13.9022 13.6125 13.9022H7.99999" stroke="currentColor" />
    <path d="M6.15479 7.2395H9.05644" stroke="currentColor" />
  </svg>
}


/** Reuse the admitted file contour for formats without a selected type mark. */
export function DshPresentedFileIcon({ path, ...props }: GlyphProps & { path: string }) {
  const name = path.replaceAll('\\', '/').split('/').at(-1) ?? '', dot = name.lastIndexOf('.')
  const extension = dot > 0 ? name.slice(dot + 1).toLowerCase() : ''
  if (extension === 'md' || extension === 'markdown') return <DshMarkdownFileIcon {...props} />
  if (extension === 'docx' || extension === 'xlsx' || extension === 'pptx') return <DshOfficeFileIcon {...props} kind={extension} />
  return <FileGlyph {...props} color="var(--dsw-static-neutral-500, rgb(115, 115, 115))">{null}</FileGlyph>
}

/** Pinned DSH Regular folder-open artwork for the native Reveal action. */
export function DshRevealFolderIcon({ size = 16, className }: GlyphProps) {
  return <svg width={size} height={size} className={className} viewBox="0 0 16 16" fill="none"
    xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
    <path d="M12.3994 13.5986H2.04956C1.49728 13.5986 1.04956 13.1509 1.04956 12.5986V3.40137C1.04956 2.84908 1.49728 2.40137 2.04956 2.40137H4.76632C5.01016 2.40137 5.24561 2.49046 5.42836 2.6519L6.94088 3.98799C7.12364 4.14943 7.35908 4.23852 7.60293 4.23852H12.3994C12.9517 4.23852 13.3994 4.68624 13.3994 5.23852V7.16991" stroke="currentColor" />
    <path d="M2.55911 7.93683C2.67584 7.49906 3.07229 7.19446 3.52536 7.19446H13.6491C14.3061 7.19446 14.7846 7.81725 14.6153 8.45209L13.4411 12.856C13.3244 13.2938 12.9279 13.5984 12.4748 13.5984H2.35113C1.69411 13.5984 1.21562 12.9756 1.38489 12.3407L2.55911 7.93683Z" stroke="currentColor" />
  </svg>
}

/** Pinned DSH Regular warning triangle for the spreadsheet preview. */
export function DshExcelWarningTriangle({ size = 16, className }: GlyphProps) {
  return <svg width={size} height={size} className={className} viewBox="0 0 16 16" fill="none"
    xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1} strokeLinecap="round" strokeLinejoin="round">
    <path d="M6.87 2.6a1.33 1.33 0 0 1 2.26 0l5.34 9.33A1.33 1.33 0 0 1 13.33 14H2.67a1.33 1.33 0 0 1-1.14-2.07Z" stroke="currentColor" />
    <path d="M8 6v3m0 2.33h.01" stroke="currentColor" />
  </svg>
}
