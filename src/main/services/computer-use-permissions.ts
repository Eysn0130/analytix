import { desktopCapturer, shell, systemPreferences } from 'electron'
import { selectHostControlBackend } from '../../../packages/runtime/src/adapters/computer-use/backend-factory.js'
import { getChromeBrowserUseStatus } from './chrome-browser-use-service'

export type ComputerUsePermissionState = 'granted' | 'denied' | 'unknown'

export type ComputerUsePermissions = {
  platform: NodeJS.Platform
  /** Whether the host backend can run on this platform at all. */
  supported: boolean
  /** Whether the OS gates input/capture behind a permission prompt (macOS). */
  needsPermission: boolean
  /** Accessibility permission (controls mouse/keyboard injection on macOS). */
  accessibility: ComputerUsePermissionState
  /** Screen Recording permission (controls screenshots on macOS). */
  screenRecording: ComputerUsePermissionState
  /**
   * macOS only: Accessibility is enabled in System Settings, but the running
   * process still reports untrusted because AXIsProcessTrusted is cached until
   * relaunch. The UI shows a "restart to take effect" hint instead of "not
   * granted" so the granted-but-stale state is not mistaken for a failure.
   */
  accessibilityNeedsRestart: boolean
}

export type ComputerUseDoctorCheck = {
  id: string
  label: string
  status: 'passed' | 'warning' | 'failed' | 'unknown'
  message: string
}

export type ComputerUseDoctorResult = {
  platform: NodeJS.Platform
  backend: {
    preferred: 'analytix-computer-use'
    selected: 'analytix-computer-use' | 'nut-js'
    available: boolean
    reason?: string
  }
  checks: ComputerUseDoctorCheck[]
}

/**
 * The native macOS permission helper bundled with @computer-use/nut-js.
 * Its `askFor*` calls use the canonical TCC registration APIs
 * (AXIsProcessTrustedWithOptions / CGRequestScreenCaptureAccess), which
 * proactively add the app to the Accessibility / Screen Recording lists.
 */
type MacPermissions = {
  getAuthStatus(type: 'accessibility' | 'screen'): string
  askForAccessibilityAccess(): unknown
  askForScreenCaptureAccess(openPreferences?: boolean): unknown
}

let macPermissionsLoaded = false
let macPermissions: MacPermissions | null = null

async function loadMacPermissions(): Promise<MacPermissions | null> {
  if (!macPermissionsLoaded) {
    macPermissionsLoaded = true
    try {
      // node-mac-permissions is a transitive native dep (a .node addon), so it
      // is NOT externalized by electron-vite's direct-deps plugin. Load it via
      // a variable specifier + @vite-ignore so Rollup leaves it as a runtime
      // import instead of trying to bundle the native binary.
      const specifier = '@computer-use/node-mac-permissions'
      const ns = (await import(/* @vite-ignore */ specifier)) as Record<string, unknown>
      macPermissions = ((ns as { default?: unknown }).default ?? ns) as MacPermissions
    } catch {
      macPermissions = null
    }
  }
  return macPermissions
}

function normalizeState(status: string | undefined): ComputerUsePermissionState {
  if (status === 'authorized' || status === 'granted') return 'granted'
  if (status === 'not determined' || status === 'not-determined' || status === undefined) return 'unknown'
  return 'denied'
}

/**
 * Report the host-control OS permission status. On macOS, computer-use needs
 * Accessibility (input injection) and Screen Recording (capture); on
 * Windows/Linux no special permission gate applies. Checks are read-only.
 */
export async function getComputerUsePermissions(): Promise<ComputerUsePermissions> {
  const platform = process.platform
  if (platform !== 'darwin') {
    return {
      platform,
      supported: true,
      needsPermission: false,
      accessibility: 'granted',
      screenRecording: 'granted',
      accessibilityNeedsRestart: false
    }
  }

  let liveTrust = false
  try {
    liveTrust = systemPreferences.isTrustedAccessibilityClient(false)
  } catch {
    liveTrust = false
  }

  let accessibilityGrantedInSettings = false
  try {
    const native = await loadMacPermissions()
    accessibilityGrantedInSettings = native?.getAuthStatus('accessibility') === 'authorized'
  } catch {
    accessibilityGrantedInSettings = false
  }

  let screenRecording: ComputerUsePermissionState = 'unknown'
  try {
    screenRecording = normalizeState(systemPreferences.getMediaAccessStatus('screen'))
  } catch {
    screenRecording = 'unknown'
  }

  return {
    platform,
    supported: true,
    needsPermission: true,
    accessibility: liveTrust ? 'granted' : 'denied',
    screenRecording,
    accessibilityNeedsRestart: !liveTrust && accessibilityGrantedInSettings
  }
}

/**
 * Proactively enroll Analytix in the relevant macOS permission list and open
 * the settings pane. Prefers the native node-mac-permissions APIs; falls back
 * to Electron primitives if the native module is unavailable.
 */
export async function requestComputerUsePermission(
  kind: 'accessibility' | 'screenRecording'
): Promise<ComputerUsePermissions> {
  if (process.platform !== 'darwin') return getComputerUsePermissions()
  const native = await loadMacPermissions()
  try {
    if (kind === 'accessibility') {
      if (native?.askForAccessibilityAccess) {
        native.askForAccessibilityAccess()
      } else {
        systemPreferences.isTrustedAccessibilityClient(true)
      }
    } else {
      if (native?.askForScreenCaptureAccess) {
        native.askForScreenCaptureAccess(true)
      } else {
        try {
          await desktopCapturer.getSources({ types: ['screen'], thumbnailSize: { width: 1, height: 1 } })
        } catch {
          // Best effort.
        }
      }
      await shell.openExternal(
        'x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture'
      )
    }
  } catch {
    // Best effort; fall through to returning the current status.
  }
  return getComputerUsePermissions()
}

export async function getComputerUseDoctor(): Promise<ComputerUseDoctorResult> {
  const platform = process.platform
  const checks: ComputerUseDoctorCheck[] = []
  const selection = await selectHostControlBackend()
  const nut = await detectNutJs()
  if (platform === 'darwin') {
    const permissions = await getComputerUsePermissions()
    checks.push({
      id: 'macos-accessibility',
      label: '辅助功能',
      status: permissions.accessibility === 'granted' ? 'passed' : 'failed',
      message: permissions.accessibilityNeedsRestart
        ? '系统设置中看起来已启用辅助功能；请重启 analytix 让 TCC 状态刷新。'
        : permissions.accessibility === 'granted'
          ? '辅助功能已授权，可用于鼠标/键盘注入。'
          : '需要辅助功能权限才能进行鼠标/键盘注入。'
    })
    checks.push({
      id: 'macos-screen-recording',
      label: '屏幕录制',
      status: permissions.screenRecording === 'granted' ? 'passed' : 'failed',
      message: permissions.screenRecording === 'granted'
        ? '屏幕录制已授权，可用于截图捕获。'
        : '需要屏幕录制权限才能捕获截图。'
    })
  } else if (platform === 'win32') {
    const sessionName = process.env.SESSIONNAME ?? ''
    const interactive = sessionName === 'Console' || /^RDP-/i.test(sessionName)
    checks.push({
      id: 'windows-uia',
      label: 'UIAutomation',
      status: 'passed',
      message: 'Analytix Computer Use 可用时，Windows UIAutomation 是首选语义后端。'
    })
    checks.push({
      id: 'windows-desktop-session',
      label: 'Desktop session',
      status: interactive ? 'passed' : 'warning',
      message: interactive
        ? `检测到交互式桌面会话 (${sessionName})。`
        : '未检测到 Console/RDP 桌面会话；SSH service 和 Session 0 无法可靠控制可见桌面。'
    })
    checks.push({
      id: 'windows-uipi',
      label: 'UIPI/elevated windows',
      status: 'warning',
      message: '受 Windows UIPI 隔离影响，非管理员权限运行的 analytix 不能控制管理员权限目标窗口。'
    })
  } else {
    checks.push({
      id: 'platform',
      label: 'Platform',
      status: 'warning',
      message: '当前 analytix 构建优先交付 macOS 和 Windows 的 Computer Use 自动化。'
    })
  }
  const openComputerUseAvailable = selection.selectedBackendId === 'analytix-computer-use' && selection.readiness.available
  checks.push({
    id: 'backend-analytix-computer-use',
    label: 'Analytix Computer Use backend',
    status: openComputerUseAvailable ? 'passed' : 'warning',
    message: openComputerUseAvailable
      ? 'Analytix Computer Use MCP backend 已被 runtime computer_use 选中。'
      : 'Analytix Computer Use MCP backend 未被选中；runtime 会在可行时降级 fallback。'
  })
  checks.push({
    id: 'backend-nut-js',
    label: 'nut-js fallback',
    status: selection.selectedBackendId === 'nut-js' && selection.readiness.available
      ? 'passed'
      : nut.available
        ? 'warning'
        : 'failed',
    message: selection.selectedBackendId === 'nut-js' && selection.readiness.available
      ? '因为 Analytix Computer Use 不可用，已选择 nut-js fallback。'
      : nut.available
      ? 'nut-js fallback modules are available.'
      : 'nut-js fallback modules are unavailable.'
  })
  await appendChromeBrowserUseDoctorChecks(checks)
  const selected = selection.selectedBackendId === 'analytix-computer-use' ? 'analytix-computer-use' : 'nut-js'
  const available = selection.readiness.available
  return {
    platform,
    backend: {
      preferred: 'analytix-computer-use',
      selected,
      available,
      ...(available ? {} : { reason: 'No computer-use backend is available.' })
    },
    checks
  }
}

async function appendChromeBrowserUseDoctorChecks(checks: ComputerUseDoctorCheck[]): Promise<void> {
  try {
    const chrome = await getChromeBrowserUseStatus()
    checks.push({
      id: 'chrome-browser-use-extension',
      label: 'Chrome extension',
      status: chrome.extension.status === 'enabled'
        ? 'passed'
        : chrome.extension.status === 'missing' || chrome.extension.status === 'disabled'
          ? 'failed'
          : 'warning',
      message: chrome.extension.status === 'enabled'
        ? `Chrome Browser Use 扩展已在 ${chrome.extension.selectedProfileDirectory ?? '当前 Chrome 配置'} 中启用。`
        : chrome.extension.problem ?? `Chrome Browser Use 扩展状态：${chrome.extension.status}。`
    })
    checks.push({
      id: 'chrome-browser-use-native-host',
      label: 'Chrome native host',
      status: chrome.nativeHost.status === 'configured'
        ? 'passed'
        : chrome.nativeHost.status === 'missing' || chrome.nativeHost.status === 'invalid'
          ? 'failed'
          : 'warning',
      message: chrome.nativeHost.status === 'configured'
        ? `Chrome native messaging host 已配置：${chrome.nativeHost.manifestPath ?? chrome.nativeHost.registryManifestPath ?? '已检测到的 manifest 路径'}。`
        : chrome.nativeHost.problem ?? `Chrome native host 状态：${chrome.nativeHost.status}。`
    })
    checks.push({
      id: 'chrome-browser-use-handshake',
      label: 'Chrome Browser Use handshake',
      status: chrome.connected
        ? 'passed'
        : chrome.connectionProbe.status === 'failed'
          ? 'failed'
          : 'warning',
      message: chrome.connected
        ? 'Chrome Browser Use 握手已连接。'
        : chrome.reason ?? chrome.connectionProbe.problem ?? 'Chrome Browser Use 连接尚未被真实握手验证。'
    })
  } catch {
    checks.push({
      id: 'chrome-browser-use-diagnostics',
      label: 'Chrome Browser Use diagnostics',
      status: 'warning',
      message: 'Chrome Browser Use diagnostics are unavailable.'
    })
  }
}

async function detectNutJs(): Promise<{ available: boolean; reason: string }> {
  try {
    const nut = await import(/* @vite-ignore */ '@computer-use/nut-js') as Record<string, unknown>
    const screen = (nut.screen ?? (typeof nut.default === 'object' && nut.default ? (nut.default as Record<string, unknown>).screen : undefined)) as { grab?: unknown } | undefined
    if (typeof screen?.grab !== 'function') {
      return { available: false, reason: '@computer-use/nut-js loaded but screen.grab is unavailable.' }
    }
    return { available: true, reason: '@computer-use/nut-js is available.' }
  } catch (error) {
    return {
      available: false,
      reason: error instanceof Error ? error.message : String(error)
    }
  }
}
