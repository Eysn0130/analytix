//go:build darwin

package processauthority

import (
	"crypto/sha1" // #nosec G505 -- Apple CDHash compatibility requires SHA-1 CodeDirectories.
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	machO64HeaderSize             = 32
	machOLoadCommandCodeSignature = 0x1d
	codeSignatureSuperBlobMagic   = 0xfade0cc0
	codeDirectoryMagic            = 0xfade0c02
	codeDirectoryPrimarySlot      = 0
	codeDirectoryAlternateBase    = 0x1000
	codeDirectoryAlternateLast    = 0x1005
	maxMachOLoadCommandsBytes     = 16 * 1024 * 1024
	maxCodeSignatureBytes         = 64 * 1024 * 1024
	maxCodeSignatureBlobCount     = 1024
	csOpsCDHash                   = 5
	cdHashBytes                   = 20
)

var errDarwinCodeIdentityInvalid = errors.New("darwin code identity invalid")

func openedFileCodeDirectoryHashes(file *os.File) ([][cdHashBytes]byte, error) {
	if file == nil {
		return nil, errDarwinCodeIdentityInvalid
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < machO64HeaderSize {
		return nil, errDarwinCodeIdentityInvalid
	}
	magic := make([]byte, 8)
	if err := readExactlyAt(file, magic, 0); err != nil {
		return nil, errDarwinCodeIdentityInvalid
	}
	switch binary.BigEndian.Uint32(magic[:4]) {
	case 0xcafebabe:
		return openedFatMachOCodeDirectoryHashes(file, info.Size(), binary.BigEndian.Uint32(magic[4:8]), false)
	case 0xcafebabf:
		return openedFatMachOCodeDirectoryHashes(file, info.Size(), binary.BigEndian.Uint32(magic[4:8]), true)
	default:
		return openedMachOSliceCodeDirectoryHashes(file, 0, info.Size())
	}
}

func openedFatMachOCodeDirectoryHashes(file *os.File, fileSize int64, count uint32, fat64 bool) ([][cdHashBytes]byte, error) {
	if count == 0 || count > 32 {
		return nil, errDarwinCodeIdentityInvalid
	}
	entrySize := uint64(20)
	if fat64 {
		entrySize = 32
	}
	headerSize := uint64(8) + uint64(count)*entrySize
	if headerSize > uint64(fileSize) {
		return nil, errDarwinCodeIdentityInvalid
	}
	entries := make([]byte, headerSize-8)
	if err := readExactlyAt(file, entries, 8); err != nil {
		return nil, errDarwinCodeIdentityInvalid
	}
	hashes := make([][cdHashBytes]byte, 0, count)
	seen := make(map[[cdHashBytes]byte]struct{}, count)
	for index := uint32(0); index < count; index++ {
		entry := uint64(index) * entrySize
		var offset uint64
		var size uint64
		if fat64 {
			offset = binary.BigEndian.Uint64(entries[entry+8 : entry+16])
			size = binary.BigEndian.Uint64(entries[entry+16 : entry+24])
		} else {
			offset = uint64(binary.BigEndian.Uint32(entries[entry+8 : entry+12]))
			size = uint64(binary.BigEndian.Uint32(entries[entry+12 : entry+16]))
		}
		if size < machO64HeaderSize || offset > uint64(fileSize) || size > uint64(fileSize)-offset {
			return nil, errDarwinCodeIdentityInvalid
		}
		sliceHashes, err := openedMachOSliceCodeDirectoryHashes(file, int64(offset), int64(size))
		if err != nil {
			return nil, err
		}
		for _, hash := range sliceHashes {
			if _, duplicate := seen[hash]; !duplicate {
				seen[hash] = struct{}{}
				hashes = append(hashes, hash)
			}
		}
	}
	if len(hashes) == 0 {
		return nil, errDarwinCodeIdentityInvalid
	}
	return hashes, nil
}

func openedMachOSliceCodeDirectoryHashes(file *os.File, base int64, sliceSize int64) ([][cdHashBytes]byte, error) {
	if base < 0 || sliceSize < machO64HeaderSize {
		return nil, errDarwinCodeIdentityInvalid
	}
	header := make([]byte, machO64HeaderSize)
	if err := readExactlyAt(file, header, base); err != nil {
		return nil, errDarwinCodeIdentityInvalid
	}
	byteOrder, err := machOByteOrder(header[:4])
	if err != nil {
		return nil, err
	}
	commandCount := byteOrder.Uint32(header[16:20])
	commandsSize := byteOrder.Uint32(header[20:24])
	if commandCount == 0 || commandCount > 4096 || commandsSize < 8 || commandsSize > maxMachOLoadCommandsBytes ||
		int64(machO64HeaderSize)+int64(commandsSize) > sliceSize {
		return nil, errDarwinCodeIdentityInvalid
	}
	commands := make([]byte, commandsSize)
	if err := readExactlyAt(file, commands, base+machO64HeaderSize); err != nil {
		return nil, errDarwinCodeIdentityInvalid
	}
	var signatureOffset uint32
	var signatureSize uint32
	cursor := uint32(0)
	for index := uint32(0); index < commandCount; index++ {
		if cursor > commandsSize-8 {
			return nil, errDarwinCodeIdentityInvalid
		}
		command := byteOrder.Uint32(commands[cursor : cursor+4])
		commandSize := byteOrder.Uint32(commands[cursor+4 : cursor+8])
		if commandSize < 8 || commandSize%4 != 0 || commandSize > commandsSize-cursor {
			return nil, errDarwinCodeIdentityInvalid
		}
		if command == machOLoadCommandCodeSignature {
			if commandSize != 16 || signatureSize != 0 {
				return nil, errDarwinCodeIdentityInvalid
			}
			signatureOffset = byteOrder.Uint32(commands[cursor+8 : cursor+12])
			signatureSize = byteOrder.Uint32(commands[cursor+12 : cursor+16])
		}
		cursor += commandSize
	}
	if cursor != commandsSize || signatureSize < 12 || signatureSize > maxCodeSignatureBytes ||
		int64(signatureOffset)+int64(signatureSize) > sliceSize {
		return nil, errDarwinCodeIdentityInvalid
	}
	signature := make([]byte, signatureSize)
	if err := readExactlyAt(file, signature, base+int64(signatureOffset)); err != nil {
		return nil, errDarwinCodeIdentityInvalid
	}
	return codeDirectoryHashesFromSignature(signature)
}

func machOByteOrder(magic []byte) (binary.ByteOrder, error) {
	if len(magic) != 4 {
		return nil, errDarwinCodeIdentityInvalid
	}
	if binary.LittleEndian.Uint32(magic) == 0xfeedfacf {
		return binary.LittleEndian, nil
	}
	if binary.BigEndian.Uint32(magic) == 0xfeedfacf {
		return binary.BigEndian, nil
	}
	return nil, errDarwinCodeIdentityInvalid
}

func codeDirectoryHashesFromSignature(signature []byte) ([][cdHashBytes]byte, error) {
	if len(signature) < 12 || binary.BigEndian.Uint32(signature[:4]) != codeSignatureSuperBlobMagic {
		return nil, errDarwinCodeIdentityInvalid
	}
	length := binary.BigEndian.Uint32(signature[4:8])
	count := binary.BigEndian.Uint32(signature[8:12])
	if length < 12 || length > uint32(len(signature)) || count == 0 || count > maxCodeSignatureBlobCount ||
		uint64(12)+uint64(count)*8 > uint64(length) {
		return nil, errDarwinCodeIdentityInvalid
	}
	hashes := make([][cdHashBytes]byte, 0, count)
	seen := make(map[[cdHashBytes]byte]struct{}, count)
	for index := uint32(0); index < count; index++ {
		entry := 12 + index*8
		slot := binary.BigEndian.Uint32(signature[entry : entry+4])
		if slot != codeDirectoryPrimarySlot && (slot < codeDirectoryAlternateBase || slot > codeDirectoryAlternateLast) {
			continue
		}
		offset := binary.BigEndian.Uint32(signature[entry+4 : entry+8])
		if offset > length-8 {
			return nil, errDarwinCodeIdentityInvalid
		}
		blob := signature[offset:length]
		if binary.BigEndian.Uint32(blob[:4]) != codeDirectoryMagic {
			return nil, errDarwinCodeIdentityInvalid
		}
		blobLength := binary.BigEndian.Uint32(blob[4:8])
		if blobLength < 40 || blobLength > uint32(len(blob)) {
			return nil, errDarwinCodeIdentityInvalid
		}
		hash, err := codeDirectoryHash(blob[:blobLength])
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[hash]; !duplicate {
			seen[hash] = struct{}{}
			hashes = append(hashes, hash)
		}
	}
	if len(hashes) == 0 {
		return nil, errDarwinCodeIdentityInvalid
	}
	return hashes, nil
}

func codeDirectoryHash(codeDirectory []byte) ([cdHashBytes]byte, error) {
	var result [cdHashBytes]byte
	if len(codeDirectory) < 40 {
		return result, errDarwinCodeIdentityInvalid
	}
	switch codeDirectory[37] {
	case 1:
		digest := sha1.Sum(codeDirectory) // #nosec G401 -- required by the signed executable format.
		copy(result[:], digest[:])
	case 2, 3:
		digest := sha256.Sum256(codeDirectory)
		copy(result[:], digest[:cdHashBytes])
	case 4:
		digest := sha512.Sum384(codeDirectory)
		copy(result[:], digest[:cdHashBytes])
	default:
		return result, errDarwinCodeIdentityInvalid
	}
	return result, nil
}

func loadedProcessCDHash(pid int) ([cdHashBytes]byte, error) {
	var result [cdHashBytes]byte
	if pid <= 0 {
		return result, errDarwinCodeIdentityInvalid
	}
	_, _, errno := syscall.Syscall6(
		syscall.SYS_CSOPS,
		uintptr(pid),
		csOpsCDHash,
		uintptr(unsafe.Pointer(&result[0])),
		uintptr(len(result)),
		0,
		0,
	)
	runtime.KeepAlive(&result)
	if errno != 0 {
		return [cdHashBytes]byte{}, errDarwinCodeIdentityInvalid
	}
	return result, nil
}

func codeDirectoryHashMatches(loaded [cdHashBytes]byte, opened [][cdHashBytes]byte) bool {
	for _, candidate := range opened {
		if candidate == loaded {
			return true
		}
	}
	return false
}

func readExactlyAt(reader io.ReaderAt, payload []byte, offset int64) error {
	read, err := reader.ReadAt(payload, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if read != len(payload) {
		return io.ErrUnexpectedEOF
	}
	return nil
}
