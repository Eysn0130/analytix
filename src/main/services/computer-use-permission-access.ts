import type { ComputerUsePermissionKind } from '../../shared/analytix-api'

type NativePermissionHelper = {
  getAuthStatus?: (kind: 'accessibility') => unknown
  askForAccessibilityAccess?: () => unknown
  askForScreenCaptureAccess?: (openSettings: boolean) => unknown
}

export type PermissionEnrollmentPorts = {
  importNative: () => Promise<unknown>
  promptAccessibility: () => unknown
  promptScreenCapture: () => Promise<unknown>
  openScreenSettings: () => Promise<unknown>
}

/** A process-local session: import once, enroll on demand, never cache a grant. */
export function createPermissionEnrollment(ports: PermissionEnrollmentPorts) {
  let importAttempt: Promise<NativePermissionHelper | null> | undefined
  const helper = (): Promise<NativePermissionHelper | null> => {
    importAttempt ??= Promise.resolve().then(ports.importNative).then((loaded) => {
      const namespace = loaded as { default?: unknown } | null
      return (namespace?.default ?? loaded) as NativePermissionHelper | null
    }).catch(() => null)
    return importAttempt
  }

  const enrollment: Record<ComputerUsePermissionKind, {
    method: 'askForAccessibilityAccess' | 'askForScreenCaptureAccess'
    arguments: unknown[]
    fallback: () => unknown
    continueAfterFallbackFailure: boolean
    after: Array<() => Promise<unknown>>
  }> = {
    accessibility: {
      method: 'askForAccessibilityAccess', arguments: [],
      fallback: ports.promptAccessibility, continueAfterFallbackFailure: false, after: []
    },
    screenRecording: {
      method: 'askForScreenCaptureAccess', arguments: [true],
      fallback: ports.promptScreenCapture, continueAfterFallbackFailure: true,
      after: [ports.openScreenSettings]
    }
  }

  return {
    async configuredAccessibility(): Promise<unknown> {
      return (await helper())?.getAuthStatus?.('accessibility')
    },
    async request(kind: ComputerUsePermissionKind, platform: NodeJS.Platform): Promise<void> {
      if (platform !== 'darwin' || !Object.hasOwn(enrollment, kind)) return
      const plan = enrollment[kind]
      const native = await helper()
      try {
        const prompt = native?.[plan.method]
        if (typeof prompt === 'function') {
          // Native prompts retain their nonblocking UI behavior. Consume an
          // asynchronous rejection without exposing it as an unhandled error.
          void Promise.resolve(Reflect.apply(prompt, native, plan.arguments)).catch(() => undefined)
        } else {
          const attempt = Promise.resolve().then(plan.fallback)
          await (plan.continueAfterFallbackFailure ? attempt.catch(() => undefined) : attempt)
        }
        for (const action of plan.after) await action()
      } catch {
        // Enrollment is best effort; callers report fresh OS status afterward.
      }
    }
  }
}
