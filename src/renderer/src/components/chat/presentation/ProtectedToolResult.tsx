import { createContext, useCallback, useContext, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ToolBlock } from '../../../agent/types'
import {
  toolResultLocalSelectorSchemaV1, toolResultLocalViewResultSchemaV1,
  toolResultLocalEffectResultSchemaV1, type ToolResultLocalDisplayV1
} from '../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { sharedActiveCaseContextTransitionPending, subscribeBeforeSharedActiveCaseContextChange } from '../../../data-analysis/services/analysis/stats-case-overview-resource'
import { TerminalBlock } from './TerminalBlock'
import { ReadBlock } from './ReadBlock'

export const ProtectedToolResultScope = createContext<string | null>(null)

// Payload lives only in this mounted lazy leaf. No transcript/markdown, tool
// argument, model state, clipboard/Blob or cross-thread cache receives it.
export function ProtectedToolResult({ block, threadId }: { block: ToolBlock; threadId?: string | null }) {
  const inheritedThread = useContext(ProtectedToolResultScope)
  const { t } = useTranslation('common')
  const [payload, setPayload] = useState<{ key: string; value: ToolResultLocalDisplayV1 } | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)
  const [, setRetirementRevision] = useState(0)
  const copyCurrent = useRef<() => void>(() => undefined)
  const generation = useRef(0)
  const viewId = useRef<string | null>(null)
  const api = window.analytix.runtime
  const reference = toolResultLocalSelectorSchemaV1.omit({ threadId: true }).safeParse(block.meta?.localResult)
  const referenceMatches = reference.success &&
    (!block.meta?.turnId || reference.data.turnId === block.meta.turnId) &&
    (!block.meta?.callId || reference.data.callId === block.meta.callId)
  const selector = toolResultLocalSelectorSchemaV1.safeParse(referenceMatches ? {
    ...reference.data, threadId: threadId ?? inheritedThread
  } : null)
  const selectorKey = selector.success ? JSON.stringify(selector.data) : ''
  const scopeKey = selectorKey + ':' + block.status + ':' + Boolean(referenceMatches)
  const currentKey = useRef(scopeKey)
  const display = payload?.key === scopeKey && !sharedActiveCaseContextTransitionPending() ? payload.value : null
  const clear = useCallback(() => {
    generation.current += 1
    setRetirementRevision(value => value + 1)
    copyCurrent.current = () => undefined
    const id = viewId.current
    viewId.current = null
    setPayload(null); setPending(false); setCopied(false); setOpen(false); setMessage('')
    if (id) void api.closeToolResultLocalDisplay(id).catch(() => undefined)
  }, [api])
  useLayoutEffect(() => {
    currentKey.current = scopeKey
    const off = api.onToolResultLocalDisplayInvalidated?.(id => {
      if (id === null || id === viewId.current) { clear(); if (id !== null) setMessage(t('protectedToolUnavailable')) }
    })
    const runtimeOff = api.onRuntimeStatus?.(status => { if (status.state !== 'running') clear() })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(clear)
    const hidden = () => { if (document.hidden) clear() }
    document.addEventListener('visibilitychange', hidden)
    return () => { clear(); off?.(); runtimeOff?.(); caseOff(); document.removeEventListener('visibilitychange', hidden) }
  }, [clear, api, scopeKey, t])

  useLayoutEffect(() => {
    if (!open || !display || !selector.success || !referenceMatches || block.status === 'running') return
    const intersects = (event: Event) => {
      const target = event.target instanceof Element ? event.target : null
      // Input/editor selections have their own copy semantics. A stale DOM
      // selection elsewhere must not take over a focused ordinary editor.
      for (const element of event.type === 'dragstart' ? [target] : [target, document.activeElement]) {
        if (element instanceof Element && element.closest('input,textarea,[contenteditable]:not([contenteditable="false"]),[role="textbox"]') &&
          !element.closest('[data-protected-tool-body]')) return []
      }
      const selection = document.getSelection()
      const matches: Element[] = []
      for (const body of document.querySelectorAll('[data-protected-tool-body]')) {
        let selected = false
        for (let index = 0; selection && !selection.isCollapsed && index < selection.rangeCount; index++) {
          try { if (selection.getRangeAt(index).intersectsNode(body)) selected = true } catch { /* Retired range. */ }
        }
        if (selected || event.type === 'dragstart' && target && body.contains(target)) {
          const leaf = body.closest('[data-protected-tool-result]')
          if (leaf && !matches.includes(leaf)) matches.push(leaf)
        }
      }
      return matches
    }
    const copy = (event: Event) => {
      const matches = intersects(event)
      if (!matches.length) return
      event.preventDefault()
      if (matches.length === 1 && matches[0] === rootRef.current) copyCurrent.current()
    }
    const blockTransfer = (event: Event) => { if (intersects(event).length) event.preventDefault() }
    document.addEventListener('copy', copy, true)
    document.addEventListener('cut', blockTransfer, true)
    document.addEventListener('dragstart', blockTransfer, true)
    return () => {
      document.removeEventListener('copy', copy, true)
      document.removeEventListener('cut', blockTransfer, true)
      document.removeEventListener('dragstart', blockTransfer, true)
    }
  }, [open, display, scopeKey])
  copyCurrent.current = () => undefined
  if (!referenceMatches || !selector.success || block.status === 'running') return null

  const load = async () => {
    if (sharedActiveCaseContextTransitionPending()) return
    clear()
    const epoch = generation.current
    const key = scopeKey
    const id = crypto.randomUUID()
    viewId.current = id
    setOpen(true); setPending(true)
    try {
      const parsed = toolResultLocalViewResultSchemaV1.safeParse(await api.openToolResultLocalDisplay({ ...selector.data, viewId: id }))
      if (epoch !== generation.current || currentKey.current !== key || viewId.current !== id || sharedActiveCaseContextTransitionPending()) return
      if (parsed.success && parsed.data.ok) setPayload({ key, value: parsed.data.display })
      else setMessage(t('protectedToolUnavailable'))
    } catch {
      if (epoch === generation.current) setMessage(t('protectedToolUnavailable'))
    } finally {
      if (epoch === generation.current) setPending(false)
    }
  }
  const effect = async (action: 'copy' | 'save') => {
    if (!display || !viewId.current || pending || currentKey.current !== scopeKey || sharedActiveCaseContextTransitionPending()) return
    const epoch = generation.current
    setPending(true); setMessage('')
    try {
      const request = { ...selector.data, viewId: viewId.current, snapshotDigest: display.snapshotDigest }
      const parsed = toolResultLocalEffectResultSchemaV1.safeParse(await (action === 'copy'
        ? api.copyToolResultLocalDisplay(request) : api.saveToolResultLocalDisplay(request)))
      if (epoch !== generation.current) return
      if (parsed.success && parsed.data.ok) { if (action === 'copy') setCopied(true) }
      else if (parsed.success && !parsed.data.ok && parsed.data.code === 'canceled') return
      else if (parsed.success && !parsed.data.ok && parsed.data.code === 'unsupported') setMessage(t('protectedToolUnsupported'))
      else { clear(); setMessage(t('protectedToolUnavailable')) }
    } catch {
      if (epoch === generation.current) { clear(); setMessage(t('protectedToolUnavailable')) }
    } finally { if (epoch === generation.current) setPending(false) }
  }
  copyCurrent.current = () => { void effect('copy') }
  const labels = {
    copy: t('protectedToolCopy'), copied: t('protectedToolCopied'), save: t('protectedToolSave'),
    collapse: t('processCollapseDetail'), collapseAria: t('processCollapseDetail'),
    expand: (hidden: number) => t('protectedToolShowLines', { count: hidden }),
    expandAria: (hidden: number) => t('protectedToolShowLines', { count: hidden }),
    codeLabel: t('protectedToolText'), wrapLabel: t('protectedToolWrap'), unwrapLabel: t('protectedToolUnwrap'),
    window: (shown: number, total: number) => t('protectedToolWindow', { shown, total }),
    signal: (signal: string) => signal, exitCode: (code: number) => t('protectedToolExit', { code }),
    noExitCode: t('protectedToolUnknown'), running: t('protectedToolUnknown'),
    failed: t('protectedToolFailed'), done: t('protectedToolCompleted'),
    noOutput: t('protectedToolEmpty'), status: (status: string) => t('protectedToolStatus.' + status)
  }
  const capture = display?.capture
  const readText = capture?.body ?? ''
  const lines = readText === '' ? [] : readText.split(/\r\n|\r|\n/).map((text, index, all) =>
    index === all.length - 1 && text === '' ? null : { number: (capture?.startLine ?? 1) + index, text }
  ).filter((line): line is { number: number; text: string } => line !== null)
  const ext = capture?.label.match(/\.([A-Za-z0-9]+)$/)?.[1]?.toLowerCase()
  const lang = ext ? ({ tsx: 'tsx', jsx: 'jsx', sh: 'bash', yml: 'yaml', md: 'markdown' } as Record<string, string>)[ext] ?? ext : undefined
  return (
    <div ref={rootRef} data-protected-tool-result>
      <button type="button" aria-expanded={open} disabled={!open && (pending || sharedActiveCaseContextTransitionPending())}
        className="rounded px-2 py-1 text-xs text-ds-muted hover:bg-ds-hover"
        onClick={() => { if (open) clear(); else void load() }}>
        {open ? t('processCollapseDetail') : t('protectedToolOpen')}
      </button>
      {pending && <span role="status" className="px-2 text-xs text-ds-muted">{t('protectedToolLoading')}</span>}
      {message && <p role="status" className="text-xs text-ds-muted">{message}
        {!display && <button type="button" disabled={pending} onClick={() => void load()} className="ml-2 underline">{t('protectedToolRetry')}</button>}
      </p>}
      {open && display && capture && <div data-protected-tool-body inert={pending ? true : undefined}>
        {capture.kind === 'shell'
          ? <TerminalBlock key={display.snapshotDigest} command={capture.label} output={capture.body}
            status={capture.status} exitCode={capture.exitCode} labels={labels} copied={copied}
            onCopy={() => void effect('copy')} onSave={() => void effect('save')} />
          : <ReadBlock key={display.snapshotDigest} label={capture.label} lines={lines}
            totalLines={capture.totalLines} lang={lang} labels={labels} copied={copied}
            onCopy={() => void effect('copy')} onSave={() => void effect('save')} />}
        <div className="text-xs text-ds-faint">
          {capture.truncated && <span>{t('protectedToolTruncated')} · </span>}
          {capture.kind === 'shell' && <span>{t('protectedToolDuration', { duration: capture.durationMs })}</span>}
        </div>
      </div>}
    </div>
  )
}
