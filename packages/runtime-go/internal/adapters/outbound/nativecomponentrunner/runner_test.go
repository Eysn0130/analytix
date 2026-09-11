package nativecomponentrunner

import (
	"context"
	"errors"
	"sync"
	"testing"

	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
)

func TestRunnerCloseNeverForgetsUnconfirmedSessionTermination(t *testing.T) {
	session := &persistentCloseFailureSession{}
	runner := &Runner{gate: make(chan struct{}, 1), session: session, sessionBinding: "binding"}
	runner.gate <- struct{}{}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := runner.Close(); !errors.Is(err, ErrTermination) {
			t.Fatalf("close attempt %d error = %v, want %v", attempt, err, ErrTermination)
		}
		if runner.closed || runner.session != session || runner.sessionBinding != "binding" {
			t.Fatalf("close attempt %d forgot failed termination", attempt)
		}
	}
	if session.CloseCalls() != 2 {
		t.Fatalf("session close calls = %d, want persistent retry evidence", session.CloseCalls())
	}
}

type persistentCloseFailureSession struct {
	mu    sync.Mutex
	calls int
}

func (*persistentCloseFailureSession) PID() int { return 1 }

func (*persistentCloseFailureSession) AuthenticateReadiness(context.Context, int, processauthority.ReadinessValidator) error {
	return nil
}

func (*persistentCloseFailureSession) RoundTrip(context.Context, []byte, int) ([]byte, error) {
	return nil, nil
}

func (session *persistentCloseFailureSession) Close() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.calls++
	return processauthority.ErrTermination
}

func (session *persistentCloseFailureSession) CloseCalls() int {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.calls
}
