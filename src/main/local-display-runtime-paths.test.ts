import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import {
  isLocalDisplayRuntimePathV1,
  LOCAL_DISPLAY_RUNTIME_PATHS_V1
} from './local-display-runtime-paths'

describe('typed local-display runtime path allowlist', () => {
  it('admits the complete closed production path family including deterministic cleaning', () => {
    expect(LOCAL_DISPLAY_RUNTIME_PATHS_V1).toEqual([
      '/v1/runtime/task-jobs/steer',
      '/v1/local-display/user-input',
      '/v1/local-display/tool-result-snapshot',
      '/v1/local-display/schedule-admission',
      '/v1/local-display/process/read',
      '/v1/local-display/process/target',
      '/v1/local-display/generated-artifact',
      '/v1/local-display/object-editing',
      '/v1/local-display/browser-selection',
  '/v1/local-display/workspace-read',
  '/v1/local-display/inline-completion',
      '/v1/local-display/plugin-package-host',
      '/v1/local-display/office-private-admission',
      '/v1/local-display/import-mapping-preview',
      '/v1/local-display/cleaning-diff-preview',
      '/v1/local-display/direct-source-preview',
      '/v1/local-display/accepted-slot-display',
      '/v1/local-display/funds-import/stage',
      '/v1/local-display/funds-import/confirm',
      '/v1/local-display/funds-import/cancel',
      '/v1/local-display/funds-import/status',
      '/v1/local-display/funds-cleaning/run',
      '/v1/local-display/funds-cleaning/revoke'
    ])
    for (const path of LOCAL_DISPLAY_RUNTIME_PATHS_V1) {
      expect(isLocalDisplayRuntimePathV1(path)).toBe(true)
    }
  })

  it('rejects unknown, prefixed, and query-bearing paths', () => {
    expect(isLocalDisplayRuntimePathV1('/v1/local-display/funds-cleaning')).toBe(false)
    expect(isLocalDisplayRuntimePathV1('/v1/local-display/tool-result-snapshot?thread=forged')).toBe(false)
    expect(isLocalDisplayRuntimePathV1('/v1/local-display/tool-result-snapshot/')).toBe(false)
    expect(isLocalDisplayRuntimePathV1('/prefix/v1/local-display/funds-cleaning/run')).toBe(false)
    expect(isLocalDisplayRuntimePathV1('/v1/local-display/funds-cleaning/run?case=forged')).toBe(false)
  })

  it('guards the production typed-local transport with the same closed predicate', () => {
    const mainSource = readFileSync(new URL('./index.ts', import.meta.url), 'utf8')
    expect(mainSource).toContain("import { isLocalDisplayRuntimePathV1 } from './local-display-runtime-paths'")
    expect(mainSource).toContain('if (!isLocalDisplayRuntimePathV1(path)) {')
    expect(mainSource).not.toContain('const LOCAL_DISPLAY_RUNTIME_PATHS = new Set(')
  })
})


it('admits only the exact Main Office admission route',()=>{
  const path='/v1/local-display/office-private-admission'
  expect(isLocalDisplayRuntimePathV1(path)).toBe(true)
  for(const value of [path+'/',path+'?root=/private',path+'#fragment',path+'/assets','/prefix'+path,path.replace('/v1/','/')])expect(isLocalDisplayRuntimePathV1(value)).toBe(false)
})

it('keeps protected process routes exact and bounds their Main transport reader', () => {
  for (const path of ['/v1/local-display/process/read', '/v1/local-display/process/target']) {
    expect(isLocalDisplayRuntimePathV1(path)).toBe(true)
    for (const invalid of [path + '/', path + '?viewerToken=forged', path + '#fragment', '/prefix' + path, path + '/reveal']) {
      expect(isLocalDisplayRuntimePathV1(invalid)).toBe(false)
    }
  }
  const source = readFileSync(new URL('./index.ts', import.meta.url), 'utf8')
  expect(source).toContain('path === PROCESS_LOCAL_DISPLAY_READ_PATH_V1 || path === PROCESS_LOCAL_DISPLAY_TARGET_PATH_V1')
  expect(source).toContain('size > protectedResponseMaxBytes')
  expect(source).toContain("new TextDecoder('utf-8', { fatal: true })")
  expect(source).toContain('captureProcessDisplayRuntimeOwner: () => {')
})

it('keeps protected question readback on its exact private route', () => {
  const path = '/v1/local-display/user-input'
  expect(isLocalDisplayRuntimePathV1(path)).toBe(true)
  for (const invalid of [path + '/', path + '?requestId=forged', path + '#fragment', '/prefix' + path, path + '/read']) {
    expect(isLocalDisplayRuntimePathV1(invalid)).toBe(false)
  }
})

it('admits the exact Host child-input route and bounds its authorized Main transport', () => {
  const path = '/v1/runtime/task-jobs/steer'
  expect(isLocalDisplayRuntimePathV1(path)).toBe(true)
  for (const invalid of [path + '/', path + '?mode=inspect', path + '#fragment', '/prefix' + path]) {
    expect(isLocalDisplayRuntimePathV1(invalid)).toBe(false)
  }
  const source = readFileSync(new URL('./index.ts', import.meta.url), 'utf8')
  expect(source).toContain("path === '/v1/runtime/task-jobs/steer' || path === '/v1/local-display/schedule-admission' ? 4096")
  expect(source).toContain("headers.set('X-Analytix-Local-Display', 'typed-v1')")
})
