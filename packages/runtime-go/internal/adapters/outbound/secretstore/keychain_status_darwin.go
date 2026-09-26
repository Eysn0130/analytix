//go:build darwin

package secretstore

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const keychainStatusArgument = "--analytix-private-keychain-status-v1"

// This child only reads lock metadata. It cannot unlock, read an item, change
// settings or select a different authority. Running it out of process bounds
// Security.framework calls and keeps its noninteractive setting process-local.
func init() {
	if len(os.Args) != 2 || os.Args[1] != keychainStatusArgument {
		return
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 4097))
	if err != nil || len(input) > 4096 || !darwinSafeTaskKeychainPath(string(input)) {
		os.Exit(1)
	}
	if _, err := explicitDarwinKeychainSecurityIdentity(string(input)); err != nil {
		os.Exit(1)
	}
	if !nativeTaskKeychainUnlocked(string(input)) {
		os.Exit(1)
	}
	os.Exit(0)
}

func taskKeychainUnlocked(ctx context.Context, database string) bool {
	if !darwinSafeTaskKeychainPath(database) || ctx == nil || ctx.Err() != nil {
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	command := exec.CommandContext(ctx, executable, keychainStatusArgument)
	command.Stdin = strings.NewReader(database)
	command.Stdout, command.Stderr = io.Discard, io.Discard
	return command.Run() == nil
}

func nativeTaskKeychainUnlocked(database string) bool {
	// No API in this process is allowed to ask SecurityAgent for interaction.
	result, _, _ := keychainSyscall6(secInteractionAddress, 0, 0, 0, 0, 0, 0)
	if int32(result) != 0 {
		return false
	}
	path, err := syscall.BytePtrFromString(database)
	if err != nil {
		return false
	}
	var keychain uintptr
	result, _, _ = keychainSyscall6(secOpenAddress, uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&keychain)), 0, 0, 0, 0)
	runtime.KeepAlive(path)
	if int32(result) != 0 || keychain == 0 {
		return false
	}
	defer keychainSyscall6(cfReleaseAddress, keychain, 0, 0, 0, 0, 0)
	var status uint32
	result, _, _ = keychainSyscall6(secStatusAddress, keychain, uintptr(unsafe.Pointer(&status)), 0, 0, 0, 0)
	return int32(result) == 0 && status&1 != 0 // kSecUnlockStateStatus
}

// The runtime build uses CGO_ENABLED=0, as do the existing Darwin process
// adapters. Only system framework symbols are dynamically linked here.
//
//go:linkname keychainSyscall6 syscall.syscall6
//go:uintptrescapes
func keychainSyscall6(function, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, errno syscall.Errno)

var secInteractionAddress, secOpenAddress, secStatusAddress, cfReleaseAddress uintptr

//go:cgo_import_dynamic analytix_sec_interaction SecKeychainSetUserInteractionAllowed "/System/Library/Frameworks/Security.framework/Versions/A/Security"
//go:cgo_import_dynamic analytix_sec_open SecKeychainOpen "/System/Library/Frameworks/Security.framework/Versions/A/Security"
//go:cgo_import_dynamic analytix_sec_status SecKeychainGetStatus "/System/Library/Frameworks/Security.framework/Versions/A/Security"
//go:cgo_import_dynamic analytix_cf_release CFRelease "/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation"
