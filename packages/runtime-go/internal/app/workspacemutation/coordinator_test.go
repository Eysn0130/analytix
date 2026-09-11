package workspacemutation

import (
	"context"
	"errors"
	"testing"
)

func TestCoordinatorSerializesMutationAndHonorsCancellation(t *testing.T) {
	coordinator := NewCoordinator()
	release, err := coordinator.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.Acquire(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting mutation did not honor cancellation: %v", err)
	}
	release()
	nextRelease, err := coordinator.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nextRelease()
	nextRelease()
}
