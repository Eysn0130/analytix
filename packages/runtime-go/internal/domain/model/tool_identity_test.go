package model

import (
	"bytes"
	"strings"
	"testing"
)

func TestHostToolCallIDIsOpaqueAndEntropyBound(t *testing.T) {
	first, err := NewHostToolCallIDV1(bytes.Repeat([]byte{0x11}, HostToolCallIDEntropyBytesV1))
	if err != nil || !IsHostToolCallIDV1(first) {
		t.Fatalf("host tool-call identity is invalid: id=%q err=%v", first, err)
	}
	if strings.Contains(first, "6222020202020202020") {
		t.Fatalf("host tool-call identity leaked provider bytes: %q", first)
	}
	second, err := NewHostToolCallIDV1(bytes.Repeat([]byte{0x22}, HostToolCallIDEntropyBytesV1))
	if err != nil || first == second {
		t.Fatalf("distinct host entropy did not produce distinct identities: first=%q second=%q err=%v", first, second, err)
	}
	if _, err := NewHostToolCallIDV1([]byte("short")); err == nil {
		t.Fatal("host tool-call identity accepted invalid entropy")
	}
	if IsHostToolCallIDV1("call_host_not-a-digest") {
		t.Fatal("host tool-call identity accepted an invalid value")
	}
}
