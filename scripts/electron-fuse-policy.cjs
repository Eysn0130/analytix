const { createHash } = require('node:crypto')

const ELECTRON_FUSE_POLICY_V1_CONTRACT = 'analytix.electron-fuse-policy/v1'
const ELECTRON_FUSE_POLICY_V1_DOMAIN = 'AnalytixElectronFusePolicyV1\0'

// These numeric keys are the public @electron/fuses FuseV1Options wire
// positions. Keep this as the sole packaged policy: electron-builder's
// automatic electronFuses pass is deliberately disabled so it cannot mutate
// the executable after the packaged payload authority has been issued.
const ELECTRON_FUSE_POLICY_V1 = Object.freeze({
  0: true, // RunAsNode: required by the packaged CLI launcher.
  1: true, // EnableCookieEncryption.
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

function canonicalJSON(value) {
  return JSON.stringify(value)
}

function electronFusePolicyV1() {
  return { ...ELECTRON_FUSE_POLICY_V1 }
}

function electronFusePolicyV1Digest() {
  return createHash('sha256')
    .update(ELECTRON_FUSE_POLICY_V1_DOMAIN, 'utf8')
    .update(canonicalJSON(ELECTRON_FUSE_POLICY_V1), 'utf8')
    .digest('hex')
}

exports.ELECTRON_FUSE_POLICY_V1_CONTRACT = ELECTRON_FUSE_POLICY_V1_CONTRACT
exports.ELECTRON_FUSE_POLICY_V1 = ELECTRON_FUSE_POLICY_V1
exports.electronFusePolicyV1 = electronFusePolicyV1
exports.electronFusePolicyV1Digest = electronFusePolicyV1Digest
