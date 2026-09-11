import { useCallback, useEffect, useMemo, useState } from 'react'
import type { AnalytixInstallerBridge, AnalytixInstallerState } from '../vite-env'
import { AnalytixLoadingLogo } from '../components/brand/AnalytixLoadingPage'
import './windows-package-installer.css'

function installerBridge(): AnalytixInstallerBridge | undefined {
  return typeof window !== 'undefined' ? window.analytixInstaller : undefined
}

function installerHostname(): string {
  return typeof window !== 'undefined' ? window.location.hostname : ''
}

function installerHashParams(): URLSearchParams {
  if (typeof window === 'undefined') return new URLSearchParams()
  const hash = window.location.hash || ''
  const queryIndex = hash.indexOf('?')
  return new URLSearchParams(queryIndex >= 0 ? hash.slice(queryIndex + 1) : '')
}

export function isWindowsPackageInstallerHost(): boolean {
  if (typeof window === 'undefined') return false
  if (installerHostname() === 'analytix-installer.local') return true
  const hashParams = installerHashParams()
  return hashParams.get('install') === 'package' && hashParams.get('platform') === 'win32'
}

function normalizeState(state: AnalytixInstallerState | null | undefined): AnalytixInstallerState {
  return state ?? {
    platform: 'win32',
    lifecycleState: 'idle',
    progressPercent: 0,
    installed: false,
    canInstall: false,
    canUninstall: false,
    installPath: ''
  }
}

function isBusy(state: AnalytixInstallerState): boolean {
  return state.lifecycleState === 'installing' || state.lifecycleState === 'uninstalling'
}

function primaryLabel(state: AnalytixInstallerState): string {
  if (state.installed) return state.canUpdate ? '更新' : '重新安装'
  return '安装'
}

function compareVersionText(left: string | null | undefined, right: string | null | undefined): number {
  const leftParts = String(left || '').trim().replace(/^v/i, '').split(/[+\-\s]/)[0]?.split('.') ?? []
  const rightParts = String(right || '').trim().replace(/^v/i, '').split(/[+\-\s]/)[0]?.split('.') ?? []
  const length = Math.max(leftParts.length, rightParts.length, 3)
  for (let index = 0; index < length; index += 1) {
    const leftValue = Number.parseInt(leftParts[index] ?? '0', 10) || 0
    const rightValue = Number.parseInt(rightParts[index] ?? '0', 10) || 0
    if (leftValue !== rightValue) return leftValue > rightValue ? 1 : -1
  }
  return 0
}

function statusTitle(state: AnalytixInstallerState): string {
  if (state.lifecycleState === 'completed') return state.installed ? '安装完成' : '卸载完成'
  if (state.lifecycleState === 'failed') return '操作失败'
  if (state.lifecycleState === 'installing') return state.canUpdate ? '正在更新' : '正在安装'
  if (state.lifecycleState === 'uninstalling') return '正在卸载'
  if (state.installed) {
    if (state.canUpdate) return '发现可用更新'
    const versionCompare = compareVersionText(state.installedVersion, state.version)
    if (versionCompare > 0) return '已安装较新版本'
    if (versionCompare === 0) return '已是最新版'
    return '已安装'
  }
  return '准备安装'
}

function statusDetail(state: AnalytixInstallerState): string {
  if (state.lastError) return state.lastError
  if (state.detail) return state.detail
  if (state.installed && state.installedVersion && state.version && compareVersionText(state.installedVersion, state.version) > 0) {
    return `已安装版本 ${state.installedVersion}，高于安装包 ${state.version}`
  }
  if (state.installed && state.installedVersion && state.version && compareVersionText(state.installedVersion, state.version) === 0) {
    return `当前版本 ${state.installedVersion}`
  }
  if (state.installed && state.installedVersion) return `当前版本 ${state.installedVersion}`
  if (state.version) return `将安装版本 ${state.version}`
  return 'analytix Windows 标准安装器'
}

function notifyInstallerSurfaceReady(): void {
  document.body.classList.add('analytix-installer-surface')
  document.body.classList.add('analytix-react-ready')
  try {
    window.__analytixReleaseInstallerPrepaint?.()
  } catch {
    // Native release is still the source of truth; this only prevents a stuck prepaint pause.
  }
  try {
    const webview = (window as unknown as { chrome?: { webview?: { postMessage: (message: unknown) => void } } }).chrome?.webview
    webview?.postMessage({
      channel: 'analytix-installer-surface',
      method: 'surfaceReady'
    })
  } catch {
    // Surface readiness is best-effort; the native fallback timer still reveals the installer.
  }
}

export function WindowsPackageInstaller(): React.ReactElement {
  const bridge = useMemo(installerBridge, [])
  const [state, setState] = useState<AnalytixInstallerState>(() => normalizeState(null))
  const [installPath, setInstallPath] = useState('')
  const [localError, setLocalError] = useState('')
  const busy = isBusy(state)

  useEffect(() => {
    notifyInstallerSurfaceReady()
  }, [])

  useEffect(() => {
    if (!bridge) {
      setLocalError('安装器桥接不可用，请重新打开安装程序。')
      return
    }
    let canceled = false
    void bridge.getPackageInstallerState().then((nextState) => {
      if (canceled) return
      const normalized = normalizeState(nextState)
      setState(normalized)
      setInstallPath(normalized.installPath || '')
    }).catch((error) => {
      if (!canceled) setLocalError(error instanceof Error ? error.message : '读取安装状态失败。')
    })
    const unsubscribe = bridge.onPackageInstallerState((nextState) => {
      const normalized = normalizeState(nextState)
      setState(normalized)
      if (normalized.installPath) setInstallPath(normalized.installPath)
    })
    return () => {
      canceled = true
      unsubscribe()
    }
  }, [bridge])

  const runInstall = useCallback(() => {
    if (!bridge || busy) return
    setLocalError('')
    void bridge.installPackage({ installPath }).then((nextState) => {
      const normalized = normalizeState(nextState)
      setState(normalized)
      if (normalized.installPath) setInstallPath(normalized.installPath)
    }).catch((error) => {
      setLocalError(error instanceof Error ? error.message : '安装失败。')
    })
  }, [bridge, busy, installPath])

  const runUninstall = useCallback(() => {
    if (!bridge || busy) return
    setLocalError('')
    void bridge.uninstallPackage().then((nextState) => {
      const normalized = normalizeState(nextState)
      setState(normalized)
      if (normalized.installPath) setInstallPath(normalized.installPath)
    }).catch((error) => {
      setLocalError(error instanceof Error ? error.message : '卸载失败。')
    })
  }, [bridge, busy])

  const choosePath = useCallback(() => {
    if (!bridge || busy || state.installed) return
    void bridge.pickDirectory().then((path) => {
      if (path) setInstallPath(path)
    }).catch((error) => {
      setLocalError(error instanceof Error ? error.message : '选择安装路径失败。')
    })
  }, [bridge, busy, state.installed])

  const closeInstaller = useCallback(() => {
    void bridge?.closeWindow()
  }, [bridge])

  const progress = Math.max(0, Math.min(100, Number(state.progressPercent ?? 0)))
  const errorText = localError || state.lastError || ''

  return (
    <main className="win-package-installer startup-animation-stage" aria-label="analytix Windows installer">
      <section className="win-package-installer__panel">
        <div className="win-package-installer__brand">
          <AnalytixLoadingLogo className="win-package-installer__logo" size={52} />
          <div>
            <span className="win-package-installer__eyebrow">analytix Standard</span>
            <h1>Windows 安装器</h1>
          </div>
        </div>

        <div className="win-package-installer__status">
          <div>
            <span className="win-package-installer__status-title">{statusTitle(state)}</span>
            <span className="win-package-installer__status-detail">{statusDetail(state)}</span>
          </div>
          <span className="win-package-installer__version">{state.version || '1.0.6'}</span>
        </div>

        <div className="win-package-installer__progress" aria-label="安装进度">
          <span style={{ width: `${progress}%` }} />
        </div>

        <label className="win-package-installer__path">
          <span>安装位置</span>
          <div>
            <input
              value={installPath}
              disabled={busy || state.installed}
              onChange={(event) => setInstallPath(event.target.value)}
            />
            <button type="button" disabled={busy || state.installed || !state.canChooseInstallPath} onClick={choosePath}>
              浏览
            </button>
          </div>
        </label>

        {errorText ? <p className="win-package-installer__error">{errorText}</p> : null}

        <div className="win-package-installer__actions">
          <button type="button" className="win-package-installer__primary" disabled={busy || !state.canInstall} onClick={runInstall}>
            {primaryLabel(state)}
          </button>
          {state.installed ? (
            <button type="button" className="win-package-installer__danger" disabled={busy || !state.canUninstall} onClick={runUninstall}>
              卸载
            </button>
          ) : null}
          <button type="button" disabled={busy} onClick={closeInstaller}>
            退出
          </button>
        </div>
      </section>
    </main>
  )
}
