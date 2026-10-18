/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useContext, useEffect, useRef, useState } from 'react'
import { Loader2 } from '../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import type { GeneratedArtifactMetadata } from '../../../../../packages/runtime/src/contracts/generated-artifact'
import { openGeneratedArtifact } from '../../office/open-generated-artifact'
import { useChatStore } from '../../store/chat-store'
import { ProtectedToolResultScope } from './presentation/ProtectedToolResult'
import { DshOfficeFileIcon } from './presentation/DeliverableIcons'
import { DshChevronDownOutlineRegular, DshChevronUpOutlineRegular } from './presentation/DshToolIcons'
import css from './GeneratedOfficeArtifact.module.css'

const COLLAPSED_COUNT = 4
const SUCCESS_HOLD_MS = 5000
const SUCCESS_FADE_MS = 200
type OpenPhase = 'opening' | 'opened' | 'error'

/** The signed receipt supplies type and size; an explicit gesture resolves its private path. */
export function GeneratedOfficeArtifact({ artifact }: { artifact: GeneratedArtifactMetadata }) {
  const { t } = useTranslation('common')
  const inheritedThreadId = useContext(ProtectedToolResultScope)
  const activeThreadId = useChatStore(state => state.activeThreadId)
  const workspace = useChatStore(state => state.workspaceRoot)
  const threadId = inheritedThreadId ?? activeThreadId
  const [phase, setPhase] = useState<OpenPhase>()
  const inFlight = useRef(false), mounted = useRef(true)
  const successTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      if (successTimer.current !== null) clearTimeout(successTimer.current)
    }
  }, [])
  const busy = phase === 'opening'
  const name = t(artifact.kind === 'xlsx' ? 'generatedOfficeWorkbook' : artifact.kind === 'pptx' ? 'generatedOfficePresentation' : 'generatedOfficeDocument')
  const metadata = artifact.kind.toUpperCase() + ' · ' + Math.ceil(artifact.byteSize / 1024) + ' KB'
  const status = phase === 'opening' ? t('dshDeliverables.opening') : phase === 'opened' ? t('dshDeliverables.opened')
    : phase === 'error' ? t('generatedOfficeUnavailable') : metadata
  const open = async () => {
    if (inFlight.current) return
    inFlight.current = true
    if (successTimer.current !== null) clearTimeout(successTimer.current)
    setPhase('opening')
    try {
      const opened = await openGeneratedArtifact(artifact.artifactId, { threadId: threadId ?? '', workspace })
      if (!mounted.current) return
      setPhase(opened ? 'opened' : 'error')
      if (opened) successTimer.current = setTimeout(() => {
        if (mounted.current) setPhase(undefined)
      }, SUCCESS_HOLD_MS + SUCCESS_FADE_MS)
    } catch {
      if (mounted.current) setPhase('error')
    } finally {
      inFlight.current = false
    }
  }
  return <div className={css.file} data-generated-office-artifact>
    <button type="button" className={css.cardPreview} aria-label={t('dshDeliverables.previewCard', { name })}
      disabled={busy} aria-busy={busy} onClick={() => void open()} />
    <span className={css.fileIcon}><DshOfficeFileIcon kind={artifact.kind} size={20} /></span>
    <div className={css.fileBody}>
      <div className={css.details}>
        <span className={css.fileName}>{name}</span>
        <span className={css.description} data-generated-office-description title={status}
          role={phase === undefined ? undefined : 'status'} data-error={phase === 'error' || undefined}>
          <span className={css.secondaryText} data-success={phase === 'opened' || undefined}
            style={phase === 'opened' ? { animationDelay: SUCCESS_HOLD_MS + 'ms', animationDuration: SUCCESS_FADE_MS + 'ms' } : undefined}>{status}</span>
          <span className={css.previewHint}>{t('dshDeliverables.preview')}</span>
        </span>
      </div>
      <div className={css.actions}>
        <button type="button" className={css.openAction} disabled={busy} aria-busy={busy}
          aria-label={t('dshDeliverables.openCard', { name })} onClick={() => void open()}>
          {busy ? <Loader2 size={14} className={css.spinner} aria-hidden="true" /> : null}{t('nativeOfficeOpen')}
        </button>
      </div>
    </div>
  </div>
}

/** Only existing, validated and deduplicated Office receipts enter this group. */
export function GeneratedOfficeDeliverables({ artifacts }: { artifacts: GeneratedArtifactMetadata[] }) {
  const { t } = useTranslation('common')
  const [expanded, setExpanded] = useState(false)
  if (artifacts.length === 0) return null
  const collapsible = artifacts.length > COLLAPSED_COUNT
  const shown = collapsible && !expanded ? artifacts.slice(0, COLLAPSED_COUNT) : artifacts
  return <div className={css.root} data-generated-office-deliverables>
    <div className={css.presented} data-generated-office-files-row data-single={artifacts.length === 1 || undefined}>
      {shown.map(artifact => <GeneratedOfficeArtifact key={artifact.artifactId} artifact={artifact} />)}
    </div>
    {collapsible ? <button type="button" className={css.toggle} aria-expanded={expanded}
      aria-label={t(expanded ? 'dshDeliverables.collapseAria' : 'dshDeliverables.expandAria', { count: artifacts.length })}
      onClick={() => setExpanded(value => !value)}>
      <span>{t(expanded ? 'dshDeliverables.collapse' : 'dshDeliverables.all', { count: artifacts.length })}</span>
      {expanded ? <DshChevronUpOutlineRegular /> : <DshChevronDownOutlineRegular />}
    </button> : null}
  </div>
}
