import { resolve } from 'path'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    alias: {
      '@renderer': resolve('src/renderer/src'),
      '@shared': resolve('src/shared')
    }
  },
  test: {
    environment: 'node',
    maxWorkers: 1,
    fileParallelism: false,
    tags: [
      { name: 'macos-integration', description: 'Real macOS process containment; required native CI lane.' }
    ],
    include: [
      'src/**/*.test.{ts,tsx}',
      'packages/runtime/tests/**/*.test.{ts,tsx}',
      'packages/runtime/src/**/*.test.{ts,tsx}'
    ]
  }
})
