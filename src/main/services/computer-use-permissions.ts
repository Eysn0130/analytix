import { desktopCapturer, shell, systemPreferences } from 'electron'
import { selectHostControlBackend } from '../../../packages/runtime/src/adapters/computer-use/backend-factory.js'
import { getChromeBrowserUseStatus } from './chrome-browser-use-service'
import { readHostPermissionSnapshot } from './computer-use-permission-status'
import { createPermissionEnrollment } from './computer-use-permission-access'
import type { ComputerUsePermissionKind, ComputerUsePermissions as PermissionSnapshot } from '../../shared/analytix-api'

export type { ComputerUsePermissionState } from '../../shared/analytix-api'
export type ComputerUsePermissions = PermissionSnapshot & { platform: NodeJS.Platform }

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

const permissionEnrollment = createPermissionEnrollment({
  importNative: async () => {
    // Resolve the native addon at runtime; it must stay outside the JS bundle.
    const nativePackage = '@computer-use/node-mac-permissions'
    return import(/* @vite-ignore */ nativePackage)
  },
  promptAccessibility: () => systemPreferences.isTrustedAccessibilityClient(true),
  promptScreenCapture: () => desktopCapturer.getSources({
    types: ['screen'], thumbnailSize: { width: 1, height: 1 }
  }),
  openScreenSettings: () => shell.openExternal(
    'x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture'
  )
})

export async function getComputerUsePermissions(): Promise<ComputerUsePermissions> {
  return readHostPermissionSnapshot({
    platform: process.platform,
    liveAccessibility: () => systemPreferences.isTrustedAccessibilityClient(false),
    configuredAccessibility: permissionEnrollment.configuredAccessibility,
    screenCapture: () => systemPreferences.getMediaAccessStatus('screen')
  })
}

export async function requestComputerUsePermission(
  kind: ComputerUsePermissionKind
): Promise<ComputerUsePermissions> {
  await permissionEnrollment.request(kind, process.platform)
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
