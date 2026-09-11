//go:build !darwin && !linux && !windows

package finalauthority

func privateCASBindingPathEqual(string, string) bool { return false }
