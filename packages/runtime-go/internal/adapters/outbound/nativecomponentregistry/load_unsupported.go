//go:build !darwin

package nativecomponentregistry

import (
	"context"
	"os"
)

func load(Config) (*Registry, error) {
	return nil, ErrUnavailable
}

func duplicateExecutionFile(context.Context, Component) (*os.File, error) {
	return nil, ErrUnavailable
}
