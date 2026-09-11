#!/usr/bin/env node

const arch = process.argv[2]
if (arch !== 'arm64' && arch !== 'x64') {
  console.error('Usage: node scripts/zip-mac-app.cjs <arm64|x64>')
  process.exit(1)
}

// A standalone zipper cannot prove that an existing .app was produced by the
// current source, passed afterPack, or retained native release authority. Keep
// this public package-shaped command fail-closed until task 9.11a supplies a
// same-build package authority that this process can verify cryptographically.
console.error(`[zip-mac-app] package_release_authority_unavailable (${arch})`)
process.exit(1)
