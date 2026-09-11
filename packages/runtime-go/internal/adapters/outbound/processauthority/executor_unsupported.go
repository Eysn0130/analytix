//go:build !darwin

package processauthority

import "context"

func executeSystemTool(context.Context, SystemToolRequest) (Result, error) {
	return Result{}, ErrUnavailable
}
