import { useEffect, useRef, useState } from 'react'
import { isShellMotionReduced } from '../../lib/apply-theme'

const DEFAULT_DURATION_MS = 220

type UseShellPanelMotionOptions = {
  isVisible: boolean
  size: number
  durationMs?: number
}

type ShellPanelMotionState = {
  animatedSize: number
  isMounted: boolean
  opacity: number
  progress: number
}

function clampProgress(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(1, value))
}

function easeOutCubic(value: number): number {
  const progress = clampProgress(value)
  return 1 - Math.pow(1 - progress, 3)
}

export function useShellPanelMotion({
  isVisible,
  size,
  durationMs = DEFAULT_DURATION_MS
}: UseShellPanelMotionOptions): ShellPanelMotionState {
  const safeSize = Number.isFinite(size) ? Math.max(0, size) : 0
  const [progress, setProgress] = useState(() => (isVisible ? 1 : 0))
  const [isMounted, setIsMounted] = useState(isVisible)
  const progressRef = useRef(progress)

  useEffect(() => {
    progressRef.current = progress
  }, [progress])

  useEffect(() => {
    const target = isVisible ? 1 : 0

    if (typeof window === 'undefined') {
      progressRef.current = target
      setProgress(target)
      setIsMounted(isVisible)
      return undefined
    }

    if (isVisible) {
      setIsMounted(true)
    }

    const startProgress = clampProgress(progressRef.current)
    if (
      isShellMotionReduced() ||
      durationMs <= 0 ||
      Math.abs(startProgress - target) < 0.001
    ) {
      progressRef.current = target
      setProgress(target)
      if (target === 0) {
        setIsMounted(false)
      }
      return undefined
    }

    const startedAt = performance.now()
    let frameId = 0

    const tick = (now: number): void => {
      const elapsed = clampProgress((now - startedAt) / durationMs)
      const nextProgress = startProgress + (target - startProgress) * easeOutCubic(elapsed)
      progressRef.current = nextProgress
      setProgress(nextProgress)

      if (elapsed < 1) {
        frameId = window.requestAnimationFrame(tick)
        return
      }

      progressRef.current = target
      setProgress(target)
      if (target === 0) {
        setIsMounted(false)
      }
    }

    frameId = window.requestAnimationFrame(tick)
    return () => window.cancelAnimationFrame(frameId)
  }, [durationMs, isVisible])

  const clampedProgress = clampProgress(progress)

  return {
    animatedSize: Math.max(0, safeSize * clampedProgress),
    isMounted,
    opacity: clampedProgress,
    progress: clampedProgress
  }
}
