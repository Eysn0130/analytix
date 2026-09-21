import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import {
  isLocalDisplayRuntimePathV1,
  LOCAL_DISPLAY_RUNTIME_PATHS_V1
} from './local-display-runtime-paths'

describe('typed local-display runtime path allowlist', () => {
  it('admits the complete closed production path family including deterministic cleaning', () => {
    expect(LOCAL_DISPLAY_RUNTIME_PATHS_V1).toEqual([
      '/v1/local-display/generated-artifact',
      '/v1/local-display/object-editing',
      '/v1/local-display/workspace-read',
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
