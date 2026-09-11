import { useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import './startup-auth.css'

type StartupSplashProps = {
  brand?: string
  className?: string
  durationMs?: number
  freezeProgress?: number | null
  loop?: boolean
  loopDelayMs?: number
  onComplete?: () => void
  onProgressSettled?: () => void
  progressComplete?: boolean
  progressContext?: 'boot' | 'install' | 'update' | 'uninstall'
  progressDetail?: string
  progressPercent?: number | null
  progressSurface?: ReactNode
  showProgressBar?: boolean
}

const BASE_TIMELINE_MS = 4000
const BASE_SWEEP_MS = 2000
const FIRST_REVEAL_DELAY_MS = 220
const LAST_REVEAL_PADDING_MS = 320
const LETTER_REVEAL_DURATION_MS = 720
const BRAND_MARK_REVEAL_MS = 640
const BRAND_MARK_TRIGGER_RATIO = 0.995
const STARTUP_PROGRESS_WAITING_CAP_PERCENT = 99.5
const STARTUP_PROGRESS_TRANSITION_FALLBACK_MS = 720

const PROGRESS_COPY = {
  boot: {
    title: 'SYSTEM LOADING....',
    complete: 'ANALYTIX READY · 加载完成 analytix',
    messages: [
      'PREPARING LOCAL SERVICES',
      'READING LOCAL PROVIDER REGISTRY',
      'VERIFYING SELECTED PROVIDER',
      'WARMING WORKBENCH SHELL',
      'PRELOADING LOCAL SETUP SURFACE'
    ]
  },
  install: {
    title: 'INSTALLING ANALYTIX....',
    complete: 'INSTALLATION COMPLETE',
    messages: ['PREPARING INSTALL PACKAGE', 'VERIFYING LOCAL RUNTIME', 'STARTING LOCAL SERVICES']
  },
  update: {
    title: 'UPDATING ANALYTIX....',
    complete: 'UPDATE COMPLETE',
    messages: ['PREPARING UPDATE PACKAGE', 'VALIDATING UPDATE', 'FINALIZING UPDATE']
  },
  uninstall: {
    title: 'UNINSTALLING ANALYTIX....',
    complete: 'UNINSTALL COMPLETE',
    messages: ['STOPPING LOCAL SERVICES', 'REMOVING APPLICATION FILES', 'FINALIZING REMOVAL']
  }
} as const

function clamp(value: number, min = 0, max = 100): number {
  return Math.min(max, Math.max(min, value))
}

function clampRatio(value: number): number {
  return Math.min(1, Math.max(0, value))
}

function notifyStartupSurfaceReady(): void {
  window.requestAnimationFrame(() => {
    document.body.classList.add('analytix-react-ready')
    window.requestAnimationFrame(() => {
      window.setTimeout(() => {
        void window.analytix?.app?.startupSurfaceReady?.()
      }, 0)
    })
  })
}

export function StartupSplash({
  brand = 'Analytix',
  className,
  durationMs = 3200,
  freezeProgress = null,
  loop = false,
  loopDelayMs = 0,
  onComplete,
  onProgressSettled,
  progressComplete = false,
  progressContext = 'boot',
  progressDetail,
  progressPercent = null,
  progressSurface,
  showProgressBar = false
}: StartupSplashProps): React.ReactElement {
  const letters = useMemo(() => Array.from(brand), [brand])
  const shouldRenderBrandMark = brand.trim().toLowerCase() === 'analytix'
  const brandMarkLetterIndex = useMemo(() => {
    let matchIndex = -1
    letters.forEach((letter, index) => {
      if (letter.toLowerCase() === 'i') matchIndex = index
    })
    return matchIndex
  }, [letters])
  const visibleLetterIndices = useMemo(
    () => letters.reduce<number[]>((result, letter, index) => {
      if (letter.trim()) result.push(index)
      return result
    }, []),
    [letters]
  )
  const progressCopy = PROGRESS_COPY[progressContext]
  const [messageIndex, setMessageIndex] = useState(0)
  const [progressValue, setProgressValue] = useState(() => (progressComplete ? 100 : progressPercent ?? 0))
  const progressBarRef = useRef<HTMLDivElement | null>(null)
  const externalProgressPercent = typeof progressPercent === 'number' && Number.isFinite(progressPercent)
    ? clamp(progressPercent)
    : null
  const playState = freezeProgress === null ? 'running' : 'paused'
  const iterationCount = loop ? 'infinite' : '1'
  const persistFinalState = !loop && freezeProgress === null
  const visibleLetterCount = Math.max(visibleLetterIndices.length, 1)
  const safeDurationMs = Math.max(durationMs, 200)
  const sweepMs = Math.max(Math.round((safeDurationMs / BASE_TIMELINE_MS) * BASE_SWEEP_MS), 120)
  const letterRevealMs = Math.max(Math.round((safeDurationMs / BASE_TIMELINE_MS) * LETTER_REVEAL_DURATION_MS), 240)
  const brandMarkRevealMs = Math.max(Math.round((safeDurationMs / BASE_TIMELINE_MS) * BRAND_MARK_REVEAL_MS), 260)
  const revealWindowMs = Math.max(sweepMs - FIRST_REVEAL_DELAY_MS - LAST_REVEAL_PADDING_MS, 0)
  const finalLetterDelayMs = visibleLetterCount === 1
    ? FIRST_REVEAL_DELAY_MS
    : Math.round(FIRST_REVEAL_DELAY_MS + revealWindowMs)
  const phaseMs = freezeProgress === null ? 0 : -Math.round(clampRatio(freezeProgress) * safeDurationMs)
  const phaseSweepMs = freezeProgress === null ? 0 : -Math.round(clampRatio(freezeProgress) * sweepMs)
  const brandMarkDelayMs = Math.round(
    Math.max(sweepMs * BRAND_MARK_TRIGGER_RATIO, finalLetterDelayMs + letterRevealMs * 0.12)
  )

  useEffect(() => {
    notifyStartupSurfaceReady()
  }, [])

  useEffect(() => {
    if (freezeProgress !== null || loop || !onComplete) return
    const timer = window.setTimeout(() => {
      onComplete?.()
    }, safeDurationMs + Math.max(loopDelayMs, 0))
    return () => window.clearTimeout(timer)
  }, [freezeProgress, loop, loopDelayMs, onComplete, safeDurationMs])

  useEffect(() => {
    setMessageIndex(0)
  }, [progressContext, showProgressBar])

  useEffect(() => {
    if (!showProgressBar || progressComplete || typeof progressPercent === 'number') return
    const timer = window.setInterval(() => {
      setMessageIndex((current) => current + 1)
    }, 620)
    return () => window.clearInterval(timer)
  }, [progressComplete, progressPercent, showProgressBar])

  useEffect(() => {
    if (!showProgressBar) return
    if (externalProgressPercent !== null) {
      setProgressValue((current) => Math.max(current, externalProgressPercent))
      return
    }
    if (progressComplete) {
      setProgressValue(100)
      return
    }
  }, [externalProgressPercent, progressComplete, showProgressBar])

  useEffect(() => {
    if (!showProgressBar || progressComplete) return
    const timer = window.setInterval(() => {
      setProgressValue((current) => {
        const floor = externalProgressPercent ?? 0
        const currentValue = Math.max(current, floor)
        if (currentValue >= STARTUP_PROGRESS_WAITING_CAP_PERCENT) return currentValue
        return Math.min(
          STARTUP_PROGRESS_WAITING_CAP_PERCENT,
          currentValue + Math.max((STARTUP_PROGRESS_WAITING_CAP_PERCENT - currentValue) * 0.04, 0.08)
        )
      })
    }, 220)
    return () => window.clearInterval(timer)
  }, [externalProgressPercent, progressComplete, showProgressBar])

  useEffect(() => {
    if (!showProgressBar || !progressComplete || !onProgressSettled) return
    const progressBar = progressBarRef.current
    let done = false
    const finish = () => {
      if (done) return
      done = true
      onProgressSettled()
    }
    const timer = window.setTimeout(finish, STARTUP_PROGRESS_TRANSITION_FALLBACK_MS)
    const handleTransitionEnd = (event: TransitionEvent) => {
      if (event.target === progressBar && event.propertyName === 'width') finish()
    }
    progressBar?.addEventListener('transitionend', handleTransitionEnd)
    return () => {
      window.clearTimeout(timer)
      progressBar?.removeEventListener('transitionend', handleTransitionEnd)
    }
  }, [onProgressSettled, progressComplete, showProgressBar])

  const progressMessage = progressComplete
    ? progressCopy.complete
    : progressDetail || progressCopy.messages[messageIndex % progressCopy.messages.length]
  const progressStyle = {
    '--startup-progress-percent': `${clamp(progressComplete ? 100 : progressValue).toFixed(2)}%`
  } as CSSProperties
  const splashStyle = {
    '--startup-duration-ms': `${safeDurationMs}ms`,
    '--startup-letter-reveal-ms': `${letterRevealMs}ms`,
    '--startup-sweep-ms': `${sweepMs}ms`,
    '--startup-phase-ms': `${phaseMs}ms`,
    '--startup-phase-sweep-ms': `${phaseSweepMs}ms`,
    '--startup-brand-mark-reveal-ms': `${brandMarkRevealMs}ms`,
    '--startup-brand-mark-delay-ms': `${brandMarkDelayMs}ms`,
    '--startup-brand-bloom-final-opacity': persistFinalState ? 0.9 : 0,
    '--startup-brand-bloom-center-final-opacity': persistFinalState ? 0.92 : 0,
    '--startup-iteration-count': iterationCount,
    '--startup-final-opacity': persistFinalState ? 1 : 0,
    '--startup-play-state': playState
  } as CSSProperties
  const hasProgressSurface = progressSurface !== undefined && progressSurface !== null

  return (
    <div
      className={[
        'startup-splash',
        hasProgressSurface || showProgressBar ? 'startup-splash--progress-surface' : '',
        className
      ].filter(Boolean).join(' ')}
      style={splashStyle}
    >
      <div className="loader-wrapper" aria-label={`${brand} startup animation`}>
        {letters.map((letter, index) => {
          const visibleOrder = visibleLetterIndices.indexOf(index)
          const revealRatio = visibleLetterCount === 1 ? 0 : visibleOrder / (visibleLetterCount - 1)
          const delayMs = letter.trim()
            ? Math.round(FIRST_REVEAL_DELAY_MS + revealWindowMs * revealRatio)
            : 0
          const letterDelay = phaseMs + delayMs
          const isBrandI = shouldRenderBrandMark && index === brandMarkLetterIndex
          return (
            <span
              className={isBrandI ? 'loader-letter loader-letter--brand-i' : 'loader-letter'}
              key={`${letter}-${index}`}
              style={{
                '--loader-letter-delay': `${letterDelay}ms`,
                animationDelay: `${letterDelay}ms`
              } as CSSProperties}
            >
              {isBrandI ? (
                <>
                  <span className="brand-i-stem" aria-hidden="true">ı</span>
                  <span className="brand-i-dot-anchor" aria-hidden="true">
                    <span className="brand-i-dot-core" />
                    <span className="brand-i-bloom">
                      <svg viewBox="0 0 68 44" aria-hidden="true">
                        <g className="brand-i-bloom-piece brand-i-bloom-piece--left">
                          <rect x="4" y="10" width="24" height="24" rx="8" transform="rotate(-18 16 22)" fill="#f8b84a" />
                        </g>
                        <g className="brand-i-bloom-piece brand-i-bloom-piece--center">
                          <rect x="20" y="10" width="24" height="24" rx="8" transform="rotate(18 32 22)" fill="#5b5ff0" />
                        </g>
                        <g className="brand-i-bloom-piece brand-i-bloom-piece--right">
                          <rect x="34" y="10" width="24" height="24" rx="8" transform="rotate(-18 46 22)" fill="#2acfb4" />
                        </g>
                      </svg>
                    </span>
                  </span>
                </>
              ) : letter === ' ' ? '\u00A0' : letter}
            </span>
          )
        })}
        <div className="loader" />
      </div>
      {showProgressBar || progressSurface ? (
        <div className={`startup-progress-panel${progressSurface ? ' startup-progress-panel--custom' : ''}`}>
          {progressSurface ?? (
            <>
              <div className="startup-progress-loader">
                <div ref={progressBarRef} className="startup-progress-loader__bar" style={progressStyle} />
              </div>
              <div className="startup-progress-status">
                <span className="startup-progress-status__title">{progressCopy.title}</span>
                <span key={progressMessage} className="startup-progress-status__message">
                  {progressMessage}
                </span>
              </div>
            </>
          )}
        </div>
      ) : null}
    </div>
  )
}

export default StartupSplash
