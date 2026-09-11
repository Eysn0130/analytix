import { readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..')
export const requiredBuildOutputs = [
  'out/main/index.js', 'out/preload/index.cjs', 'out/renderer/index.html',
  'packages/runtime/dist/index.js', 'packages/runtime/dist/cli/serve-entry.js',
  'packages/runtime/dist/contracts/index.js'
]

export function verifySourceBuild(root = repo) {
  const missing = requiredBuildOutputs.filter(name => {
    try { const stat = statSync(join(root, name)); return !stat.isFile() || stat.size === 0 } catch { return true }
  })
  if (!missing.includes('out/renderer/index.html')) {
    const html = readFileSync(join(root, 'out/renderer/index.html'), 'utf8')
    const scripts = [...html.matchAll(/<script\b[^>]*\bsrc="([^"]+)"/g)].map(match => match[1])
    if (!scripts.length) missing.push('renderer script entry')
    for (const script of scripts) {
      if (!script.startsWith('./assets/') || script.includes('..')) { missing.push('unexpected renderer script path'); continue }
      try { if (!statSync(join(root, 'out/renderer', script)).size) missing.push(script) } catch { missing.push(script) }
    }
  }
  return { passed: missing.length === 0, scope: 'Source build outputs only; not GUI, Go runtime or installer acceptance', missing }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = verifySourceBuild()
  console.log(JSON.stringify(result, null, 2))
  if (!result.passed) process.exitCode = 1
}
