import { describe, expect, it } from 'vitest'
import {
  exactTaskGroupMembers,
  normalQuitFailureCode,
  residualMembersAlreadyPinned,
  parseRuntimeStartupNumericDiagnostic,
  parseRuntimeTransitionPhase,
  parseGoStartupPhase,
  parseGoOwnerPhase,
  isGoStartupAttemptMarker,
  exactMainCommandMatches,
  exactCoreRuntimeCommandMatches,
  observedSingleInstanceLockFailure,
  processProbeFailureKind
} from '../../../scripts/lib/k10-child-diagnostics.mjs'

describe('K10 single-instance diagnostic', () => {
  it('ignores the normal startup checkpoint and recognizes an explicit lock refusal', () => {
    expect(observedSingleInstanceLockFailure('stage=single instance lock checked')).toBe(false)
    expect(observedSingleInstanceLockFailure('gotSingleInstanceLock=false')).toBe(true)
  })
})

describe('K10 process probe evidence', () => {
  it('does not turn an unknown ps error into an exited process or zero residuals', () => {
    expect(processProbeFailureKind({ status: 1 }, 'ps')).toBe('unknown')
    expect(processProbeFailureKind({ status: 1 }, 'pgrep')).toBe('no_match')
    expect(processProbeFailureKind({ code: 'ETIMEDOUT' }, 'pgrep')).toBe('timeout')
  })

  it('keeps Main, Helper and Go group members and refuses an unrelated member', () => {
    const rows = [
      '100 100 /Applications/analytix.app/Contents/MacOS/analytix --remote-debugging-port=1234',
      '101 100 /Applications/analytix.app/Contents/Frameworks/analytix Helper.app/Contents/MacOS/analytix Helper',
      '102 100 /Applications/analytix.app/Contents/Resources/runtime-go/bin/runtime-server --data-dir /private/tmp/task',
      '200 200 /usr/bin/other'
    ].join('\n')
    expect(exactTaskGroupMembers(rows, 100, '/Applications/analytix.app/Contents'))
      .toEqual([100, 101, 102])
    expect(exactTaskGroupMembers(`${rows}\n103 100 /usr/bin/other`, 100,
      '/Applications/analytix.app/Contents')).toBeNull()
    expect(exactTaskGroupMembers(`${rows}\n103 100`, 100,
      '/Applications/analytix.app/Contents')).toBeNull()
  })

  it('pins only the exact spawned Main parent, process group, executable and port', () => {
    const command = '42 100 /Applications/analytix.app/Contents/MacOS/analytix --remote-debugging-port=1234 --no-first-run'
    const matches = (row: string) => exactMainCommandMatches(row, 42, 100,
      '/Applications/analytix.app/Contents/MacOS/analytix', 1234)
    expect(matches(command)).toBe(true)
    expect(matches(command.replace('42 100', '1 100'))).toBe(false)
    expect(matches(command.replace('42 100', '42 101'))).toBe(false)
    expect(matches(command.replace('port=1234', 'port=4321'))).toBe(false)
  })

  it('does not count the Funds materializer as the Core runtime spawn', () => {
    const executable = '/Applications/analytix.app/Contents/Resources/runtime-go/bin/runtime-server'
    const dataDir = '/private/tmp/task/runtime-data'
    const matches = (command: string) => exactCoreRuntimeCommandMatches(command, executable, dataDir)
    const core = `${executable} --addr 127.0.0.1:8899 --data-dir ${dataDir} --private-startup-frame-v1`
    const materializer = `${executable} bundled-plugin materialize-funds-v1 --data-dir ${dataDir}`
    expect(matches(core)).toBe(true)
    expect(matches(materializer)).toBe(false)
    expect(matches(core.replace(dataDir, `${dataDir}-other`))).toBe(false)
    expect(matches(core.replace('--private-startup-frame-v1', ''))).toBe(false)
  })

  it('does not certify a normal quit after fallback or unknown Helper/Go residuals', () => {
    const normal = { quitRequestOk: true, targetClosed: true, mainExited: true, mainExitCode: 0,
      mainSignal: null, fallbackUsed: false, residualProcessCount: 0 }
    expect(normalQuitFailureCode(normal)).toBe('')
    expect(normalQuitFailureCode({ ...normal, fallbackUsed: true }))
      .toBe('normal_quit_required_fallback')
    expect(normalQuitFailureCode({ ...normal, quitRequestOk: false }))
      .toBe('normal_quit_request_failed')
    expect(normalQuitFailureCode({ ...normal, residualProcessCount: null }))
      .toBe('task_owned_process_residual_unknown')
    expect(normalQuitFailureCode({ ...normal, residualProcessCount: 1 }))
      .toBe('task_owned_process_residual')
  })

  it('never adopts a newly observed process in the old Main process group', () => {
    expect(residualMembersAlreadyPinned([101], new Set([100, 101]))).toBe(true)
    expect(residualMembersAlreadyPinned([101, 102], new Set([100, 101]))).toBe(false)
    expect(residualMembersAlreadyPinned(null, new Set([100, 101]))).toBe(false)
  })

  it('accepts only bounded, one-field Main timing diagnostics', () => {
    const prefix = '[analytix] [main] event=main_info detail='
    expect(parseRuntimeStartupNumericDiagnostic(`${prefix}{"maxEventLoopLagMs":42}`))
      .toEqual({ key: 'maxEventLoopLagMs', value: 42 })
    expect(parseRuntimeStartupNumericDiagnostic(`${prefix}{"maxConcurrentWaitForHealthProbes":1}`))
      .toEqual({ key: 'maxConcurrentWaitForHealthProbes', value: 1 })
    expect(parseRuntimeStartupNumericDiagnostic(`${prefix}{"maxEventLoopLagMs":42,"path":"/private"}`))
      .toBeNull()
    expect(isGoStartupAttemptMarker(`${prefix}{"stage":"go preflight:begin","elapsedMs":18}`)).toBe(true)
    expect(isGoStartupAttemptMarker(`${prefix}{"stage":"go preflight:begin","elapsedMs":18,"token":"x"}`)).toBe(false)
    expect(parseGoStartupPhase(`${prefix}{"stage":"go ready line:received","elapsedMs":903}`))
      .toEqual({ phase: 'readyLineReceived', elapsedMs: 903 })
    expect(parseGoStartupPhase(`${prefix}{"stage":"go ready line:received","elapsedMs":903,"path":"/private"}`))
      .toBeNull()
    expect(parseGoStartupPhase(`${prefix}{"stage":"unapproved phase","elapsedMs":903}`))
      .toBeNull()
    expect(parseGoOwnerPhase(`${prefix}{"stage":"go_semantic_prepared","durationMs":31000}`))
      .toEqual({ stage: 'go_semantic_prepared', durationMs: 31000 })
    expect(parseGoOwnerPhase(`${prefix}{"stage":"go_semantic_prepared","durationMs":31000,"path":"/private"}`))
      .toBeNull()
    expect(parseGoOwnerPhase(`${prefix}{"stage":"unapproved","durationMs":31000}`))
      .toBeNull()
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"runtime settings apply:begin","elapsedMs":72}`))
      .toEqual({ phase: 'runtime settings apply:begin', elapsedMs: 72 })
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"runtime settings apply:restart begin","elapsedMs":74}`))
      .toEqual({ phase: 'runtime settings apply:restart begin', elapsedMs: 74 })
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"runtime IPC restart:requested","elapsedMs":85}`))
      .toEqual({ phase: 'runtime IPC restart:requested', elapsedMs: 85 })
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"app before quit:runtime stopped","elapsedMs":95}`))
      .toEqual({ phase: 'app before quit:runtime stopped', elapsedMs: 95 })
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"runtime IPC restart:requested","elapsedMs":85,"path":"/private"}`))
      .toBeNull()
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"runtime unknown restart","elapsedMs":85}`))
      .toBeNull()
  })
})
