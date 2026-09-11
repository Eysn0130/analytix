package nativecomponentrunner

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

var (
	ErrUnavailable    = nativecomponentport.ErrUnavailable
	ErrRequestInvalid = nativecomponentport.ErrRequestInvalid
	ErrRegistry       = nativecomponentport.ErrRegistryInvalid
	ErrProtocol       = nativecomponentport.ErrProtocolInvalid
	ErrTermination    = nativecomponentport.ErrTerminationUnconfirmed
)

const runnerCloseDeadline = 15 * time.Second

type Config struct {
	Registry                  *nativecomponentregistry.Registry
	WorkingDirectory          string
	WorkingDirectoryAuthority *os.File
	StagingRoot               string
	StagingRootAuthority      *os.File
}

type executionLease interface {
	TakeExecutionFile() (*os.File, nativecomponentregistry.ExecutionIdentity, error)
	Close() error
}

type executionRegistry interface {
	Digest() string
	Acquire(context.Context, string) (executionLease, error)
}

type sessionOpenRequest struct {
	Executable                *os.File
	ExpectedExecutableSHA256  string
	ExpectedExecutableSize    int64
	ReadOnlyInput             *sessionReadOnlyInput
	ReadWriteOutput           *sessionReadWriteOutput
	Arguments                 []string
	WorkingDirectory          string
	WorkingDirectoryAuthority *os.File
	StagingRoot               string
	StagingRootAuthority      *os.File
	Environment               []string
}

type sessionReadOnlyInput struct {
	File           *os.File
	ExpectedSHA256 string
	ExpectedSize   int64
}

type sessionReadWriteOutput struct {
	File *os.File
}

type runnerSession = processauthority.Session

type sessionOpenFunc func(context.Context, sessionOpenRequest) (runnerSession, error)

type Runner struct {
	gate             chan struct{}
	registry         executionRegistry
	registryDigest   string
	workingDirectory string
	workingAuthority *os.File
	stagingRoot      string
	stagingAuthority *os.File
	arguments        []string
	sessionOpener    sessionOpenFunc
	session          runnerSession
	sessionBinding   string
	closed           bool
	poisoned         atomic.Bool
}

var _ nativecomponentport.TransactionSourceRowPageRunner = (*Runner)(nil)
var _ nativecomponentport.FundsCanonicalCSVSnapshotBuilder = (*Runner)(nil)
var _ nativecomponentport.DeterministicCleaningRunner = (*Runner)(nil)

func New(config Config) (*Runner, error) {
	return newRunner(config)
}

func (runner *Runner) Close() error {
	if runner == nil {
		return nil
	}
	timer := time.NewTimer(runnerCloseDeadline)
	defer timer.Stop()
	select {
	case <-runner.gate:
		defer runner.release()
		if runner.closed {
			return nil
		}
		if err := runner.closeSessionLocked(); err != nil {
			runner.poisoned.Store(true)
			return ErrTermination
		}
		runner.closed = true
		return nil
	case <-timer.C:
		runner.poisoned.Store(true)
		return ErrTermination
	}
}

func (runner *Runner) acquire(ctx context.Context) error {
	if runner == nil || ctx == nil {
		return ErrRequestInvalid
	}
	select {
	case <-runner.gate:
		if ctx.Err() != nil {
			runner.release()
			return contextError(ctx)
		}
		return nil
	case <-ctx.Done():
		return contextError(ctx)
	}
}

func (runner *Runner) release() {
	select {
	case runner.gate <- struct{}{}:
	default:
		panic("native component runner gate released twice")
	}
}

func (runner *Runner) closeSessionLocked() error {
	session := runner.session
	if session == nil {
		return nil
	}
	if err := session.Close(); err != nil {
		return err
	}
	runner.session = nil
	runner.sessionBinding = ""
	return nil
}

func (runner *Runner) failSessionLocked(runErr error) error {
	if closeErr := runner.closeSessionLocked(); closeErr != nil {
		runner.poisoned.Store(true)
		return ErrTermination
	}
	return runErr
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return ErrRequestInvalid
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return ErrUnavailable
}
