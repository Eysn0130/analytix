import { spawn } from 'node:child_process'
import { createInterface } from 'node:readline'

// External acceptance is excluded before execution, never admitted as a skip.
export function goTestPartition(packageName, selected, requiredSubtests = []) {
  if (!packageName || !Array.isArray(selected) || selected.length === 0 ||
    selected.some(name => typeof name !== 'string' || !/^(Test|Example|Fuzz)[\p{L}\p{N}_]*$/u.test(name)) ||
    new Set(selected).size !== selected.length || !Array.isArray(requiredSubtests) ||
    new Set(requiredSubtests).size !== requiredSubtests.length ||
    requiredSubtests.some(name => typeof name !== 'string' || !name.includes('/') || !selected.includes(name.split('/')[0]))) {
    throw new Error('Go partition failed: invalid or empty selection.')
  }
  const states = new Map()
  let packagePasses = 0
  let invalid = false
  return {
    observeLine(line) {
      try { this.observe(JSON.parse(line)) } catch { this.observe(null) }
    },
    observe(event) {
      if (!event || typeof event !== 'object' || Array.isArray(event) || event.Package !== packageName ||
        !['start', 'run', 'pause', 'cont', 'output', 'pass', 'skip', 'fail'].includes(event.Action) ||
        (event.Test !== undefined && (typeof event.Test !== 'string' || !event.Test))) {
        invalid = true
        return
      }
      if (event.Action === 'fail' || event.Action === 'skip' || packagePasses !== 0) invalid = true
      if (event.Test !== undefined) {
        if (!selected.includes(event.Test.split('/')[0])) invalid = true
        if (event.Action === 'run') {
          if (states.has(event.Test)) invalid = true
          states.set(event.Test, 'run')
        } else if (event.Action === 'pass') {
          if (states.get(event.Test) !== 'run') invalid = true
          states.set(event.Test, 'pass')
        }
      } else if (event.Action === 'pass') packagePasses++
    },
    assert() {
      if (invalid || packagePasses !== 1 ||
        [...selected, ...requiredSubtests].some(name => states.get(name) !== 'pass') ||
        [...states.values()].some(state => state !== 'pass')) {
        throw new Error('Go partition failed: every selected test and observed child must run and pass once; failures, skips and incomplete JSON execution are rejected.')
      }
    }
  }
}

export async function runGoTestPartition(args, packageName, selected, requiredSubtests = []) {
  const partition = goTestPartition(packageName, selected, requiredSubtests)
  const child = spawn('go', args, { stdio: ['ignore', 'pipe', 'inherit'] })
  let spawnFailed = false
  const completion = new Promise(resolve => {
    child.once('error', () => { spawnFailed = true })
    child.once('close', (code, signal) => resolve({ code, signal }))
  })
  for await (const line of createInterface({ input: child.stdout, crlfDelay: Infinity })) {
    process.stdout.write(`${line}\n`)
    partition.observeLine(line)
  }
  const { code, signal } = await completion
  if (spawnFailed || signal || code !== 0) throw new Error('Go partition process failed; execution was not admitted.')
  partition.assert()
  console.log('PASS exact Go partition execution without failures or skips')
}
