package authoritycredentials

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const MaxClientChainCertificatesV1 = 8

var clientChainMagicV1 = []byte("analytix.client-cert-chain/v1\x00")

// ClientChainV1Bytes serializes leaf-first DER certificates using one closed,
// length-prefixed representation. PEM whitespace, headers, encryption, and
// concatenation heuristics are deliberately outside this contract.
func ClientChainV1Bytes(certificates [][]byte) ([]byte, error) {
	if len(certificates) == 0 || len(certificates) > MaxClientChainCertificatesV1 {
		return nil, errors.New("authority client certificate chain count is invalid")
	}
	total := len(clientChainMagicV1) + 4
	for _, certificate := range certificates {
		if len(certificate) == 0 || len(certificate) > int(MaxRootCABytesV1) ||
			total > int(MaxClientChainBytesV1)-4-len(certificate) {
			return nil, errors.New("authority client certificate chain size is invalid")
		}
		total += 4 + len(certificate)
	}
	out := make([]byte, 0, total)
	out = append(out, clientChainMagicV1...)
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], uint32(len(certificates)))
	out = append(out, encoded[:]...)
	for _, certificate := range certificates {
		binary.BigEndian.PutUint32(encoded[:], uint32(len(certificate)))
		out = append(out, encoded[:]...)
		out = append(out, certificate...)
	}
	return out, nil
}

func ParseClientChainV1(body []byte) ([][]byte, error) {
	if len(body) <= len(clientChainMagicV1)+4 || len(body) > int(MaxClientChainBytesV1) ||
		!bytes.Equal(body[:minIntV1(len(body), len(clientChainMagicV1))], clientChainMagicV1) {
		return nil, errors.New("authority client certificate chain framing is invalid")
	}
	offset := len(clientChainMagicV1)
	count := int(binary.BigEndian.Uint32(body[offset : offset+4]))
	offset += 4
	if count == 0 || count > MaxClientChainCertificatesV1 {
		return nil, errors.New("authority client certificate chain count is invalid")
	}
	certificates := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		if offset > len(body)-4 {
			return nil, errors.New("authority client certificate chain is truncated")
		}
		length := int(binary.BigEndian.Uint32(body[offset : offset+4]))
		offset += 4
		if length <= 0 || length > int(MaxRootCABytesV1) || offset > len(body)-length {
			return nil, errors.New("authority client certificate chain certificate size is invalid")
		}
		certificates = append(certificates, append([]byte(nil), body[offset:offset+length]...))
		offset += length
	}
	if offset != len(body) {
		return nil, errors.New("authority client certificate chain contains trailing bytes")
	}
	return certificates, nil
}

func minIntV1(left, right int) int {
	if left < right {
		return left
	}
	return right
}
