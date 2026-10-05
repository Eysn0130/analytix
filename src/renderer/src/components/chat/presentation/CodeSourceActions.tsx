import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronUp, Download } from '../../../design/AnalytixUiIcons'
import { extensionForLanguage } from '../../../lib/code-highlighting'
import { Tooltip } from './Tooltip'
import css from './CodeCard.module.css'

export function downloadCodeSource(code: string, language = ''): void {
  downloadTextFile(code, `code.${extensionForLanguage(language)}`)
}

export function downloadTextFile(text: string, filename: string, mimeType = 'text/plain;charset=utf-8'): void {
  const url = URL.createObjectURL(new Blob([text], { type: mimeType }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.append(link)
  try { link.click() } finally { link.remove(); URL.revokeObjectURL(url) }
}

export function CodeSourceActions({ code, lang, collapsible, expanded, onToggle, contentId }: {
  contentId: string; code: string; lang?: string; collapsible: boolean; expanded: boolean; onToggle: () => void
}) {
  const { t } = useTranslation('common')
  const collapseLabel = t(expanded ? 'codeBlockCollapse' : 'codeBlockExpand')
  return <>
    <Tooltip label={t('codeBlockDownload')} side="top" portal>
      <button type="button" className={css.action} aria-label={t('codeBlockDownload')} onClick={() => downloadCodeSource(code, lang)}>
        <Download size={14} />
      </button>
    </Tooltip>
    {collapsible && <Tooltip label={collapseLabel} side="top" portal>
      <button type="button" className={css.action} aria-label={collapseLabel} aria-expanded={expanded} aria-controls={contentId} onClick={onToggle}>
        {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
      </button>
    </Tooltip>}
  </>
}
