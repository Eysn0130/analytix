let seedPromise: Promise<void> | null = null

/**
 * Bundled mascot seeding is retired. Keep the entry point so older IPC code
 * can call it while the app no longer ships the legacy image pack.
 */
export function ensureBundledUiPlugins(_analytixHomeDir: string): Promise<void> {
  seedPromise ??= Promise.resolve()
  return seedPromise
}
