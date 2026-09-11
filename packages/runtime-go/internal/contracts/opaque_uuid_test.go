package contracts

import (
	"testing"
	"time"
)

func TestCanonicalOpaqueUUIDV4Contract(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"00000000-0000-4000-8000-000000000000",
	} {
		if !IsCanonicalOpaqueUUIDV4(value) {
			t.Fatalf("canonical UUIDv4 rejected: %q", value)
		}
	}
	for _, value := range []string{
		"", "6222021234567890123", "F47AC10B-58CC-4372-A567-0E02B2C3D479",
		"f47ac10b-58cc-1372-a567-0e02b2c3d479", "f47ac10b-58cc-4372-c567-0e02b2c3d479",
	} {
		if IsCanonicalOpaqueUUIDV4(value) {
			t.Fatalf("non-canonical public identifier accepted: %q", value)
		}
	}
	generated := NewOpaqueUUIDV4("test", "seed", time.Unix(0, 1).UTC())
	if !IsCanonicalOpaqueUUIDV4(generated) {
		t.Fatalf("host generator returned invalid UUIDv4: %q", generated)
	}
}
