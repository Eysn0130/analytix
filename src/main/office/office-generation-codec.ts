import {
  buildDocumentDocxBytes,
  DOCUMENT_DOCX_IMAGE_ID,
  DOCUMENT_DOCX_IMAGE_LIMITS,
  type DocumentDocxImage
} from '../services/write-docx-service'

const MAX_MARKDOWN_BYTES = 1024 * 1024
const MAX_TITLE_LENGTH = 256
const MAX_IMAGE_BASE64_LENGTH = Math.ceil(DOCUMENT_DOCX_IMAGE_LIMITS.maxBytes / 3) * 4

export type OfficeGenerationInputV1 = {
  schemaVersion: 1
  kind: 'docx'
  markdown: string
  title?: string
  images?: Array<{ id: string; type: DocumentDocxImage['type']; dataBase64: string }>
}

function invalidInput(): Error {
  return new Error('office-generation-invalid-input')
}

function dataRecord(input: unknown, allowed: string[], required: string[]): Record<string, unknown> {
  if (!input || typeof input !== 'object' || Array.isArray(input)) throw invalidInput()
  const prototype = Object.getPrototypeOf(input)
  if (prototype !== Object.prototype && prototype !== null) throw invalidInput()
  const descriptors = Object.getOwnPropertyDescriptors(input)
  const value: Record<string, unknown> = Object.create(null)
  for (const key of Reflect.ownKeys(descriptors)) {
    if (typeof key !== 'string' || !allowed.includes(key) || !('value' in descriptors[key])) throw invalidInput()
    value[key] = descriptors[key].value
  }
  if (required.some((key) => !Object.hasOwn(descriptors, key))) throw invalidInput()
  return value
}

function imageArray(input: unknown): unknown[] {
  if (!Array.isArray(input) || Object.getPrototypeOf(input) !== Array.prototype) throw invalidInput()
  const descriptors = Object.getOwnPropertyDescriptors(input as object)
  const length = descriptors.length.value
  if (!Number.isInteger(length) || length < 0 || length > DOCUMENT_DOCX_IMAGE_LIMITS.maxCount) throw invalidInput()
  const keys = Reflect.ownKeys(descriptors)
  if (keys.length !== length + 1 || keys.some((key) => typeof key !== 'string')) throw invalidInput()
  const values: unknown[] = []
  for (let index = 0; index < length; index += 1) {
    const descriptor = descriptors[String(index)]
    if (!descriptor || !('value' in descriptor)) throw invalidInput()
    values.push(descriptor.value)
  }
  return values
}

/** Data-only format adapter. The caller owns authorization, projection and persistence. */
export async function encodeOfficeGenerationV1(input: unknown): Promise<Buffer> {
  try {
    const value = dataRecord(input, ['schemaVersion', 'kind', 'markdown', 'title', 'images'], ['schemaVersion', 'kind', 'markdown'])
    if (value.schemaVersion !== 1 || value.kind !== 'docx' || typeof value.markdown !== 'string' || Buffer.byteLength(value.markdown, 'utf8') > MAX_MARKDOWN_BYTES) throw invalidInput()
    if (Object.hasOwn(value, 'title') && (typeof value.title !== 'string' || value.title.length > MAX_TITLE_LENGTH)) throw invalidInput()
    const images = new Map<string, DocumentDocxImage>()
    let totalBytes = 0
    if (Object.hasOwn(value, 'images')) {
      for (const raw of imageArray(value.images)) {
        const image = dataRecord(raw, ['id', 'type', 'dataBase64'], ['id', 'type', 'dataBase64'])
        if (typeof image.id !== 'string' || !DOCUMENT_DOCX_IMAGE_ID.test(image.id) || images.has(image.id)) throw invalidInput()
        if (image.type !== 'png' && image.type !== 'jpg' && image.type !== 'gif' && image.type !== 'bmp') throw invalidInput()
        if (typeof image.dataBase64 !== 'string' || image.dataBase64.length === 0 || image.dataBase64.length > MAX_IMAGE_BASE64_LENGTH) throw invalidInput()
        const data = Buffer.from(image.dataBase64, 'base64')
        if (data.toString('base64') !== image.dataBase64 || data.length > DOCUMENT_DOCX_IMAGE_LIMITS.maxBytes) throw invalidInput()
        totalBytes += data.length
        if (totalBytes > DOCUMENT_DOCX_IMAGE_LIMITS.maxTotalBytes) throw invalidInput()
        images.set(image.id, { type: image.type, data })
      }
    }
    return await buildDocumentDocxBytes({ publicContent: value.markdown, title: value.title as string | undefined, images })
  } catch {
    // Parser, image and packer diagnostics must not return caller content.
    throw invalidInput()
  }
}
