package server

import (
	"strings"
	"testing"
)

func TestNewRuntimeServerHandlerFromComponentsRequiresHostIdentity(t *testing.T) {
	_, err := NewRuntimeServerHandlerFromComponents(RuntimeServerConfig{}, RuntimeServerComponents{})
	if err == nil || !strings.Contains(err.Error(), "host identity authority is required") {
		t.Fatalf("error = %v, want missing host identity authority", err)
	}
}
