//go:build darwin

package processsandbox

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

const sandboxExecutable = "/usr/bin/sandbox-exec"

const sandboxDNSUnixSocket = "/private/var/run/mDNSResponder"

const (
	sandboxStorageProfilerPath    = "/usr/sbin/system_profiler"
	sandboxDiskArbitrationService = "com.apple.DiskArbitration.diskarbitrationd"
)

func preparePlatform(
	invocation Invocation,
	denyRoots, allowRoots []string,
	allowLoopbackTCPPort uint16,
) (Invocation, error) {
	return prepareDarwin(
		invocation, denyRoots, allowRoots, allowLoopbackTCPPort, sandboxExecutable,
	)
}

func prepareDarwin(
	invocation Invocation,
	denyRoots, allowRoots []string,
	allowLoopbackTCPPort uint16,
	sandboxPath string,
) (Invocation, error) {
	if !validSandboxExecutable(sandboxPath) {
		return Invocation{}, ErrUnavailable
	}

	profile := buildProfile(denyRoots, allowRoots, allowLoopbackTCPPort)
	arguments := make([]string, 0, len(invocation.Args)+3)
	arguments = append(arguments, "-p", profile, invocation.Executable)
	arguments = append(arguments, invocation.Args...)
	return Invocation{
		Executable: sandboxPath,
		Args:       arguments,
	}, nil
}

func validSandboxExecutable(path string) bool {
	info, err := os.Lstat(path)
	if err != nil ||
		!info.Mode().IsRegular() ||
		info.Mode().Perm()&0o111 == 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}

func buildProfile(denyRoots, allowRoots []string, allowLoopbackTCPPort uint16) string {
	var profile strings.Builder
	profile.WriteString("(version 1)\n")
	profile.WriteString("(allow default)\n")
	profile.WriteString("(deny process-info*)\n")
	// Self inspection is required by ordinary Apple command-line tools, while
	// inspection of the host or sibling processes remains denied.
	profile.WriteString("(allow process-info* (target self))\n")
	// A pathname-only deny is not a complete boundary: an untrusted child can
	// otherwise ask an unsandboxed host service to read the protected object on
	// its behalf. Keep ordinary fork/exec and remote IP networking available,
	// but close host-delegation surfaces before applying filesystem exceptions.
	profile.WriteString("(deny appleevent-send)\n")
	profile.WriteString("(deny job-creation lsopen system-socket)\n")
	profile.WriteString("(deny mach-bootstrap mach-lookup mach-register mach-per-user-lookup mach-cross-domain-lookup)\n")
	profile.WriteString("(deny mach-task-name mach-task-read mach-task-special-port* mach-priv-host-port)\n")
	// The cache helper consumes read-only storage metadata from the system
	// profiler. Scope Disk Arbitration to that fixed Apple reporter executable;
	// arbitrary shell descendants, including diskutil, retain the broad deny.
	profile.WriteString("(with-filter (process-path ")
	profile.WriteString(quoteSeatbeltString(sandboxStorageProfilerPath))
	profile.WriteString(")\n  (allow mach-lookup (global-name ")
	profile.WriteString(quoteSeatbeltString(sandboxDiskArbitrationService))
	profile.WriteString(")))\n")
	profile.WriteString("(deny ipc-posix* ipc-sysv*)\n")
	profile.WriteString("(deny network-outbound (remote unix-socket))\n")
	profile.WriteString("(deny network-outbound (remote ip \"localhost:*\"))\n")
	if allowLoopbackTCPPort != 0 {
		profile.WriteString("(allow network-outbound (remote ip \"localhost:")
		profile.WriteString(strconv.Itoa(int(allowLoopbackTCPPort)))
		profile.WriteString("\"))\n")
	}
	// DNS is the one fixed local socket exception. Its protocol resolves names;
	// it does not accept caller-selected filesystem paths or launch effects.
	profile.WriteString("(allow network-outbound (remote unix-socket (path-literal ")
	profile.WriteString(quoteSeatbeltString(sandboxDNSUnixSocket))
	profile.WriteString(")))\n")
	for _, root := range denyRoots {
		// Seatbelt combines sibling filters in one rule with AND semantics. A
		// separate rule per root is therefore required: putting two disjoint
		// subpaths in one deny rule silently denies neither of them.
		profile.WriteString("(deny file-read* file-write* process-exec (subpath ")
		profile.WriteString(quoteSeatbeltString(root))
		profile.WriteString("))\n")
	}
	for _, root := range allowRoots {
		profile.WriteString("(allow file-read* process-exec (subpath ")
		profile.WriteString(quoteSeatbeltString(root))
		profile.WriteString("))\n")
	}
	return profile.String()
}

func quoteSeatbeltString(value string) string {
	var escaped strings.Builder
	escaped.Grow(len(value) + 2)
	escaped.WriteByte('"')
	for _, character := range value {
		switch character {
		case '\\', '"':
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(character)
	}
	escaped.WriteByte('"')
	return escaped.String()
}
