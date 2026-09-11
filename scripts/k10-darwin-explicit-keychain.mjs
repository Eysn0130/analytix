import { createHash, randomBytes } from 'node:crypto'
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdirSync,
  realpathSync
} from 'node:fs'
import { spawn } from 'node:child_process'
import { isAbsolute, join, relative, resolve, sep } from 'node:path'

const SECURITY_PATH = '/usr/bin/security'
const EXPECT_PATH = '/usr/bin/expect'
const KEYCHAIN_DIRECTORY = 'darwin-secret-store-keychain'
const KEYCHAIN_REQUEST_NAME = 'analytix-task.keychain-db'
const OPERATION_TIMEOUT_MS = 20_000
const SAFE_ABSOLUTE_TOKEN_PATTERN = /^\/[A-Za-z0-9._/-]+$/u

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function strictDescendant(parent, candidate) {
  const value = relative(parent, candidate)
  return Boolean(value) && value !== '..' && !value.startsWith(`..${sep}`) && !isAbsolute(value)
}

function exactSafeAbsoluteToken(path) {
  return typeof path === 'string' && path.length <= 1024 && path === path.trim() &&
    !/login\.keychain/iu.test(path) && SAFE_ABSOLUTE_TOKEN_PATTERN.test(path) && isAbsolute(path) && resolve(path) === path
}

function exactOwnerOnlyDirectory(path) {
  try {
    const identity = lstatSync(path)
    return identity.isDirectory() && !identity.isSymbolicLink() &&
      identity.uid === process.getuid() && (identity.mode & 0o077) === 0 &&
      realpathSync(path) === path
  } catch {
    return false
  }
}

function exactOwnerOnlyDatabase(path) {
  try {
    const identity = lstatSync(path)
    return identity.isFile() && !identity.isSymbolicLink() && identity.nlink === 1 &&
      identity.uid === process.getuid() && (identity.mode & 0o777) === 0o600 &&
      identity.size > 0 && realpathSync(path) === path
  } catch {
    return false
  }
}

function expectProgram(operation, keychainPath) {
  if (operation !== 'create-keychain' && operation !== 'unlock-keychain') {
    throw new Error('k10_task_keychain_operation_invalid')
  }
  if (!exactSafeAbsoluteToken(keychainPath)) {
    throw new Error('k10_task_keychain_path_invalid')
  }
  const pathHex = Buffer.from(keychainPath, 'utf8').toString('hex')
  return [
    'set timeout 20',
    'log_user 0',
    'set secret [gets stdin]',
    'if {[string length $secret] < 32} { exit 126 }',
    `set keychain [encoding convertfrom utf-8 [binary format H* {${pathHex}}]]`,
    'set prompts 0',
    `spawn -noecho ${SECURITY_PATH} ${operation} -- $keychain`,
    'expect {',
    '  -re {(?i)password[^\\r\\n]*:} { incr prompts; send -- "$secret\\r"; exp_continue }',
    '  eof {}',
    '  timeout { exit 124 }',
    '}',
    'set secret {}',
    'set waited [wait]',
    'if {$prompts < 1} { exit 123 }',
    'set code [lindex $waited 3]',
    'if {![string is integer -strict $code]} { exit 125 }',
    'exit $code',
    ''
  ].join('\n')
}

async function runTaskKeychainOperation(operation, keychainPath, workingDirectory, password) {
  const child = spawn(EXPECT_PATH, ['-c', expectProgram(operation, keychainPath)], {
    cwd: workingDirectory,
    env: { PATH: '/usr/bin:/bin:/usr/sbin:/sbin', LANG: 'C', LC_ALL: 'C' },
    stdio: ['pipe', 'ignore', 'ignore']
  })
  const outcome = await new Promise((resolveOutcome) => {
    let timedOut = false
    const timeout = setTimeout(() => {
      timedOut = true
      child.kill('SIGTERM')
    }, OPERATION_TIMEOUT_MS)
    child.once('error', () => {
      clearTimeout(timeout)
      resolveOutcome({ ok: false, timedOut: false })
    })
    child.once('close', (code, signal) => {
      clearTimeout(timeout)
      resolveOutcome({ ok: !timedOut && code === 0 && !signal, timedOut })
    })
    child.stdin.on('error', () => {})
    child.stdin.write(password)
    child.stdin.end(Buffer.from('\n', 'ascii'))
  })
  if (!outcome.ok) {
    throw new Error(outcome.timedOut
      ? 'k10_task_keychain_operation_timed_out'
      : 'k10_task_keychain_operation_failed')
  }
}

export function k10DarwinTaskKeychainPaths(isolationRoot) {
  if (!exactSafeAbsoluteToken(isolationRoot)) {
    throw new Error('k10_task_keychain_root_invalid')
  }
  const parentDir = join(isolationRoot, KEYCHAIN_DIRECTORY)
  const requestedPath = join(parentDir, KEYCHAIN_REQUEST_NAME)
  return Object.freeze({ parentDir, requestedPath, databasePath: requestedPath })
}

export async function createK10DarwinExplicitTaskKeychain({ isolationRoot, cacheRoot }) {
  if (process.platform !== 'darwin' || typeof process.getuid !== 'function') {
    throw new Error('k10_task_keychain_requires_darwin')
  }
  if (!exactSafeAbsoluteToken(cacheRoot) || !exactSafeAbsoluteToken(isolationRoot) ||
    !exactOwnerOnlyDirectory(cacheRoot) || !exactOwnerOnlyDirectory(join(cacheRoot, 'tmp')) ||
    !exactOwnerOnlyDirectory(isolationRoot) ||
    !strictDescendant(join(cacheRoot, 'tmp'), isolationRoot) || realpathSync(isolationRoot) !== isolationRoot) {
    throw new Error('k10_task_keychain_root_invalid')
  }
  const paths = k10DarwinTaskKeychainPaths(isolationRoot)
  mkdirSync(paths.parentDir, { recursive: false, mode: 0o700 })
  chmodSync(paths.parentDir, 0o700)
  if (!exactOwnerOnlyDirectory(paths.parentDir) ||
    existsSync(paths.requestedPath) || existsSync(paths.databasePath)) {
    throw new Error('k10_task_keychain_preexisting_state_rejected')
  }
  const password = Buffer.from(randomBytes(32).toString('hex'), 'ascii')
  let disposed = false
  let unlockCount = 0
  try {
    await runTaskKeychainOperation('create-keychain', paths.requestedPath, isolationRoot, password)
    if (!exactOwnerOnlyDatabase(paths.databasePath)) {
      throw new Error('k10_task_keychain_database_invalid')
    }
  } catch (error) {
    password.fill(0)
    disposed = true
    throw error
  }
  const evidence = () => Object.freeze({
    ready: !disposed && exactOwnerOnlyDatabase(paths.databasePath),
    unlockCount,
    reusedForTwoLaunches: unlockCount >= 2,
    requestedPathFingerprint: sha256(paths.requestedPath),
    databasePathFingerprint: sha256(paths.databasePath),
    passwordTransport: 'process-memory-to-stdin-to-task-owned-pty',
    passwordInArgv: false,
    passwordInEnvironment: false,
    passwordInFile: false
  })
  return Object.freeze({
    paths,
    evidence,
    async unlockForLaunch() {
      if (disposed || !exactOwnerOnlyDatabase(paths.databasePath)) {
        throw new Error('k10_task_keychain_binding_drift')
      }
      await runTaskKeychainOperation('unlock-keychain', paths.databasePath, isolationRoot, password)
      unlockCount += 1
      return evidence()
    },
    dispose() {
      if (disposed) return
      disposed = true
      password.fill(0)
    }
  })
}
