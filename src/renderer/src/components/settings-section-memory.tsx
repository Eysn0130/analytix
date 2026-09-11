import { useMemo, useState } from 'react'
import type { ReactElement } from 'react'
import { Ban, BrainCircuit, Pencil, Plus, Trash2 } from 'lucide-react'
import type { CoreMemoryRecordJson } from '../agent/analytix-contract'
import { SettingsCard, SettingRow } from './settings-controls'

type MemoryScope = 'user' | 'workspace' | 'project'

type MemoryDraft = {
  content: string
  scope: MemoryScope
  tags: string
  confidence: number
}

const EMPTY_DRAFT: MemoryDraft = {
  content: '',
  scope: 'workspace',
  tags: '',
  confidence: 1
}

export function MemorySettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const {
    t,
    memoryRecords,
    memoryDiagnostics,
    createMemoryRecord,
    updateMemoryRecord,
    disableMemoryRecord,
    deleteMemoryRecord
  } = ctx

  const [editingId, setEditingId] = useState<string | null>(null)
  const [draft, setDraft] = useState<MemoryDraft>(EMPTY_DRAFT)
  const [creating, setCreating] = useState(false)
  const [scopeFilter, setScopeFilter] = useState<'all' | MemoryScope>('all')

  const filteredRecords = useMemo(() => {
    const records: CoreMemoryRecordJson[] = memoryRecords ?? []
    if (scopeFilter === 'all') return records
    return records.filter((record) => record.scope === scopeFilter)
  }, [memoryRecords, scopeFilter])

  const beginCreate = (): void => {
    setEditingId(null)
    setDraft(EMPTY_DRAFT)
    setCreating(true)
  }

  const beginEdit = (record: CoreMemoryRecordJson): void => {
    setCreating(false)
    setEditingId(record.id)
    setDraft({
      content: record.content,
      scope: record.scope,
      tags: record.tags.join(', '),
      confidence: record.confidence
    })
  }

  const cancelEditor = (): void => {
    setEditingId(null)
    setCreating(false)
    setDraft(EMPTY_DRAFT)
  }

  const parseTags = (raw: string): string[] =>
    raw
      .split(',')
      .map((tag) => tag.trim())
      .filter(Boolean)

  const saveDraft = async (): Promise<void> => {
    const trimmed = draft.content.trim()
    if (!trimmed) return
    let ok = false
    if (creating) {
      ok = await createMemoryRecord({
        content: trimmed,
        scope: draft.scope,
        tags: parseTags(draft.tags),
        confidence: draft.confidence
      })
    } else if (editingId) {
      ok = await updateMemoryRecord(editingId, {
        content: trimmed,
        tags: parseTags(draft.tags),
        confidence: draft.confidence
      })
    }
    if (ok) cancelEditor()
    // On failure, keep the editor open so the user doesn't lose their draft.
    // The error is surfaced via runtimeDiagnosticsNotice in the parent handler.
  }

  return (
    <SettingsCard title={t('sectionMemory')}>
      <SettingRow
        title={t('memoryOverview')}
        description={t('memoryOverviewDesc')}
        wideControl
        control={
          <div className="grid grid-cols-3 gap-2 text-[12px]">
            <div className="rounded-xl border border-ds-border-muted bg-ds-main/40 px-3 py-2">
              <div className="text-ds-faint">{t('memoryActiveCount')}</div>
              <div className="mt-0.5 font-mono text-[15px] font-semibold text-ds-ink">
                {memoryDiagnostics?.activeCount ?? (Array.isArray(memoryRecords) ? memoryRecords.length : '—')}
              </div>
            </div>
            <div className="rounded-xl border border-ds-border-muted bg-ds-main/40 px-3 py-2">
              <div className="text-ds-faint">{t('memoryTombstoneCount')}</div>
              <div className="mt-0.5 font-mono text-[15px] font-semibold text-ds-ink">
                {memoryDiagnostics?.tombstoneCount ?? '—'}
              </div>
            </div>
            <div className="rounded-xl border border-ds-border-muted bg-ds-main/40 px-3 py-2">
              <div className="text-ds-faint">{t('memoryEnabled')}</div>
              <div className="mt-0.5 font-mono text-[15px] font-semibold text-ds-ink">
                {memoryDiagnostics?.status === 'unavailable' ? t('memoryUnavailable') : t('memoryOn')}
              </div>
            </div>
          </div>
        }
      />

      <SettingRow
        title={t('memoryRecords')}
        description={t('memoryRecordsDesc')}
        wideControl
        control={
          <div className="flex flex-col gap-3">
            {/* Toolbar: scope filter + create button */}
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-1 text-[12px]">
                {(['all', 'user', 'workspace', 'project'] as const).map((scope) => (
                  <button
                    key={scope}
                    type="button"
                    onClick={() => setScopeFilter(scope)}
                    className={`rounded-lg px-2 py-1 font-medium transition ${
                      scopeFilter === scope
                        ? 'bg-ds-ink text-ds-main'
                        : 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
                    }`}
                  >
                    {t(`memoryScope_${scope}`)}
                  </button>
                ))}
              </div>
              <button
                type="button"
                onClick={beginCreate}
                className="inline-flex items-center gap-1.5 rounded-lg bg-ds-ink px-2.5 py-1.5 text-[12px] font-semibold text-ds-main transition hover:opacity-85"
              >
                <Plus className="h-3.5 w-3.5" strokeWidth={2} />
                {t('memoryCreate')}
              </button>
            </div>

            {/* Editor */}
            {(creating || editingId !== null) && (
              <div className="rounded-xl border border-ds-border bg-ds-surface-subtle px-3 py-3">
                <div className="mb-2 flex items-center gap-2 text-[13px] font-semibold text-ds-ink">
                  <Pencil className="h-3.5 w-3.5" strokeWidth={1.8} />
                  {creating ? t('memoryCreateTitle') : t('memoryEditTitle')}
                </div>
                <textarea
                  value={draft.content}
                  onChange={(e) => setDraft((prev) => ({ ...prev, content: e.target.value }))}
                  rows={3}
                  placeholder={t('memoryContentPlaceholder')}
                  className="w-full resize-y rounded-lg border border-ds-border-muted bg-ds-main px-2.5 py-2 text-[13px] text-ds-ink outline-none focus:border-ds-ink/40"
                />
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  {creating && (
                    <select
                      value={draft.scope}
                      onChange={(e) => setDraft((prev) => ({ ...prev, scope: e.target.value as MemoryScope }))}
                      className="rounded-lg border border-ds-border-muted bg-ds-main px-2 py-1 text-[12px] text-ds-ink outline-none"
                    >
                      <option value="user">{t('memoryScope_user')}</option>
                      <option value="workspace">{t('memoryScope_workspace')}</option>
                      <option value="project">{t('memoryScope_project')}</option>
                    </select>
                  )}
                  <input
                    type="text"
                    value={draft.tags}
                    onChange={(e) => setDraft((prev) => ({ ...prev, tags: e.target.value }))}
                    placeholder={t('memoryTagsPlaceholder')}
                    className="min-w-[120px] flex-1 rounded-lg border border-ds-border-muted bg-ds-main px-2 py-1 text-[12px] text-ds-ink outline-none"
                  />
                  <div className="flex items-center gap-1 text-[12px] text-ds-faint">
                    <span>{t('memoryConfidence')}</span>
                    <input
                      type="number"
                      min={0}
                      max={1}
                      step={0.1}
                      value={draft.confidence}
                      onChange={(e) =>
                        setDraft((prev) => ({ ...prev, confidence: Number(e.target.value) || 0 }))
                      }
                      className="w-14 rounded-lg border border-ds-border-muted bg-ds-main px-1.5 py-1 text-[12px] text-ds-ink outline-none"
                    />
                  </div>
                </div>
                <div className="mt-2 flex justify-end gap-2">
                  <button
                    type="button"
                    onClick={cancelEditor}
                    className="rounded-lg px-2.5 py-1 text-[12px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                  >
                    {t('memoryCancel')}
                  </button>
                  <button
                    type="button"
                    onClick={() => void saveDraft()}
                    disabled={!draft.content.trim()}
                    className="rounded-lg bg-ds-ink px-2.5 py-1 text-[12px] font-semibold text-ds-main transition hover:opacity-85 disabled:cursor-not-allowed disabled:opacity-45"
                  >
                    {t('memorySave')}
                  </button>
                </div>
              </div>
            )}

            {/* List */}
            {filteredRecords.length === 0 && !creating && editingId === null ? (
              <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-ds-border-muted bg-ds-main/40 px-3 py-8 text-center">
                <BrainCircuit className="h-6 w-6 text-ds-faint" strokeWidth={1.5} />
                <div className="text-[13px] text-ds-faint">{t('memoryEmpty')}</div>
              </div>
            ) : (
              filteredRecords.map((memory) =>
                editingId === memory.id ? null : (
                  <div
                    key={memory.id}
                    className={`rounded-xl border px-3 py-2 transition ${
                      memory.disabledAt
                        ? 'border-ds-border-muted bg-ds-main/20 opacity-60'
                        : 'border-ds-border-muted bg-ds-main/40'
                    }`}
                  >
                    <div className="flex min-w-0 items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="whitespace-pre-wrap break-words text-[13px] font-medium text-ds-ink">
                          {memory.content}
                        </div>
                        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-ds-faint">
                          <span className="rounded bg-ds-hover/60 px-1.5 py-0.5 font-medium">{memory.scope}</span>
                          {memory.confidence !== undefined && memory.confidence !== 1 && (
                            <span className="font-mono">★ {memory.confidence.toFixed(2)}</span>
                          )}
                          {memory.tags?.length ? (
                            <span>{memory.tags.join(' · ')}</span>
                          ) : null}
                          {memory.disabledAt ? <span className="text-amber-600">{t('memoryDisabled')}</span> : null}
                          <span className="font-mono opacity-60">{memory.id.slice(0, 8)}</span>
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-0.5">
                        <button
                          type="button"
                          onClick={() => beginEdit(memory)}
                          className="rounded-lg p-1.5 text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                          aria-label={t('memoryEdit')}
                          title={t('memoryEdit')}
                        >
                          <Pencil className="h-3.5 w-3.5" strokeWidth={1.8} />
                        </button>
                        <button
                          type="button"
                          disabled={Boolean(memory.disabledAt)}
                          onClick={() => void disableMemoryRecord(memory.id)}
                          className="rounded-lg p-1.5 text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45"
                          aria-label={t('memoryDisable')}
                          title={t('memoryDisable')}
                        >
                          <Ban className="h-3.5 w-3.5" strokeWidth={1.8} />
                        </button>
                        <button
                          type="button"
                          onClick={() => void deleteMemoryRecord(memory.id)}
                          className="rounded-lg p-1.5 text-ds-muted transition hover:bg-red-500/10 hover:text-red-600"
                          aria-label={t('memoryDelete')}
                          title={t('memoryDelete')}
                        >
                          <Trash2 className="h-3.5 w-3.5" strokeWidth={1.8} />
                        </button>
                      </div>
                    </div>
                  </div>
                )
              )
            )}
          </div>
        }
      />
    </SettingsCard>
  )
}
