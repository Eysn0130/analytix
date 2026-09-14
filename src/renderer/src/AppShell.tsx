import { lazy, Suspense, useEffect, useRef } from 'react'
import { useChatStore } from './store/chat-store'
import { supportsDesktopTitleBar, WindowsTitleBar } from './components/WindowsTitleBar'
import { RuntimeStatusBanner } from './components/RuntimeStatusBanner'
import { TopNoticeHost } from './components/TopNoticeHost'
import { AnalytixLoadingPage } from './components/brand/AnalytixLoadingPage'
import i18n from './i18n'

const loadWorkbench = () =>
  import('./components/Workbench').then((module) => ({ default: module.Workbench }))
const loadSettingsView = () =>
  import('./components/SettingsView').then((module) => ({ default: module.SettingsView }))
const loadInitialSetupDialog = () =>
  import('./components/InitialSetupDialog').then((module) => ({
    default: module.InitialSetupDialog
  }))

const workbenchModulePromise = loadWorkbench()
const settingsViewModulePromise = loadSettingsView()
let initialSetupDialogModulePromise: ReturnType<typeof loadInitialSetupDialog> | null = null

function loadInitialSetupDialogOnce(): ReturnType<typeof loadInitialSetupDialog> {
  if (!initialSetupDialogModulePromise) {
    initialSetupDialogModulePromise = loadInitialSetupDialog()
  }
  return initialSetupDialogModulePromise
}

export async function prewarmAppShellSurfaces(): Promise<void> {
  await Promise.all([
    workbenchModulePromise,
    settingsViewModulePromise,
    loadInitialSetupDialogOnce()
  ])
}

const Workbench = lazy(() => workbenchModulePromise)
const SettingsView = lazy(() => settingsViewModulePromise)
const InitialSetupDialog = lazy(loadInitialSetupDialogOnce)

function RouteFallback(): React.ReactElement {
  return <AnalytixLoadingPage fillParent label={i18n.t('loading')} />
}

export default function AppShell(): React.ReactElement {
  const route = useChatStore((s) => s.route)
  const runtimeConnection = useChatStore((s) => s.runtimeConnection)
  const activeThreadId = useChatStore((s) => s.activeThreadId)
  const selectThread = useChatStore((s) => s.selectThread)
  const initialSetupOpen = useChatStore((s) => s.initialSetupOpen)
  const platform = typeof window !== 'undefined' ? window.analytix?.app?.platform ?? 'unknown' : 'unknown'
  const hasDesktopTitleBar = supportsDesktopTitleBar(platform)
  const initialThreadIdRef = useRef(
    typeof window !== 'undefined'
      ? new URL(window.location.href).searchParams.get('threadId')?.trim() || ''
      : ''
  )

  useEffect(() => {
    void prewarmAppShellSurfaces()
  }, [])

  useEffect(() => {
    if (route === 'settings') {
      void workbenchModulePromise
      return
    }
    void settingsViewModulePromise
  }, [route])

  useEffect(() => {
    const initialThreadId = initialThreadIdRef.current
    if (!initialThreadId || runtimeConnection !== 'ready') return
    if (activeThreadId === initialThreadId) {
      initialThreadIdRef.current = ''
      return
    }
    void selectThread(initialThreadId).finally(() => {
      initialThreadIdRef.current = ''
    })
  }, [activeThreadId, runtimeConnection, selectThread])

  return (
    <div className={hasDesktopTitleBar ? 'ds-windows-app-frame relative flex h-full min-h-0 flex-col bg-ds-main' : 'relative flex h-full min-h-0 flex-col bg-transparent'}>
      <div aria-hidden className="ds-native-window-controls-hitbox" />
      {hasDesktopTitleBar ? <WindowsTitleBar platform={platform} /> : null}
      <div className="flex min-h-0 flex-1 flex-col">
        <RuntimeStatusBanner />
        <Suspense fallback={<RouteFallback />}>
          {route === 'settings' ? <SettingsView /> : <Workbench />}
        </Suspense>
      </div>
      <TopNoticeHost />
      {initialSetupOpen ? (
        <Suspense fallback={<AnalytixLoadingPage overlay label={i18n.t('loading')} />}>
          <InitialSetupDialog />
        </Suspense>
      ) : null}
    </div>
  )
}
