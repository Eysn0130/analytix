import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  join(process.cwd(), 'scripts', 'runtime-go-packaged-gui-smoke.mjs'),
  'utf8'
)

describe('packaged terminal PTY smoke contract', () => {
  it('requires the renderer-to-main real PTY public seam', () => {
    expect(source).toContain("'packaged-terminal-pty'")
    expect(source).toContain('api.terminal.create({')
    expect(source).toContain('api.terminal.write({')
    expect(source).toContain('api.terminal.dispose(terminalSessionId)')
    expect(source).toContain('api.terminal.onData((payload)')
    expect(source).toContain('api.terminal.onExit((payload)')
    expect(source).toContain('renderer?.terminalTtyMarkerObserved === true')
    expect(source).toContain('renderer?.terminalOrdinaryWorkspaceOk === true')
    expect(source).toContain('renderer?.terminalProtectedDirectBlocked === true')
    expect(source).toContain('renderer?.terminalProtectedSymlinkBlocked === true')
    expect(source).toContain('renderer?.terminalAmbientEnvClean === true')
    expect(source).toContain('renderer?.terminalRuntimeBoundaryCreateOk === true')
    expect(source).toContain('renderer?.terminalRuntimeBoundaryWriteOk === true')
    expect(source).toContain('renderer?.terminalRuntimeBoundaryReadyObserved === true')
    expect(source).toContain('renderer?.terminalRuntimeBoundaryPrePatchExitAbsent === true')
    expect(source).toContain('renderer?.terminalRuntimeBoundaryExitObserved === true')
    expect(source).toContain('renderer?.terminalPostRestartProtectedBlocked === true')
    expect(source).toContain('terminalProjectionScanComplete === true')
    expect(source).toContain('terminalProjectionLeakFree === true')
    expect(source).toContain('renderer?.terminalExitCode === 0')
    expect(source).toContain("SHELL: '/bin/zsh'")
    expect(source).not.toContain("'initial-protected-targets.txt'")
    expect(source).not.toContain("'post-restart-protected-target.txt'")
  })

  it('does not return raw terminal output in packaged evidence', () => {
    expect(source).not.toMatch(/out\.terminalOutput\s*=/u)
    expect(source).toContain("terminalOutput = '';")
    expect(source).toContain('terminalOutput.length < 8192')
    expect(source).toContain('boundedDirectoryContains(root, needle')
    expect(source).toContain('terminalProjectionNeedles.every((needle) =>')
    expect(source).toContain('!stdout.includes(needle)')
    expect(source).toContain('!stderr.includes(needle)')
  })
})
