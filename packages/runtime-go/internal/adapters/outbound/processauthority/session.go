package processauthority

import (
	"context"
	"errors"
	"os"
)

const MaxSessionFrameBytes = 4 * 1024 * 1024

var ErrProtocol = errors.New("process_authority_protocol_invalid")

type SessionConfig struct {
	Executable                *os.File
	ExpectedExecutableSHA256  string
	ExpectedExecutableSize    int64
	ReadOnlyInput             *ReadOnlyInput
	ReadWriteOutput           *ReadWriteOutput
	Arguments                 []string
	WorkingDirectory          string
	WorkingDirectoryAuthority *os.File
	StagingRoot               string
	StagingRootAuthority      *os.File
	Environment               []string
}

// ReadOnlyInput is one already-unlinked, host-verified private object that is
// inherited by the child at the process-authority-owned descriptor number.
// OpenSession consumes File on every outcome. Callers cannot choose the child
// descriptor, a path, SQL, or another process effect through this value.
type ReadOnlyInput struct {
	File           *os.File
	ExpectedSHA256 string
	ExpectedSize   int64
}

// ReadWriteOutput is one already-unlinked, host-created empty private object
// inherited by the child at a process-authority-owned descriptor number.
// OpenSession consumes File on every outcome. The child receives no pathname
// authority and can mutate only the exact inode represented by this handle.
type ReadWriteOutput struct {
	File *os.File
}

type ReadinessValidator func(frame []byte, processID int) bool

type Session interface {
	PID() int
	AuthenticateReadiness(context.Context, int, ReadinessValidator) error
	RoundTrip(context.Context, []byte, int) ([]byte, error)
	Close() error
}

func OpenSession(ctx context.Context, config SessionConfig) (Session, error) {
	if ctx == nil {
		if config.Executable != nil {
			_ = config.Executable.Close()
		}
		if config.ReadOnlyInput != nil && config.ReadOnlyInput.File != nil {
			_ = config.ReadOnlyInput.File.Close()
		}
		if config.ReadWriteOutput != nil && config.ReadWriteOutput.File != nil {
			_ = config.ReadWriteOutput.File.Close()
		}
		return nil, ErrRequestInvalid
	}
	return openSession(ctx, config)
}
