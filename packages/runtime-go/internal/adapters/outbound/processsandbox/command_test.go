package processsandbox

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareWithoutDenyRootsPreservesInvocation(t *testing.T) {
	arguments := []string{"-c", "exit 0"}
	invocation, err := Prepare("/bin/sh", arguments, FilesystemPolicy{})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	arguments[0] = "mutated"

	if invocation.Executable != "/bin/sh" {
		t.Fatalf("Executable = %q, want /bin/sh", invocation.Executable)
	}
	if len(invocation.Args) != 2 || invocation.Args[0] != "-c" || invocation.Args[1] != "exit 0" {
		t.Fatalf("Args = %#v, want copied original arguments", invocation.Args)
	}
}

func TestPrepareRejectsLoopbackExceptionWithoutContainment(t *testing.T) {
	_, err := Prepare(
		"/usr/bin/true",
		nil,
		FilesystemPolicy{AllowLoopbackTCPPort: 9787},
	)
	if !errors.Is(err, ErrInvalidNetworkPolicy) {
		t.Fatalf("Prepare() error = %v, want ErrInvalidNetworkPolicy", err)
	}
}

func TestPrepareRejectsNonNormalizedDenyRootsWithoutDisclosure(t *testing.T) {
	privateMarker := "private-deny-root-marker"
	invalidRoots := []string{
		privateMarker,
		string(filepath.Separator) + privateMarker + string(filepath.Separator) + ".." + string(filepath.Separator) + privateMarker,
		string(filepath.Separator) + privateMarker + string(filepath.Separator),
		string(filepath.Separator) + privateMarker + "\nchild",
	}

	for _, root := range invalidRoots {
		_, err := Prepare(
			"/usr/bin/true",
			nil,
			FilesystemPolicy{DenyRoots: []string{root}},
		)
		if !errors.Is(err, ErrInvalidDenyRoot) {
			t.Fatalf("Prepare(%q) error = %v, want ErrInvalidDenyRoot", root, err)
		}
		if strings.Contains(err.Error(), privateMarker) {
			t.Fatalf("error disclosed deny root marker: %q", err)
		}
	}
}

func TestCommandContextRejectsNilContextAndInvalidArguments(t *testing.T) {
	testCases := []struct {
		name       string
		ctx        context.Context
		executable string
		args       []string
	}{
		{name: "nil context", executable: "/usr/bin/true"},
		{name: "empty executable", ctx: context.Background()},
		{name: "nul executable", ctx: context.Background(), executable: "/bin/\x00sh"},
		{name: "nul argument", ctx: context.Background(), executable: "/bin/sh", args: []string{"a\x00b"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := CommandContext(
				testCase.ctx,
				testCase.executable,
				testCase.args,
				FilesystemPolicy{},
			)
			if !errors.Is(err, ErrInvalidInvocation) {
				t.Fatalf("CommandContext() error = %v, want ErrInvalidInvocation", err)
			}
		})
	}
}

func TestPrepareRejectsAllowRootOutsideDenyAncestorWithoutDisclosure(t *testing.T) {
	privateMarker := "private-allow-root-marker"
	denyRoot := filepath.Join(string(filepath.Separator), "denied")
	allowRoot := filepath.Join(string(filepath.Separator), privateMarker)

	_, err := Prepare(
		"/usr/bin/true",
		nil,
		FilesystemPolicy{
			DenyRoots:             []string{denyRoot},
			AllowReadExecuteRoots: []string{allowRoot},
		},
	)
	if !errors.Is(err, ErrInvalidAllowRoot) {
		t.Fatalf("Prepare() error = %v, want ErrInvalidAllowRoot", err)
	}
	if strings.Contains(err.Error(), privateMarker) {
		t.Fatalf("error disclosed allow root marker: %q", err)
	}
}
