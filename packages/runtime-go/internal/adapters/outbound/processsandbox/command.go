package processsandbox

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	// ErrInvalidInvocation is deliberately path-free so callers can surface it
	// without disclosing process arguments or protected filesystem locations.
	ErrInvalidInvocation = errors.New("process_sandbox_invocation_invalid")
	// ErrInvalidDenyRoot is deliberately path-free because deny roots are
	// private enforcement inputs.
	ErrInvalidDenyRoot = errors.New("process_sandbox_deny_root_invalid")
	// ErrInvalidAllowRoot is deliberately path-free because allow roots are
	// private enforcement inputs.
	ErrInvalidAllowRoot = errors.New("process_sandbox_allow_root_invalid")
	// ErrInvalidNetworkPolicy means a narrow exception was requested without
	// the containment policy that it is allowed to refine.
	ErrInvalidNetworkPolicy = errors.New("process_sandbox_network_policy_invalid")
	// ErrUnavailable means the host cannot enforce the requested deny roots.
	// Callers must not retry the command without containment.
	ErrUnavailable = errors.New("process_sandbox_unavailable")
)

// Invocation is the executable and argument vector to pass to os/exec. When
// deny roots are present on a supported host, Executable is the host sandbox
// launcher and Args contain the original invocation behind its policy.
//
// Args may contain private enforcement inputs and must not be logged.
type Invocation struct {
	Executable string
	Args       []string
}

// FilesystemPolicy denies all reads and writes under DenyRoots. A strict
// descendant may be listed in AllowReadExecuteRoots when a pinned executable
// or plugin tree inside a denied ancestor must remain launchable. Such an
// exception restores reads and execution only; writes remain denied. A
// host-verified local deputy may receive one exact loopback TCP port while all
// other local deputy transports remain denied.
//
// Roots are private enforcement inputs. They must be canonical absolute paths
// (including resolved ancestors), as supplied by production's protected-root
// projection, and must not be logged. Seatbelt matches physical paths.
type FilesystemPolicy struct {
	DenyRoots             []string
	AllowReadExecuteRoots []string
	AllowLoopbackTCPPort  uint16
}

// Prepare returns a contained invocation. An empty policy preserves the
// original command and does not require a host sandbox facility.
func Prepare(
	executable string,
	args []string,
	policy FilesystemPolicy,
) (Invocation, error) {
	if executable == "" || !validArgument(executable) {
		return Invocation{}, ErrInvalidInvocation
	}
	copiedArgs := append([]string(nil), args...)
	for _, argument := range copiedArgs {
		if !validArgument(argument) {
			return Invocation{}, ErrInvalidInvocation
		}
	}

	denyRoots, err := validateRoots(policy.DenyRoots, ErrInvalidDenyRoot)
	if err != nil {
		return Invocation{}, err
	}
	allowRoots, err := validateRoots(
		policy.AllowReadExecuteRoots,
		ErrInvalidAllowRoot,
	)
	if err != nil {
		return Invocation{}, err
	}
	if !validAllowRoots(denyRoots, allowRoots) {
		return Invocation{}, ErrInvalidAllowRoot
	}
	if policy.AllowLoopbackTCPPort != 0 && len(denyRoots) == 0 {
		return Invocation{}, ErrInvalidNetworkPolicy
	}
	invocation := Invocation{
		Executable: executable,
		Args:       copiedArgs,
	}
	if len(denyRoots) == 0 {
		return invocation, nil
	}
	return preparePlatform(invocation, denyRoots, allowRoots, policy.AllowLoopbackTCPPort)
}

// CommandContext prepares an invocation and returns an exec.Cmd so callers can
// attach the same directory, environment, streams, and process lifecycle they
// would attach to the original command.
func CommandContext(
	ctx context.Context,
	executable string,
	args []string,
	policy FilesystemPolicy,
) (*exec.Cmd, error) {
	if ctx == nil {
		return nil, ErrInvalidInvocation
	}
	invocation, err := Prepare(executable, args, policy)
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, invocation.Executable, invocation.Args...), nil
}

func validArgument(value string) bool {
	return !strings.ContainsRune(value, '\x00')
}

func validateRoots(values []string, invalidError error) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(values))
	roots := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" ||
			!utf8.ValidString(value) ||
			strings.ContainsRune(value, '\x00') ||
			containsUnsupportedControl(value) ||
			!filepath.IsAbs(value) ||
			filepath.Clean(value) != value {
			return nil, invalidError
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		roots = append(roots, value)
	}
	sort.Strings(roots)
	return roots, nil
}

func validAllowRoots(denyRoots, allowRoots []string) bool {
	if len(allowRoots) == 0 {
		return true
	}
	if len(denyRoots) == 0 {
		return false
	}
	for _, allowed := range allowRoots {
		withinDenyRoot := false
		for _, denied := range denyRoots {
			if strictDescendant(denied, allowed) {
				withinDenyRoot = true
				break
			}
		}
		if !withinDenyRoot {
			return false
		}
	}
	return true
}

func strictDescendant(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil &&
		relative != "." &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func containsUnsupportedControl(value string) bool {
	for _, character := range value {
		if character < ' ' || character == '\x7f' {
			return true
		}
	}
	return false
}
