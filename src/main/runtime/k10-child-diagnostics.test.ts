import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import {
  classifyExactTargetList,
  createFundsCSVAdmissionAttemptRecorderV2,
  exactTaskGroupMembers,
  normalQuitFailureCode,
  residualMembersAlreadyPinned,
  parseRuntimeStartupNumericDiagnostic,
  parseRuntimeTransitionPhase,
  parseGoStartupPhase,
  parseGoOwnerPhase,
  parseFundsCSVAdmissionMarkerV2,
  projectFundsCSVAdmissionDiagnosticV2,
  isGoStartupAttemptMarker,
  exactMainCommandMatches,
  exactCoreRuntimeCommandMatches,
  observedSingleInstanceLockFailure,
  processProbeFailureKind
} from '../../../scripts/lib/k10-child-diagnostics.mjs'

describe('K10 Core Funds CSV admission report', () => {
  const marker = 'ANALYTIX_FUNDS_CSV_ADMISSION_V2 '
  const anomaly = 'ANALYTIX_FUNDS_CSV_ADMISSION_ANOMALY_V1 '
  const phase = (stage: string, elapsedMs: number) =>
    `[analytix] [main] event=main_info detail=${JSON.stringify({ stage, elapsedMs })}`

  it('parses only exact fixed codes, without retaining child text', () => {
    for (const code of [
      'native_owner_unavailable', 'shared_evidence_enrollment_absent',
      'shared_evidence_credential_unavailable', 'dataset_snapshot_unavailable',
      'ready', 'other_unavailable'
    ]) expect(parseFundsCSVAdmissionMarkerV2(`${marker}activation ${code}`))
      .toEqual({ phase: 'activation', code })
    expect(parseFundsCSVAdmissionMarkerV2(`${marker}semantic_preparation not_evaluated`))
      .toEqual({ phase: 'semantic_preparation', code: 'not_evaluated' })
    for (const line of [
      `${marker}activation ready path=/private/case`, `${marker}activation ready\r`, `${marker}activation ready\n`,
      `error=/private/case ${marker}activation ready`, `${marker}activation unknown`,
      `${marker}semantic_preparation ready`, `${marker}activation not_evaluated`,
      `${marker}activation ready${marker}activation ready`, `x${marker}activation ready`,
      `${'x'.repeat(200)}${marker}activation ready`
    ]) expect(parseFundsCSVAdmissionMarkerV2(line)).toBeNull()
  })

  it('attributes both phases to the exact Core attempt and uses activation for owner reachability', () => {
    const recorder = createFundsCSVAdmissionAttemptRecorderV2(true)
    const semantic = `${marker}semantic_preparation not_evaluated`
    const activation = `${marker}activation shared_evidence_enrollment_absent`
    recorder.acceptLine('stdout', activation)
    recorder.acceptLine('stdout', phase('go preflight:begin', 1))
    recorder.acceptLine('stdout', activation)
    recorder.acceptLine('stdout', phase('go process:spawned', 2))
    recorder.acceptLine('stdout', semantic)
    recorder.acceptLine('stdout', activation, false)
    recorder.acceptLine('stderr', activation)
    recorder.acceptLine('stdout', `${activation} error=/private/case`)
    recorder.acceptLine('stdout', activation)
    expect(recorder.evidence()).toEqual([{
      coreAttemptOrdinal: 1, status: 'complete', semanticPreparationCode: 'not_evaluated',
      code: 'shared_evidence_enrollment_absent', nativeOwner: 'available'
    }])
    recorder.acceptLine('stdout', phase('go adapter:done', 3))
    recorder.acceptLine('stdout', activation)
    recorder.acceptLine('stdout', phase('go preflight:begin', 4))
    recorder.acceptLine('stdout', phase('go process:spawned', 5))
    recorder.acceptLine('stdout', semantic)
    recorder.acceptLine('stdout', `${marker}activation native_owner_unavailable`)
    expect(projectFundsCSVAdmissionDiagnosticV2([
      createFundsCSVAdmissionAttemptRecorderV2(false), recorder
    ])).toEqual({
      fundsCSVAdmissionDiagnostic: [
        { launchOrdinal: 2, coreAttemptOrdinal: 1, status: 'complete',
          semanticPreparationCode: 'not_evaluated', code: 'shared_evidence_enrollment_absent', nativeOwner: 'available' },
        { launchOrdinal: 2, coreAttemptOrdinal: 2, status: 'complete',
          semanticPreparationCode: 'not_evaluated', code: 'native_owner_unavailable', nativeOwner: 'unavailable' }
      ]
    })
  })

  it('marks missing or duplicate phase evidence incomplete without retaining child text', () => {
    const disabled = createFundsCSVAdmissionAttemptRecorderV2(false)
    disabled.acceptLine('stdout', phase('go preflight:begin', 1))
    disabled.acceptLine('stdout', phase('go process:spawned', 2))
    expect(projectFundsCSVAdmissionDiagnosticV2([disabled])).toEqual({})
    const recorder = createFundsCSVAdmissionAttemptRecorderV2(true)
    recorder.acceptLine('stdout', phase('go preflight:begin', 1))
    recorder.acceptLine('stdout', `${marker}activation ready`)
    expect(recorder.evidence()).toEqual([])
    recorder.acceptLine('stdout', phase('go process:spawned', 2))
    recorder.acceptLine('stdout', `${marker}semantic_preparation not_evaluated`)
    expect(recorder.evidence()).toEqual([{
      coreAttemptOrdinal: 1, status: 'incomplete', semanticPreparationCode: 'not_evaluated',
      code: null, nativeOwner: 'unknown'
    }])
    recorder.acceptLine('stdout', `${marker}activation ready`)
    recorder.acceptLine('stdout', `${anomaly}activation`)
    expect(recorder.evidence()[0]).toMatchObject({ status: 'incomplete', code: 'ready', nativeOwner: 'unknown' })
  })

  it('reads only packaged Main stdout and projects codes in both final outcomes', () => {
    const source = readFileSync(new URL('../../../scripts/k10-local-product-seam.mjs', import.meta.url), 'utf8')
    const stdoutStart = source.indexOf("child.stdout.on('data', (chunk) => {")
    const stderrStart = source.indexOf("child.stderr.on('data', (chunk) => {")
    expect(stdoutStart).toBeGreaterThan(0)
    expect(stderrStart).toBeGreaterThan(stdoutStart)
    expect(source.slice(stdoutStart, stderrStart)).toContain("fundsCSVAdmission.acceptLine('stdout', completedLine, !lineOverflowed)")
    expect(source.slice(stderrStart, source.indexOf('const closed =', stderrStart)))
      .not.toContain('fundsCSVAdmission.acceptLine')
    expect(source.split('...projectFundsCSVAdmissionDiagnosticV2(fundsCSVAdmissionByLaunch)').length - 1).toBe(2)
  })
})

describe('K10 single-instance diagnostic', () => {
  it('ignores the normal startup checkpoint and recognizes an explicit lock refusal', () => {
    expect(observedSingleInstanceLockFailure('stage=single instance lock checked')).toBe(false)
    expect(observedSingleInstanceLockFailure('gotSingleInstanceLock=false')).toBe(true)
  })
})

describe('K10 process probe evidence', () => {
  it('compares only the captured CDP target id and keeps malformed responses unknown', () => {
    expect(classifyExactTargetList([
      { id: 'unrelated', type: 'page', url: 'unsafe' },
      { id: 'captured', type: 'other', title: 'unsafe' }
    ], 'captured')).toBe('target_present')
    expect(classifyExactTargetList([{ id: 'unrelated', type: 'page' }], 'captured'))
      .toBe('target_absent')
    expect(classifyExactTargetList([{ type: 'page' }], 'captured')).toBe('malformed')
    expect(classifyExactTargetList(null, 'captured')).toBe('malformed')
    expect(classifyExactTargetList([], '')).toBe('malformed')
  })

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
    for (const phase of [
      'app before quit:closing', 'app before quit:close allowed',
      'app before quit:close veto prior', 'app before quit:close veto unprepared',
      'app before quit:close veto frame', 'app before quit:close veto loading',
      'app before quit:close veto late', 'app before quit:window closed',
      'app before quit:close unknown', 'app before quit:windows closed',
      'app before quit:office teardown settled', 'app before quit:office teardown failed',
      'app before quit:stop failed'
    ]) {
      expect(parseRuntimeTransitionPhase(`${prefix}${JSON.stringify({ stage: phase, elapsedMs: 96 })}`))
        .toEqual({ phase, elapsedMs: 96 })
    }
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"runtime IPC restart:requested","elapsedMs":85,"path":"/private"}`))
      .toBeNull()
    expect(parseRuntimeTransitionPhase(`${prefix}{"stage":"runtime unknown restart","elapsedMs":85}`))
      .toBeNull()
  })
})
