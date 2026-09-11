import { createHash } from 'node:crypto'

export type NativePayloadFormat = 'mach-o' | 'pe' | 'elf'

export type NativePayloadIdentity = {
  format: NativePayloadFormat
  arch: string
  payloadSha256: string
  payloadSize: number
  binarySha256: string
  binarySize: number
}

function sha256(bytes: Buffer): string {
  return createHash('sha256').update(bytes).digest('hex')
}

function machOIdentity(bytes: Buffer): NativePayloadIdentity {
  if (bytes.length < 32) throw new Error('Mach-O header is truncated')
  const magicBE = bytes.readUInt32BE(0)
  const magicLE = bytes.readUInt32LE(0)
  let read32: (offset: number) => number
  let read64: (offset: number) => bigint
  let write32: (target: Buffer, value: number, offset: number) => number
  let write64: (target: Buffer, value: bigint, offset: number) => number
  if (magicBE === 0xfeedfacf) {
    read32 = (offset) => bytes.readUInt32BE(offset)
    read64 = (offset) => bytes.readBigUInt64BE(offset)
    write32 = (target, value, offset) => target.writeUInt32BE(value, offset)
    write64 = (target, value, offset) => target.writeBigUInt64BE(value, offset)
  } else if (magicLE === 0xfeedfacf) {
    read32 = (offset) => bytes.readUInt32LE(offset)
    read64 = (offset) => bytes.readBigUInt64LE(offset)
    write32 = (target, value, offset) => target.writeUInt32LE(value, offset)
    write64 = (target, value, offset) => target.writeBigUInt64LE(value, offset)
  } else {
    throw new Error('Mach-O magic is invalid')
  }
  const cpu = read32(4)
  const commandCount = read32(16)
  const commandsEnd = 32 + read32(20)
  if (read32(12) !== 2 || commandCount === 0 || commandCount > 4096 || commandsEnd > bytes.length) {
    throw new Error('Mach-O executable header is invalid')
  }
  let commandOffset = 32
  let executableSegment = false
  const linkEdits: Array<{ commandOffset: number; fileOffset: number; fileSize: number }> = []
  const signatures: Array<{ commandOffset: number; dataOffset: number; dataSize: number }> = []
  for (let index = 0; index < commandCount; index += 1) {
    if (commandOffset + 8 > commandsEnd) throw new Error('Mach-O load command is truncated')
    const command = read32(commandOffset)
    const commandSize = read32(commandOffset + 4)
    if (commandSize < 8 || commandSize % 4 !== 0 || commandOffset + commandSize > commandsEnd) {
      throw new Error('Mach-O load command size is invalid')
    }
    if (command === 0x19) {
      if (commandSize < 72) throw new Error('Mach-O segment command is truncated')
      const name = bytes.toString('ascii', commandOffset + 8, commandOffset + 24).replace(/\0.*$/s, '')
      const fileOffset = Number(read64(commandOffset + 40))
      const fileSize = Number(read64(commandOffset + 48))
      if (!Number.isSafeInteger(fileOffset) || !Number.isSafeInteger(fileSize) || fileOffset + fileSize > bytes.length) {
        throw new Error('Mach-O segment range is invalid')
      }
      if ((read32(commandOffset + 60) & 0x4) !== 0 && fileSize > 0) executableSegment = true
      if (name === '__LINKEDIT') linkEdits.push({ commandOffset, fileOffset, fileSize })
    } else if (command === 0x1d) {
      if (commandSize !== 16) throw new Error('Mach-O code-signature command size is invalid')
      signatures.push({
        commandOffset,
        dataOffset: read32(commandOffset + 8),
        dataSize: read32(commandOffset + 12)
      })
    }
    commandOffset += commandSize
  }
  if (commandOffset !== commandsEnd || !executableSegment || linkEdits.length !== 1 || signatures.length !== 1) {
    throw new Error('Mach-O signed executable structure is invalid')
  }
  const linkEdit = linkEdits[0]
  const signature = signatures[0]
  const superBlobSize = bytes.readUInt32BE(signature.dataOffset + 4)
  if (
    signature.dataSize < 12 ||
    signature.dataOffset < commandsEnd ||
    signature.dataOffset % 8 !== 0 ||
    signature.dataOffset + signature.dataSize !== bytes.length ||
    linkEdit.fileOffset > signature.dataOffset ||
    linkEdit.fileOffset + linkEdit.fileSize !== bytes.length ||
    bytes.readUInt32BE(signature.dataOffset) !== 0xfade0cc0 ||
    superBlobSize < 12 ||
    superBlobSize > signature.dataSize
  ) {
    throw new Error('Mach-O code-signature range is invalid or ambiguous')
  }
  const payload = Buffer.from(bytes.subarray(0, signature.dataOffset))
  const linkEditPayloadSize = BigInt(signature.dataOffset - linkEdit.fileOffset)
  write64(payload, linkEditPayloadSize, linkEdit.commandOffset + 32)
  write64(payload, linkEditPayloadSize, linkEdit.commandOffset + 48)
  write32(payload, 0, signature.commandOffset + 12)
  const arch = cpu === 0x01000007 ? 'x64' : cpu === 0x0100000c ? 'arm64' : `cpu-${cpu}`
  return {
    format: 'mach-o',
    arch,
    payloadSha256: sha256(payload),
    payloadSize: payload.length,
    binarySha256: sha256(bytes),
    binarySize: bytes.length
  }
}

function peIdentity(bytes: Buffer): NativePayloadIdentity {
  if (bytes.length < 0x40 || bytes.toString('ascii', 0, 2) !== 'MZ') throw new Error('PE DOS header is invalid')
  const peOffset = bytes.readUInt32LE(0x3c)
  if (peOffset < 0x40 || bytes.toString('ascii', peOffset, peOffset + 4) !== 'PE\0\0') {
    throw new Error('PE signature is invalid')
  }
  const machine = bytes.readUInt16LE(peOffset + 4)
  const sectionCount = bytes.readUInt16LE(peOffset + 6)
  const optionalSize = bytes.readUInt16LE(peOffset + 20)
  const optionalOffset = peOffset + 24
  const optionalEnd = optionalOffset + optionalSize
  if (
    sectionCount === 0 ||
    sectionCount > 96 ||
    optionalSize < 152 ||
    optionalEnd > bytes.length ||
    bytes.readUInt16LE(optionalOffset) !== 0x20b ||
    (bytes.readUInt16LE(peOffset + 22) & 0x0002) === 0 ||
    bytes.readUInt32LE(optionalOffset + 16) === 0 ||
    bytes.readUInt32LE(optionalOffset + 108) < 5
  ) {
    throw new Error('PE32+ executable header is invalid')
  }
  const sizeOfHeaders = bytes.readUInt32LE(optionalOffset + 60)
  if (!sizeOfHeaders || sizeOfHeaders > bytes.length || optionalEnd + sectionCount * 40 > bytes.length) {
    throw new Error('PE section table is invalid')
  }
  let executableSection = false
  let sectionEnd = 0
  for (let index = 0; index < sectionCount; index += 1) {
    const sectionOffset = optionalEnd + index * 40
    const rawSize = bytes.readUInt32LE(sectionOffset + 16)
    const rawOffset = bytes.readUInt32LE(sectionOffset + 20)
    if (rawSize > 0) {
      if (rawOffset < sizeOfHeaders || rawOffset + rawSize > bytes.length) throw new Error('PE section range is invalid')
      sectionEnd = Math.max(sectionEnd, rawOffset + rawSize)
      if ((bytes.readUInt32LE(sectionOffset + 36) & 0x20000000) !== 0) executableSection = true
    }
  }
  if (!executableSection) throw new Error('PE executable section is missing')
  const checksumOffset = optionalOffset + 64
  const certificateDirectory = optionalOffset + 144
  const certificateOffset = bytes.readUInt32LE(certificateDirectory)
  const certificateSize = bytes.readUInt32LE(certificateDirectory + 4)
  if ((certificateOffset === 0) !== (certificateSize === 0)) {
    throw new Error('PE certificate table offset and size must be paired')
  }
  let payloadEnd = bytes.length
  if (certificateOffset === 0) {
    if (sectionEnd !== bytes.length) throw new Error('Unsigned PE contains an unsupported overlay or gap')
  } else {
    if (
      certificateOffset % 8 !== 0 ||
      certificateSize < 8 ||
      certificateSize % 8 !== 0 ||
      certificateOffset !== sectionEnd ||
      certificateOffset + certificateSize !== bytes.length
    ) {
      throw new Error('PE certificate table range is invalid or ambiguous')
    }
    let cursor = certificateOffset
    while (cursor < bytes.length) {
      const recordLength = bytes.readUInt32LE(cursor)
      if (
        recordLength < 8 ||
        recordLength % 8 !== 0 ||
        cursor + recordLength > bytes.length ||
        ![0x0100, 0x0200].includes(bytes.readUInt16LE(cursor + 4)) ||
        bytes.readUInt16LE(cursor + 6) !== 0x0002
      ) {
        throw new Error('PE WIN_CERTIFICATE record is invalid')
      }
      cursor += recordLength
    }
    if (cursor !== bytes.length) throw new Error('PE certificate table walk is incomplete')
    payloadEnd = certificateOffset
  }
  const payload = Buffer.from(bytes.subarray(0, payloadEnd))
  payload.fill(0, checksumOffset, checksumOffset + 4)
  payload.fill(0, certificateDirectory, certificateDirectory + 8)
  const arch = machine === 0x8664 ? 'x64' : machine === 0xaa64 ? 'arm64' : `machine-${machine}`
  return {
    format: 'pe',
    arch,
    payloadSha256: sha256(payload),
    payloadSize: payload.length,
    binarySha256: sha256(bytes),
    binarySize: bytes.length
  }
}

function elfIdentity(bytes: Buffer): NativePayloadIdentity {
  if (bytes.length < 64 || !bytes.subarray(0, 4).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46]))) {
    throw new Error('ELF64 identification is invalid')
  }
  if (bytes[4] !== 2 || ![1, 2].includes(bytes[5]) || bytes[6] !== 1) throw new Error('ELF64 identification is invalid')
  const little = bytes[5] === 1
  const read16 = little ? (offset: number) => bytes.readUInt16LE(offset) : (offset: number) => bytes.readUInt16BE(offset)
  const read32 = little ? (offset: number) => bytes.readUInt32LE(offset) : (offset: number) => bytes.readUInt32BE(offset)
  const read64 = little ? (offset: number) => bytes.readBigUInt64LE(offset) : (offset: number) => bytes.readBigUInt64BE(offset)
  const machine = read16(18)
  const programOffset = Number(read64(32))
  const programSize = read16(54)
  const programCount = read16(56)
  if (
    ![2, 3].includes(read16(16)) ||
    read32(20) !== 1 ||
    read16(52) < 64 ||
    programSize < 56 ||
    programCount === 0 ||
    !Number.isSafeInteger(programOffset) ||
    programOffset < read16(52) ||
    programOffset + programSize * programCount > bytes.length
  ) {
    throw new Error('ELF64 executable header is invalid')
  }
  let executableSegment = false
  for (let index = 0; index < programCount; index += 1) {
    const item = programOffset + index * programSize
    const fileOffset = Number(read64(item + 8))
    const fileSize = Number(read64(item + 32))
    if (!Number.isSafeInteger(fileOffset) || !Number.isSafeInteger(fileSize) || fileOffset + fileSize > bytes.length) {
      throw new Error('ELF64 segment range is invalid')
    }
    if (read32(item) === 1 && (read32(item + 4) & 0x1) !== 0 && fileSize > 0) executableSegment = true
  }
  if (!executableSegment) throw new Error('ELF64 executable segment is missing')
  const arch = machine === 62 ? 'x64' : machine === 183 ? 'arm64' : `machine-${machine}`
  return {
    format: 'elf',
    arch,
    payloadSha256: sha256(bytes),
    payloadSize: bytes.length,
    binarySha256: sha256(bytes),
    binarySize: bytes.length
  }
}

export function inspectNativePayload(bytes: Buffer, format: NativePayloadFormat): NativePayloadIdentity {
  if (format === 'mach-o') return machOIdentity(bytes)
  if (format === 'pe') return peIdentity(bytes)
  return elfIdentity(bytes)
}
