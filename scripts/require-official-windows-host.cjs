function unsignedQaOverrideEnabled(env = process.env) {
  return /^(?:1|true|yes|on)$/iu.test(String(env.ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA || '').trim())
}

function expectedSignerConfigured(env = process.env) {
  return /^[0-9a-f]{40}$/iu.test(String(env.ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 || '').trim())
}

if (require.main === module) {
  if (
    process.platform === 'win32' &&
    process.arch === 'x64' &&
    !unsignedQaOverrideEnabled() &&
    expectedSignerConfigured()
  ) {
    console.log('[official-win] Windows x64 host verified for official packaging.')
    process.exit(0)
  }

  console.error(
    [
      '[official-win] Official Windows x64 release packaging must run on a Windows x64 host.',
      '[official-win] ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA must be absent for official packaging.',
      '[official-win] ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 must pin the approved signer thumbprint.',
      '[official-win] This ensures the final Standard Windows bootstrapper, native binaries, and Authenticode checks use the release host.',
      '[official-win] Use npm run release:win or npm run dist:win:official on the Windows build machine.'
    ].join('\n')
  )
  process.exit(1)
}

module.exports = {
  isOfficialWindowsHost() {
    return process.platform === 'win32' && process.arch === 'x64'
  },
  expectedSignerConfigured,
  unsignedQaOverrideEnabled
}
