import { mkdtempSync, existsSync, mkdirSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import type { AppSettingsV1 } from '../../shared/app-settings'
import {
  buildWriteInfographicPrompt,
  requestWriteInfographic,
  type PrivateMediaImageRequest
} from './write-infographic-service'

let workspace: string
const PNG_BYTES = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64'
)

function settingsWithImageGen(overrides: Record<string, unknown> = {}): AppSettingsV1 {
  return {
    runtime: {
      imageGeneration: {
        enabled: true,
        baseUrl: 'https://must-not-cross-main.example/v1',
        model: 'must-not-cross-main-model',
        defaultSize: '',
        timeoutMs: 180000,
        ...overrides
      }
    }
  } as unknown as AppSettingsV1
}

function successfulExecutor(requests: PrivateMediaImageRequest[]) {
  return async (request: PrivateMediaImageRequest) => {
    requests.push(request)
    return { ok: true as const, imageBase64: PNG_BYTES.toString('base64'), mimeType: 'image/png' }
  }
}

describe('write infographic service', () => {
  beforeEach(() => {
    workspace = realpathSync(mkdtempSync(join(tmpdir(), 'write-infographic-')))
  })

  afterEach(() => {
    rmSync(workspace, { recursive: true, force: true })
  })

  it('rejects when the private Go media executor is unavailable', async () => {
    const result = await requestWriteInfographic(settingsWithImageGen(), {
      text: 'some text',
      filePath: join(workspace, 'doc.md'),
      workspaceRoot: workspace
    })
    expect(result).toEqual({ ok: false, message: 'image generation provider is not configured' })
  })

  it('delegates a bounded key-free image intent to the private Go media executor', async () => {
    const mediaRequests: PrivateMediaImageRequest[] = []
    const result = await requestWriteInfographic(settingsWithImageGen(), {
      text: '季度营收增长 25%，主要来自海外市场。',
      filePath: join(workspace, 'notes', 'report.md'),
      workspaceRoot: workspace
    }, { executeMediaRequest: successfulExecutor(mediaRequests) })

    expect(mediaRequests).toEqual([
      expect.objectContaining({
        operation: 'image.generate',
        prompt: expect.stringContaining('季度营收增长 25%'),
        size: '768x1024',
        timeoutMs: 180000
      })
    ])
    expect(JSON.stringify(mediaRequests)).not.toMatch(/apiKey|baseUrl|credentialRef|endpoint|proxy|Authorization|must-not-cross-main/i)
    expect(result).toMatchObject({ ok: true, relativePath: expect.stringMatching(/^\.\.\/img\//) })
  })

  it('fails closed before invoking the executor for an outside document path', async () => {
    const mediaRequests: PrivateMediaImageRequest[] = []
    const result = await requestWriteInfographic(settingsWithImageGen(), {
      text: 'some text',
      filePath: '/tmp/elsewhere/doc.md',
      workspaceRoot: workspace
    }, { executeMediaRequest: successfulExecutor(mediaRequests) })

    expect(result).toEqual({ ok: false, message: 'document must be inside the write workspace' })
    expect(mediaRequests).toEqual([])
  })

  it('does not treat key-free size metadata as credential readiness', async () => {
    const result = await requestWriteInfographic(settingsWithImageGen({ defaultSize: '1024x1536' }), {
      text: 'fixed-size provider content',
      filePath: join(workspace, 'doc.md'),
      workspaceRoot: workspace
    })
    expect(result).toEqual({ ok: false, message: 'image generation provider is not configured' })
    expect(existsSync(join(workspace, 'img'))).toBe(false)
  })

  it('does not expose a private executor failure', async () => {
    const sentinel = 'HTTP 400: /private/customer-pii-13900000017 raw-provider-body'
    const result = await requestWriteInfographic(settingsWithImageGen(), {
      text: 'some text',
      filePath: join(workspace, 'doc.md'),
      workspaceRoot: workspace
    }, { executeMediaRequest: async () => { throw new Error(sentinel) } })

    expect(result).toEqual({ ok: false, message: 'image generation failed' })
    expect(JSON.stringify(result)).not.toContain(sentinel)
    expect(existsSync(join(workspace, 'img'))).toBe(false)
  })

  it('leaves a hostile output target untouched after bounded execution', async () => {
    const outputPathSentinel = 'customer-pii-13900000028'
    const blockedImageDir = join(workspace, outputPathSentinel)
    writeFileSync(blockedImageDir, 'preserve-output-placeholder', 'utf8')

    const result = await requestWriteInfographic(settingsWithImageGen(), {
      text: 'public infographic content',
      filePath: join(workspace, 'doc.md'),
      workspaceRoot: workspace,
      imageDir: outputPathSentinel
    }, { executeMediaRequest: successfulExecutor([]) })

    expect(result).toEqual({ ok: false, message: 'image output write failed' })
    expect(JSON.stringify(result)).not.toContain(outputPathSentinel)
    expect(readFileSync(blockedImageDir, 'utf8')).toBe('preserve-output-placeholder')
  })

  it('clips overlong selections and keeps the media prompt bounded', () => {
    const prompt = buildWriteInfographicPrompt(
      `核心结论：${'增长、留存、转化、复购、风险。'.repeat(300)}`,
      '',
      'infographic',
      { maxPromptChars: 1500 }
    )
    expect(prompt.length).toBeLessThanOrEqual(1500)
    expect(prompt).toContain('核心结论')
  })

  it('uses the configured key-free prompt prefix', () => {
    expect(buildWriteInfographicPrompt('内容', '请生成手绘风格的信息图。'))
      .toBe('请生成手绘风格的信息图。\n\n内容')
    expect(buildWriteInfographicPrompt('需求内容', '', 'design')).toContain('UI design mockup')
  })

  it('delegates a bounded reference image without Provider configuration', async () => {
    const referencePath = join(workspace, '.analytixsdd', 'requirements', 'draft-1', 'img', 'source.png')
    mkdirSync(dirname(referencePath), { recursive: true })
    writeFileSync(referencePath, PNG_BYTES)
    const mediaRequests: PrivateMediaImageRequest[] = []

    const result = await requestWriteInfographic(settingsWithImageGen(), {
      text: '根据参考图重绘一个更精致的旅行社区首页。',
      filePath: join(workspace, '.analytixsdd', 'requirements', 'draft-1', 'requirement.md'),
      workspaceRoot: workspace,
      imageDir: '.analytixsdd/requirements/draft-1/img',
      kind: 'design',
      referenceImagePath: referencePath
    }, { executeMediaRequest: successfulExecutor(mediaRequests) })

    expect(mediaRequests).toEqual([
      expect.objectContaining({
        operation: 'image.edit',
        images: [{ name: 'source.png', mimeType: 'image/png', dataBase64: PNG_BYTES.toString('base64') }]
      })
    ])
    expect(JSON.stringify(mediaRequests)).not.toMatch(/apiKey|baseUrl|credentialRef|endpoint|proxy|Authorization/i)
    expect(result.ok).toBe(true)
    expect(readFileSync(referencePath).equals(PNG_BYTES)).toBe(true)
  })

  it('rejects an imageDir that escapes the workspace', async () => {
    const result = await requestWriteInfographic(settingsWithImageGen(), {
      text: 'some text',
      filePath: join(workspace, 'doc.md'),
      workspaceRoot: workspace,
      imageDir: '../outside'
    }, { executeMediaRequest: successfulExecutor([]) })

    expect(result).toEqual({ ok: false, message: 'image output write failed' })
    expect(existsSync(join(workspace, '..', 'outside'))).toBe(false)
  })
})
