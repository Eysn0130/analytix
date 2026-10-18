import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { MarkdownText, type MarkdownLabels } from './presentation/markdown/MarkdownText'
import { MarkdownDelegateProvider } from './presentation/markdown/MarkdownDelegate'
import { useChatStore } from '../../store/chat-store'

export function DshAssistant({ text, streaming, className }: { text: string; streaming: boolean; className?: string }) {
  const { t, i18n } = useTranslation('common')
  const showTopNotice = useChatStore(s => s.showTopNotice)
  const labels = useMemo<MarkdownLabels>(() => ({ code: {
    copyLabel: t('codeBlockCopy'), copiedLabel: t('copySuccess'),
    toolbarLabels: { codeLabel: t('code'), wrapLabel: t('codeBlockWrap'), unwrapLabel: t('codeBlockUnwrap') }
  }, footnotes: t('markdownFootnotes'), table: {
    title: t('markdownTableTitle'), copy: t('markdownTableCopy'), copied: t('copySuccess'), copyFailed: t('copyFailed'),
    download: t('markdownTableDownload'), downloadFailed: t('markdownTableDownloadFailed'), fullscreen: t('markdownTableFullscreen'),
    close: t('markdownTableClose'), csv: t('markdownTableCsv'), tsv: t('markdownTableTsv'), markdown: t('markdownTableMarkdown'),
    exportHint: t('markdownTableExportHint')
  } }), [t, i18n.resolvedLanguage])
  const openExternalLink = (href: string): void => {
    const reportFailure = () => showTopNotice({ tone: 'error', message: t('markdownExternalOpenFailed') })
    try {
      const app = window.analytix?.app
      if (typeof app?.openExternal !== 'function') {
        reportFailure()
        return
      }
      void app.openExternal(href).catch(reportFailure)
    } catch {
      reportFailure()
    }
  }
  return <MarkdownDelegateProvider openExternalLink={openExternalLink}>
    <MarkdownText text={text} streaming={streaming} labels={labels} className={['ds-assistant-markdown', className].filter(Boolean).join(' ')} />
  </MarkdownDelegateProvider>
}
