//go:build analytix_prod

package runtimego

import (
	"context"
	"errors"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

func TestProductionRootFactoryCannotBypassPersistenceLease(t *testing.T) {
	root := t.TempDir()
	config := RuntimeServerConfig{RuntimeToken: "prod-lease-test", DataDir: root, ProductionDurableRoot: root}
	first, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdown, ok := first.(interface{ Shutdown(context.Context) error })
	if !ok {
		t.Fatal("production root handler does not own a shutdown lifecycle")
	}
	if _, err := NewRuntimeServerHandlerE(config); !errors.Is(err, persistencefs.ErrPersistenceInUse) {
		_ = shutdown.Shutdown(context.Background())
		t.Fatalf("second production root factory bypassed the composite lease: %v", err)
	}
	if err := shutdown.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("production root lease was not released: %v", err)
	}
	if lifecycle, ok := restarted.(interface{ Shutdown(context.Context) error }); ok {
		_ = lifecycle.Shutdown(context.Background())
	}
}
