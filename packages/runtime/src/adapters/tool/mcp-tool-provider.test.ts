import { homedir } from 'node:os'
import { describe, expect, it } from 'vitest'
import { buildMcpStdioEnvironment } from '../../tool-test-support/tool/mcp-tool-provider.js'

describe('buildMcpStdioEnvironment', () => {
  it('adds user-local Node paths even when GUI launch env does not expose HOME', () => {
    const env = buildMcpStdioEnvironment({}, {
      platform: 'darwin',
      baseEnv: {
        PATH: '/usr/bin:/bin'
      }
    })

    expect(env.PATH?.split(':')).toEqual(expect.arrayContaining([
      '/usr/bin',
      '/bin',
      `${homedir()}/.local/node/current/bin`,
      `${homedir()}/.local/bin`
    ]))
  })

  it('lets server-provided PATH entries take precedence', () => {
    const env = buildMcpStdioEnvironment({
      PATH: '/plugin/bin'
    }, {
      platform: 'darwin',
      baseEnv: {
        PATH: '/usr/bin:/bin'
      }
    })

    expect(env.PATH?.split(':').slice(0, 3)).toEqual([
      '/plugin/bin',
      '/opt/homebrew/bin',
      '/usr/local/bin'
    ])
  })
})
