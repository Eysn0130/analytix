import type { CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { ToolResultLocalInspectV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import css from '../ToolRow.module.css'

/** Pure text leaf. Its mounted existing owner supplies visibility and authority. */
export function ToolRawIO({ input, outputError, outputText, wrapping = true }: {
  input?: ProcessDisplayBlockV1['input']; outputError?: ToolResultLocalInspectV1['outputError']; outputText?: ToolResultLocalInspectV1['outputText']
  wrapping?: boolean
}) {
  const { t } = useTranslation('common')
  if (!input && !outputError && !outputText) return null
  const unavailable = (reason: string) => t(reason === 'limit' ? 'dshToolInspect.captureLimit' : 'dshToolInspect.unavailable')
  const textStyle: CSSProperties = { whiteSpace: wrapping ? 'pre-wrap' : 'pre', wordBreak: wrapping ? 'break-word' : 'normal' }
  return <div className={css.ioCard} data-tool-io={input ? 'input' : 'output'} data-protected-tool-readonly>
    {input ? <div className={css.ioSection}><span className={css.ioLabel}>IN</span>
      <pre className={css.ioText} style={textStyle} data-tool-raw-input>{input.availability === 'available' ? input.inputJSON : unavailable(input.reason)}</pre>
    </div> : null}
    {outputText ? <div className={css.ioSection}><span className={css.ioLabel}>OUT</span>
      {outputText.availability === 'available' ? <><pre className={css.ioText} style={textStyle} data-tool-text-output
        data-source-classification={outputText.classification}>{outputText.body}</pre>
        <span className="text-xs text-ds-faint" data-tool-text-limit>{t('dshToolInspect.boundedText', { limit: 32 })}
          {outputText.truncated ? <> · {t('protectedToolTruncated')}</> : null}</span></>
        : <span className="text-xs text-ds-faint" data-tool-text-unavailable>{unavailable(outputText.reason)}</span>}
    </div> : null}
    {outputError ? <div className={css.ioSection}><span className={css.ioLabel}>OUT</span>
      {outputError.availability === 'available' ? outputError.entries.map((entry, index) =>
        <pre key={entry.parallelIndex ?? index} className={css.ioText} style={textStyle} data-tool-raw-error data-error
          data-source-classification={entry.classification} data-parallel-index={entry.parallelIndex} data-job-id={entry.jobId}>
          {entry.message}
        </pre>) : <pre className={css.ioText} style={textStyle} data-tool-raw-error-unavailable>{unavailable(outputError.reason)}</pre>}
    </div> : null}
  </div>
}
