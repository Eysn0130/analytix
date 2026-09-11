const { spawnSync } = require('node:child_process')
const path = require('node:path')

if (process.env.ANALYTIX_STANDARD_WIN_PAYLOAD_ONLY === '1') {
  console.warn('[package] standard-win payload-only mode enabled; final bootstrapper installer is not produced on this host.')
  process.exit(0)
}

if (process.platform !== 'win32') {
  console.error(
    '[package] Standard Windows final artifact must be wrapped by the WinForms/WebView2 bootstrapper on Windows.\n' +
      '[package] Set ANALYTIX_STANDARD_WIN_PAYLOAD_ONLY=1 only when producing an internal payload for a separate Windows bootstrapper build.'
  )
  process.exit(1)
}

const scriptPath = path.join(__dirname, 'build-standard-win-bootstrapper.ps1')
const result = spawnSync(
  'powershell.exe',
  ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', scriptPath],
  { stdio: 'inherit' }
)

process.exit(result.status === null ? 1 : result.status)
