import { rmSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const runtimeRoot = resolve(here, '..')

rmSync(resolve(runtimeRoot, 'dist'), { recursive: true, force: true })
