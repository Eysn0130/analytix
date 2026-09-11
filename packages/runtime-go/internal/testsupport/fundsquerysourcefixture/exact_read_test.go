package fundsquerysourcefixture

import (
	"context"
	"errors"
	"os"
	"testing"

	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

func TestExactReadLeaseV1CopiesOnceToUnlinkedDestination(t *testing.T) {
	copies := 0
	lease := NewExactReadLeaseV1WithCounter([]byte("exact-duckdb"), &copies)
	destination := MustNewUnlinkedDestinationV1(t)

	if _, err := os.Stat(destination.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination remained linked: %v", err)
	}
	if err := lease.CopyExactTo(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	if got := string(MustReadExactDestinationV1(t, destination)); got != "exact-duckdb" {
		t.Fatalf("copied body = %q", got)
	}
	if lease.CopyCount() != 1 || copies != 1 {
		t.Fatalf("copy count = %d/%d", lease.CopyCount(), copies)
	}
	if err := lease.CopyExactTo(context.Background(), destination); !errors.Is(err, fundsquerysourceport.ErrUnavailable) {
		t.Fatalf("second copy error = %v", err)
	}
}

func TestExactReadLeaseV1ClosesAndConsumesFailures(t *testing.T) {
	failure := errors.New("fixture failure")
	lease := NewExactReadLeaseV1([]byte("private"))
	lease.SetFailure(failure)
	destination := MustNewUnlinkedDestinationV1(t)

	if err := lease.CopyExactTo(context.Background(), destination); !errors.Is(err, failure) {
		t.Fatalf("copy error = %v", err)
	}
	if lease.CopyCount() != 1 {
		t.Fatalf("copy count = %d", lease.CopyCount())
	}
	if got := MustReadExactDestinationV1(t, destination); len(got) != 0 {
		t.Fatalf("failure wrote %d bytes", len(got))
	}

	closed := NewExactReadLeaseV1([]byte("private"))
	closed.Close()
	if err := closed.CopyExactTo(context.Background(), destination); !errors.Is(err, fundsquerysourceport.ErrUnavailable) {
		t.Fatalf("post-close copy error = %v", err)
	}
}
