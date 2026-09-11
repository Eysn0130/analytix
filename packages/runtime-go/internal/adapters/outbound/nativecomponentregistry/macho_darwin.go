//go:build darwin

package nativecomponentregistry

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
)

const (
	machOHeader64Bytes       = 32
	machOSegment64Command    = 0x19
	machOCodeSignature       = 0x1d
	machOExecutableFileType  = 2
	machOCodeSuperBlobMagic  = 0xfade0cc0
	maxMachOLoadCommandBytes = 16 * 1024 * 1024
)

type machoLinkEdit struct {
	commandOffset int64
	fileOffset    uint64
	fileSize      uint64
}

type machoSignature struct {
	commandOffset int64
	dataOffset    uint32
	dataSize      uint32
}

type bytePatch struct {
	offset int64
	bytes  []byte
}

// InspectDarwinPayload returns the signing-invariant payload identity used by
// both registry admission and the build-only generation publisher.
func InspectDarwinPayload(file *os.File, fileSize int64) (string, int64, string, error) {
	return machOPayloadIdentity(file, fileSize)
}

func machOPayloadIdentity(file *os.File, fileSize int64) (string, int64, string, error) {
	if file == nil || fileSize < machOHeader64Bytes || fileSize > MaxNativeBinaryBytes {
		return "", 0, "", ErrComponentInvalid
	}
	header := make([]byte, machOHeader64Bytes)
	if readAtExact(file, header, 0) != nil {
		return "", 0, "", ErrComponentInvalid
	}
	order, ok := machoByteOrder(header[:4])
	if !ok || order.Uint32(header[12:16]) != machOExecutableFileType {
		return "", 0, "", ErrComponentInvalid
	}
	arch := machoArch(order.Uint32(header[4:8]))
	if arch == "" {
		return "", 0, "", ErrComponentInvalid
	}
	commandCount := order.Uint32(header[16:20])
	commandsSize := order.Uint32(header[20:24])
	if commandCount == 0 || commandCount > 4096 || commandsSize < 8 || commandsSize > maxMachOLoadCommandBytes ||
		int64(machOHeader64Bytes)+int64(commandsSize) > fileSize {
		return "", 0, "", ErrComponentInvalid
	}
	commands := make([]byte, int(commandsSize))
	if readAtExact(file, commands, machOHeader64Bytes) != nil {
		return "", 0, "", ErrComponentInvalid
	}
	var signature *machoSignature
	var linkEdit *machoLinkEdit
	executableSegment := false
	cursor := uint32(0)
	for index := uint32(0); index < commandCount; index++ {
		if cursor > commandsSize-8 {
			return "", 0, "", ErrComponentInvalid
		}
		command := order.Uint32(commands[cursor : cursor+4])
		commandSize := order.Uint32(commands[cursor+4 : cursor+8])
		if commandSize < 8 || commandSize%4 != 0 || commandSize > commandsSize-cursor {
			return "", 0, "", ErrComponentInvalid
		}
		absolute := int64(machOHeader64Bytes) + int64(cursor)
		switch command {
		case machOSegment64Command:
			if commandSize < 72 {
				return "", 0, "", ErrComponentInvalid
			}
			segment := commands[cursor : cursor+commandSize]
			fileOffset := order.Uint64(segment[40:48])
			segmentSize := order.Uint64(segment[48:56])
			if fileOffset > uint64(fileSize) || segmentSize > uint64(fileSize)-fileOffset {
				return "", 0, "", ErrComponentInvalid
			}
			if order.Uint32(segment[60:64])&0x4 != 0 && segmentSize > 0 {
				executableSegment = true
			}
			name := segment[8:24]
			if fixedCString(name) == "__LINKEDIT" {
				if linkEdit != nil {
					return "", 0, "", ErrComponentInvalid
				}
				linkEdit = &machoLinkEdit{commandOffset: absolute, fileOffset: fileOffset, fileSize: segmentSize}
			}
		case machOCodeSignature:
			if commandSize != 16 || signature != nil {
				return "", 0, "", ErrComponentInvalid
			}
			signature = &machoSignature{
				commandOffset: absolute,
				dataOffset:    order.Uint32(commands[cursor+8 : cursor+12]),
				dataSize:      order.Uint32(commands[cursor+12 : cursor+16]),
			}
		}
		cursor += commandSize
	}
	commandsEnd := int64(machOHeader64Bytes) + int64(commandsSize)
	if cursor != commandsSize || !executableSegment || signature == nil || linkEdit == nil || signature.dataSize < 12 ||
		int64(signature.dataOffset) < commandsEnd || signature.dataOffset%8 != 0 ||
		int64(signature.dataOffset)+int64(signature.dataSize) != fileSize ||
		linkEdit.fileOffset > uint64(signature.dataOffset) || linkEdit.fileOffset+linkEdit.fileSize != uint64(fileSize) {
		return "", 0, "", ErrComponentInvalid
	}
	superBlob := make([]byte, 12)
	if readAtExact(file, superBlob, int64(signature.dataOffset)) != nil ||
		binary.BigEndian.Uint32(superBlob[:4]) != machOCodeSuperBlobMagic {
		return "", 0, "", ErrComponentInvalid
	}
	superBlobSize := binary.BigEndian.Uint32(superBlob[4:8])
	if superBlobSize < 12 || superBlobSize > signature.dataSize {
		return "", 0, "", ErrComponentInvalid
	}
	payloadSize := int64(signature.dataOffset)
	linkEditPayloadSize := uint64(signature.dataOffset) - linkEdit.fileOffset
	patches := []bytePatch{
		{offset: linkEdit.commandOffset + 32, bytes: encodeUint64(order, linkEditPayloadSize)},
		{offset: linkEdit.commandOffset + 48, bytes: encodeUint64(order, linkEditPayloadSize)},
		{offset: signature.commandOffset + 12, bytes: encodeUint32(order, 0)},
	}
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	for offset := int64(0); offset < payloadSize; {
		chunkSize := int64(len(buffer))
		if payloadSize-offset < chunkSize {
			chunkSize = payloadSize - offset
		}
		chunk := buffer[:chunkSize]
		if readAtExact(file, chunk, offset) != nil {
			return "", 0, "", ErrComponentInvalid
		}
		for _, patch := range patches {
			overlayPatch(chunk, offset, patch)
		}
		if _, err := hash.Write(chunk); err != nil {
			return "", 0, "", ErrComponentInvalid
		}
		offset += chunkSize
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", 0, "", ErrComponentInvalid
	}
	return hex.EncodeToString(hash.Sum(nil)), payloadSize, arch, nil
}

func machoByteOrder(magic []byte) (binary.ByteOrder, bool) {
	if len(magic) != 4 {
		return nil, false
	}
	if binary.BigEndian.Uint32(magic) == 0xfeedfacf {
		return binary.BigEndian, true
	}
	if binary.LittleEndian.Uint32(magic) == 0xfeedfacf {
		return binary.LittleEndian, true
	}
	return nil, false
}

func machoArch(cpu uint32) string {
	switch cpu {
	case 0x01000007:
		return "x64"
	case 0x0100000c:
		return "arm64"
	default:
		return ""
	}
}

func fixedCString(value []byte) string {
	for index, item := range value {
		if item == 0 {
			return string(value[:index])
		}
	}
	return string(value)
}

func encodeUint32(order binary.ByteOrder, value uint32) []byte {
	encoded := make([]byte, 4)
	order.PutUint32(encoded, value)
	return encoded
}

func encodeUint64(order binary.ByteOrder, value uint64) []byte {
	encoded := make([]byte, 8)
	order.PutUint64(encoded, value)
	return encoded
}

func overlayPatch(chunk []byte, chunkOffset int64, patch bytePatch) {
	chunkEnd := chunkOffset + int64(len(chunk))
	patchEnd := patch.offset + int64(len(patch.bytes))
	if patch.offset >= chunkEnd || patchEnd <= chunkOffset {
		return
	}
	copyStart := patch.offset
	if copyStart < chunkOffset {
		copyStart = chunkOffset
	}
	copyEnd := patchEnd
	if copyEnd > chunkEnd {
		copyEnd = chunkEnd
	}
	copy(
		chunk[copyStart-chunkOffset:copyEnd-chunkOffset],
		patch.bytes[copyStart-patch.offset:copyEnd-patch.offset],
	)
}

func readAtExact(file *os.File, payload []byte, offset int64) error {
	read, err := file.ReadAt(payload, offset)
	if err != nil && err != io.EOF {
		return err
	}
	if read != len(payload) {
		return io.ErrUnexpectedEOF
	}
	return nil
}
