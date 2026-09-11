import { Fragment, useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { StartupSplash } from './StartupSplash'
import { useChatStore } from '../store/chat-store'
import './desktop-startup-gate.css'

type StartupAuthGateProps = {
  children: ReactNode
}

type StartupPhase = 'splash' | 'handoff' | 'main'
type StartupProgress = {
  detail: string
  percent: number
}

const SPLASH_DURATION_MS = 3200
const HANDOFF_DURATION_MS = 760

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

async function prewarmStartupSurfaces(): Promise<void> {
  const { prewarmAppShellSurfaces } = await import('../AppShell')
  await prewarmAppShellSurfaces()
}

async function bootLocalProviderWorkspace(): Promise<void> {
  await useChatStore.getState().boot()
}

export function StartupAuthGate({ children }: StartupAuthGateProps): React.ReactElement {
  const [phase, setPhase] = useState<StartupPhase>('splash')
  const [animationComplete, setAnimationComplete] = useState(false)
  const [bootComplete, setBootComplete] = useState(false)
  const [progress, setProgress] = useState<StartupProgress>({
    detail: 'CHECKING LOCAL PROVIDER',
    percent: 12
  })
  const committedRef = useRef(false)

  useEffect(() => {
    let cancelled = false
    const run = async () => {
      setProgress({ detail: 'PREWARMING APP SHELL', percent: 18 })
      const prewarmPromise = prewarmStartupSurfaces()
      const bootPromise = bootLocalProviderWorkspace()
      await Promise.all([
        prewarmPromise.then(() => {
          if (!cancelled) setProgress((current) => ({ detail: 'APP SHELL PREWARMED', percent: Math.max(current.percent, 64) }))
        }),
        bootPromise.then(() => {
          if (!cancelled) setProgress({ detail: 'LOCAL PROVIDER READINESS COMPLETE', percent: 92 })
        })
      ])
      await delay(120)
      if (cancelled) return
      setProgress({ detail: 'ANALYTIX READY', percent: 100 })
      setBootComplete(true)
    }
    void run().catch(() => {
      if (cancelled) return
      setProgress({ detail: 'LOCAL PROVIDER RECOVERY REQUIRED', percent: 100 })
      setBootComplete(true)
    })
    return () => {
      cancelled = true
    }
  }, [])

  const commitTarget = useCallback((target: StartupPhase) => {
    if (committedRef.current) return
    committedRef.current = true
    if (target === 'main') {
      setPhase('handoff')
      window.setTimeout(() => setPhase('main'), HANDOFF_DURATION_MS)
      return
    }
    setPhase(target)
  }, [])

  useEffect(() => {
    if (!animationComplete || !bootComplete) return
    commitTarget('main')
  }, [animationComplete, bootComplete, commitTarget])

  const showStartupProgressBar = animationComplete && !bootComplete

  const showChildren = phase === 'handoff' || phase === 'main'
  return (
    <>
      {showChildren ? <Fragment key="startup-auth-children">{children}</Fragment> : null}
      {phase === 'splash' || phase === 'handoff' ? (
        <div key="startup-auth-overlay" className={`desktop-startup-gate desktop-startup-gate--${phase}`}>
          <StartupSplash
            brand="Analytix"
            durationMs={SPLASH_DURATION_MS}
            onComplete={() => setAnimationComplete(true)}
            onProgressSettled={() => commitTarget('main')}
            progressComplete={animationComplete && bootComplete}
            progressContext="boot"
            progressDetail={progress.detail}
            progressPercent={progress.percent}
            showProgressBar={showStartupProgressBar}
          />
        </div>
      ) : null}
    </>
  )
}

export default StartupAuthGate
