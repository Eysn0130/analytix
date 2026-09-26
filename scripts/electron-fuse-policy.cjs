const { createHash } = require('node:crypto')

const ELECTRON_FUSE_POLICY_V1_CONTRACT = 'analytix.electron-fuse-policy/v1'
const ELECTRON_FUSE_POLICY_V1_DOMAIN = 'AnalytixElectronFusePolicyV1\0'

// These numeric keys are the public @electron/fuses FuseV1Options wire
// positions. Keep this as the sole packaged policy: electron-builder's
// automatic electronFuses pass is deliberately disabled so it cannot mutate
// the executable after the packaged payload authority has been issued.
const ELECTRON_FUSE_POLICY_V1 = Object.freeze({
  0: true, // RunAsNode: required by the packaged CLI launcher.
  // macOS cookie encryption uses Keychain. Existing encrypted Chromium state
  // is retained in its old session directory and is not opened by the new
  // versioned sessionData path.
  1: false, // EnableCookieEncryption.
  2: false, // EnableNodeOptionsEnvironmentVariable.
  3: false, // EnableNodeCliInspectArguments.
  4: true, // EnableEmbeddedAsarIntegrityValidation.
  5: true, // OnlyLoadAppFromAsar.
  // Supported Electron bundles do not contain a separately
  // generated browser_v8_context_snapshot.bin. Enabling this fuse without
  // that payload terminates the packaged app before its renderer starts.
  6: false, // LoadBrowserProcessSpecificV8Snapshot.
  7: false, // GrantFileProtocolExtraPrivileges.
  8: true, // WasmTrapHandlers.
  version: '1',
  strictlyRequireAllFuses: true
})

const ELECTRON_FUSE_POLICY_V1_OTHER = Object.freeze({
  ...ELECTRON_FUSE_POLICY_V1,
  1: true // Windows and Linux retain the previous cookie policy.
})

function canonicalJSON(value) {
  return JSON.stringify(value)
}

function electronFusePolicyV1(platform = 'darwin') {
  return { ...(platform === 'darwin' ? ELECTRON_FUSE_POLICY_V1 : ELECTRON_FUSE_POLICY_V1_OTHER) }
}

function electronFusePolicyV1Digest(platform = 'darwin') {
  return createHash('sha256')
    .update(ELECTRON_FUSE_POLICY_V1_DOMAIN, 'utf8')
    .update(canonicalJSON(electronFusePolicyV1(platform)), 'utf8')
    .digest('hex')
}

exports.ELECTRON_FUSE_POLICY_V1_CONTRACT = ELECTRON_FUSE_POLICY_V1_CONTRACT
exports.ELECTRON_FUSE_POLICY_V1 = ELECTRON_FUSE_POLICY_V1
exports.ELECTRON_FUSE_POLICY_V1_OTHER = ELECTRON_FUSE_POLICY_V1_OTHER
exports.electronFusePolicyV1 = electronFusePolicyV1
exports.electronFusePolicyV1Digest = electronFusePolicyV1Digest
