import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { MarkdownText, type MarkdownLabels } from './presentation/markdown/MarkdownText'
import { MarkdownDelegateProvider } from './presentation/markdown/MarkdownDelegate'
import './presentation/tokens.css'
import './presentation/shiki.css'

export function DshAssistant({ text, streaming, className }: { text: string; streaming: boolean; className?: string }) {
  const { t, i18n } = useTranslation('common')
  const labels = useMemo<MarkdownLabels>(() => ({ code: {
    copyLabel: t('codeBlockCopy'), copiedLabel: t('copySuccess'),
    toolbarLabels: { codeLabel: t('code'), wrapLabel: t('codeBlockWrap'), unwrapLabel: t('codeBlockUnwrap') }
  }, footnotes: t('markdownFootnotes') }), [t, i18n.resolvedLanguage])
  return <MarkdownDelegateProvider openExternalLink={href => { void window.analytix?.app?.openExternal?.(href).catch(() => undefined) }}>
    <MarkdownText text={text} streaming={streaming} labels={labels} className={['ds-assistant-markdown', className].filter(Boolean).join(' ')} />
  </MarkdownDelegateProvider>
}
