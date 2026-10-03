/** Shared only by renderer action factories for one main-thread subscription.
 * Never persisted or sent across the desktop bridge. Natural EOF keeps the
 * generation; replacement and explicit cancellation revoke pending HTTP work. */
const bindings = new WeakMap<object, { generation: number }>()

export function getThreadBinding(owner: { current: AbortController | null }): { generation: number } {
  let binding = bindings.get(owner)
  if (!binding) {
    binding = { generation: 0 }
    bindings.set(owner, binding)
  }
  return binding
}
