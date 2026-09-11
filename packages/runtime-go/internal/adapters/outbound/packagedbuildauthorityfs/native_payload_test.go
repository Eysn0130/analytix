package packagedbuildauthorityfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestInspectNativePayloadV2MachOSigningInvariant(t *testing.T) {
	first := syntheticMachOV2(12)
	second := syntheticMachOV2(20)
	firstIdentity, err := inspectNativePayloadV2(first)
	if err != nil {
		t.Fatal(err)
	}
	secondIdentity, err := inspectNativePayloadV2(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstIdentity != secondIdentity || firstIdentity.Format != "mach-o" || firstIdentity.Arch != "arm64" || firstIdentity.PayloadBytes != 256 {
		t.Fatalf("unexpected signing-invariant identities: first=%#v second=%#v", firstIdentity, secondIdentity)
	}

	tampered := append([]byte(nil), first...)
	tampered[240] ^= 0xff
	tamperedIdentity, err := inspectNativePayloadV2(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if tamperedIdentity.PayloadSHA256 == firstIdentity.PayloadSHA256 {
		t.Fatal("Mach-O payload mutation was ignored")
	}
}

func TestInspectNativePayloadV2PEAuthenticodeInvariant(t *testing.T) {
	first := syntheticPEV2(true)
	identity, err := inspectNativePayloadV2(first)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Format != "pe" || identity.Arch != "x64" || identity.PayloadBytes != 1024 {
		t.Fatalf("unexpected PE identity: %#v", identity)
	}

	unsigned := syntheticPEV2(false)
	unsignedIdentity, err := inspectNativePayloadV2(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	if unsignedIdentity != identity {
		t.Fatalf("PE signing changed payload identity: signed=%#v unsigned=%#v", identity, unsignedIdentity)
	}

	withOverlay := append(append([]byte(nil), unsigned...), 0)
	if _, err := inspectNativePayloadV2(withOverlay); err == nil {
		t.Fatal("unsigned PE overlay was accepted")
	}
}

func TestInspectNativePayloadV2ELFRequiresExecutableSegment(t *testing.T) {
	body := syntheticELFV2(true)
	identity, err := inspectNativePayloadV2(body)
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(body)
	if identity.Format != "elf" || identity.Arch != "arm64" || identity.PayloadBytes != int64(len(body)) ||
		identity.PayloadSHA256 != hex.EncodeToString(expected[:]) {
		t.Fatalf("unexpected ELF identity: %#v", identity)
	}
	if _, err := inspectNativePayloadV2(syntheticELFV2(false)); err == nil {
		t.Fatal("ELF without executable segment was accepted")
	}
}

func syntheticMachOV2(signatureBytes int) []byte {
	const signatureOffset = 256
	body := make([]byte, signatureOffset+signatureBytes)
	order := binary.LittleEndian
	order.PutUint32(body[0:4], 0xfeedfacf)
	order.PutUint32(body[4:8], 0x0100000c)
	order.PutUint32(body[12:16], machOExecutableV2)
	order.PutUint32(body[16:20], 3)
	order.PutUint32(body[20:24], 160)

	writeSegment := func(offset int, name string, fileOffset, fileSize uint64, initProtection uint32) {
		order.PutUint32(body[offset:offset+4], machOSegment64V2)
		order.PutUint32(body[offset+4:offset+8], 72)
		copy(body[offset+8:offset+24], name)
		order.PutUint64(body[offset+40:offset+48], fileOffset)
		order.PutUint64(body[offset+48:offset+56], fileSize)
		order.PutUint32(body[offset+60:offset+64], initProtection)
	}
	writeSegment(32, "__TEXT", 0, signatureOffset, 5)
	writeSegment(104, "__LINKEDIT", 192, uint64(len(body)-192), 1)
	order.PutUint32(body[176:180], machOCodeSignatureV2)
	order.PutUint32(body[180:184], 16)
	order.PutUint32(body[184:188], signatureOffset)
	order.PutUint32(body[188:192], uint32(signatureBytes))
	for index := 192; index < signatureOffset; index++ {
		body[index] = byte(index)
	}
	binary.BigEndian.PutUint32(body[signatureOffset:signatureOffset+4], machOSuperBlobMagicV2)
	binary.BigEndian.PutUint32(body[signatureOffset+4:signatureOffset+8], uint32(signatureBytes))
	binary.BigEndian.PutUint32(body[signatureOffset+8:signatureOffset+12], 0)
	return body
}

func syntheticPEV2(signed bool) []byte {
	const (
		peOffset       = 0x80
		optionalOffset = peOffset + 24
		optionalSize   = 240
		sectionOffset  = optionalOffset + optionalSize
		sectionRaw     = 512
		sectionSize    = 512
	)
	length := sectionRaw + sectionSize
	if signed {
		length += 8
	}
	body := make([]byte, length)
	copy(body[:2], "MZ")
	binary.LittleEndian.PutUint32(body[0x3c:0x40], peOffset)
	copy(body[peOffset:peOffset+4], "PE\x00\x00")
	binary.LittleEndian.PutUint16(body[peOffset+4:peOffset+6], 0x8664)
	binary.LittleEndian.PutUint16(body[peOffset+6:peOffset+8], 1)
	binary.LittleEndian.PutUint16(body[peOffset+20:peOffset+22], optionalSize)
	binary.LittleEndian.PutUint16(body[optionalOffset:optionalOffset+2], 0x20b)
	binary.LittleEndian.PutUint32(body[optionalOffset+108:optionalOffset+112], 16)
	binary.LittleEndian.PutUint32(body[optionalOffset+64:optionalOffset+68], 0x12345678)
	binary.LittleEndian.PutUint32(body[sectionOffset+16:sectionOffset+20], sectionSize)
	binary.LittleEndian.PutUint32(body[sectionOffset+20:sectionOffset+24], sectionRaw)
	binary.LittleEndian.PutUint32(body[sectionOffset+36:sectionOffset+40], 0x60000020)
	for index := sectionRaw; index < sectionRaw+sectionSize; index++ {
		body[index] = byte(index)
	}
	certificateDirectory := optionalOffset + 112 + 4*8
	if signed {
		binary.LittleEndian.PutUint32(body[certificateDirectory:certificateDirectory+4], sectionRaw+sectionSize)
		binary.LittleEndian.PutUint32(body[certificateDirectory+4:certificateDirectory+8], 8)
		binary.LittleEndian.PutUint32(body[sectionRaw+sectionSize:sectionRaw+sectionSize+4], 8)
		binary.LittleEndian.PutUint16(body[sectionRaw+sectionSize+4:sectionRaw+sectionSize+6], 0x0200)
		binary.LittleEndian.PutUint16(body[sectionRaw+sectionSize+6:sectionRaw+sectionSize+8], 0x0002)
	}
	return body
}

func syntheticELFV2(executable bool) []byte {
	body := make([]byte, 128)
	copy(body[:4], []byte{0x7f, 'E', 'L', 'F'})
	body[4] = 2
	body[5] = 1
	binary.LittleEndian.PutUint16(body[16:18], 2)
	binary.LittleEndian.PutUint16(body[18:20], 183)
	binary.LittleEndian.PutUint64(body[32:40], 64)
	binary.LittleEndian.PutUint16(body[54:56], 56)
	binary.LittleEndian.PutUint16(body[56:58], 1)
	binary.LittleEndian.PutUint32(body[64:68], 1)
	if executable {
		binary.LittleEndian.PutUint32(body[68:72], 1)
	}
	binary.LittleEndian.PutUint64(body[72:80], 0)
	binary.LittleEndian.PutUint64(body[96:104], uint64(len(body)))
	return bytes.Clone(body)
}
