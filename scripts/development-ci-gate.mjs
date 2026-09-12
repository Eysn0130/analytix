import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const requiredDevelopmentJobs = [
  'source-baseline', 'application-tests', 'macos-integration', 'filesystem-contracts', 'go-tests', 'backend-tests', 'rust-tests'
]

export function assertDevelopmentCISuccess(results) {
  if (!results || typeof results !== 'object' || Array.isArray(results) ||
    Object.keys(results).length !== requiredDevelopmentJobs.length ||
    Object.keys(results).some(name => !requiredDevelopmentJobs.includes(name))) {
    throw new Error('Development gate failed: missing or unexpected job results.')
  }
  const failed = requiredDevelopmentJobs.filter(name => results[name]?.result !== 'success')
  if (failed.length) throw new Error(`Development gate failed: ${failed.join(', ')} did not succeed.`)
  return true
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    let results
    try { results = JSON.parse(process.env.DEVELOPMENT_CI_RESULTS ?? '') } catch {
      throw new Error('Development gate failed: invalid job result input.')
    }
    assertDevelopmentCISuccess(results)
    console.log('PASS all required development CI jobs; scoped product acceptance is still required.')
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
