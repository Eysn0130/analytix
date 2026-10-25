/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useTranslation } from 'react-i18next'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { LinkIconMedium } from '../../LinkIcon'
import { ProtectedToolResult } from '../../ProtectedToolResult'
import type { ToolViewProps } from '../ToolViews'
import { ToolRow } from '../ToolRow'
import { toolRowState } from '../models/tool-call-model'
import { webFetchCardModel } from '../models/web-fetch-card-model'
import type { ToolBlock } from '../../../../../agent/types'
import card from '../../CodeCard.module.css'
import css from '../ToolRow.module.css'

/** Exactly the pinned three-path Browse artwork; no global icon dependency. */
export function BrowseOutlineArtwork() {
  return <>
    <path d="M4.9375 5.90295H11.0625" stroke="currentColor" />
    <path d="M4.9375 9.02991H8.27841" stroke="currentColor" />
    <path d="M12.5 1.32617C13.3039 1.32617 14 1.95171 14 2.77637V13.2246C13.9996 14.0489 13.3036 14.6738 12.5 14.6738H3.5C2.69637 14.6738 2.00042 14.0489 2 13.2246V2.77637C2 1.95171 2.69613 1.32617 3.5 1.32617H12.5ZM3.5 2.32617C3.1993 2.32617 3 2.55186 3 2.77637V13.2246C3.00044 13.4489 3.19963 13.6738 3.5 13.6738H12.5C12.8004 13.6738 12.9996 13.4489 13 13.2246V2.77637C13 2.55186 12.8007 2.32617 12.5 2.32617H3.5Z" fill="currentColor" />
  </>
}
export function BrowseOutlineRegular({ size = 14 }: { size?: number }) {
  return <svg width={size} height={size} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}><BrowseOutlineArtwork /></svg>
}

function FetchContent({ block, display, openWebLink }: { block: ToolBlock; display: ToolResultLocalDisplayV1; openWebLink: (role: 'request' | 'result') => void }) {
  const { t } = useTranslation('common')
  if (display.capture.kind !== 'web_fetch') return null
  const capture = display.capture, web = capture.web, model = webFetchCardModel(block, display)
  const inputJSON = display.inspect ? display.inspect.inputJSON : web.inputJSON
  return <>
    {model ? <div data-web-fetch-card className={`${card.card} ${css.webFetchCard}`}>
      {display.canOpenWeb?.result ? <button type="button" role="link" className={css.webFetchUrl} data-protected-tool-copy-section="result-url"
        onClick={() => openWebLink('result')}><LinkIconMedium kind="url" className={css.webFetchLinkIcon} />{model.url}</button>
        : <span className={css.webFetchUrl} data-protected-tool-copy-section="result-url">{model.url}</span>}
      <div data-web-fetch-meta className={css.webFetchMeta}>
        <span>{t('dshWebFetch.http', { status: model.statusCode })}</span>
        {web.fetchTruncated ? <span className={css.webFetchTruncated}>{t('dshWebFetch.trunc')}</span> : null}
      </div>
    </div> : <div className={css.ioCard} data-tool-io>
      {inputJSON !== undefined ? <div className={css.ioSection} data-protected-tool-readonly={Boolean(display.inspect) || undefined}
        data-protected-tool-copy-section={display.inspect ? undefined : 'payload'}><span className={css.ioLabel}>IN</span><pre className={css.ioText}>{inputJSON}</pre></div> : null}
      {inputJSON !== undefined && capture.body !== '' ? <div className={css.ioDivider} /> : null}
      {capture.body !== '' ? <div className={css.ioSection} data-protected-tool-copy-section="result"><span className={css.ioLabel}>OUT</span><pre className={css.ioText} data-error={web.toolIsError || undefined}>{capture.body}</pre></div> : null}
    </div>}
    {capture.truncated ? <p className={css.webFetchCaptureLimit}>{t('dshToolInspect.captureLimit')}</p> : null}
    {web.requestUrlTruncated || web.finalUrlTruncated || (display.inspect ? display.inspect.inputTruncated : web.inputTruncated) ? <p className={css.webFetchCaptureLimit}>{t('dshWebFetch.fieldLimit')}</p> : null}
  </>
}

/** Real private capture supplies DSH Fetch; the public status remains transport-only. */
export function WebFetchRow({ block, visible, capture, onInspect, resetKey, privateGeneration }: ToolViewProps) {
  const { t } = useTranslation('common')
  if (block.meta?.toolName !== 'web_fetch' || block.toolKind !== 'tool_call') return null
  const state = toolRowState(block)
  const props = { icon: <BrowseOutlineRegular />, title: t('dshWebFetch.title'), toolName: 'web_fetch', variant: 'web' }
  const row = state === 'running' || state === 'preparing' ? <ToolRow {...props} state={state}
    summary={capture?.requestUrl ? <span data-protected-process-block={capture.blockId}>{capture.requestUrl}</span> : undefined}
    onOpenLink={capture?.requestUrl ? capture.openWebLink : undefined} />
    : block.meta?.localResult && visible ? <ProtectedToolResult key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} preload onInspect={onInspect}
      renderContent={(display, openWebLink) => <FetchContent block={block} display={display} openWebLink={openWebLink} />}
      renderView={controls => {
        const result = controls.display?.capture.kind === 'web_fetch' ? controls.display.capture : null
        const failed = controls.state === 'error' || controls.state === 'stopped' || controls.state === 'unknown'
        const error = failed ? result?.body.split(/\r\n|\r|\n/).find(line => line.trim()) : undefined
        const summary = error ?? result?.web.requestUrl
        return <ToolRow {...props} state={controls.state ?? state}
          summary={summary ? <span data-protected-tool-summary data-protected-tool-copy-section={error ? 'result' : 'request-url'}>{summary}</span> : undefined}
          onOpenLink={!failed && controls.display?.canOpenWeb?.request ? () => controls.onOpenWebLink('request') : undefined}
          expanded={controls.open} expandable={controls.canOpen} onToggle={controls.onToggle} inspect={controls.inspect}
          actions={controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : undefined}
          body={controls.body ? { kind: 'web', content: controls.body } : undefined} />
      }} /> : <ToolRow {...props} state={state} summary={visible ? t('dshWebFetch.unknown') : undefined} />
  return <div data-web-fetch-row data-analytix-process-block-id={block.id} data-analytix-process-block-kind={block.kind}
    data-analytix-process-block-status={block.status} data-tool-view="web">{row}</div>
}
