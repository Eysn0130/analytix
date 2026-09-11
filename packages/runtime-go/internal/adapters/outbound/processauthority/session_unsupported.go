//go:build !darwin

package processauthority

import "context"

func openSession(_ context.Context, config SessionConfig) (Session, error) {
	if config.Executable != nil {
		_ = config.Executable.Close()
	}
	if config.ReadOnlyInput != nil && config.ReadOnlyInput.File != nil {
		_ = config.ReadOnlyInput.File.Close()
	}
	if config.ReadWriteOutput != nil && config.ReadWriteOutput.File != nil {
		_ = config.ReadWriteOutput.File.Close()
	}
	return nil, ErrUnavailable
}
