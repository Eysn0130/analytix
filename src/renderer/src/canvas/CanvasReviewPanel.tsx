import { useCallback, useEffect, useRef, useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'
import { canvasHostResponseSchema, type CanvasDocument, type CanvasHostRequest, type CanvasHostResponse, type CanvasProposal } from '../../../../packages/runtime/src/contracts/canvas-host'
import { canvasImageOperationsSchema, type CanvasImageOperation } from '../../../../packages/runtime/src/contracts/canvas-editing'
import type { nativeOfficeRecoverySchema } from '../../../../packages/runtime/src/contracts/native-office-editing'
import type { objectEditingReceiptSchema } from '../../../../packages/runtime/src/contracts/object-editing'

type Recovery = z.infer<typeof nativeOfficeRecoverySchema>
type Receipt = z.infer<typeof objectEditingReceiptSchema>
type Snapshot = { proposals: CanvasProposal[]; recovery: Recovery }
const local = (value: unknown): string => typeof value === 'string' ? value : JSON.stringify(value, null, 2)
const invoke = async (input: CanvasHostRequest): Promise<CanvasHostResponse> => canvasHostResponseSchema.parse(await window.analytix.canvas.request(input))

/** Protected-local review consumer. Reads never invoke Apply/Resume; mutations
 * require a distinct click. Polls and replies are fenced to this exact object
 * presentation. Unknown writes retain the original proposal/operation owner. */
export function CanvasReviewPanel({ document, imageSize, onCommitted }: {
  document: CanvasDocument
  imageSize?: { width: number; height: number }
  onCommitted: (receipt: Receipt) => void
}): ReactElement {
  const { t } = useTranslation('canvas')
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [error, setError] = useState(false)
  const [busy, setBusy] = useState(false)
  const [uncertain, setUncertain] = useState<string[]>([])
  const owner = useRef({ live: true, sequence: 0, busy: false })
  const committed = useRef(onCommitted); committed.current = onCommitted
  const { sessionId, threadId, objectId, revision } = document
  const binding = { sessionId, threadId }
  const load = useCallback(async (ticket: number): Promise<Snapshot | null> => {
    const proposals = await invoke({ operation: 'proposals-list', sessionId, threadId })
    if (!owner.current.live || owner.current.sequence !== ticket) return null
    const recovery = await invoke({ operation: 'object-recovery', sessionId, threadId })
    if (!owner.current.live || owner.current.sequence !== ticket) return null
    if (!proposals.ok || !('proposals' in proposals) || !recovery.ok || !('recovery' in recovery)) throw new Error('canvas_review_unavailable')
    const next = { proposals: proposals.proposals, recovery: recovery.recovery }
    setSnapshot(next); setError(false)
    return next
  }, [sessionId, threadId])
  useEffect(() => {
    const state = owner.current
    state.live = true; state.sequence++; state.busy = false
    setSnapshot(null); setUncertain([]); setError(false); setBusy(false)
    let stopped = false, timer: ReturnType<typeof setTimeout> | undefined, delay = 2000
    const poll = async () => {
      if (stopped) return
      if (!state.busy) {
        const ticket = ++state.sequence
        try { await load(ticket); delay = 2000 }
        catch { if (!stopped && state.sequence === ticket) { setError(true); delay = Math.min(delay * 2, 16000) } }
      }
      if (!stopped) timer = setTimeout(() => void poll(), delay)
    }
    void poll()
    return () => { stopped = true; state.live = false; state.sequence++; if (timer) clearTimeout(timer) }
  }, [sessionId, threadId, objectId, revision, load])

  const action = async (request: CanvasHostRequest, proposal?: CanvasProposal): Promise<void> => {
    const state = owner.current
    if (!state.live || state.busy) return
    const ticket = ++state.sequence
    state.busy = true; setBusy(true)
    // A transport failure may follow a committed write. Disable Save until a
    // separate original-operation query, never use Save as a retry/poll.
    if (request.operation === 'proposal-apply' && proposal) setUncertain(ids => [...new Set([...ids, proposal.proposalId])])
    try {
      const response = await invoke(request)
      if (!state.live || state.sequence !== ticket) return
      if ('receipt' in response && response.receipt?.status === 'committed' && response.ok) {
        committed.current(response.receipt)
        return
      }
      const next = await load(ticket)
      if (!state.live || state.sequence !== ticket) return
      if (response.ok && 'rejected' in response && proposal) setUncertain(ids => ids.filter(id => id !== proposal.proposalId))
      if (!response.ok) {
        // A read-only not-found plus fresh absent recovery can prove that an
        // earlier request never prepared an operation. Retrying remains manual.
        if (request.operation === 'proposal-status' && response.code === 'operation_not_found' && proposal &&
            next && !next.recovery.pending && next.proposals.some(p => p.proposalId === proposal.proposalId && p.status === 'proposed')) {
          setUncertain(ids => ids.filter(id => id !== proposal.proposalId))
        } else setError(true)
      }
    } catch { if (state.live && state.sequence === ticket) setError(true) }
    finally { if (state.live && state.sequence === ticket) { state.busy = false; setBusy(false) } }
  }
  const refresh = async (): Promise<void> => {
    const state = owner.current
    if (!state.live || state.busy) return
    const ticket = ++state.sequence
    try { await load(ticket) } catch { if (state.live && state.sequence === ticket) setError(true) }
  }
  const ready = !!snapshot && !error && !busy
  const pending = snapshot?.recovery.pending, current = snapshot?.recovery.current
  const proposals = snapshot?.proposals.filter(p => p.status !== 'rejected') ?? []
  return <section className="canvas-workspace-review" aria-label={t('canvasReview')} aria-busy={busy}>
    <div className="canvas-review-heading"><strong>{t('canvasReview')}</strong>
      <button type="button" disabled={busy} onClick={() => void refresh()}>{t('canvasCheckStatus')}</button></div>
    {error ? <p role="alert">{t('canvasReviewFailed')}</p> : null}
    {imageSize ? <CanvasImageProposal imageSize={imageSize} disabled={!ready || !!pending} onPropose={operations =>
      action({ operation: 'propose-image', ...binding, baseRevision: document.revision, operations })} /> : null}
    {!proposals.length && snapshot ? <p>{t('canvasNoProposals')}</p> : null}
    {proposals.map(proposal => <article key={proposal.proposalId} className="canvas-review-proposal" aria-label={t('canvasProposal')}>
      <p>{t(`canvasState_${proposal.status}`)}</p>
      {proposal.kind === 'canvas' ? proposal.sceneDiff.map((change, index) => <div key={index} className="canvas-review-diff">
        <strong>{change.factLabel || change.id} · {change.field}</strong>
        <pre><del>{local(change.before)}</del></pre><pre><ins>{local(change.after)}</ins></pre>
      </div>) : proposal.imageDiff.map((change, index) => <div key={index} className="canvas-review-diff">
        <strong>{t(`canvasImage_${change.operation.kind}`)}</strong>
        <pre>{local(change.operation)}</pre>
        <p><del>{change.before.width} × {change.before.height}</del> → <ins>{change.after.width} × {change.after.height}</ins></p>
      </div>)}
      <div className="canvas-review-actions">
        {proposal.status === 'proposed' && !uncertain.includes(proposal.proposalId) ? <button type="button"
          disabled={!ready || !!pending || proposal.baseRevision !== document.revision}
          onClick={() => void action({ operation: 'proposal-apply', ...binding, proposalId: proposal.proposalId }, proposal)}>{t('canvasAccept')}</button> : null}
        {['proposed', 'stale'].includes(proposal.status) && !uncertain.includes(proposal.proposalId) ? <button type="button" disabled={!ready || !!pending}
          onClick={() => void action({ operation: 'proposal-reject', ...binding, proposalId: proposal.proposalId }, proposal)}>{t('canvasReject')}</button> : null}
        {['pending', 'applied'].includes(proposal.status) || uncertain.includes(proposal.proposalId) ? <button type="button" disabled={busy}
          onClick={() => void action({ operation: 'proposal-status', ...binding, proposalId: proposal.proposalId }, proposal)}>{t('canvasQueryOriginal')}</button> : null}
      </div>
      {uncertain.includes(proposal.proposalId) || proposal.status === 'pending' ? <p>{t('canvasUnknownResult')}</p> : null}
    </article>)}
    {current?.canUndo ? <button type="button" disabled={!ready || !!pending}
      onClick={() => void action({ operation: 'undo-change', ...binding, changeId: current.changeId, baseRevision: document.revision })}>{t('canvasUndo')}</button> : null}
    {pending ? <div className="canvas-review-actions"><p>{t('canvasPendingRecovery')}</p>
      {pending.canResume ? <button type="button" disabled={!ready} onClick={() => void action({ operation: 'resume-change', ...binding, changeId: pending.changeId, baseRevision: document.revision })}>{t('canvasResume')}</button> : null}
      {pending.canRetryUndo ? <button type="button" disabled={!ready} onClick={() => void action({ operation: 'undo-change', ...binding, changeId: pending.changeId, baseRevision: document.revision })}>{t('canvasRetryUndo')}</button> : null}
      {pending.canCancel ? <button type="button" disabled={!ready} onClick={() => void action({ operation: 'cancel-change', ...binding, changeId: pending.changeId, baseRevision: document.revision })}>{t('canvasCancelPending')}</button> : null}
    </div> : null}
  </section>
}

function CanvasImageProposal({ imageSize, disabled, onPropose }: {
  imageSize: { width: number; height: number }; disabled: boolean
  onPropose: (operations: CanvasImageOperation[]) => Promise<void>
}): ReactElement {
  const { t } = useTranslation('canvas')
  const [kind, setKind] = useState<'rotate' | 'crop' | 'mark'>('rotate')
  const [degrees, setDegrees] = useState<90 | 180 | 270>(90)
  const [bounds, setBounds] = useState({ x: 0, y: 0, width: imageSize.width, height: imageSize.height })
  const [invalid, setInvalid] = useState(false)
  const propose = (): void => {
    const { x, y, width, height } = bounds
    if (kind !== 'rotate' && (!Object.values(bounds).every(Number.isInteger) || x < 0 || y < 0 || width < 1 || height < 1 || x + width > imageSize.width || y + height > imageSize.height)) { setInvalid(true); return }
    const operation = kind === 'rotate' ? { kind, degrees } : kind === 'crop' ? { kind, region: bounds } : {
      kind, mark: { id: `mark-${crypto.randomUUID()}`, shape: 'rectangle', x1: x, y1: y, x2: x + width - 1, y2: y + height - 1, stroke: '#E53935', strokeWidth: 2 }
    }
    const parsed = canvasImageOperationsSchema.safeParse([operation])
    setInvalid(!parsed.success)
    if (parsed.success) void onPropose(parsed.data)
  }
  return <details><summary>{t('canvasImageCreateProposal')}</summary><fieldset disabled={disabled} className="canvas-image-form">
    <label>{t('canvasImageOperation')}<select value={kind} onChange={e => { setKind(e.target.value as typeof kind); setInvalid(false) }}>
      {(['rotate', 'crop', 'mark'] as const).map(value => <option key={value} value={value}>{t(`canvasImage_${value}`)}</option>)}
    </select></label>
    {kind === 'rotate' ? <label>{t('canvasImageDegrees')}<select value={degrees} onChange={e => setDegrees(Number(e.target.value) as typeof degrees)}>
      {[90, 180, 270].map(value => <option key={value} value={value}>{value}°</option>)}
    </select></label> : <>{(['x', 'y', 'width', 'height'] as const).map(field => <label key={field}>{t(`canvasImage_${field}`)}
      <input type="number" step="1" min={field === 'width' || field === 'height' ? 1 : 0}
        max={field === 'x' || field === 'width' ? imageSize.width : imageSize.height} value={bounds[field]}
        onChange={e => setBounds(value => ({ ...value, [field]: e.target.valueAsNumber }))} />
    </label>)}</>}
    <button type="button" onClick={propose}>{t('canvasImageCreateProposal')}</button>
    <p>{t('canvasProposalDoesNotSave')}</p>
    {invalid ? <p role="alert">{t('canvasImageInvalid')}</p> : null}
  </fieldset></details>
}
