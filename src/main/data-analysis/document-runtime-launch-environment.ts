import {
  DOCUMENT_RUNTIME_UNAVAILABLE,
  packagedDocumentRuntimeEnvironment
} from './document-runtime-authority'
import { sanitizePackagedNativeEnvironment } from './native-runtime-paths'

export type DataAnalysisBackendLaunchEnvironment = {
  env: NodeJS.ProcessEnv
  documentRuntimeAvailable: boolean
  documentRuntimeBlocker: string
}

export function resolveDataAnalysisBackendLaunchEnvironment(options: {
  env?: NodeJS.ProcessEnv
  resourcesRoot: string
  packaged: boolean
  platform?: NodeJS.Platform
  arch?: string
  testOnlyDocumentAuthorityLock?: Buffer
}): DataAnalysisBackendLaunchEnvironment {
  const platform = options.platform || process.platform
  const env = sanitizePackagedNativeEnvironment(options.env || process.env, options.packaged, platform)
  if (!options.packaged) {
    return { env, documentRuntimeAvailable: false, documentRuntimeBlocker: DOCUMENT_RUNTIME_UNAVAILABLE }
  }
  try {
    return {
      env: {
        ...env,
        ...packagedDocumentRuntimeEnvironment({
          resourcesRoot: options.resourcesRoot,
          packaged: true,
          platform,
          arch: options.arch || process.arch,
          testOnlyAuthorityLock: options.testOnlyDocumentAuthorityLock
        })
      },
      documentRuntimeAvailable: true,
      documentRuntimeBlocker: ''
    }
  } catch {
    return { env, documentRuntimeAvailable: false, documentRuntimeBlocker: DOCUMENT_RUNTIME_UNAVAILABLE }
  }
}
