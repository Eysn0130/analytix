import { encodeOfficeGenerationV1 } from './office-generation-codec'

// Fixed host-owned subprocess entry. stdin contains data, never executable code,
// credentials or workspace paths. Only generated file bytes go to stdout.
async function main(): Promise<void> {
  const chunks: Buffer[] = []
  let size = 0
  for await (const chunk of process.stdin) {
    const bytes = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)
    size += bytes.length
    if (size > 36 * 1024 * 1024) throw new Error('codec-input-too-large')
    chunks.push(bytes)
  }
  const output = await encodeOfficeGenerationV1(JSON.parse(Buffer.concat(chunks).toString('utf8')))
  if (output.length > 16 * 1024 * 1024) throw new Error('codec-output-too-large')
  await new Promise<void>((resolve, reject) => process.stdout.write(output, error => error ? reject(error) : resolve()))
}

void main().catch(() => { process.exitCode = 1 })
