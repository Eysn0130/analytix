package runtimeapp

import (
	"errors"
	"testing"

	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
)

func TestNativeAdmissionFailureClassificationKeepsOptionalCaseCapabilityAdditive(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "unavailable", err: nativecomponenthost.ErrUnavailable, want: true},
		{name: "wrapped unavailable", err: errors.Join(errors.New("package absent"), nativecomponenthost.ErrUnavailable), want: true},
		{name: "untrusted", err: errors.Join(nativecomponenthost.ErrTrust, errors.New("receipt mismatch")), want: true},
		{name: "lifecycle", err: nativecomponenthost.ErrLifecycle, want: false},
		{name: "unavailable with unproved cleanup", err: errors.Join(nativecomponenthost.ErrUnavailable, nativecomponenthost.ErrLifecycle), want: false},
		{name: "untrusted with unproved cleanup", err: errors.Join(nativecomponenthost.ErrTrust, nativecomponenthost.ErrLifecycle), want: false},
		{name: "unknown", err: errors.New("unknown native failure"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeAdmissionMayDisableCaseCapability(test.err); got != test.want {
				t.Fatalf("classification = %v, want %v for %v", got, test.want, test.err)
			}
		})
	}
}
