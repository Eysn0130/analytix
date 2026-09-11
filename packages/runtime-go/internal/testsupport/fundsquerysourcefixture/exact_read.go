// Package fundsquerysourcefixture provides test-only exact-source leases and
// unlinked destinations for application-layer tests. Production packages must
// not import it.
package fundsquerysourcefixture

import (
	"context"
	"os"
	"sync"
	"testing"

	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

// ExactReadLeaseV1 is a callback-scoped, one-shot test implementation of the
// private funds query source lease.
type ExactReadLeaseV1 struct {
	mu      sync.Mutex
	active  bool
	used    bool
	body    []byte
	failure error
	copies  int
	counter *int
}

func NewExactReadLeaseV1(body []byte) *ExactReadLeaseV1 {
	return &ExactReadLeaseV1{
		active: true,
		body:   append([]byte(nil), body...),
	}
}

func NewExactReadLeaseV1WithCounter(body []byte, counter *int) *ExactReadLeaseV1 {
	lease := NewExactReadLeaseV1(body)
	lease.counter = counter
	return lease
}

func (lease *ExactReadLeaseV1) CopyExactTo(ctx context.Context, destination *os.File) error {
	if lease == nil {
		return fundsquerysourceport.ErrUnavailable
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.active || lease.used {
		return fundsquerysourceport.ErrUnavailable
	}
	if ctx == nil || destination == nil {
		return fundsquerysourceport.ErrMismatch
	}
	lease.used = true
	lease.copies++
	if lease.counter != nil {
		*lease.counter = lease.copies
	}
	if lease.failure != nil {
		return lease.failure
	}
	_, err := destination.WriteAt(lease.body, 0)
	return err
}

func (lease *ExactReadLeaseV1) Close() {
	if lease == nil {
		return
	}
	lease.mu.Lock()
	lease.active = false
	lease.body = nil
	lease.mu.Unlock()
}

func (lease *ExactReadLeaseV1) CopyCount() int {
	if lease == nil {
		return 0
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	return lease.copies
}

func (lease *ExactReadLeaseV1) SetFailure(failure error) {
	if lease == nil {
		return
	}
	lease.mu.Lock()
	lease.failure = failure
	lease.mu.Unlock()
}

func NewUnlinkedDestinationV1(directory string) (*os.File, error) {
	file, err := os.CreateTemp(directory, "analytix-exact-destination-")
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := os.Remove(file.Name()); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func MustNewUnlinkedDestinationV1(t testing.TB) *os.File {
	t.Helper()
	file, err := NewUnlinkedDestinationV1(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func ReadExactDestinationV1(file *os.File) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	body := make([]byte, info.Size())
	if len(body) == 0 {
		return body, nil
	}
	_, err = file.ReadAt(body, 0)
	return body, err
}

func MustReadExactDestinationV1(t testing.TB, file *os.File) []byte {
	t.Helper()
	body, err := ReadExactDestinationV1(file)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

var _ fundsquerysourceport.ExactReadLease = (*ExactReadLeaseV1)(nil)
