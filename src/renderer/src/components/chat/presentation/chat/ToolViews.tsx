/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useLayoutEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock, ToolBlock } from '../../../../agent/types'
import { useChatStore } from '../../../../store/chat-store'
import { previewWorkspaceFile } from '../../../../lib/workspace-file-preview'
import { openWorkspacePathInEditor } from '../../../../lib/open-workspace-path'
import { ProtectedToolResult } from '../ProtectedToolResult'
import { SubagentDetailBody, isSubagentProcessBlock, subagentLineText } from '../../SubagentCallCard'
import { ToolAttachmentPreviews } from '../../message-timeline-bubbles'
import { AssistantMarkdown } from '../../AssistantMarkdown'
import { describeProcessBlock, getProcessDetail, publicToolOperationTitle, toolFilePath,
  countPublicPatchStats, type ProcessActivity, type ProcessDetail, type ChatPresentationPolicy } from './chat-view-model'
import { RuntimeDiagnosticsAction, RuntimeMetadata, RuntimeMetadataDetails } from './ToolViewActions'
import { DshAgentPresetOutlineRegular, DshApiOutlineRegular, DshBrowseOutlineRegular, DshCodeOutlineRegular, DshEditOutlineRegular, DshGlobeOutlineRegular, DshSearchOutlineRegular, DshSparkleRegular } from '../DshToolIcons'
import { ToolRow, type ToolRowBody } from './ToolRow'
import { useDisclosure } from './use-disclosure'
import { hasPublicFocus } from './searchable-hidden'
import type { ProcessCaptureSlot } from './ToolCallTree'
import { toolRowState, safeWebHref } from './models/tool-call-model'
import { detailsCardModel } from './models/details-card-model'
import { SearchRow as ProtectedSearchRow } from './toolviews/search-row'
import { ToolDetails } from './toolviews/ToolDetails'
import css from './ToolRow.module.css'
export type ToolFamily = 'read' | 'diff' | 'shell' | 'web' | 'search' | 'plan' | 'subagent' | 'tool'
const TOOL_VIEWS: Readonly<Record<string, ToolFamily>> = {
  read: 'read', read_file: 'read', read_image: 'read', write: 'diff', write_file: 'diff', edit: 'diff', edit_file: 'diff', multi_edit: 'diff', apply_patch: 'diff', create_file: 'diff',
  bash: 'shell', shell: 'shell', terminal: 'shell', run_command: 'shell', exec: 'shell', exec_command: 'shell', write_stdin: 'shell', pwsh: 'shell',
  web_search: 'web', web_fetch: 'web', fetch_url: 'web', browse: 'web',
  grep: 'search', grep_files: 'search', search: 'search', search_files: 'search', find: 'search', glob: 'search', ls: 'search', list: 'search', list_dir: 'search',
  create_plan: 'plan', update_plan: 'plan', todo: 'plan', todo_write: 'plan', task: 'subagent', delegate: 'subagent', parallel: 'subagent', delegate_task: 'subagent', parallel_tasks: 'subagent', subagent: 'subagent'
}
export function toolFamilyFor(block: ToolBlock): ToolFamily {
  return toolFamilyForName(block.meta?.toolName, block.toolKind)
}
export function toolFamilyForName(toolName: unknown, toolKind?: ToolBlock['toolKind']): ToolFamily {
  // Exact producer names and existing typed host kinds select views. Summary,
  // path, timestamps and result prose never classify or correlate a call.
  const name = typeof toolName === 'string' ? toolName.toLowerCase() : ''
  const named = Object.hasOwn(TOOL_VIEWS, name) ? TOOL_VIEWS[name] : undefined
  if (named) return named
  if (toolKind === 'subagent') return 'subagent'
  if (toolKind === 'command_execution') return 'shell'
  if (toolKind === 'file_change') return 'diff'
  return 'tool'
}
const PROCESS_ACTIVITIES: Readonly<Record<string, ProcessActivity>> = {
  read: 'read', read_file: 'read', read_image: 'readImage', write: 'write', write_file: 'write', create_file: 'write',
  edit: 'edit', edit_file: 'edit', multi_edit: 'edit', apply_patch: 'edit',
  bash: 'commands', shell: 'commands', terminal: 'commands', run_command: 'commands', exec: 'commands', exec_command: 'commands', write_stdin: 'commands', pwsh: 'commands',
  web_search: 'webSearch', web_fetch: 'webFetch', fetch_url: 'webFetch', browse: 'webFetch',
  grep: 'search', grep_files: 'search', search: 'search', search_files: 'search', find: 'search', glob: 'search', ls: 'search', list: 'search', list_dir: 'search',
  create_plan: 'plan', update_plan: 'plan', todo: 'plan', todo_write: 'plan', create_goal: 'plan', update_goal: 'plan', get_goal: 'plan',
  task: 'subagents', delegate: 'subagents', parallel: 'subagents', delegate_task: 'subagents', parallel_tasks: 'subagents', subagent: 'subagents',
  user_input: 'questions', ask_user_question: 'questions', request_user_input: 'questions', run_code: 'code'
}
export function processActivityFor(block: ToolBlock): ProcessActivity {
  return processActivityForName(block.meta?.toolName, block.toolKind)
}
export function processActivityForName(toolName: unknown, toolKind?: ToolBlock['toolKind']): ProcessActivity {
  const name = typeof toolName === 'string' ? toolName.toLowerCase() : ''
  if (Object.hasOwn(PROCESS_ACTIVITIES, name)) return PROCESS_ACTIVITIES[name]
  // Pinned presentation naming only; typed view dispatch remains exact in toolFamilyForName.
  // A trailing inspect wins over the terminal/subagent prefixes, as in the fixed pin.
  if (name.endsWith('_inspect') && name.length > '_inspect'.length) return 'search'
  if (name.startsWith('terminal_') && name.length > 'terminal_'.length) return 'commands'
  if (name.startsWith('subagent_') && name.length > 'subagent_'.length) return 'subagents'
  return toolKind === 'command_execution' ? 'commands' : toolKind === 'subagent' ? 'subagents' : 'tools'
}
export function PublicToolBody({ block, detail = getProcessDetail(block), processing = false, error = false }: { block: ChatBlock; detail?: ProcessDetail; processing?: boolean; error?: boolean }) {
  const { t } = useTranslation('common')
  if (block.kind === 'tool' && block.meta?.localResult) return null
  const typed = block.kind === 'tool' ? detailsCardModel(block, t) : null
  return <>
    <RuntimeMetadataDetails block={block} />
    {block.kind !== 'system' ? <RuntimeMetadata block={block} t={t} /> : null}
    {block.kind === 'tool' ? <ToolAttachmentPreviews meta={block.meta} /> : null}
    {isSubagentProcessBlock(block) ? <SubagentDetailBody block={block} /> : typed ? <ToolDetails model={typed} />
      : detail.kind === 'assistant' ? <AssistantMarkdown text={detail.text} streaming={processing} />
      : detail.kind === 'tool' || detail.kind === 'text' ? <div className={css.ioCard}><div className={css.ioSection}>
        <span className={css.ioLabel}>OUT</span><span className={css.ioText} data-error={error || detail.kind === 'tool' && detail.isError || undefined}>{detail.text}</span>
      </div></div> : null}

  </>
}
export type ToolViewProps = { block: ToolBlock; processing: boolean; policy: ChatPresentationPolicy; visible: boolean; publicVisible?: boolean; publicResetKey?: string; resetKey: string; privateGeneration: number; capture?: ProcessCaptureSlot; onInspect?: (turnId: string, callId: string) => void }
export function ToolView({ block, processing, visible, resetKey, publicVisible = visible, publicResetKey = resetKey, privateGeneration, capture, family, onInspect }: ToolViewProps & { family: ToolFamily }) {
  const { t } = useTranslation('common'), workspaceRoot = useChatStore(state => state.workspaceRoot)
  const summary = family === 'subagent' ? subagentLineText(block, t).text : describeProcessBlock(block, t), detail = getProcessDetail(block, summary)
  const publicRoot = useRef<HTMLDivElement>(null), publicReset = useRef(publicResetKey)
  // Preserve a focused public body before a parent's fold can unmount it.
  // Protected descendants cannot hold this public disclosure open.
  const publicFocused = hasPublicFocus(publicRoot.current)
  if (!publicFocused) publicReset.current = publicResetKey
  const disclosure = useDisclosure(block.id + ':' + publicReset.current)
  const shown = publicVisible || publicFocused
  const activeCapture = visible ? capture : undefined
  const privateOpenIntent = useRef(false), privateIntentGeneration = useRef(privateGeneration)
  useLayoutEffect(() => {
    if (!visible || privateIntentGeneration.current !== privateGeneration) privateOpenIntent.current = false
    privateIntentGeneration.current = privateGeneration
    if (visible && privateOpenIntent.current && disclosure.open && capture?.expandable && !capture.expanded) capture.setOpen(true)
  }, [visible, disclosure.open, capture, privateGeneration])
  const title = publicToolOperationTitle(block, t) ?? (family === 'tool' && typeof block.meta?.toolName === 'string' ? block.meta.toolName : t('dshToolFamily.' + family))
  const state = toolRowState(block)
  // Error output is already the projected literal first line. A coincidental
  // title prefix belongs to that output, not to the normal operation label.
  const rowSummary = state === 'error' ? summary
    : summary.startsWith(title + ' ') ? summary.slice(title.length + 1) : summary === title ? '' : summary
  const path = toolFilePath(block), line = typeof block.meta?.line === 'number' && Number.isSafeInteger(block.meta.line) && block.meta.line > 0 ? block.meta.line : undefined
  const href = family === 'web' ? safeWebHref(block.meta?.url) : undefined
  // Legacy public patch counts remain a summary of the received public patch only.
  // They never supply the protected applied-diff model or reconstruct mutation input.
  const publicPatchStats = block.status === 'success' && block.toolKind === 'file_change' && !block.meta?.localResult && detail.kind === 'tool' && detail.isPatch
    && /^--- .+/m.test(detail.text) && /^\+\+\+ .+/m.test(detail.text) && /^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@/m.test(detail.text) ? countPublicPatchStats(detail.text) : undefined
  const Icon = processActivityFor(block) === 'code' ? DshCodeOutlineRegular : ({ read: DshBrowseOutlineRegular, diff: DshEditOutlineRegular, shell: DshApiOutlineRegular, web: DshGlobeOutlineRegular, search: DshSearchOutlineRegular, plan: DshSparkleRegular, subagent: DshAgentPresetOutlineRegular, tool: DshSparkleRegular })[family]
  const props = { icon: <Icon size={14} />, title, toolName: typeof block.meta?.toolName === 'string' ? block.meta.toolName : undefined,
    variant: family, state, summary: rowSummary, summarySuffix: activeCapture?.preview, diffStats: activeCapture?.diffStats ?? publicPatchStats,
    errorSummary: state === 'error' || state === 'stopped' ? rowSummary : undefined, filePath: path, filePathLine: line, href,
    onOpenFile: path ? (filePath: string, fileLine?: number) => previewWorkspaceFile({ path: filePath, line: fileLine, workspaceRoot }) : undefined,
    onOpenFileInEditor: path ? (filePath: string, fileLine?: number) => { void openWorkspacePathInEditor({ path: filePath, line: fileLine }, workspaceRoot) } : undefined,
    actions: <RuntimeDiagnosticsAction block={block} t={t} /> }
  const hasMetadata = ['attachmentIds', 'attachments', 'activeSkillIds', 'injectedMemoryIds', 'sources'].some(key => Array.isArray(block.meta?.[key]) && (block.meta![key] as unknown[]).length > 0)
  const publicBody: ToolRowBody = { kind: 'details', content: <PublicToolBody block={block} detail={detail} processing={processing} error={state === 'error'} /> }
  const toggle = () => { const next = !disclosure.open; privateOpenIntent.current = visible && next; disclosure.setOpen(next); if (activeCapture?.expandable) activeCapture.setOpen(next) }
  // ProcessPane owns the applied mutation result. The existing protected leaf
  // supplies only its independently authorized Inspect action and inspector.
  return <div ref={publicRoot} data-analytix-process-block-id={block.id} data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status} data-tool-view={family}>
    {block.meta?.localResult && block.status !== 'running' && block.toolKind === 'file_change' ? visible ?
      <ProtectedToolResult block={block} preload onInspect={onInspect} renderView={({ inspect, pending }) =>
        <ToolRow {...props} expanded={shown && disclosure.open} expandable={Boolean(activeCapture?.expandable || inspect)} onToggle={toggle} inspect={inspect}
          actions={<>{props.actions}{pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>}
          body={{ kind: 'details', content: activeCapture?.body }} />} /> : <ToolRow {...props} />
      : block.meta?.localResult && block.status !== 'running' ? visible ? <ProtectedToolResult block={block} onInspect={onInspect} renderTrigger={({ open, pending, canOpen, bodyId, onToggle, state: captureState }) =>
      <ToolRow {...props} state={captureState ?? state} expanded={visible && open} expandable={visible && canOpen} externalBodyId={bodyId} onToggle={onToggle}
        actions={<>{props.actions}{pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>} />} /> : <ToolRow {...props} />
      : <ToolRow {...props} expanded={shown && disclosure.open} expandable={isSubagentProcessBlock(block) || hasMetadata || detail.kind !== 'none' || activeCapture?.expandable}
        onToggle={toggle}
        body={{ kind: publicBody.kind, content: <>{activeCapture?.body}{publicBody.content}</> }} />}
  </div>
}
export const ReadRow = (props: ToolViewProps) => <ToolView {...props} family="read" />
export const FileMutationRow = (props: ToolViewProps) => <ToolView {...props} family="diff" />
export { BashRow } from './toolviews/bash-row'
export const WebRow = (props: ToolViewProps) => <ToolView {...props} family="web" />
export function SearchRow(props: ToolViewProps) {
  return <ProtectedSearchRow {...props} renderFallback={rowProps => <ToolView {...rowProps} family="search" />} />
}
export const PlanRow = (props: ToolViewProps) => <ToolView {...props} family="plan" />
export { SubagentRow } from './toolviews/subagent-row'

export const DetailsRow = (props: ToolViewProps) => <ToolView {...props} family="tool" />
