//go:build darwin

package packagedbuildauthorityfs

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

func TestDarwinPackageAnchorAllowsSlowValidVerificationInsideFixedBudget(t *testing.T) {
	if packageAnchorDevelopmentVerificationBudgetV2 != 180*time.Second {
		t.Fatalf("unexpected development package anchor verification budget: %s", packageAnchorDevelopmentVerificationBudgetV2)
	}
	const resources = "/private/tmp/Analytix.app/Contents/Resources"
	authority := darwinDevelopmentAuthorityV2()
	calls := 0
	runner := func(ctx context.Context, arguments ...string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("package anchor verification has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining < 179*time.Second || remaining > packageAnchorDevelopmentVerificationBudgetV2 {
			t.Fatalf("package anchor verification did not receive the fixed budget: %s", remaining)
		}
		select {
		case <-time.After(30 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		calls++
		switch calls {
		case 1:
			want := []string{"--verify", "--deep", "--strict", "--verbose=2", "/private/tmp/Analytix.app"}
			if !reflect.DeepEqual(arguments, want) {
				t.Fatalf("verification arguments changed: got %#v want %#v", arguments, want)
			}
			return nil, nil
		case 2:
			want := []string{"--display", "--verbose=4", "/private/tmp/Analytix.app"}
			if !reflect.DeepEqual(arguments, want) {
				t.Fatalf("metadata arguments changed: got %#v want %#v", arguments, want)
			}
			return []byte("Executable=/private/tmp/Analytix.app/Contents/MacOS/analytix\n" +
				"Identifier=com.analytix.desktop\n" +
				"Signature=adhoc\n" +
				"TeamIdentifier=not set\n"), nil
		default:
			t.Fatalf("unexpected codesign call %d", calls)
			return nil, errors.New("unexpected codesign call")
		}
	}

	anchor, err := verifyPackageAnchorWithRunnerV2(
		context.Background(), resources, authority, runner,
	)
	if err != nil || anchor != "macos_nonpublishable_resource_seal" || calls != 2 {
		t.Fatalf("slow valid package anchor was rejected: anchor=%q calls=%d err=%v", anchor, calls, err)
	}
}

func TestDarwinPackageAnchorBudgetIsDevelopmentOnly(t *testing.T) {
	development := darwinDevelopmentAuthorityV2()
	controlledDisposition := &domainauthority.ControlledReleaseDispositionV2{
		Kind: domainauthority.ControlledDispositionKindV2,
	}
	unknownDevelopment := &domainauthority.DevelopmentDispositionV2{Kind: "unknown"}
	tests := []struct {
		name      string
		authority domainauthority.ParsedAuthorityV2
		want      time.Duration
	}{
		{name: "development", authority: development, want: 180 * time.Second},
		{name: "controlled", authority: domainauthority.ParsedAuthorityV2{Controlled: controlledDisposition}, want: 60 * time.Second},
		{name: "empty", authority: domainauthority.ParsedAuthorityV2{}, want: 60 * time.Second},
		{
			name: "ambiguous", authority: domainauthority.ParsedAuthorityV2{
				Development: development.Development, Controlled: controlledDisposition,
			}, want: 60 * time.Second,
		},
		{
			name: "unknown development", authority: domainauthority.ParsedAuthorityV2{
				Development: unknownDevelopment,
			}, want: 60 * time.Second,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := packageAnchorBudgetV2(test.authority); got != test.want {
				t.Fatalf("package anchor budget = %s, want %s", got, test.want)
			}
		})
	}
}

func TestDarwinPackageAnchorRejectsVerificationPastCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	runner := func(ctx context.Context, _ ...string) ([]byte, error) {
		calls++
		select {
		case <-time.After(100 * time.Millisecond):
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	anchor, err := verifyPackageAnchorWithRunnerV2(
		ctx, "/private/tmp/Analytix.app/Contents/Resources", darwinDevelopmentAuthorityV2(), runner,
	)
	if err == nil || anchor != "" || calls != 1 {
		t.Fatalf("over-budget package anchor did not fail closed: anchor=%q calls=%d err=%v", anchor, calls, err)
	}
}

func darwinDevelopmentAuthorityV2() domainauthority.ParsedAuthorityV2 {
	return domainauthority.ParsedAuthorityV2{
		Development: &domainauthority.DevelopmentDispositionV2{
			Kind: domainauthority.DevelopmentDispositionKindV2,
		},
	}
}

func TestDarwinPackageAnchorMetadataHelpers(t *testing.T) {
	body := []byte("Executable=/Applications/Analytix.app/Contents/MacOS/analytix\n" +
		"Identifier=com.analytix.desktop\n" +
		"CodeDirectory v=20500 size=100 flags=0x10000(runtime) hashes=1+0 location=embedded\n" +
		"Authority=Developer ID Application: Analytix (ABCDE12345)\n" +
		"TeamIdentifier=ABCDE12345\n" +
		"Timestamp=Jul 23, 2026 at 12:00:00\n")
	if metadataValueV2(body, "TeamIdentifier") != "ABCDE12345" || metadataValueV2(body, "Timestamp") == "" ||
		!hasMetadataPrefixV2(body, "Authority", "Developer ID Application:") || !hasHardenedRuntimeFlagV2(body) {
		t.Fatal("Developer ID metadata was not parsed exactly")
	}
	if hasMetadataPrefixV2(body, "Authority", "Apple Development:") {
		t.Fatal("unrelated authority prefix was accepted")
	}
}

func TestDarwinDeveloperIDRequirementPinsExactTeam(t *testing.T) {
	requirement := developerIDRequirementV2("ABCDE12345")
	if requirement != `identifier "com.analytix.desktop" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "ABCDE12345"` {
		t.Fatalf("unexpected requirement: %q", requirement)
	}
}
