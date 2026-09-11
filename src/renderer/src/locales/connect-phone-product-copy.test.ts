import { describe, expect, it } from 'vitest'
import enCommon from '../locales/en/common.json'
import enSettings from '../locales/en/settings.json'
import zhCommon from '../locales/zh/common.json'
import zhSettings from '../locales/zh/settings.json'

type LocaleMap = Record<string, unknown>

function valuesWithPaths(value: unknown, path: string[] = []): Array<{ path: string; text: string }> {
  if (typeof value === 'string') return [{ path: path.join('.'), text: value }]
  if (!value || typeof value !== 'object' || Array.isArray(value)) return []
  return Object.entries(value as LocaleMap).flatMap(([key, nested]) => valuesWithPaths(nested, [...path, key]))
}

describe('Connect Phone product copy', () => {
  it('keeps user-visible locale strings off legacy product names', () => {
    const localeValues = [
      ...valuesWithPaths(enCommon, ['en', 'common']),
      ...valuesWithPaths(enSettings, ['en', 'settings']),
      ...valuesWithPaths(zhCommon, ['zh', 'common']),
      ...valuesWithPaths(zhSettings, ['zh', 'settings'])
    ]

    for (const entry of localeValues) {
      expect(entry.text, entry.path).not.toMatch(/\b(?:Claw|LobsterAI)\b/)
    }
  })

  it('keeps rewind and rollback locale strings off proof/oracle construction terms', () => {
    const localeValues = [
      ...valuesWithPaths(enCommon, ['en', 'common']),
      ...valuesWithPaths(zhCommon, ['zh', 'common'])
    ].filter((entry) => /\.(?:rewind|rollback)/i.test(entry.path))

    for (const entry of localeValues) {
      expect(entry.text, entry.path).not.toMatch(/\b(?:proof|oracle|fixture|fake)\b/i)
    }
  })
})
