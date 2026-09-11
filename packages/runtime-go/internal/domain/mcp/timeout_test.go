package mcp

import (
	"testing"
	"time"
)

func TestResolveServerTimeoutV1UsesClosedBoundedContract(t *testing.T) {
	for _, test := range []struct {
		input int64
		want  time.Duration
	}{
		{0, 30 * time.Second},
		{1, time.Millisecond},
		{45_000, 45 * time.Second},
		{MaxServerTimeoutMSV1, time.Hour},
	} {
		got, err := ResolveServerTimeoutV1(test.input)
		if err != nil || got != test.want {
			t.Fatalf("timeout projection mismatch: input=%d got=%s err=%v", test.input, got, err)
		}
	}
	for _, input := range []int64{-1, MaxServerTimeoutMSV1 + 1} {
		if got, err := ResolveServerTimeoutV1(input); err == nil || got != 0 {
			t.Fatalf("invalid timeout passed: input=%d got=%s err=%v", input, got, err)
		}
	}
}
