//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

func TestRuntimeOriginalRegistryTrustUsesIndependentManifestWithoutCredentials(t *testing.T) {
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	ctx := context.Background()
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	config.AuthorityCredentialProfileRoot = filepath.Join(t.TempDir(), "absent-profile")
	config.AuthorityCredentialBundleRoot = filepath.Join(t.TempDir(), "absent-bundle")
	attempts := fixture.TotalAttempts()
	trust, err := prepareRuntimeOriginalRegistryTrustV2(ctx, config, authority)
	if err != nil || trust == nil || trust.projection.InstallationID == "" || trust.projection.Enrollment.EnrollmentID == "" || len(trust.witnessKey) == 0 {
		t.Fatalf("original trust required a credential or omitted its independently anchored identity: %v", err)
	}
	if err := trust.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadRuntimeSharedEvidenceEnrollmentV2(ctx, config, authority); err == nil {
		t.Error("live enrollment loader accepted absent credential configuration")
	}
	if fixture.TotalAttempts() != attempts {
		t.Error("original trust acquired fresh witness authority")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := trust.Revalidate(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("original trust lost cancellation: %v", err)
	}
	other, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "other-authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareRuntimeOriginalRegistryTrustV2(ctx, config, other); err == nil {
		t.Error("original trust accepted a different current installation key")
	}
	manifest := filepath.Join(config.AuthorityManifestRoot, runtimeAuthorityManifestFileNameV2)
	before, err := os.Stat(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(manifest, before.Mode().Perm()) })
	if err := trust.Revalidate(ctx); err == nil {
		t.Error("original trust accepted changed protected manifest permissions")
	}
}
