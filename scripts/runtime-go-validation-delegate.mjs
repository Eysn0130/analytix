#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

export function runValidationDelegate(script, args = process.argv.slice(2)) {
  const result = spawnSync(process.execPath, [join(here, script), ...args], {
    cwd: process.cwd(),
    env: process.env,
    stdio: 'inherit'
  })
  if (result.error) {
    console.error(result.error.message)
    process.exit(1)
  }
  if (result.signal) {
    process.kill(process.pid, result.signal)
  }
  process.exit(result.status ?? 1)
}
