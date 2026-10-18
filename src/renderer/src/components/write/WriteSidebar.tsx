import type { FormEvent, ReactElement } from 'react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useShallow } from 'zustand/react/shallow'
import { FolderOpen } from '../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import type { WorkspaceEntry } from '@shared/workspace-file'
import type { ComposerFileReference } from '../../lib/composer-file-references'
import { composerFileReferenceFromPath } from '../../lib/composer-file-references'
import { previewWorkspaceFile } from '../../lib/workspace-file-preview'
import { formatWorkspacePickerError } from '../../lib/format-workspace-picker-error'
import { useWriteWorkspaceStore, writeWorkspaceHasUnsettledDraft, writeJoinPath } from '../../write/write-workspace-store'
import { SidebarCommandRow } from '../sidebar/SidebarPrimitives'
import { WriteFileTree } from './WriteFileTree'

type EntryDialog =
  | {kind:'create-file';parentDirectory?:string;value:string}
  | {kind:'create-folder';parentDirectory?:string;value:string}
  | {kind:'rename';entry:WorkspaceEntry;value:string}
  | {kind:'delete';entry:WorkspaceEntry}
type Translate = (key:string,opts?:Record<string,unknown>) => string

/** Generic file browser. Its temporary root never changes writing settings or a retained draft. */
export function WriteSidebar({initialWorkspaceRoot,selectedPath,onOpenFile,onAddReference}: {
  initialWorkspaceRoot?:string;selectedPath?:string | null
  onOpenFile?:(workspaceRoot:string,path:string) => void
  onAddReference?:(reference:ComposerFileReference & {type:'file' | 'directory'}) => void
} = {}): ReactElement {
  const { t } = useTranslation('common')
  const storedRoot = useWriteWorkspaceStore(state => state.workspaceRoot)
  const [root,setRoot] = useState(initialWorkspaceRoot || storedRoot)
  const currentRoot = useRef(root);currentRoot.current = root
  const mounted = useRef(true), operation = useRef(0), inFlight = useRef(false)
  const [entryDialog,setEntryDialog] = useState<EntryDialog | null>(null)
  const [error,setError] = useState<string | null>(null)
  const [busy,setBusy] = useState(false)
  const {entriesByDir,expandedDirs,loadingDirs,treeError,activeFilePath,loadDirectory,toggleDirectory,createFile,createDirectory,renameEntry,deleteEntry} = useWriteWorkspaceStore(useShallow(state => ({
    entriesByDir:state.entriesByDir,expandedDirs:state.expandedDirs,loadingDirs:state.loadingDirs,treeError:state.treeError,
    activeFilePath:state.activeFilePath,loadDirectory:state.loadDirectory,toggleDirectory:state.toggleDirectory,
    createFile:state.createFile,createDirectory:state.createDirectory,renameEntry:state.renameEntry,deleteEntry:state.deleteEntry
  })))
  useEffect(() => { mounted.current = true;return () => { mounted.current = false;operation.current++ } }, [])
  useEffect(() => { operation.current++;setRoot(initialWorkspaceRoot || storedRoot);setEntryDialog(null);setError(null) }, [initialWorkspaceRoot,storedRoot])
  useEffect(() => { if (root.trim()) void loadDirectory(root,root) }, [root,loadDirectory])
  const refresh = async (owner = root): Promise<void> => {
    await loadDirectory(owner,owner)
    await Promise.all([...useWriteWorkspaceStore.getState().expandedDirs].filter(path => path.startsWith(owner + '/')).map(path => loadDirectory(owner,path)))
  }
  const open = (owner:string,path:string): void => {
    if (onOpenFile) onOpenFile(owner,path)
    else previewWorkspaceFile({workspaceRoot:owner,path})
  }
  const pickWorkspace = async (): Promise<void> => {
    const attempt = ++operation.current
    try {
      setError(null)
      if (typeof window.analytix?.workspace?.pickDirectory !== 'function') throw new Error('workspace:pick-directory unavailable')
      const picked = await window.analytix.workspace.pickDirectory(root || undefined)
      if (mounted.current && attempt === operation.current && !picked.canceled && picked.path) { setRoot(picked.path);setEntryDialog(null) }
    } catch (failure) { if (mounted.current && attempt === operation.current) setError(formatWorkspacePickerError(failure)) }
  }
  const createEntry = (kind:'create-file' | 'create-folder',parentDirectory?:string): void => {
    if (!root.trim()) { void pickWorkspace();return }
    setEntryDialog({kind,parentDirectory:parentDirectory || root,value:kind === 'create-file' ? 'untitled.md' : 'new-folder'})
  }
  const submitEntryDialog = async (event:FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault()
    if (!entryDialog || inFlight.current || useWriteWorkspaceStore.getState().shutdownFrozen) return
    const dialog = entryDialog, owner = root, attempt = operation.current
    const draft = useWriteWorkspaceStore.getState()
    const normalize = (path:string) => path.replaceAll('\\','/').replace(/\/+$/,'')
    if ((dialog.kind === 'delete' || dialog.kind === 'rename') && draft.activeFilePath &&
      writeWorkspaceHasUnsettledDraft(draft)) {
      const active = normalize(draft.activeFilePath), affected = normalize(dialog.entry.path)
      if (active === affected || active.startsWith(affected + '/')) { setError(t('workspaceLegacyDraftDescription'));return }
    }
    const current = () => mounted.current && currentRoot.current === owner && operation.current === attempt
    const value = dialog.kind === 'delete' ? '' : dialog.value.trim()
    if (dialog.kind !== 'delete' && !value) return
    inFlight.current = true;setBusy(true);setError(null)
    try {
      let completed = false, created:string | null = null
      if (dialog.kind === 'delete') completed = await deleteEntry(owner,dialog.entry.path)
      else if (dialog.kind === 'rename') completed = dialog.entry.name === value || !!(await renameEntry(owner,dialog.entry.path,value!))
      else if (dialog.kind === 'create-file') {
        // Keep the existing shutdown/permission guards, without opening the old Write adapter.
        created = await createFile(owner,writeJoinPath(dialog.parentDirectory || owner,value!), '', {openAfterCreate:false})
        completed = !!created
      } else completed = !!(await createDirectory(owner,writeJoinPath(dialog.parentDirectory || owner,value!)))
      if (completed) {
        await refresh(owner)
        if (current()) { setEntryDialog(null);if (created) open(owner,created) }
      }
    } catch (failure) { if (current()) setError(failure instanceof Error ? failure.message : String(failure)) }
    finally { inFlight.current = false;if (mounted.current) setBusy(false) }
  }
  const portal = typeof document === 'undefined' ? null : document.body
  return <>
    <div className="flex h-full min-h-0 flex-col overflow-hidden" aria-busy={busy}>
      <SidebarCommandRow icon={<FolderOpen className="h-4 w-4" strokeWidth={1.75} />} label={t('writeAddWorkspace')} onClick={() => { void pickWorkspace() }} />
      {error ? <p role="alert" className="p-2 text-xs text-ds-muted">{error}</p> : null}
      <WriteFileTree rootDirectory={root} entriesByDir={entriesByDir} expandedDirs={expandedDirs} loadingDirs={loadingDirs}
        selectedFilePath={selectedPath ?? activeFilePath} error={treeError} rootLoading={!!loadingDirs[root] || !!root && !entriesByDir[root]}
        onToggleDir={path => { void toggleDirectory(root,path) }} onSelectFile={path => open(root,path)}
        onAddReference={onAddReference ? entry => onAddReference({...composerFileReferenceFromPath(entry.path,root),type:entry.type}) : undefined}
        onCreateFile={parent => createEntry('create-file',parent)} onCreateDirectory={parent => createEntry('create-folder',parent)}
        onRenameEntry={entry => setEntryDialog({kind:'rename',entry,value:entry.name})} onDeleteEntry={entry => setEntryDialog({kind:'delete',entry})}
        onRefresh={() => { void refresh() }} />
    </div>
    {entryDialog && portal ? createPortal(<WriteEntryDialog dialog={entryDialog} onClose={() => { if (!inFlight.current) setEntryDialog(null) }}
      onValueChange={value => { if (!inFlight.current) setEntryDialog(current => current?.kind === 'delete' || !current ? current : {...current,value}) }}
      onSubmit={event => { void submitEntryDialog(event) }} t={t} />,portal) : null}
  </>
}

function entryDialogTitle(dialog: EntryDialog, t: Translate): string {
  if (dialog.kind === 'create-file') return t('writeCreateFile')
  if (dialog.kind === 'create-folder') return t('writeCreateFolder')
  if (dialog.kind === 'rename') return t('writeRenameEntry')
  return dialog.entry.type === 'directory' ? t('writeDeleteFolder') : t('writeDeleteFile')
}

function entryDialogSubmitLabel(dialog: EntryDialog, t: Translate): string {
  if (dialog.kind === 'rename') return t('writeEntryDialogRename')
  if (dialog.kind === 'delete') return t('writeEntryDialogDelete')
  return t('writeEntryDialogCreate')
}

function entryDialogDescription(dialog: EntryDialog, t: Translate): string {
  if (dialog.kind === 'delete') {
    return dialog.entry.type === 'directory'
      ? t('writeDeleteFolderConfirm', { name: dialog.entry.name })
      : t('writeDeleteFileConfirm', { name: dialog.entry.name })
  }
  if (dialog.kind === 'rename') return t('writeRenameEntryPrompt')
  if (dialog.kind === 'create-file') return t('writeCreateFilePrompt')
  return t('writeCreateFolderPrompt')
}

function WriteEntryDialog({
  dialog,
  onClose,
  onValueChange,
  onSubmit,
  t
}: {
  dialog: EntryDialog
  onClose: () => void
  onValueChange: (value: string) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  t: Translate
}): ReactElement {
  const deleting = dialog.kind === 'delete'
  return (
    <div
      className="ds-no-drag fixed inset-0 z-[80] flex items-center justify-center bg-slate-950/18 px-4 backdrop-blur-[2px] dark:bg-black/35"
      onMouseDown={onClose}
    >
      <form
        onSubmit={onSubmit}
        onMouseDown={(event) => event.stopPropagation()}
        className="w-full max-w-sm rounded-[24px] border border-ds-border bg-ds-card p-5 shadow-[0_24px_72px_rgba(20,47,95,0.22)]"
      >
        <h2 className="text-[18px] font-semibold tracking-[-0.035em] text-ds-ink">
          {entryDialogTitle(dialog, t)}
        </h2>
        <p className="mt-2 text-[13px] leading-6 text-ds-muted">
          {entryDialogDescription(dialog, t)}
        </p>
        {!deleting ? (
          <input
            autoFocus
            value={dialog.value}
            onChange={(event) => onValueChange(event.target.value)}
            className="mt-4 w-full rounded-xl border border-ds-border bg-ds-main/65 px-3 py-2 text-[14px] text-ds-ink outline-none transition focus:border-accent/40 focus:ring-1 focus:ring-accent/25"
          />
        ) : null}
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
          >
            {t('writeEntryDialogCancel')}
          </button>
          <button
            type="submit"
            className={`rounded-xl px-3 py-2 text-[13px] font-semibold text-white transition hover:brightness-110 ${
              deleting ? 'bg-red-500' : 'bg-accent'
            }`}
          >
            {entryDialogSubmitLabel(dialog, t)}
          </button>
        </div>
      </form>
    </div>
  )
}
