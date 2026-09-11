import { describe, expect, it } from 'vitest'
import {
  bashApprovalSafetySummary,
  classifyShellCommandSafety,
  containsShellSyntax
} from '../src/shared/shell-safety.js'

describe('shell safety classification', () => {
  it('matches Reasonix read-only command membership without auto-approving execution', () => {
    for (const command of [
      'git status',
      'git diff HEAD',
      'git rev-parse HEAD',
      'git for-each-ref',
      'ls -la',
      'cat package.json',
      'rg "needle" src',
      'go version',
      'go env',
      'npm view react version',
      'docker ps',
      'kubectl get pods',
      'node --version',
      'python3 --version'
    ]) {
      expect(classifyShellCommandSafety(command)).toMatchObject({ readOnly: true })
    }
  })

  it('fails closed for write-capable, unknown, and shell-syntax commands', () => {
    for (const command of [
      'rm -rf /tmp/x',
      'git push',
      'git reset --hard',
      'go test ./...',
      'npm install',
      'docker rm container',
      'kubectl apply -f app.yaml',
      'frobnicate --all',
      'git status && rm -rf /tmp/x',
      'cat a | tee b',
      'echo $(rm x)'
    ]) {
      expect(classifyShellCommandSafety(command)).toMatchObject({ readOnly: false })
    }
  })

  it('summarizes bash approval safety without changing policy decisions', () => {
    expect(bashApprovalSafetySummary({ command: 'git status' }))
      .toBe('Shell safety: read-only command (git status).')
    expect(bashApprovalSafetySummary({ command: 'git status && rm x' }))
      .toBe('Shell safety: review required (shell syntax detected).')
    expect(bashApprovalSafetySummary({ command: 'npm install' }))
      .toBe('Shell safety: review required (not in read-only command table).')
    expect(bashApprovalSafetySummary({ action: 'poll', session_id: 'bash_1' })).toBeNull()
  })

  it('detects shell syntax that can hide side effects', () => {
    expect(containsShellSyntax('git status')).toBe(false)
    for (const command of ['a && b', 'a || b', 'a | b', 'a; b', 'a > f', 'a < f', 'a & ', '$(x)', '`x`', 'a\nb']) {
      expect(containsShellSyntax(command)).toBe(true)
    }
  })
})
