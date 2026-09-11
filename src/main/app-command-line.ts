import { app } from 'electron'

export const LINUX_WAYLAND_IME_SWITCHES = [
  { name: 'ozone-platform-hint', value: 'auto' },
  { name: 'enable-wayland-ime' },
  // 显式选用 text-input-v3:Chromium 开启 enable-wayland-ime 后默认走
  // text-input-v1,而 niri / sway / Hyprland 等 wlroots/smithay 系合成器
  // 只实现 v3,不加这一项会导致纯 Wayland 下中文输入法完全无法使用
  // (issue:archlinux + niri 无法切换中文输入法)。GNOME/KDE 同样支持 v3,
  // 故无条件开启;Chromium 的 v1 实现本身也不稳定。
  { name: 'wayland-text-input-version', value: '3' }
] as const

export function shouldConfigureLinuxWaylandImeSwitches(platform = process.platform): boolean {
  return platform === 'linux'
}

export function configureLinuxWaylandImeSwitches(platform = process.platform): void {
  if (!shouldConfigureLinuxWaylandImeSwitches(platform)) return

  for (const commandLineSwitch of LINUX_WAYLAND_IME_SWITCHES) {
    if (app.commandLine.hasSwitch(commandLineSwitch.name)) continue

    if ('value' in commandLineSwitch) {
      app.commandLine.appendSwitch(commandLineSwitch.name, commandLineSwitch.value)
    } else {
      app.commandLine.appendSwitch(commandLineSwitch.name)
    }
  }
}
