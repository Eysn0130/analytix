package processauthority

import (
	"context"
	"errors"
	"os"
	"time"
)

const (
	MaxArguments       = 256
	MaxArgumentBytes   = 1024 * 1024
	MaxEnvironment     = 256
	MaxEnvironmentByte = 1024 * 1024
)

var (
	ErrUnavailable        = errors.New("process_authority_unavailable")
	ErrRequestInvalid     = errors.New("process_authority_request_invalid")
	ErrExecutableIdentity = errors.New("process_authority_executable_identity_invalid")
	ErrWorkingDirectory   = errors.New("process_authority_workdir_invalid")
	ErrTimeout            = errors.New("process_authority_timeout")
	ErrCanceled           = errors.New("process_authority_canceled")
	ErrOutputLimit        = errors.New("process_authority_output_limit")
	ErrTermination        = errors.New("process_authority_termination_failed")
	ErrAbnormalExit       = errors.New("process_authority_abnormal_exit")
)

type TerminationStatus string

const (
	TerminationExited   TerminationStatus = "exited"
	TerminationSignaled TerminationStatus = "signaled"
)

type SystemTool string

const (
	SystemToolDarwinCodeSign SystemTool = "darwin_codesign"
)

type SystemToolRequest struct {
	Tool                      SystemTool
	Arguments                 []string
	WorkingDirectory          string
	WorkingDirectoryAuthority *os.File
	Timeout                   time.Duration
}

type Result struct {
	ExitCode          int
	TerminationStatus TerminationStatus
	Signal            int
	Stdout            []byte
	Stderr            []byte
}

func ExecuteSystemTool(ctx context.Context, request SystemToolRequest) (Result, error) {
	if ctx == nil {
		return Result{}, ErrRequestInvalid
	}
	return executeSystemTool(ctx, request)
}
