import { useEffect, useRef, useState, type ReactElement } from 'react'
import { Shapes } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useChatStore } from '../store/chat-store'
import { openCanvasWorkspaceObject } from './canvas-workspace-open'

export function CanvasOpenButton({ enabled }: { enabled: boolean }): ReactElement {
  const { t } = useTranslation('canvas')
  const threadId = useChatStore(state => state.activeThreadId)
  const workspace = useChatStore(state => state.threads.find(thread => thread.id === state.activeThreadId)?.workspace || state.workspaceRoot)
  const identity = JSON.stringify([threadId, workspace, enabled])
  const current = useRef({ identity, epoch: 0 })
  if (current.current.identity !== identity) current.current = { identity, epoch: current.current.epoch + 1 }
  const mounted = useRef(true), inFlight = useRef(false)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const pick = async (): Promise<void> => {
    if (!enabled || !threadId || !workspace || inFlight.current) return
    const epoch = current.current.epoch
    const stillCurrent = () => mounted.current && current.current.epoch === epoch && current.current.identity === identity
    inFlight.current = true; setBusy(true); setError(null)
    try {
      const result = await window.analytix.canvas.pickFile({ workspace })
      if (!stillCurrent()) return
      if (!result.ok || result.path && !/\.(canvas|png)$/i.test(result.path)) { setError(identity); return }
      if (result.path) openCanvasWorkspaceObject(workspace, result.path)
    } catch { if (stillCurrent()) setError(identity) }
    finally { inFlight.current = false; if (mounted.current) setBusy(false) }
  }
  return <>
    <button type="button" disabled={!enabled || !threadId || !workspace || busy} aria-busy={busy} onClick={() => void pick()}>
      <Shapes size={17} aria-hidden="true" /><span>{t('canvasWorkspace')}</span>
    </button>
    {error === identity ? <p role="alert" className="px-3 text-xs text-ds-muted">{t('canvasPreviewFailed')}</p> : null}
  </>
}
