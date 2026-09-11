//go:build !analytix_prod

package processauthority

import (
	"errors"
	"testing"
)

func TestBootstrapDispatcherRejectsForgedOrAmbiguousMarkers(t *testing.T) {
	t.Setenv(darwinBootstrapEnvironment, "")
	for name, arguments := range map[string][]string{
		"missing reserved environment": {"runtime-server", darwinBootstrapMarker},
		"marker is not final":          {"runtime-server", darwinBootstrapMarker, "--user-flag"},
		"duplicate marker":             {"runtime-server", darwinBootstrapMarker, darwinBootstrapMarker},
	} {
		t.Run(name, func(t *testing.T) {
			handled, err := dispatchBootstrapInvocation(arguments)
			if !handled || !errors.Is(err, errBootstrapInvalid) {
				t.Fatalf("forged bootstrap marker survived: handled=%t err=%v", handled, err)
			}
		})
	}
	if handled, err := dispatchBootstrapInvocation([]string{"runtime-server", "--insecure"}); handled || err != nil {
		t.Fatalf("ordinary invocation entered bootstrap: handled=%t err=%v", handled, err)
	}
}

func TestBootstrapBuildIdentityAllowlistIsExact(t *testing.T) {
	for buildPath, want := range map[string]bool{
		processAuthorityTestMainPath:                        true,
		nativeComponentRunnerTestMainPath:                   true,
		"analytix.local/runtime-go/cmd/runtime-server":      false,
		"analytix.local/runtime-go/cmd/runtime-server.test": false,
		"evil.test":                                   false,
		processAuthorityTestMainPath + ".forged":      false,
		"prefix." + nativeComponentRunnerTestMainPath: false,
		"": false,
	} {
		if got := bootstrapTestMainPathAllowed(buildPath); got != want {
			t.Fatalf("bootstrapTestMainPathAllowed(%q) = %t, want %t", buildPath, got, want)
		}
	}
	if !currentBootstrapTestMainAllowed() {
		t.Fatal("the processauthority test main lost its exact immutable build identity")
	}
}
