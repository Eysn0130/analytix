export const LOCAL_DISPLAY_RUNTIME_PATHS_V1 = [
  '/v1/local-display/object-editing',
  '/v1/local-display/plugin-package-host',
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
] as const

const localDisplayRuntimePathSetV1 = new Set<string>(LOCAL_DISPLAY_RUNTIME_PATHS_V1)

export function isLocalDisplayRuntimePathV1(path: string): boolean {
  return localDisplayRuntimePathSetV1.has(path)
}
