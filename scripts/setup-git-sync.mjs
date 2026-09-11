import { existsSync, readdirSync } from 'node:fs'
import { isAbsolute, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { assertPublicHistory, gitRead, publicSourcePolicy } from './check-public-push.mjs'

export function setupGitSync({ cwd = process.cwd(), policy = publicSourcePolicy, hooksPath } = {}) {
  const root = gitRead(cwd, ['rev-parse', '--show-toplevel']).trim()
  assertPublicHistory(root, 'HEAD', policy)
  const wanted = resolve(hooksPath ?? resolve(root, '.githooks'))
  if (!existsSync(resolve(wanted, 'pre-push'))) throw new Error('The public pre-push hook is missing.')
  let configured = ''
  try { configured = gitRead(root, ['config', '--get', 'core.hooksPath']).trim() } catch {}
  if (configured && resolve(root, configured) !== wanted) throw new Error('An existing hooksPath must be reviewed before installing the public Git hook.')
  if (!configured) {
    const common = gitRead(root, ['rev-parse', '--git-common-dir']).trim()
    const hooks = resolve(isAbsolute(common) ? common : resolve(root, common), 'hooks')
    if (existsSync(hooks) && readdirSync(hooks).some(name => !name.endsWith('.sample'))) {
      throw new Error('Existing Git hooks must be preserved and reviewed before installation.')
    }
  }
  const remotes = gitRead(root, ['remote']).trim().split('\n')
  if (!remotes.includes('origin')) gitRead(root, ['remote', 'add', 'origin', 'https://github.com/Eysn0130/analytix.git'])
  for (const [key, value] of Object.entries({
    'branch.main.remote': 'origin',
    'branch.main.merge': 'refs/heads/main',
    'branch.main.rebase': 'false',
    'pull.ff': 'only',
    'pull.rebase': 'false',
    'push.default': 'simple',
    'push.followTags': 'false',
    'core.hooksPath': wanted
  })) gitRead(root, ['config', '--local', key, value])
  return root
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    setupGitSync()
    console.log('Configured origin/main tracking, fast-forward-only pull, simple push and the public-history hook.')
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
