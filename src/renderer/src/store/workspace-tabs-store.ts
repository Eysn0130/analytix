import { create } from 'zustand'
import { userInputLocalSelectorSchemaV1, type UserInputLocalSelectorV1 } from '../../../../packages/runtime/src/contracts/user-input-local-display'
import type { ToolResultLocalEffectRequestV1, ToolResultLocalPresentFileRequestV1 } from '../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { RightPanelMode } from '../components/chat/WorkbenchTopBar'

export type QuestionPlanTabOwner = { selector: UserInputLocalSelectorV1; callId: string | null }
export function questionPlanWorkspaceTabId(selector: UserInputLocalSelectorV1): string {
  return 'question-plan:' + JSON.stringify(userInputLocalSelectorSchemaV1.parse(selector))
}
export function questionPlanReviewIdentity(owner: QuestionPlanTabOwner): string {
  return JSON.stringify([owner.selector.threadId, owner.callId === null ? 'review:' + owner.selector.requestId : 'call:' + owner.callId])
}
export type WorkspaceTab = {
  id: string
  kind: 'document' | 'file' | 'tool'
  mode: Exclude<RightPanelMode, null>
  title: string
  /** Task navigation only; the shared Host catalog owns the current record. */
  scheduleTaskId?: string
  workspaceRoot?: string
  path?: string
  /** Local display choice, never an authorization grant or provider context. */
  preview?: 'canvas' | 'source'
  questionPlan?: QuestionPlanTabOwner
  /** Metadata-only privacy disposition survives dock/overlay remount. */
  questionPlanRetired?: boolean
  protectedRead?: ToolResultLocalEffectRequestV1
  protectedPresent?: ToolResultLocalPresentFileRequestV1
  sourceLine?: number
  instanceId?: string
  dirty?: boolean
  loading?: boolean
  error?: boolean
}

/** Identities stay in protected local memory; layout storage never contains paths or content. */
export function workspaceObjectTabId(workspaceRoot: string, path: string): string {
  return `object:${JSON.stringify([workspaceRoot.replaceAll('\\', '/').replace(/\/+$/, ''), path.replaceAll('\\', '/')])}`
}

type WorkspaceTabsState = {
  open: boolean
  selectorOpen: boolean
  focused: boolean
  activeTabId: string | null
  tabs: WorkspaceTab[]
  openedQuestionPlanReviews: Record<string, true>
  openQuestionPlanReview: (owner: QuestionPlanTabOwner, title: string, automatic?: boolean) => void
  retireQuestionPlanReview: (owner: QuestionPlanTabOwner) => void
  retireQuestionPlanReviews: () => void
  openTab: (tab: WorkspaceTab) => void
  activateTab: (id: string) => void
  closeTab: (id: string) => void
  patchTab: (id: string, patch: Partial<Omit<WorkspaceTab, 'id'>>) => void
  reorderTab: (id: string, toIndex: number) => void
  showSelector: () => void
  toggleOpen: () => void
  setOpen: (open: boolean) => void
  setFocused: (focused: boolean) => void
}

export const useWorkspaceTabsStore = create<WorkspaceTabsState>((set) => ({
  open: false, selectorOpen: true, focused: false, activeTabId: null, tabs: [], openedQuestionPlanReviews: {},
  openQuestionPlanReview: (owner, title, automatic = false) => set((state) => {
    const selector = userInputLocalSelectorSchemaV1.safeParse(owner.selector)
    if (!selector.success || owner.callId !== null && (owner.callId.trim() !== owner.callId || owner.callId.length === 0 || owner.callId.length > 1024)) return state
    const canonical = { selector: selector.data, callId: owner.callId }, identity = questionPlanReviewIdentity(canonical)
    if (automatic && state.openedQuestionPlanReviews[identity]) return state
    const tab: WorkspaceTab = { id: questionPlanWorkspaceTabId(canonical.selector), kind: 'tool', mode: 'plan', title, questionPlan: canonical }
    return { open: true, selectorOpen: false, activeTabId: tab.id,
      openedQuestionPlanReviews: { ...state.openedQuestionPlanReviews, [identity]: true },
      tabs: state.tabs.some(item => item.id === tab.id) ? state.tabs.map(item => item.id === tab.id ? tab : item) : [...state.tabs, tab] }
  }),
  retireQuestionPlanReview: (owner) => set((state) => {
    const selector = userInputLocalSelectorSchemaV1.safeParse(owner.selector)
    if (!selector.success) return state
    const id = questionPlanWorkspaceTabId(selector.data)
    if (!state.tabs.some(tab => tab.id === id && tab.questionPlan?.callId === owner.callId && !tab.questionPlanRetired)) return state
    return { tabs: state.tabs.map(tab => tab.id === id && tab.questionPlan?.callId === owner.callId && !tab.questionPlanRetired
      ? { ...tab, questionPlanRetired: true } : tab) }
  }),
  retireQuestionPlanReviews: () => set(state => state.tabs.some(tab => tab.questionPlan && !tab.questionPlanRetired)
    ? { tabs: state.tabs.map(tab => tab.questionPlan ? { ...tab, questionPlanRetired: true } : tab) } : state),
  openTab: (tab) => set((state) => ({
    open: true, selectorOpen: false, activeTabId: tab.id,
    tabs: state.tabs.some((item) => item.id === tab.id)
      ? state.tabs.map((item) => item.id === tab.id ? { ...item, ...tab } : item)
      : [...state.tabs, tab]
  })),
  activateTab: (id) => set((state) => state.tabs.some((tab) => tab.id === id)
    ? { activeTabId: id, selectorOpen: false, open: true }
    : state),
  closeTab: (id) => set((state) => {
    const index = state.tabs.findIndex((tab) => tab.id === id)
    if (index < 0) return state
    const tabs = state.tabs.filter((tab) => tab.id !== id)
    const activeTabId = state.activeTabId === id
      ? (tabs[Math.min(index, tabs.length - 1)]?.id ?? null)
      : state.activeTabId
    return { tabs, activeTabId, selectorOpen: tabs.length === 0 || state.selectorOpen }
  }),
  patchTab: (id, patch) => set((state) => ({ tabs: state.tabs.map((tab) => tab.id === id ? { ...tab, ...patch } : tab) })),
  reorderTab: (id, toIndex) => set((state) => {
    const fromIndex = state.tabs.findIndex((tab) => tab.id === id)
    if (fromIndex < 0) return state
    const tabs = [...state.tabs]
    const [tab] = tabs.splice(fromIndex, 1)
    tabs.splice(Math.max(0, Math.min(toIndex, tabs.length)), 0, tab)
    return { tabs }
  }),
  showSelector: () => set({ open: true, selectorOpen: true }),
  toggleOpen: () => set((state) => ({ open: !state.open, focused: false })),
  setOpen: (open) => set({ open, ...(!open ? { focused: false } : {}) }),
  setFocused: (focused) => set({ focused })
}))

// AppShell owns this metadata-only observer even while QuestionPlan leaves are
// unmounted. A global revoke retires inactive tabs before automatic fallback;
// a specific lease revoke remains the mounted native leaf's responsibility.
export function observeQuestionPlanAuthority(
  api: Partial<Pick<typeof window.analytix.runtime, 'onUserInputLocalDisplayInvalidated' | 'onRuntimeStatus'>> | undefined,
  beforeCaseContextChange: (listener: () => void) => () => void
): () => void {
  const retire = () => useWorkspaceTabsStore.getState().retireQuestionPlanReviews()
  const invalidationOff = api?.onUserInputLocalDisplayInvalidated?.((id, reason) => {
    if (id === null && reason !== 'background') retire()
  })
  const runtimeOff = api?.onRuntimeStatus?.(runtime => { if (runtime.state !== 'running') retire() })
  const caseOff = beforeCaseContextChange(retire)
  return () => {
    // Detaching the observer is not a native authority event. In particular,
    // StrictMode effect replay and ordinary routes must not retire valid tabs.
    invalidationOff?.(); runtimeOff?.(); caseOff()
  }
}
