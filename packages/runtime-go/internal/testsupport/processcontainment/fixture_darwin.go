//go:build darwin

// Package processcontainment provides operating-system fixtures for process
// containment integration tests. Production code must not import this package.
package processcontainment

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	processadapter "analytix.local/runtime-go/internal/adapters/outbound/process"
	"analytix.local/runtime-go/internal/ports"
)

type Fixture struct {
	ProtectedRoots []string
	ProtectedFile  string
	OrdinaryRoot   string
}

func NewFixture(t testing.TB) Fixture {
	t.Helper()
	firstProtectedRoot := t.TempDir()
	secondProtectedRoot := t.TempDir()
	ordinaryRoot := t.TempDir()
	protectedFile := filepath.Join(secondProtectedRoot, "sentinel.txt")
	if err := os.WriteFile(protectedFile, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Fixture{
		ProtectedRoots: []string{firstProtectedRoot, secondProtectedRoot},
		ProtectedFile:  protectedFile,
		OrdinaryRoot:   ordinaryRoot,
	}
}

func (fixture Fixture) OrdinaryPath(name string) string {
	return filepath.Join(fixture.OrdinaryRoot, name)
}

func NewShellRunner() ports.ShellRunner {
	return processadapter.NewShellRunner()
}

type ReadDeputy struct {
	listener *net.TCPListener
	accepted chan bool
}

func StartReadDeputy(t testing.TB, protectedFile string) (*ReadDeputy, string) {
	t.Helper()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("listen for local read deputy: %v", err)
	}
	deputy := &ReadDeputy{
		listener: listener,
		accepted: make(chan bool, 1),
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		connection, acceptErr := listener.AcceptTCP()
		if acceptErr != nil {
			deputy.accepted <- false
			return
		}
		deputy.accepted <- true
		defer connection.Close()
		body, readErr := os.ReadFile(protectedFile)
		if readErr == nil {
			_, _ = connection.Write(body)
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	return deputy, fmt.Sprintf("/usr/bin/nc 127.0.0.1 %d", address.Port)
}

func (deputy *ReadDeputy) AssertNotReached(t testing.TB) {
	t.Helper()
	if deputy == nil || deputy.listener == nil {
		t.Fatal("local read deputy is unavailable")
	}
	_ = deputy.listener.Close()
	select {
	case accepted := <-deputy.accepted:
		if accepted {
			t.Fatal("contained shell reached a local process that could read protected data")
		}
	case <-time.After(time.Second):
		t.Fatal("local read deputy did not settle")
	}
}

func AssertFileContents(t testing.TB, path string, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || string(body) != want {
		t.Fatalf("ordinary shell output mismatch: body=%q err=%v", body, err)
	}
}
