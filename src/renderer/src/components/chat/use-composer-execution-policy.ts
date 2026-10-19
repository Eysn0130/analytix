import { useCallback, useEffect, useRef, useState } from 'react'
import { ApprovalPolicySchema, SandboxModeSchema } from '../../../../../packages/runtime/src/contracts/policy'
import type { NormalizedThread, ThreadExecutionPolicy } from '../../agent/types'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import i18n from '../../i18n'
import { confirmDialog } from '../../lib/confirm-dialog'
import { workspaceRootIdentityKey } from '../../lib/workspace-path'
import { useChatStore } from '../../store/chat-store'
import type { ChatState, ThreadSelectionReceipt } from '../../store/chat-store-types'
import type { ComposerExecutionSettings } from './FloatingComposerExecutionPicker'

type PendingPolicy = { token: symbol; ownerKey: string }
type PolicyOperation = { token: symbol; retire: () => void }

function observedPolicy(thread: Pick<NormalizedThread, 'approvalPolicy' | 'sandboxMode'> | null): ComposerExecutionSettings | null {
  if (!thread || !ApprovalPolicySchema.safeParse(thread.approvalPolicy).success || !SandboxModeSchema.safeParse(thread.sandboxMode).success) return null
  return { approvalPolicy: thread.approvalPolicy!, sandboxMode: thread.sandboxMode! }
}

function ownerKey(state: Pick<ChatState, 'activeThreadId' | 'workspaceRoot' | 'route' | 'runtimeConnection' | 'threads'>): string {
  const threadWorkspace = state.threads.find(thread => thread.id === state.activeThreadId)?.workspace
  return JSON.stringify([state.activeThreadId, workspaceRootIdentityKey(state.workspaceRoot), workspaceRootIdentityKey(threadWorkspace), state.route, state.runtimeConnection])
}

function isPreset(policy: ThreadExecutionPolicy): boolean {
  return policy.approvalPolicy === 'never' && policy.sandboxMode === 'read-only' ||
    policy.approvalPolicy === 'on-request' && policy.sandboxMode === 'workspace-write' ||
    policy.approvalPolicy === 'auto' && policy.sandboxMode === 'danger-full-access'
}

/** The visible owner is a persisted thread. Runtime settings only describe an unmaterialized hero. */
export function useComposerExecutionPolicy(): {
  value: ComposerExecutionSettings | null
  legacy: boolean
  applying: boolean
  unavailable: boolean
  onChange: (patch: Partial<ComposerExecutionSettings>) => void
} {
  const activeThreadId = useChatStore(state => state.activeThreadId)
  const workspaceRoot = useChatStore(state => state.workspaceRoot)
  const route = useChatStore(state => state.route)
  const runtimeConnection = useChatStore(state => state.runtimeConnection)
  const thread = useChatStore(state => state.threads.find(item => item.id === state.activeThreadId) ?? null)
  const currentOwner = ownerKey({ activeThreadId, workspaceRoot, route, runtimeConnection, threads: thread ? [thread] : [] })
  const [defaults, setDefaults] = useState<{ ownerKey: string; value: ComposerExecutionSettings | null } | null>(null)
  const [pending, setPending] = useState<PendingPolicy | null>(null)
  const operationRef = useRef<PolicyOperation | null>(null)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      operationRef.current?.retire()
    }
  }, [])

  useEffect(() => {
    if (activeThreadId || route !== 'chat' || runtimeConnection !== 'ready') return
    let retired = false
    const expectedOwner = currentOwner
    const unsubscribe = useChatStore.subscribe((next, previous) => {
      if (ownerKey(next) !== ownerKey(previous)) retired = true
    })
    void rendererRuntimeClient.getSettings().then(settings => {
      if (retired || !mountedRef.current || ownerKey(useChatStore.getState()) !== expectedOwner) return
      setDefaults({ ownerKey: expectedOwner, value: observedPolicy(settings.runtime) })
    }).catch(() => {
      if (!retired && mountedRef.current && ownerKey(useChatStore.getState()) === expectedOwner) {
        setDefaults({ ownerKey: expectedOwner, value: null })
      }
    })
    return () => { retired = true; unsubscribe() }
  }, [activeThreadId, currentOwner, route, runtimeConnection])

  const onChange = useCallback((patch: Partial<ComposerExecutionSettings>): void => {
    const entry = useChatStore.getState()
    if (!mountedRef.current || operationRef.current || entry.route !== 'chat' || entry.runtimeConnection !== 'ready') return
    // A picker emits an explicit complete preset, never a patch over ambient defaults.
    const policy = observedPolicy(patch)
    if (!policy || !isPreset(policy)) return
    const workspace = entry.threads.find(item => item.id === entry.activeThreadId)?.workspace || entry.workspaceRoot
    const workspaceKey = workspaceRootIdentityKey(workspace)
    if (!workspaceKey) return
    const token = Symbol('composer-execution-policy')
    let retired = false
    let acceptedThreadId = entry.activeThreadId
    let plannedThreadId: string | null = null
    let deliberateSelection = false
    let unsubscribe = (): void => undefined
    const operation: PolicyOperation = { token, retire: () => {
      if (retired) return
      retired = true
      unsubscribe()
      if (operationRef.current === operation) operationRef.current = null
      if (mountedRef.current) setPending(previous => previous?.token === token ? null : previous)
    } }
    const isCurrent = (): boolean => {
      const state = useChatStore.getState()
      return !retired && mountedRef.current && operationRef.current === operation && state.activeThreadId === acceptedThreadId &&
        state.route === entry.route && state.runtimeConnection === entry.runtimeConnection &&
        workspaceRootIdentityKey(state.workspaceRoot) === workspaceRootIdentityKey(entry.workspaceRoot) &&
        (!acceptedThreadId || state.threads.some(item => item.id === acceptedThreadId) &&
          workspaceRootIdentityKey(state.threads.find(item => item.id === acceptedThreadId)?.workspace || state.workspaceRoot) === workspaceKey)
    }
    operationRef.current = operation
    setPending({ token, ownerKey: ownerKey(entry) })
    unsubscribe = useChatStore.subscribe((next, previous) => {
      if (next.route !== previous.route || next.runtimeConnection !== previous.runtimeConnection ||
        workspaceRootIdentityKey(next.workspaceRoot) !== workspaceRootIdentityKey(previous.workspaceRoot)) {
        operation.retire()
        return
      }
      if (next.activeThreadId !== previous.activeThreadId) {
        if (!entry.activeThreadId && !deliberateSelection && previous.activeThreadId === null && next.activeThreadId === plannedThreadId && plannedThreadId) {
          deliberateSelection = true
          acceptedThreadId = plannedThreadId
          setPending(previousPending => previousPending?.token === token ? { token, ownerKey: ownerKey(next) } : previousPending)
        } else {
          operation.retire()
          return
        }
      }
      if (acceptedThreadId && (!next.threads.some(item => item.id === acceptedThreadId) ||
        workspaceRootIdentityKey(next.threads.find(item => item.id === acceptedThreadId)?.workspace || next.workspaceRoot) !== workspaceKey)) operation.retire()
    })
    void (async () => {
      try {
        if (policy.approvalPolicy === 'auto' && policy.sandboxMode === 'danger-full-access') {
          const confirmed = await confirmDialog(i18n.t('common:composerPermissionFullAccessConfirmTitle'), i18n.t('common:composerPermissionFullAccessConfirmBody'))
          if (!confirmed || !isCurrent()) return
        }
        if (!isCurrent()) return
        let targetId = entry.activeThreadId
        if (!targetId) {
          const selection: { receipt?: ThreadSelectionReceipt } = {}
          await entry.createThread({ workspaceRoot: workspace, materialize: true, isCurrent,
            beforeSelect: target => {
              if (!isCurrent() || acceptedThreadId !== null || workspaceRootIdentityKey(target.workspace) !== workspaceKey ||
                plannedThreadId && plannedThreadId !== target.threadId) return false
              plannedThreadId = target.threadId
              return true
            },
            onSelected: receipt => {
              if (isCurrent() && receipt.threadId === plannedThreadId) selection.receipt = receipt
            }
          })
          if (!isCurrent() || !selection.receipt?.isCurrent() || selection.receipt.threadId !== plannedThreadId || acceptedThreadId !== plannedThreadId) return
          targetId = selection.receipt.threadId
        }
        if (!targetId || !isCurrent()) return
        // The store owns PATCH/readback, version-2 confirmation and any durable uncertainty.
        await useChatStore.getState().updateThreadExecutionPolicy(targetId, policy)
        if (!isCurrent()) return
      } finally {
        operation.retire()
      }
    })().catch(() => {
      // Store actions already own contextual error reporting. Retired owners never write into a new view.
      operation.retire()
    })
  }, [])

  const value = activeThreadId ? observedPolicy(thread) : defaults?.ownerKey === currentOwner ? defaults.value : null
  const applying = pending?.ownerKey === currentOwner
  const legacy = Boolean(activeThreadId && thread?.executionPolicyVersion !== 2 && value?.approvalPolicy === 'auto' && value.sandboxMode === 'danger-full-access')
  const unavailable = route === 'chat' && (runtimeConnection !== 'ready' || applying || !value || Boolean(thread?.executionPolicyUncertain))
  return { value, legacy, applying, unavailable, onChange }
}
