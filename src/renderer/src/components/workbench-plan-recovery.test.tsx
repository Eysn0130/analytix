// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { chatBlockFromItem } from '../agent/analytix-mapper'
import { useChatStore } from '../store/chat-store'
import { useGuiPlanStore } from '../plan/plan-store'
import { extractPlanMetadataFromBlock } from '../plan/plan-tool'
import { latestSuccessfulPlanBlockId, resolvePlanResultWorkspaceRoot, useWorkbenchPlanController } from './workbench-plan-controller'

const workspace = '/Users/synthetic/plan-workspace'
const relativePath = '.analytixsdd/plan/accepted.md'
const thread = { id: 'thread-plan', title: 'Synthetic plan', workspace, updatedAt: '2026-09-13T00:00:00Z', model: 'synthetic', mode: 'plan' }
const noop = () => undefined
const translate = (key: string) => key
const send = async () => true
const initialChat = useChatStore.getState()
const initialPlan = useGuiPlanStore.getState()
let root: Root
let container: HTMLDivElement
let read: ReturnType<typeof vi.fn>
let resolveRead: (value: { ok: true; path: string; content: string }) => void
let oldApi: PropertyDescriptor | undefined

function publicPlanBlock() {
  const block = chatBlockFromItem({
    id: 'result-plan', turnId: 'turn-plan', threadId: thread.id, role: 'tool',
    status: 'completed', createdAt: thread.updatedAt, kind: 'tool_result',
    toolName: 'create_plan', callId: 'call-plan',
    output: {
      schemaVersion: 1, projectionKind: 'plan_status', disclosure: 'metadata_only',
      status: 'completed', code: 'plan_updated', messageKey: 'plan_updated',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false,
      plan: { planId: `${workspace}:${relativePath}`, relativePath, operation: 'draft',
        savedAt: thread.updatedAt, contentHash: 'd'.repeat(64), byteSize: 14 }
    }
  })
  if (!block) throw new Error('canonical plan result did not map')
  return block
}

function Harness() {
  useWorkbenchPlanController({ busy: false, mode: 'agent', route: 'chat',
    sendMessage: send, setError: noop, setMode: noop, setRightPanelMode: noop,
    setRightSidebarWidth: noop, t: translate, workspaceRoot: workspace })
  return null
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  localStorage.clear()
  useGuiPlanStore.setState({ ...initialPlan, activePlan: null })
  useChatStore.setState({ ...initialChat, activeThreadId: thread.id, threads: [thread], blocks: [publicPlanBlock()] })
  read = vi.fn(() => new Promise((resolve) => { resolveRead = resolve }))
  oldApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  Object.defineProperty(window, 'analytix', { configurable: true, value: { files: { read } } })
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})

afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  useChatStore.setState(initialChat, true)
  useGuiPlanStore.setState(initialPlan, true)
  if (oldApi) Object.defineProperty(window, 'analytix', oldApi)
  else Reflect.deleteProperty(window, 'analytix')
  vi.unstubAllGlobals()
})

it('loads a real mapped public result using current thread authority without private output fields', async () => {
  const block = publicPlanBlock()
  expect(latestSuccessfulPlanBlockId([block])).toBe(block.id)
  const metadata = extractPlanMetadataFromBlock(block)!
  expect(metadata.workspaceRoot).toBe('')
  expect(resolvePlanResultWorkspaceRoot(metadata, workspace)).toBe(workspace)
  expect(resolvePlanResultWorkspaceRoot(metadata, undefined)).toBeNull()
  expect(resolvePlanResultWorkspaceRoot(metadata, '/tmp/other')).toBeNull()
  expect(resolvePlanResultWorkspaceRoot({ ...metadata, workspaceRoot: '/tmp/injected' }, workspace)).toBeNull()
  await act(async () => root.render(createElement(Harness)))
  expect(read).toHaveBeenCalledExactlyOnceWith({ workspaceRoot: workspace, path: relativePath })
  await act(async () => resolveRead({ ok: true, path: `${workspace}/${relativePath}`, content: 'retained plan' }))
  expect(useGuiPlanStore.getState().activePlan).toMatchObject({ threadId: thread.id, workspaceRoot: workspace, relativePath })
  expect(useGuiPlanStore.getState().content).toBe('retained plan')
})

it.each(['thread', 'workspace'])('discards a pending read after the active %s changes', async (change) => {
  await act(async () => root.render(createElement(Harness)))
  expect(read).toHaveBeenCalledTimes(1)
  await act(async () => useChatStore.setState(change === 'thread'
    ? { activeThreadId: 'different-thread', blocks: [] }
    : { threads: [{ ...thread, workspace: '/tmp/other' }] }))
  await act(async () => resolveRead({ ok: true, path: `${workspace}/${relativePath}`, content: 'stale plan' }))
  expect(useGuiPlanStore.getState().activePlan).toBeNull()
})

it('waits for thread metadata without permanently consuming the result', async () => {
  useChatStore.setState({ threads: [] })
  await act(async () => root.render(createElement(Harness)))
  expect(read).not.toHaveBeenCalled()
  await act(async () => useChatStore.setState({ threads: [thread] }))
  expect(read).toHaveBeenCalledTimes(1)
  await act(async () => resolveRead({ ok: true, path: `${workspace}/${relativePath}`, content: 'retained plan' }))
  expect(useGuiPlanStore.getState().activePlan?.threadId).toBe(thread.id)
})
