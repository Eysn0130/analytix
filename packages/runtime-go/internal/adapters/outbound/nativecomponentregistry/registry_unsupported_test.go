//go:build !darwin

package nativecomponentregistry

import (
	"context"
	"errors"
	"testing"
)

func TestNativeRegistryIsDeterministicallyUnavailableOffDarwin(t *testing.T) {
	registry := &Registry{}
	if _, err := registry.AcquireExecutionLease(context.Background(), "data-engine"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("execution lease error = %v, want %v", err, ErrUnavailable)
	}
	if _, err := Load(Config{}); !errors.Is(err, ErrTrustInvalid) {
		t.Fatalf("invalid config error = %v, want %v", err, ErrTrustInvalid)
	}
}
