import { describe, expect, it } from 'vitest'
import {
  containsSecretMaterial,
  redactSecrets,
  redactSecretText
} from './secret-redaction'

describe('secret redaction', () => {
  it('redacts secret-like object keys recursively', () => {
    expect(redactSecrets({
      apiKey: 'sk-test',
      nested: { Authorization: 'Bearer token-value' },
      safe: 'visible'
    })).toEqual({
      apiKey: '<redacted>',
      nested: { Authorization: '<redacted>' },
      safe: 'visible'
    })
  })

  it('redacts inline bearer and token text', () => {
    expect(redactSecretText('Authorization: Bearer abc123 token=secret-value')).toBe(
      'Authorization=<redacted> token=<redacted>'
    )
  })

  it('redacts bare provider keys in diagnostic text', () => {
    expect(redactSecretText('provider replied with sk-liveSecretValue1234567890 in the body')).toBe(
      'provider replied with <redacted> in the body'
    )
  })

  it('closes the shared credential corpus without rewriting safe metadata', () => {
    const secrets = [
      'bearer-value-123',
      'basic-value-123',
      'api-value-123',
      'password-value-123',
      'query-value-123',
      'driver-value-123',
      'AKIA1234567890ABCDEF',
      'ghp_1234567890abcdef',
      'ghr_1234567890abcdef',
      'xapp-1-A1234567890-ABCDEFGHIJ',
      'xoxb-1234567890-abcdefgh',
      'xoxe.xoxp-1234567890-abcdefgh'
    ]
    const raw = [
      `Authorization: Bearer ${secrets[0]}`,
      `Proxy-Authorization: Basic ${secrets[1]}`,
      `X_API_KEY=${secrets[2]}`,
      `postgres://analyst:${secrets[3]}@db.example/case`,
      `https://example.test/data?access_token=${secrets[4]}`,
      `postgresql+psycopg://analyst:${secrets[5]}@db.example/case`,
      `--api-key ${secrets[2]}`,
      secrets[6],
      secrets[7],
      secrets[8],
      secrets[9],
      secrets[10],
      secrets[11],
      '-----BEGIN OPENSSH PRIVATE KEY-----\nQUJDREVGR0hJSktMTU5PUFFS'
    ].join(' ')
    const projected = redactSecretText(raw)
    for (const secret of secrets) expect(projected).not.toContain(secret)
    expect(projected).not.toContain('analyst')
    expect(projected).not.toContain('QUJDREVGR0hJ')
    expect(containsSecretMaterial(projected)).toBe(false)
  })

  it('redacts opaque structured credential values but preserves presence and budget metadata', () => {
    const raw = {
      apiKey: 'opaque-value-123',
      password: 'opaque-value-123',
      github_token: 'opaque-value-123',
      HashicorpToken: 'opaque-vault-value-123',
      IssuerSecret: 'opaque-issuer-value-123',
      aws_secret_access_key: 'opaque-aws-secret-123',
      hasApiKey: true,
      token_budget: 64_000,
      key: 'run:run-1'
    }
    expect(containsSecretMaterial(raw)).toBe(true)
    const projected = redactSecrets(raw)
    expect(projected).toEqual({
      apiKey: '<redacted>',
      password: '<redacted>',
      github_token: '<redacted>',
      HashicorpToken: '<redacted>',
      IssuerSecret: '<redacted>',
      aws_secret_access_key: '<redacted>',
      hasApiKey: true,
      token_budget: 64_000,
      key: 'run:run-1'
    })
    expect(containsSecretMaterial(projected)).toBe(false)
  })

  it('treats only boolean exact presence metadata as safe', () => {
    const raw = {
      hasApiKey: 'opaque-value-123',
      hasCredential: true,
      hasPassword: false,
      hasIssuerSecret: 'opaque-issuer-value-123',
      isCredential: 'opaque-credential-value-123'
    }
    expect(containsSecretMaterial(raw)).toBe(true)
    expect(redactSecrets(raw)).toEqual({
      hasApiKey: '<redacted>',
      hasCredential: true,
      hasPassword: false,
      hasIssuerSecret: '<redacted>',
      isCredential: '<redacted>'
    })

    expect(redactSecretText('hasApiKey=true hasCredential=false')).toBe(
      'hasApiKey=true hasCredential=false'
    )
    expect(redactSecretText('hasApiKey=opaque-value-123')).toBe('hasApiKey=<redacted>')
    expect(containsSecretMaterial('hasApiKey=opaque-value-123')).toBe(true)
  })

  it.each([
    ['api_key="opaque secret tail"', 'opaque secret tail', ['opaque', 'secret tail']],
    ["client_secret='alpha beta gamma'", 'alpha beta gamma', ['alpha', 'beta gamma']],
    ['--api-key "opaque cli secret tail"', 'opaque cli secret tail', ['opaque', 'cli secret tail']],
    ['Authorization: Basic "opaque basic tail"', 'opaque basic tail', ['opaque', 'basic tail']],
    ['Authorization: "Bearer opaque bearer tail"', 'opaque bearer tail', ['opaque', 'bearer tail']],
    ['COOKIE="session=abc value=def"', 'session=abc value=def', ['session=abc', 'value=def']],
    ['AWS_SECRET_ACCESS_KEY=opaque-aws-secret-123', 'opaque-aws-secret-123', ['opaque-aws-secret-123']]
  ])('redacts the complete quoted or provider credential from %s', (raw, secret, leakFragments) => {
    const projected = redactSecretText(raw)
    expect(projected).not.toContain(secret)
    for (const fragment of leakFragments) expect(projected).not.toContain(fragment)
    expect(projected).toContain('<redacted>')
    expect(containsSecretMaterial(projected)).toBe(false)
  })

  it('keeps already-redacted and non-secret metadata byte-stable', () => {
    for (const value of [
      'api_key=<redacted>',
      'api_key="<redacted>"',
      '--api-key <redacted>',
      'Authorization: "Bearer <redacted>"',
      'token_budget=64000',
      'hasApiKey=true',
      'passwordHash=sha256:abcdef123456'
    ]) {
      expect(redactSecretText(value)).toBe(value)
      expect(containsSecretMaterial(value)).toBe(false)
    }
  })

  it('redacts complete and truncated private-key material', () => {
    for (const value of [
      'before\n-----BEGIN RSA PRIVATE KEY-----\nCOMPLETE_PRIVATE_KEY_BYTES\n-----END RSA PRIVATE KEY-----\nafter',
      'before\n-----BEGIN OPENSSH PRIVATE KEY-----\nTRUNCATED_PRIVATE_KEY_BYTES'
    ]) {
      const projected = redactSecretText(value)
      expect(projected).not.toContain('PRIVATE_KEY_BYTES')
      expect(projected).not.toContain('BEGIN RSA PRIVATE KEY')
      expect(projected).not.toContain('BEGIN OPENSSH PRIVATE KEY')
      expect(projected).toContain('<redacted>')
      expect(containsSecretMaterial(projected)).toBe(false)
    }
  })

  it('fails closed on cyclic, over-depth, and over-budget public values', () => {
    const cyclic: Record<string, unknown> = { safe: 'value' }
    cyclic.self = cyclic
    expect(containsSecretMaterial(cyclic)).toBe(true)

    let deep: Record<string, unknown> = { safe: 'value' }
    for (let index = 0; index < 40; index += 1) deep = { child: deep }
    expect(containsSecretMaterial(deep)).toBe(true)

    expect(containsSecretMaterial(new Array(100_001).fill('safe'))).toBe(true)
  })

  it('projects hostile object keys without prototype mutation', () => {
    const raw = JSON.parse('{"__proto__":{"password":"opaque-value-123"},"safe":"visible"}') as Record<string, unknown>
    const projected = redactSecrets(raw) as Record<string, unknown>
    expect(Object.prototype.hasOwnProperty.call(projected, '__proto__')).toBe(true)
    expect(projected.__proto__).toEqual({ password: '<redacted>' })
    expect(projected.safe).toBe('visible')
    expect(({} as Record<string, unknown>).password).toBeUndefined()
  })
})
