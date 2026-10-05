/** Shared only by renderer action factories for one main-thread subscription.
 * Never persisted or sent across the desktop bridge. Natural EOF keeps the
 * generation. New submissions advance a separate counter even when reusing a
 * healthy stream; replacement and explicit cancellation revoke pending HTTP work. */
type ThreadBinding = { generation: number; submissionGeneration: number }
const bindings = new WeakMap<object, ThreadBinding>()

export function getThreadBinding(owner: { current: AbortController | null }): ThreadBinding {
  let binding = bindings.get(owner)
  if (!binding) {
    binding = { generation: 0, submissionGeneration: 0 }
    bindings.set(owner, binding)
  }
  return binding
}
