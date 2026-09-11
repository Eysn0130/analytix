import { execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, readFileSync, realpathSync } from 'node:fs'
import { isAbsolute, resolve } from 'node:path'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'

const MARKER_PREFIX = 'ANALYTIX_BACKEND_GENERATION_CONSUMED '
const SHA256_PATTERN = /^[a-f0-9]{64}$/
const MAX_OUTPUT_BYTES = 16 << 10
const EXECUTION_TIMEOUT_MS = 30_000

export type ConsumedBackendGenerationV1 = Readonly<{
  generation: number
  allocationRecordDigest: string
}>

type AuthorityExecutionResultV1 = Readonly<{
  stdout: string
  stderr: string
}>

export type BackendGenerationAuthorityExecutorV1 = (
  command: string,
  args: readonly string[],
  signal: AbortSignal | undefined
) => Promise<AuthorityExecutionResultV1>

type ExecutableIdentityV1 = Readonly<{
  device: bigint
  inode: bigint
  size: bigint
  sha256: string
}>

export class BackendGenerationAllocatorV1 {
  private queue: Promise<void> = Promise.resolve()
  private readonly command: string
  private readonly userDataRealPath: string
  private readonly executableIdentity: ExecutableIdentityV1
  private readonly execute: BackendGenerationAuthorityExecutorV1

  constructor(options: {
    runtimeServerPath: string
    userDataRealPath: string
    execute?: BackendGenerationAuthorityExecutorV1
  }) {
    this.command = validateAbsoluteRealPath(options.runtimeServerPath, 'runtime server')
    this.userDataRealPath = validateAbsoluteRealPath(options.userDataRealPath, 'user data')
    this.executableIdentity = readExecutableIdentityV1(this.command)
    this.execute = options.execute ?? executeAuthorityCommandV1
  }

  consume(signal?: AbortSignal): Promise<ConsumedBackendGenerationV1> {
    const operation = this.queue.then(() => this.consumeOnce(signal))
    this.queue = operation.then(() => undefined, () => undefined)
    return operation
  }

  executableSHA256(): string {
    return this.executableIdentity.sha256
  }

  runtimeServerRealPath(): string {
    return this.command
  }

  assertExecutableIdentity(): void {
    requireExecutableIdentityV1(this.command, this.executableIdentity)
  }

  private async consumeOnce(signal?: AbortSignal): Promise<ConsumedBackendGenerationV1> {
    if (signal?.aborted) throw fixedAllocatorErrorV1()
    requireExecutableIdentityV1(this.command, this.executableIdentity)
    let result: AuthorityExecutionResultV1
    try {
      result = await this.execute(this.command, [
        'authority',
        'consume-backend-generation-v1',
        '--user-data-dir',
        this.userDataRealPath
      ], signal)
    } catch {
      throw fixedAllocatorErrorV1()
    }
    if (signal?.aborted || result.stderr !== '') throw fixedAllocatorErrorV1()
    requireExecutableIdentityV1(this.command, this.executableIdentity)
    return parseBackendGenerationConsumedMarkerV1(result.stdout)
  }
}

export function parseBackendGenerationConsumedMarkerV1(
  output: string
): ConsumedBackendGenerationV1 {
  if (typeof output !== 'string' || Buffer.byteLength(output, 'utf8') > MAX_OUTPUT_BYTES ||
    !output.startsWith(MARKER_PREFIX) || !output.endsWith('\n') ||
    output.slice(0, -1).includes('\n') || output.includes('\r')) {
    throw fixedAllocatorErrorV1()
  }
  const bodyText = output.slice(MARKER_PREFIX.length, -1)
  try {
    const parsed = parseStrictJsonObject(Buffer.from(bodyText, 'utf8'), {
      maxBytes: 1024,
      maxDepth: 2,
      maxTokens: 16,
      maxStringBytes: 128,
      maxNumberBytes: 16
    })
    const keys = Object.keys(parsed).sort()
    if (keys.length !== 3 || keys[0] !== 'allocationRecordDigest' || keys[1] !== 'generation' ||
      keys[2] !== 'schemaVersion' || parsed.schemaVersion !== 1 ||
      !Number.isSafeInteger(parsed.generation) || Number(parsed.generation) <= 0 ||
      !SHA256_PATTERN.test(String(parsed.allocationRecordDigest))) {
      throw fixedAllocatorErrorV1()
    }
    const canonical = JSON.stringify({
      schemaVersion: 1,
      generation: parsed.generation,
      allocationRecordDigest: parsed.allocationRecordDigest
    })
    if (bodyText !== canonical) throw fixedAllocatorErrorV1()
    return Object.freeze({
      generation: Number(parsed.generation),
      allocationRecordDigest: String(parsed.allocationRecordDigest)
    })
  } catch {
    throw fixedAllocatorErrorV1()
  }
}

function executeAuthorityCommandV1(
  command: string,
  args: readonly string[],
  signal: AbortSignal | undefined
): Promise<AuthorityExecutionResultV1> {
  return new Promise((accept, reject) => {
    execFile(command, [...args], {
      encoding: 'utf8',
      env: {},
      maxBuffer: MAX_OUTPUT_BYTES,
      timeout: EXECUTION_TIMEOUT_MS,
      windowsHide: true,
      ...(signal ? { signal } : {})
    }, (error, stdout, stderr) => {
      if (error) {
        reject(fixedAllocatorErrorV1())
        return
      }
      accept({ stdout, stderr })
    })
  })
}

function validateAbsoluteRealPath(value: string, label: string): string {
  if (typeof value !== 'string' || value !== value.trim() || !isAbsolute(value) || resolve(value) !== value) {
    throw new Error(`${label} authority path is invalid`)
  }
  let real = ''
  try {
    real = realpathSync(value)
  } catch {
    throw new Error(`${label} authority path is invalid`)
  }
  return real
}

function readExecutableIdentityV1(path: string): ExecutableIdentityV1 {
  const stat = lstatSync(path, { bigint: true })
  if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1n || stat.size <= 0n) {
    throw new Error('runtime server authority executable is invalid')
  }
  const body = readFileSync(path)
  try {
    return Object.freeze({
      device: stat.dev,
      inode: stat.ino,
      size: stat.size,
      sha256: createHash('sha256').update(body).digest('hex')
    })
  } finally {
    body.fill(0)
  }
}

function requireExecutableIdentityV1(path: string, expected: ExecutableIdentityV1): void {
  const current = readExecutableIdentityV1(path)
  if (current.device !== expected.device || current.inode !== expected.inode ||
    current.size !== expected.size || current.sha256 !== expected.sha256) {
    throw fixedAllocatorErrorV1()
  }
}

function fixedAllocatorErrorV1(): Error {
  return new Error('Backend generation authority is unavailable.')
}
