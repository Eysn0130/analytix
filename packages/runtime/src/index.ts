/**
 * Analytix public surface.
 *
 * The package exposes contracts, schemas, telemetry helpers, and the
 * `analytix serve` Go launcher compatibility surface. The production agent
 * runtime lives in packages/runtime-go and is not exported from this package.
 */

export * from './contracts/index.js'
export * from './config/analytix-config.js'
export * from './telemetry/index.js'
export * from './cli/index.js'
