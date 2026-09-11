import { spawn, type ChildProcess } from 'node:child_process'

type SpawnLike = typeof spawn

export function terminateSpawnTree(
  child: ChildProcess,
  options: {
    platform?: NodeJS.Platform
    signal?: NodeJS.Signals
    spawnImpl?: SpawnLike
  } = {}
): void {
  const signal = options.signal ?? 'SIGTERM'
  const pid = child.pid
  if (!pid) {
    child.kill(signal)
    return
  }

  if ((options.platform ?? process.platform) === 'win32') {
    try {
      const taskkill = (options.spawnImpl ?? spawn)('taskkill', ['/pid', String(pid), '/T', '/F'], {
        stdio: 'ignore',
        windowsHide: true
      })
      taskkill.once('error', () => {
        child.kill(signal)
      })
      taskkill.unref?.()
      return
    } catch {
      child.kill(signal)
      return
    }
  }

  try {
    process.kill(-pid, signal)
    return
  } catch {
    child.kill(signal)
  }
}
