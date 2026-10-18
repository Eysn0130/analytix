/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useTranslation } from 'react-i18next'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { ReadCardModel } from '../models/read-card-model'
import { CHAT_READ_MAX_LINES } from '../models/read-card-model'
import type { TerminalCardModel } from '../models/terminal-card-model'
import { processTerminalCardModel } from '../models/terminal-card-model'
import { diffCardModel } from '../models/diff-card-model'
import { ReadBlock } from '../../ReadBlock'
import { TerminalBlock } from '../../TerminalBlock'
import { DiffBlock } from '../../DiffBlock'
import css from '../ToolRow.module.css'
export function toolCardLabels(t: (key: string, options?: Record<string, unknown>) => string) {
  return { openFile: t('filePreviewTitle'), copy: t('protectedToolCopy'), copied: t('protectedToolCopied'), save: t('protectedToolSave'),
    collapse: t('processCollapseDetail'), collapseAria: t('processCollapseDetail'),
    expand: (hidden: number) => t('protectedToolShowLines', { count: hidden }), expandAria: (hidden: number) => t('protectedToolShowLines', { count: hidden }),
    codeLabel: t('protectedToolText'), wrapLabel: t('protectedToolWrap'), unwrapLabel: t('protectedToolUnwrap'),
    window: (shown: number, total: number) => t('protectedToolWindow', { shown, total }),
    signal: (signal: string) => signal, exitCode: (code: number) => t('protectedToolExit', { code }),
    noExitCode: t('protectedToolUnknown'), running: t('inspectorStatusRunning'), failed: t('protectedToolFailed'), done: t('protectedToolCompleted'),
    noOutput: t('protectedToolEmpty'), status: (status: string) => t('protectedToolStatus.' + status) }
}
export type ProtectedToolCardModel = { kind: 'read'; card: ReadCardModel } | { kind: 'terminal'; card: TerminalCardModel }
/** Called inside the authorized mounted leaf only. Effects are its existing callbacks. */
export function ProtectedToolCardBody({ model, onCopy, onSave, onOpenFile, copied }: {
  model: ProtectedToolCardModel; onOpenFile?: (() => void) | undefined; onCopy: () => void; onSave: () => void; copied: boolean
}) {
  const { t } = useTranslation('common'), labels = toolCardLabels(t)
  return model.kind === 'read'
    ? <ReadBlock {...model.card} labels={labels} maxLines={CHAT_READ_MAX_LINES} onOpenFile={onOpenFile} onCopy={onCopy} onSave={onSave} copied={copied} />
    : <TerminalBlock {...model.card} labels={labels} maxLines={Infinity} className={css.terminalBody} onCopy={onCopy} onSave={onSave} copied={copied} />
}
/** Existing ProcessPane authorization, visibility and transfer interception enclose this pure leaf. */
export function ProcessToolCardBody({ block }: { block: ProcessDisplayBlockV1 }) {
  const { t } = useTranslation('common'), labels = toolCardLabels(t)
  const terminal = processTerminalCardModel(block), diff = diffCardModel(block)
  if (terminal) return <TerminalBlock {...terminal} labels={labels} maxLines={Infinity} className={css.terminalBody} copied={false} />
  if (!block.body) return null
  return <>{diff ? <DiffBlock model={diff} labels={labels} />
      : <div className={css.ioCard} data-tool-io>{block.kind === 'shell' && block.title ? <div className={css.ioSection}>
        <span className={css.ioLabel}>IN</span><pre className={css.ioText}>{block.title}</pre></div> : null}
        <div className={css.ioSection}><span className={css.ioLabel}>OUT</span>
        <pre className={css.ioText}>{block.body.text}</pre></div>
        {['write', 'edit'].includes(block.kind) ? <>
          <div className={css.ioSection}><span className={css.ioLabel}>{t('dshProcessBefore')}</span><pre className={css.ioText}>{block.body.before}</pre></div>
          {block.body.after ? <div className={css.ioSection}><span className={css.ioLabel}>{t('dshProcessAfter')}</span><pre className={css.ioText}>{block.body.after}</pre></div> : null}
        </> : null}</div>}
    {block.body.error ? <pre className={css.publicOutput} data-error>{block.body.error}</pre> : null}
  </>
}
