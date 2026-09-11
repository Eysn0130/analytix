//go:build windows

package finalauthority

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateWindowsConcurrentFirstDirectoryCreationReopensWinnerByFileID(t *testing.T) {
	parent, err := privateWindowsOpenAbsoluteDirectory(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(parent)

	var arrivals atomic.Int32
	release := make(chan struct{})
	beforeCreate := func() {
		if arrivals.Add(1) == 2 {
			close(release)
		}
		<-release
	}
	type result struct {
		identity privateCASShardIdentity
		created  bool
		err      error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			handle, created, err := privateWindowsOpenOrCreateRelativeDirectory(
				parent, "aa",
				windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE,
				windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE,
				beforeCreate,
			)
			if err != nil {
				results <- result{err: err}
				return
			}
			identity, identityErr := privateCASWindowsIdentity(handle)
			closeErr := windows.CloseHandle(handle)
			results <- result{identity: identity, created: created, err: errors.Join(identityErr, closeErr)}
		}()
	}
	wait.Wait()
	close(results)
	values := make([]result, 0, 2)
	for current := range results {
		if current.err != nil {
			t.Fatalf("concurrent directory contender failed: %v", current.err)
		}
		values = append(values, current)
	}
	if len(values) != 2 || values[0].identity != values[1].identity {
		t.Fatalf("contenders did not converge on one FileID: %#v", values)
	}
	created := 0
	for _, current := range values {
		if current.created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("exactly one FILE_CREATE must win: %#v", values)
	}
}
