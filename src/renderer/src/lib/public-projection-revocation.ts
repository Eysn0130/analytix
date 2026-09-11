import type { NormalizedThread } from '../agent/types'

// A revocation is terminal for this renderer session. The runtime must be
// queried again in a fresh session before any projection for this thread can
// become visible; a slow response cannot silently clear this tombstone.
const revokedCaseProjectionThreadIds = new Set<string>()

function canonicalThreadId(threadId: string | null | undefined): string {
  return typeof threadId === 'string' && threadId.trim() === threadId
    ? threadId
    : ''
}

export function markPublicProjectionRevoked(threadId: string): void {
  const canonical = canonicalThreadId(threadId)
  if (canonical) revokedCaseProjectionThreadIds.add(canonical)
}

export function isPublicProjectionRevoked(threadId: string | null | undefined): boolean {
  const canonical = canonicalThreadId(threadId)
  return Boolean(canonical && revokedCaseProjectionThreadIds.has(canonical))
}

export function threadReferencesRevokedProjection(
  thread: Pick<NormalizedThread, 'id' | 'parentThreadId' | 'forkedFromThreadId'>,
  revokedThreadId?: string
): boolean {
  const candidates = [thread.id, thread.parentThreadId, thread.forkedFromThreadId]
  if (revokedThreadId) return candidates.includes(revokedThreadId)
  return candidates.some((threadId) => isPublicProjectionRevoked(threadId))
}

export function filterRevokedPublicProjectionThreads<T extends Pick<NormalizedThread, 'id' | 'parentThreadId' | 'forkedFromThreadId'>>(
  threads: readonly T[]
): T[] {
  return threads.filter((thread) => !threadReferencesRevokedProjection(thread))
}

export function resetPublicProjectionRevocationsForTests(): void {
  revokedCaseProjectionThreadIds.clear()
}
