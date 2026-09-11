import { describe, expect, it } from 'vitest'
import {
  AnalytixCapabilitiesConfig,
  McpServerConfig
} from '../src/contracts/capabilities.js'
import { REDACTED_SECRET, redactSecrets } from '../src/config/secret-redaction.js'

describe('MCP config', () => {
  it('accepts trusted stdio MCP servers', () => {
    const server = McpServerConfig.parse({
      transport: 'stdio',
      command: 'node',
      args: ['server.js'],
      env: { API_KEY: 'secret' },
      trustScope: 'workspace',
      trustedWorkspaceRoots: ['/tmp/project']
    })

    expect(server.enabled).toBe(true)
    expect(server.transport).toBe('stdio')
    expect(server.timeoutMs).toBe(30_000)
  })

  it('accepts trusted streamable HTTP MCP servers', () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'streamable-http',
            url: 'https://mcp.example.test/mcp',
            headers: { Authorization: 'Bearer token' },
            trustScope: 'user'
          }
        }
      }
    })

    expect(config.mcp.enabled).toBe(true)
    expect(config.mcp.servers.github?.transport).toBe('streamable-http')
  })

  it('accepts only purpose-bound key-free MCP and extension account metadata', () => {
    const extension = McpServerConfig.parse({
      transport: 'streamable-http',
      url: 'https://extension.example.test/mcp',
      trustScope: 'user',
      accountCredential: {
        owner: 'extension',
        provider: 'extension-a',
        accountId: 'account-a',
        purpose: 'extension-provider-account-token'
      }
    })
    expect(extension.accountCredential).toEqual({
      owner: 'extension',
      provider: 'extension-a',
      accountId: 'account-a',
      purpose: 'extension-provider-account-token'
    })
    expect(extension.accountCredential).not.toHaveProperty('token')
    expect(extension.accountCredential).not.toHaveProperty('secret')
    expect(extension.accountCredential).not.toHaveProperty('credentialRef')

    expect(McpServerConfig.safeParse({
      transport: 'streamable-http',
      url: 'https://extension.example.test/mcp',
      trustScope: 'user',
      headers: { Authorization: 'Bearer ordinary-config-token' },
      accountCredential: {
        owner: 'extension',
        provider: 'extension-a',
        accountId: 'account-a',
        purpose: 'extension-provider-account-token'
      }
    }).success).toBe(false)
  })

  it('accepts a key-free exact OAuth tuple only when bound to one MCP account scope', () => {
    const oauthBinding = {
      schemaVersion: 1 as const,
      issuer: 'https://issuer.example.test/',
      authorizationEndpoint: 'https://login.example.test/authorize',
      tokenEndpoint: 'https://tokens.example.test/token',
      revocationEndpoint: 'https://tokens.example.test/revoke',
      clientId: 'public-client',
      scopes: ['openid', 'profile'],
      redirectModeVersion: 1 as const
    }
    expect(McpServerConfig.safeParse({
      transport: 'streamable-http',
      url: 'https://mcp.example.test/mcp',
      trustScope: 'user',
      accountCredential: {
        owner: 'mcp', provider: 'server-a', accountId: 'account-a', purpose: 'mcp-oauth-access-token'
      },
      oauthBinding
    }).success).toBe(true)
    expect(McpServerConfig.safeParse({
      transport: 'streamable-http', url: 'https://mcp.example.test/mcp', trustScope: 'user', oauthBinding
    }).success).toBe(false)
    expect(McpServerConfig.safeParse({
      transport: 'streamable-http',
      url: 'https://mcp.example.test/mcp',
      trustScope: 'user',
      accountCredential: {
        owner: 'mcp', provider: 'server-a', accountId: 'account-a', purpose: 'mcp-oauth-access-token'
      },
      oauthBinding: { ...oauthBinding, tokenEndpoint: 'http://localhost/token' }
    }).success).toBe(false)
    for (const tokenEndpoint of [
      'http://127.1/token',
      'https://tokens.example.test/%2fredirect',
      'https://tokens.example.test/token?next=https://other.invalid/'
    ]) {
      expect(McpServerConfig.safeParse({
        transport: 'streamable-http',
        url: 'https://mcp.example.test/mcp',
        trustScope: 'user',
        accountCredential: {
          owner: 'mcp', provider: 'server-a', accountId: 'account-a', purpose: 'mcp-oauth-access-token'
        },
        oauthBinding: { ...oauthBinding, tokenEndpoint }
      }).success).toBe(false)
    }
    expect(McpServerConfig.safeParse({
      transport: 'streamable-http',
      url: 'https://mcp.example.test/mcp',
      trustScope: 'user',
      accountCredential: {
        owner: 'mcp', provider: 'server-a', accountId: 'account-a', purpose: 'mcp-oauth-access-token'
      },
      oauthBinding: { ...oauthBinding, clientSecret: 'must-not-be-accepted' }
    }).success).toBe(false)
  })

  it('rejects stdio servers without commands', () => {
    const result = McpServerConfig.safeParse({
      transport: 'stdio',
      trustScope: 'workspace',
      trustedWorkspaceRoots: ['/tmp/project']
    })

    expect(result.success).toBe(false)
    expect(result.error?.issues.map((issue) => issue.message).join('\n')).toMatch(/require command/)
  })

  it('rejects HTTP servers without valid URLs', () => {
    const missing = McpServerConfig.safeParse({
      transport: 'streamable-http',
      trustScope: 'user'
    })
    const invalid = McpServerConfig.safeParse({
      transport: 'sse',
      url: 'file:///tmp/mcp.sock',
      trustScope: 'user'
    })

    expect(missing.success).toBe(false)
    expect(invalid.success).toBe(false)
  })

  it('requires workspace roots for workspace-scoped trust', () => {
    const result = McpServerConfig.safeParse({
      transport: 'stdio',
      command: 'node',
      trustScope: 'workspace'
    })

    expect(result.success).toBe(false)
    expect(result.error?.issues.map((issue) => issue.message).join('\n')).toMatch(/trusted workspace/)
  })

  it('redacts common secret fields in diagnostics', () => {
    const redacted = redactSecrets({
      headers: {
        Authorization: 'Bearer token',
        'X-Api-Key': 'key'
      },
      env: {
        NORMAL: 'visible',
        CLIENT_SECRET: 'secret',
        PASSWORD: 'pw'
      }
    })

    expect(redacted.headers.Authorization).toBe(REDACTED_SECRET)
    expect(redacted.headers['X-Api-Key']).toBe(REDACTED_SECRET)
    expect(redacted.env.CLIENT_SECRET).toBe(REDACTED_SECRET)
    expect(redacted.env.PASSWORD).toBe(REDACTED_SECRET)
    expect(redacted.env.NORMAL).toBe('visible')
  })
})
