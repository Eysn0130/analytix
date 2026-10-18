import { useCallback, useEffect, useRef, useState } from 'react'
import type { UserInputLocalSelectorV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import type { UserInputWaitControls, UserInputWaitState } from '../../../../agent/types'
import { getProvider } from '../../../../agent/registry'

// Only subscribes a leaf to the existing provider's request lifetime. Unmount
// removes this observer, never the business claim or another thread's clock.
type WaitDisposition = 'waiting' | 'editing'
type RestoreOwner = { identity: string; isCurrent: () => boolean }
export function useUserInputTimedWait(selector: UserInputLocalSelectorV1 | null | undefined, enabled: boolean, status: string, restoreOwner?: RestoreOwner) {
  const key = selector ? JSON.stringify(selector) : ''
  const currentKey = useRef(key)
  currentKey.current = key
  const controls = useRef<{ key: string; owner: UserInputWaitControls } | null>(null)
  const latest = useRef({ key, enabled, status, restoreOwner })
  latest.current = { key, enabled, status, restoreOwner }
  const pendingRestore = useRef<{ key: string; identity: string; value: WaitDisposition } | null>(null)
  const restore = useCallback((value: WaitDisposition | undefined) => {
    pendingRestore.current = null
    const current = latest.current
    if (value === undefined || !current.enabled || current.status !== 'pending' || !current.restoreOwner?.isCurrent()) return
    const desired = { key: current.key, identity: current.restoreOwner.identity, value }
    const carrier = controls.current
    if (carrier?.key === current.key) {
      if (value === 'waiting') carrier.owner.takeTime()
      else carrier.owner.engage()
    } else pendingRestore.current = desired
  }, [])
  const [state, setState] = useState<{ key: string; value: UserInputWaitState } | null>(null)
  useEffect(() => {
    if (!selector) { pendingRestore.current = null; return }
    const provider = getProvider()
    if (['submitted', 'cancelled', 'continued'].includes(status)) {
      pendingRestore.current = null
      provider.settleUserInputWait?.(selector)
      return
    }
    if (!enabled || status !== 'pending' || !provider.observeUserInputWait) { pendingRestore.current = null; return }
    const owner = provider.observeUserInputWait(selector, value => {
      if (currentKey.current === key) setState({ key, value })
    })
    controls.current = { key, owner }
    const desired = pendingRestore.current, current = latest.current
    pendingRestore.current = null
    if (desired?.key === key && current.key === key && current.enabled && current.status === 'pending' && current.restoreOwner?.identity === desired.identity && current.restoreOwner.isCurrent()) {
      if (desired.value === 'waiting') owner.takeTime()
      else owner.engage()
    }
    const blur = () => owner.blur()
    window.addEventListener('blur', blur)
    return () => { owner.blur(); owner.unsubscribe(); if (controls.current?.owner === owner) controls.current = null; window.removeEventListener('blur', blur) }
    // Complete selector identity is represented by key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, enabled, status])
  useEffect(() => {
    const desired = pendingRestore.current
    if (desired && (desired.key !== key || desired.identity !== restoreOwner?.identity || !enabled || status !== 'pending' || !restoreOwner?.isCurrent())) pendingRestore.current = null
  }, [key, enabled, status, restoreOwner])
  const currentControls = useCallback(() => latest.current.enabled && latest.current.status === 'pending' && controls.current?.key === currentKey.current ? controls.current.owner : null, [])
  const focus = useCallback(() => currentControls()?.focus(), [currentControls])
  const blur = useCallback(() => currentControls()?.blur(), [currentControls])
  const engage = useCallback(() => { pendingRestore.current = null; currentControls()?.engage() }, [currentControls])
  const takeTime = useCallback(() => { pendingRestore.current = null; currentControls()?.takeTime() }, [currentControls])
  const retry = useCallback(() => currentControls()?.retry(), [currentControls])
  return { state: enabled && status === 'pending' && state?.key === key ? state.value : null, focus, blur, engage, takeTime, retry, restore }
}
