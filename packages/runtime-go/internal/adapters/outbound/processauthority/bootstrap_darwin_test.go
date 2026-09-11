//go:build darwin && !analytix_prod

package processauthority

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestDarwinBootstrapCommandRoundTripsExactHostConfiguration(t *testing.T) {
	want := darwinBootstrapCommand{
		target:      "/private/stage/target",
		arguments:   []string{"--mode", "health"},
		environment: []string{"LANG=C", "LC_ALL=C", "TZ=UTC"},
	}
	encoded, err := encodeDarwinBootstrapCommand(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readDarwinBootstrapCommand(bytes.NewReader(encoded))
	if err != nil || got.target != want.target || !equalBootstrapStrings(got.arguments, want.arguments) ||
		!equalBootstrapStrings(got.environment, want.environment) {
		t.Fatalf("bootstrap command = %#v err=%v, want %#v", got, err, want)
	}
}

func TestDarwinBootstrapCommandRejectsMalformedFrames(t *testing.T) {
	valid, err := encodeDarwinBootstrapCommand(darwinBootstrapCommand{
		target:      "/private/stage/target",
		arguments:   []string{"health"},
		environment: []string{"LANG=C"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixtures := map[string][]byte{
		"truncated":         append([]byte(nil), valid[:len(valid)-1]...),
		"unknown magic":     append([]byte("FORGED!!"), valid[8:]...),
		"empty environment": mutateDarwinBootstrapUint32(valid, 16, 0),
		"oversized length":  mutateDarwinBootstrapUint32(valid, 8, darwinBootstrapCommandLimit),
	}
	for name, payload := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, err := readDarwinBootstrapCommand(bytes.NewReader(payload)); !errors.Is(err, errBootstrapInvalid) {
				t.Fatalf("malformed bootstrap frame survived: %v", err)
			}
		})
	}
}

func mutateDarwinBootstrapUint32(payload []byte, offset int, value uint32) []byte {
	mutated := append([]byte(nil), payload...)
	binary.BigEndian.PutUint32(mutated[offset:offset+4], value)
	return mutated
}

func equalBootstrapStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
