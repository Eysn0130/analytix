package packagedbuildauthorityfs

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

const (
	machOHeaderBytesV2      = 32
	machOSegment64V2        = 0x19
	machOCodeSignatureV2    = 0x1d
	machOExecutableV2       = 2
	machOSuperBlobMagicV2   = 0xfade0cc0
	maxMachOCommandsBytesV2 = 16 << 20
)

type nativePatchV2 struct {
	offset int
	body   []byte
}

func inspectNativePayloadV2(body []byte) (RuntimeIdentityV2, error) {
	if len(body) == 0 || len(body) > maxRuntimeBinaryBytesV2 {
		return RuntimeIdentityV2{}, errors.New("native runtime image is outside its byte bound")
	}
	if len(body) >= 4 && (binary.BigEndian.Uint32(body[:4]) == 0xfeedfacf || binary.LittleEndian.Uint32(body[:4]) == 0xfeedfacf) {
		return inspectMachOPayloadV2(body)
	}
	if len(body) >= 2 && body[0] == 'M' && body[1] == 'Z' {
		return inspectPEPayloadV2(body)
	}
	if len(body) >= 4 && body[0] == 0x7f && body[1] == 'E' && body[2] == 'L' && body[3] == 'F' {
		return inspectELFPayloadV2(body)
	}
	return RuntimeIdentityV2{}, errors.New("native runtime image format is unsupported")
}

func inspectMachOPayloadV2(body []byte) (RuntimeIdentityV2, error) {
	if len(body) < machOHeaderBytesV2 {
		return RuntimeIdentityV2{}, errors.New("Mach-O runtime image is truncated")
	}
	order, ok := machOByteOrderV2(body[:4])
	if !ok || order.Uint32(body[12:16]) != machOExecutableV2 {
		return RuntimeIdentityV2{}, errors.New("Mach-O runtime image is not a thin executable")
	}
	arch := nativeArchV2(order.Uint32(body[4:8]))
	if arch == "" {
		return RuntimeIdentityV2{}, errors.New("Mach-O runtime architecture is unsupported")
	}
	commandCount := order.Uint32(body[16:20])
	commandsSize := order.Uint32(body[20:24])
	commandsEnd := uint64(machOHeaderBytesV2) + uint64(commandsSize)
	if commandCount == 0 || commandCount > 4096 || commandsSize < 8 || commandsSize > maxMachOCommandsBytesV2 || commandsEnd > uint64(len(body)) {
		return RuntimeIdentityV2{}, errors.New("Mach-O load commands are invalid")
	}

	type linkEditV2 struct {
		commandOffset int
		fileOffset    uint64
		fileSize      uint64
	}
	type signatureV2 struct {
		commandOffset int
		dataOffset    uint32
		dataSize      uint32
	}
	var linkEdit *linkEditV2
	var signature *signatureV2
	executableSegment := false
	cursor := uint64(machOHeaderBytesV2)
	for index := uint32(0); index < commandCount; index++ {
		if cursor+8 > commandsEnd {
			return RuntimeIdentityV2{}, errors.New("Mach-O load command is truncated")
		}
		command := order.Uint32(body[cursor : cursor+4])
		commandSize := order.Uint32(body[cursor+4 : cursor+8])
		if commandSize < 8 || commandSize%4 != 0 || cursor+uint64(commandSize) > commandsEnd {
			return RuntimeIdentityV2{}, errors.New("Mach-O load command size is invalid")
		}
		offset := int(cursor)
		switch command {
		case machOSegment64V2:
			if commandSize < 72 {
				return RuntimeIdentityV2{}, errors.New("Mach-O segment command is truncated")
			}
			fileOffset := order.Uint64(body[offset+40 : offset+48])
			fileSize := order.Uint64(body[offset+48 : offset+56])
			if fileOffset > uint64(len(body)) || fileSize > uint64(len(body))-fileOffset {
				return RuntimeIdentityV2{}, errors.New("Mach-O segment range is invalid")
			}
			if order.Uint32(body[offset+60:offset+64])&0x4 != 0 && fileSize > 0 {
				executableSegment = true
			}
			if fixedCStringV2(body[offset+8:offset+24]) == "__LINKEDIT" {
				if linkEdit != nil {
					return RuntimeIdentityV2{}, errors.New("Mach-O has multiple __LINKEDIT segments")
				}
				linkEdit = &linkEditV2{commandOffset: offset, fileOffset: fileOffset, fileSize: fileSize}
			}
		case machOCodeSignatureV2:
			if commandSize != 16 || signature != nil {
				return RuntimeIdentityV2{}, errors.New("Mach-O code-signature command is invalid")
			}
			signature = &signatureV2{
				commandOffset: offset,
				dataOffset:    order.Uint32(body[offset+8 : offset+12]),
				dataSize:      order.Uint32(body[offset+12 : offset+16]),
			}
		}
		cursor += uint64(commandSize)
	}
	if cursor != commandsEnd || !executableSegment || linkEdit == nil || signature == nil || signature.dataSize < 12 ||
		uint64(signature.dataOffset) < commandsEnd || signature.dataOffset%8 != 0 ||
		uint64(signature.dataOffset)+uint64(signature.dataSize) != uint64(len(body)) ||
		linkEdit.fileOffset > uint64(signature.dataOffset) || linkEdit.fileOffset+linkEdit.fileSize != uint64(len(body)) {
		return RuntimeIdentityV2{}, errors.New("Mach-O code-signature range is invalid or ambiguous")
	}
	signatureOffset := int(signature.dataOffset)
	if binary.BigEndian.Uint32(body[signatureOffset:signatureOffset+4]) != machOSuperBlobMagicV2 {
		return RuntimeIdentityV2{}, errors.New("Mach-O code-signature superblob is invalid")
	}
	superBlobSize := binary.BigEndian.Uint32(body[signatureOffset+4 : signatureOffset+8])
	if superBlobSize < 12 || superBlobSize > signature.dataSize {
		return RuntimeIdentityV2{}, errors.New("Mach-O code-signature superblob size is invalid")
	}
	payload := append([]byte(nil), body[:signatureOffset]...)
	patches := []nativePatchV2{
		{offset: linkEdit.commandOffset + 32, body: uint64BytesV2(order, uint64(signature.dataOffset)-linkEdit.fileOffset)},
		{offset: linkEdit.commandOffset + 48, body: uint64BytesV2(order, uint64(signature.dataOffset)-linkEdit.fileOffset)},
		{offset: signature.commandOffset + 12, body: uint32BytesV2(order, 0)},
	}
	for _, patch := range patches {
		if patch.offset < 0 || patch.offset+len(patch.body) > len(payload) {
			return RuntimeIdentityV2{}, errors.New("Mach-O signing-invariant patch is out of range")
		}
		copy(payload[patch.offset:], patch.body)
	}
	return runtimeIdentityV2(payload, "mach-o", arch), nil
}

func inspectPEPayloadV2(body []byte) (RuntimeIdentityV2, error) {
	if len(body) < 0x40 {
		return RuntimeIdentityV2{}, errors.New("PE runtime image is truncated")
	}
	peOffset := uint64(binary.LittleEndian.Uint32(body[0x3c:0x40]))
	if peOffset+24 > uint64(len(body)) || string(body[peOffset:peOffset+4]) != "PE\x00\x00" {
		return RuntimeIdentityV2{}, errors.New("PE runtime signature is invalid")
	}
	machine := binary.LittleEndian.Uint16(body[peOffset+4 : peOffset+6])
	arch := ""
	switch machine {
	case 0x8664:
		arch = "x64"
	case 0xaa64:
		arch = "arm64"
	default:
		return RuntimeIdentityV2{}, errors.New("PE runtime architecture is unsupported")
	}
	sectionCount := uint64(binary.LittleEndian.Uint16(body[peOffset+6 : peOffset+8]))
	if sectionCount == 0 || sectionCount > 4096 {
		return RuntimeIdentityV2{}, errors.New("PE section count is invalid")
	}
	optionalSize := uint64(binary.LittleEndian.Uint16(body[peOffset+20 : peOffset+22]))
	optionalOffset := peOffset + 24
	optionalEnd := optionalOffset + optionalSize
	if optionalSize < 152 || optionalEnd > uint64(len(body)) || binary.LittleEndian.Uint16(body[optionalOffset:optionalOffset+2]) != 0x20b {
		return RuntimeIdentityV2{}, errors.New("PE32+ optional header is invalid")
	}
	checksumOffset := optionalOffset + 64
	directoryCount := binary.LittleEndian.Uint32(body[optionalOffset+108 : optionalOffset+112])
	certificateDirectory := optionalOffset + 112 + 4*8
	if directoryCount < 5 || certificateDirectory+8 > optionalEnd {
		return RuntimeIdentityV2{}, errors.New("PE Authenticode directory is unavailable")
	}
	sectionTableEnd := optionalEnd + sectionCount*40
	if sectionTableEnd > uint64(len(body)) {
		return RuntimeIdentityV2{}, errors.New("PE section table is truncated")
	}
	var sectionEnd uint64
	executableSection := false
	for index := uint64(0); index < sectionCount; index++ {
		offset := optionalEnd + index*40
		rawSize := uint64(binary.LittleEndian.Uint32(body[offset+16 : offset+20]))
		rawOffset := uint64(binary.LittleEndian.Uint32(body[offset+20 : offset+24]))
		characteristics := binary.LittleEndian.Uint32(body[offset+36 : offset+40])
		if rawOffset > uint64(len(body)) || rawSize > uint64(len(body))-rawOffset {
			return RuntimeIdentityV2{}, errors.New("PE section range is invalid")
		}
		if rawSize > 0 && rawOffset+rawSize > sectionEnd {
			sectionEnd = rawOffset + rawSize
		}
		if rawSize > 0 && characteristics&0x20000000 != 0 {
			executableSection = true
		}
	}
	if !executableSection {
		return RuntimeIdentityV2{}, errors.New("PE executable section is missing")
	}
	certificateOffset := uint64(binary.LittleEndian.Uint32(body[certificateDirectory : certificateDirectory+4]))
	certificateSize := uint64(binary.LittleEndian.Uint32(body[certificateDirectory+4 : certificateDirectory+8]))
	if (certificateOffset == 0) != (certificateSize == 0) {
		return RuntimeIdentityV2{}, errors.New("PE certificate offset and size are inconsistent")
	}
	payloadEnd := uint64(len(body))
	if certificateOffset == 0 {
		if sectionEnd != uint64(len(body)) {
			return RuntimeIdentityV2{}, errors.New("unsigned PE contains an unsupported overlay or gap")
		}
	} else {
		if certificateOffset%8 != 0 || certificateSize < 8 || certificateSize%8 != 0 ||
			certificateOffset != sectionEnd || certificateOffset+certificateSize != uint64(len(body)) {
			return RuntimeIdentityV2{}, errors.New("PE certificate table range is invalid or ambiguous")
		}
		for cursor := certificateOffset; cursor < uint64(len(body)); {
			if cursor+8 > uint64(len(body)) {
				return RuntimeIdentityV2{}, errors.New("PE certificate record is truncated")
			}
			recordLength := uint64(binary.LittleEndian.Uint32(body[cursor : cursor+4]))
			revision := binary.LittleEndian.Uint16(body[cursor+4 : cursor+6])
			certificateType := binary.LittleEndian.Uint16(body[cursor+6 : cursor+8])
			if recordLength < 8 || recordLength%8 != 0 || cursor+recordLength > uint64(len(body)) ||
				(revision != 0x0100 && revision != 0x0200) || certificateType != 0x0002 {
				return RuntimeIdentityV2{}, errors.New("PE certificate record is invalid")
			}
			cursor += recordLength
		}
		payloadEnd = certificateOffset
	}
	payload := append([]byte(nil), body[:payloadEnd]...)
	for _, span := range [][2]uint64{{checksumOffset, checksumOffset + 4}, {certificateDirectory, certificateDirectory + 8}} {
		if span[1] > uint64(len(payload)) {
			return RuntimeIdentityV2{}, errors.New("PE signing-invariant patch is out of range")
		}
		clear(payload[span[0]:span[1]])
	}
	return runtimeIdentityV2(payload, "pe", arch), nil
}

func inspectELFPayloadV2(body []byte) (RuntimeIdentityV2, error) {
	if len(body) < 64 || body[4] != 2 || (body[5] != 1 && body[5] != 2) {
		return RuntimeIdentityV2{}, errors.New("ELF64 runtime header is invalid")
	}
	var order binary.ByteOrder = binary.LittleEndian
	if body[5] == 2 {
		order = binary.BigEndian
	}
	fileType := order.Uint16(body[16:18])
	if fileType != 2 && fileType != 3 {
		return RuntimeIdentityV2{}, errors.New("ELF64 runtime is not executable")
	}
	machine := order.Uint16(body[18:20])
	arch := ""
	switch machine {
	case 62:
		arch = "x64"
	case 183:
		arch = "arm64"
	default:
		return RuntimeIdentityV2{}, errors.New("ELF64 runtime architecture is unsupported")
	}
	programOffset := order.Uint64(body[32:40])
	programSize := uint64(order.Uint16(body[54:56]))
	programCount := uint64(order.Uint16(body[56:58]))
	if programCount == 0 || programCount > 4096 || programSize < 56 || programOffset < 64 ||
		programOffset > uint64(len(body)) || programSize*programCount > uint64(len(body))-programOffset {
		return RuntimeIdentityV2{}, errors.New("ELF64 program headers are invalid")
	}
	executableSegment := false
	for index := uint64(0); index < programCount; index++ {
		offset := programOffset + index*programSize
		segmentType := order.Uint32(body[offset : offset+4])
		flags := order.Uint32(body[offset+4 : offset+8])
		fileOffset := order.Uint64(body[offset+8 : offset+16])
		fileSize := order.Uint64(body[offset+32 : offset+40])
		if fileOffset > uint64(len(body)) || fileSize > uint64(len(body))-fileOffset {
			return RuntimeIdentityV2{}, errors.New("ELF64 segment range is invalid")
		}
		if segmentType == 1 && flags&1 != 0 && fileSize > 0 {
			executableSegment = true
		}
	}
	if !executableSegment {
		return RuntimeIdentityV2{}, errors.New("ELF64 executable segment is missing")
	}
	return runtimeIdentityV2(body, "elf", arch), nil
}

func runtimeIdentityV2(payload []byte, format, arch string) RuntimeIdentityV2 {
	digest := sha256.Sum256(payload)
	return RuntimeIdentityV2{
		PayloadSHA256: hex.EncodeToString(digest[:]),
		PayloadBytes:  int64(len(payload)),
		Format:        format,
		Arch:          arch,
	}
}

func machOByteOrderV2(magic []byte) (binary.ByteOrder, bool) {
	if binary.BigEndian.Uint32(magic) == 0xfeedfacf {
		return binary.BigEndian, true
	}
	if binary.LittleEndian.Uint32(magic) == 0xfeedfacf {
		return binary.LittleEndian, true
	}
	return nil, false
}

func nativeArchV2(cpu uint32) string {
	switch cpu {
	case 0x01000007:
		return "x64"
	case 0x0100000c:
		return "arm64"
	default:
		return ""
	}
}

func fixedCStringV2(body []byte) string {
	for index, value := range body {
		if value == 0 {
			return string(body[:index])
		}
	}
	return string(body)
}

func uint32BytesV2(order binary.ByteOrder, value uint32) []byte {
	body := make([]byte, 4)
	order.PutUint32(body, value)
	return body
}

func uint64BytesV2(order binary.ByteOrder, value uint64) []byte {
	body := make([]byte, 8)
	order.PutUint64(body, value)
	return body
}
