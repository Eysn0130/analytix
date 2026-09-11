//go:build !darwin && !linux && !windows

package persistencefs

func startupAuthorityPathNotFound(error) bool { return false }
