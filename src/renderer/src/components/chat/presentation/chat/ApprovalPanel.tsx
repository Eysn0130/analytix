/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock } from '../../../../agent/types'
import styles from './ApprovalPanel.module.css'

type Props = {
  block: Extract<ChatBlock, { kind: 'approval' }>; resolving?: boolean
  readOnly?: boolean; onOpenOwnerThread?: (requestId: string) => void
  onResolve?: (decision: 'allow' | 'deny') => Promise<void>
}
export function ApprovalPanel({ block, resolving = false, readOnly = false, onOpenOwnerThread, onResolve }: Props) {
  const { t } = useTranslation('common')
  const lock = useRef(false)
  const active = useRef(true)
  const composing = useRef(false)
  const compositionEnded = useRef(false)
  const [waiting, setWaiting] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { active.current = true; return () => { active.current = false } }, [])
  const pending = block.status === 'pending'
  const answerable = pending && !readOnly && !resolving && !waiting
  const act = async (decision: 'allow' | 'deny') => {
    if (!answerable || lock.current || !onResolve) return
    lock.current = true; setWaiting(true); setError('')
    try { await onResolve(decision) }
    catch { if (active.current) setError(t('approvalFailed')) }
    finally { if (active.current) { lock.current = false; setWaiting(false) } }
  }
  const keyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.defaultPrevented || event.repeat || composing.current || compositionEnded.current ||
      event.nativeEvent.isComposing || event.keyCode === 229 || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return
    const target = event.target as HTMLElement
    if (target.closest('input,textarea,select,[contenteditable]') || event.key === 'Enter' && target.closest('button,a')) return
    if (event.key === 'Enter' || event.key === 'Escape') {
      event.preventDefault(); void act(event.key === 'Enter' ? 'allow' : 'deny')
    }
  }
  const status = resolving || waiting ? t('requestResolving') : block.status === 'allowed'
    ? t('approvalAllowed') : block.status === 'denied' ? t('approvalDenied')
      : block.status === 'error' ? t('approvalFailed') : t('approvalPending')
  return <div className={styles.root}><section className={styles.card} aria-label={t('approvalTitle')} tabIndex={0}
    data-gate-request-id={block.approvalId} aria-busy={resolving || waiting}
    onKeyDown={keyDown} onKeyUp={() => { compositionEnded.current = false }}
    onCompositionStart={() => { composing.current = true }}
    onCompositionEnd={() => { composing.current = false; compositionEnded.current = true }}>
    <header className={styles.strip}><strong>{t('approvalTitle')}</strong><span>{status}</span></header>
    <div className={styles.body}>
      {block.toolName ? <p className={styles.command}>{t('approvalTool', { name: block.toolName })}</p> : null}
      <p className={styles.headline}>{block.summary}</p>
      {block.errorMessage || error ? <p className={styles.error} role="alert">{block.errorMessage || error}</p> : null}
      {pending && readOnly ? <div className={styles.hint}><p>{t('pendingRequestNotResolved')}</p>
        {onOpenOwnerThread ? <button type="button" className={styles.primary}
          onClick={() => onOpenOwnerThread(block.approvalId)}>{t('respondInOwnedThread')}</button>
          : <p>{t('ownedThreadUnavailable')}</p>}</div> : null}
    </div>
    {pending && !readOnly ? <footer className={styles.actionRow}>
      <button type="button" className={`${styles.secondary} ${styles.reject}`} disabled={!answerable} onClick={() => void act('deny')}>{t('approvalDeny')}</button>
      <button type="button" className={styles.primary} disabled={!answerable} onClick={() => void act('allow')}>{t('approvalAllow')}</button>
    </footer> : null}
  </section></div>
}
