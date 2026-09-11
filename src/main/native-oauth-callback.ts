import { parseOAuthNativeCallback } from './provider-oauth-lifecycle'

const MAX_PENDING_NATIVE_OAUTH_CALLBACKS = 8

export function nativeOAuthCallbackFromArgv(argv: readonly string[]): string {
  const callbacks = argv.filter((value) => parseOAuthNativeCallback(value) !== null)
  return callbacks.length === 1 ? callbacks[0]! : ''
}

export function createNativeOAuthCallbackRouter() {
  const pending: string[] = []
  let receiver: ((callbackUrl: string) => Promise<void>) | null = null

  const route = (callbackUrl: string): boolean => {
    if (!parseOAuthNativeCallback(callbackUrl)) return false
    if (receiver) {
      void receiver(callbackUrl)
      return true
    }
    if (pending.length >= MAX_PENDING_NATIVE_OAUTH_CALLBACKS || pending.includes(callbackUrl)) return false
    pending.push(callbackUrl)
    return true
  }

  return {
    route,
    async activate(next: (callbackUrl: string) => Promise<void>): Promise<void> {
      if (receiver) throw new Error('OAuth callback authority is already active.')
      receiver = next
      while (pending.length > 0) {
        const callback = pending.shift()!
        await receiver(callback)
      }
    },
    pendingCount: () => pending.length
  }
}
