package authoritycredentials

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestClientChainV1CanonicalRoundTrip(t *testing.T) {
	input := [][]byte{[]byte("leaf-der"), []byte("intermediate-der")}
	body, err := ClientChainV1Bytes(input)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseClientChainV1(body)
	if err != nil || len(parsed) != len(input) {
		t.Fatalf("client chain round trip failed: %#v err=%v", parsed, err)
	}
	for index := range input {
		if !bytes.Equal(parsed[index], input[index]) {
			t.Fatal("client chain changed certificate bytes")
		}
	}
	parsed[0][0] ^= 0xff
	again, err := ParseClientChainV1(body)
	if err != nil || !bytes.Equal(again[0], input[0]) {
		t.Fatal("client chain parser returned shared mutable bytes")
	}
}

func TestParseClientChainV1RejectsMalformedFraming(t *testing.T) {
	valid, err := ClientChainV1Bytes([][]byte{[]byte("leaf")})
	if err != nil {
		t.Fatal(err)
	}
	wrongCount := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(wrongCount[len(clientChainMagicV1):], 2)
	zeroLength := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(zeroLength[len(clientChainMagicV1)+4:], 0)
	oversizedLength := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(oversizedLength[len(clientChainMagicV1)+4:], uint32(MaxRootCABytesV1+1))
	for name, body := range map[string][]byte{
		"empty": nil, "wrong magic": append([]byte("x"), valid[1:]...),
		"truncated": valid[:len(valid)-1], "trailing": append(append([]byte(nil), valid...), 0),
		"wrong count": wrongCount, "zero length": zeroLength, "oversized length": oversizedLength,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseClientChainV1(body); err == nil {
				t.Fatal("malformed client chain was accepted")
			}
		})
	}
}
