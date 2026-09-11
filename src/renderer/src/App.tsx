import { lazy, Suspense } from 'react'
import { StartupAuthGate } from './account/StartupAuthGate'
import { AppErrorBoundary } from './components/AppErrorBoundary'
import { AnalytixLoadingPage } from './components/brand/AnalytixLoadingPage'
import { isWindowsPackageInstallerHost, WindowsPackageInstaller } from './installer/WindowsPackageInstaller'
import i18n from './i18n'

const loadAppShell = () => import('./AppShell')
const appShellModulePromise = loadAppShell()
const AppShell = lazy(() => appShellModulePromise)

function StartupShell(): React.ReactElement {
  return <AnalytixLoadingPage label={i18n.t('loading')} surface="transparent" />
}

export default function App(): React.ReactElement {
  if (isWindowsPackageInstallerHost()) {
    return (
      <AppErrorBoundary>
        <WindowsPackageInstaller />
      </AppErrorBoundary>
    )
  }

  return (
    <AppErrorBoundary>
      <StartupAuthGate>
        <Suspense fallback={<StartupShell />}>
          <AppShell />
        </Suspense>
      </StartupAuthGate>
    </AppErrorBoundary>
  )
}
