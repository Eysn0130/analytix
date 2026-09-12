import { resolve } from 'path'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    alias: {
      '@analytix': resolve('src')
    }
  },
  test: {
    environment: 'node',
    tags: [
      { name: 'macos-integration', description: 'Real macOS process containment; required native CI lane.' }
    ],
    include: ['tests/**/*.test.ts', 'src/**/*.test.ts'],
    globals: false
  }
})
