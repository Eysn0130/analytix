import { createInterface } from 'node:readline'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export function goTestSelection(packageName, testName) {
  let runs = 0
  let passes = 0
  let packagePasses = 0
  let invalid = false
  return {
    observe(event) {
      if (!event || typeof event !== 'object' || typeof event.Action !== 'string') {
        invalid = true
        return
      }
      if (event.Action === 'fail' || event.Action === 'skip') invalid = true
      if (event.Package !== packageName) return
      if (event.Test === testName && event.Action === 'run') runs++
      if (event.Test === testName && event.Action === 'pass') passes++
      if (!event.Test && event.Action === 'pass') packagePasses++
    },
    assert() {
      if (invalid || runs !== 1 || passes !== 1 || packagePasses !== 1) {
        throw new Error('Go selection failed: exact subtest and package must execute and pass once, without failures or skips.')
      }
    }
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [, , packageName, testName] = process.argv
  if (!packageName || !testName || process.argv.length !== 4) {
    console.error('Usage: go test -json ... | node go-test-selection.mjs <package> <exact-subtest>')
    process.exitCode = 1
  } else {
    const selection = goTestSelection(packageName, testName)
    for await (const line of createInterface({ input: process.stdin, crlfDelay: Infinity })) {
      // Preserve every original event, including all failure output.
      process.stdout.write(`${line}\n`)
      try { selection.observe(JSON.parse(line)) } catch { selection.observe(null) }
    }
    try {
      selection.assert()
      console.log('PASS exact Go subtest selection and package completion')
    } catch (error) {
      console.error(error.message)
      process.exitCode = 1
    }
  }
}
