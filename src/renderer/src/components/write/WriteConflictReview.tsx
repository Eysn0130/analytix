import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { openTextObject } from '../../write/object-editing-client'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
import type { WriteConflictComparison } from '../../write/write-workspace-store-types'

export function WriteConflictReview() {
  const { t } = useTranslation('common')
  const conflict = useWriteWorkspaceStore((s) => s.saveStatus === 'conflict')
  const [comparison, setComparison] = useState<WriteConflictComparison | null>(null)
  const [busy, setBusy] = useState(false)
  const dialog = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  useEffect(() => { if (comparison) dialog.current?.showModal() }, [comparison])
  useEffect(() => { if (!conflict) { dialog.current?.close(); setComparison(null) } }, [conflict])
  const compare = async () => {
    const snapshot = useWriteWorkspaceStore.getState()
    if (!snapshot.activeFilePath || !snapshot.objectSession) return
    setBusy(true)
    try {
      const disk = await openTextObject(snapshot.workspaceRoot, snapshot.activeFilePath)
      const current = useWriteWorkspaceStore.getState()
      if (disk.legacy || disk.objectId !== snapshot.objectSession.objectId ||
          current.activeFilePath !== snapshot.activeFilePath || current.workspaceRoot !== snapshot.workspaceRoot ||
          current.fileContent !== snapshot.fileContent || current.saveStatus !== 'conflict') return
      setComparison({ sessionId: disk.sessionId, objectId: disk.objectId, revision: disk.revision,
        workspaceRoot: snapshot.workspaceRoot, path: snapshot.activeFilePath,
        diskContent: disk.content, localContent: snapshot.fileContent })
    } catch (error) {
      useWriteWorkspaceStore.getState().setFileError(error instanceof Error ? error.message : t('writeObjectUnavailable'))
    } finally { setBusy(false) }
  }
  const resolve = async (choice: 'keep-draft' | 'use-disk') => {
    if (!comparison) return
    setBusy(true)
    try {
      if (await useWriteWorkspaceStore.getState().resolveFileConflict(comparison, choice)) {
        dialog.current?.close(); setComparison(null)
      }
    } finally { setBusy(false) }
  }
  if (!conflict && !comparison) return null
  return <>
    <div role="status" className="flex items-center justify-between gap-3 rounded-lg border border-amber-500/30 px-3 py-2 text-sm">
      <span>{t('writeObjectConflict')}</span>
      <button type="button" className="shrink-0 rounded-md border border-ds-border px-3 py-1.5" disabled={busy} onClick={() => void compare()}>{t('writeCompareVersions')}</button>
    </div>
    {comparison && <dialog ref={dialog} aria-labelledby={titleId} onCancel={(event) => { if (busy) event.preventDefault() }} onClose={() => setComparison(null)} className="m-auto w-[min(1000px,94vw)] rounded-xl border border-ds-border bg-ds-card p-5 text-ds-text shadow-xl backdrop:bg-black/40">
      <h2 id={titleId} className="mb-2 text-lg font-semibold">{t('writeCompareVersions')}</h2>
      <p className="mb-4 break-all text-sm text-ds-text-muted">{comparison.path}</p>
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="flex flex-col gap-2 text-sm">{t('writeCurrentDraft')}<textarea readOnly value={comparison.localContent} className="h-[45vh] resize-none rounded-md border border-ds-border bg-transparent p-3 font-mono text-sm" /></label>
        <label className="flex flex-col gap-2 text-sm">{t('writeDiskVersion')}<textarea readOnly value={comparison.diskContent} className="h-[45vh] resize-none rounded-md border border-ds-border bg-transparent p-3 font-mono text-sm" /></label>
      </div>
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        <button type="button" disabled={busy} className="rounded-md px-3 py-2" onClick={() => { dialog.current?.close(); setComparison(null) }}>{t('cancel')}</button>
        <button type="button" disabled={busy} className="rounded-md border border-ds-border px-3 py-2" onClick={() => void resolve('use-disk')}>{t('writeUseDiskVersion')}</button>
        <button type="button" disabled={busy} className="rounded-md bg-accent px-3 py-2 text-white" onClick={() => void resolve('keep-draft')}>{t('writeSaveComparedDraft')}</button>
      </div>
    </dialog>}
  </>
}
