package pluginmaterialization

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

func TestDevelopmentSourceServiceRejectsUnvalidatedBindingAndFormalEntryV1(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	seed := sha256.Sum256([]byte("development-service-test"))
	authority := serviceTestAuthority{private: ed25519.NewKeyFromSeed(seed[:])}
	store := &serviceTestStore{}
	if _, err := NewDevelopmentSourceServiceV1(store, authority, DevelopmentSourceBindingV1{}, func() time.Time { return now }); err == nil {
		t.Fatal("zero source binding accepted")
	}
	if _, err := NewDevelopmentSourceBindingV1(domainpackage.DevelopmentSourceRegistrationV1{}, "/source", domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}); err == nil {
		t.Fatal("unvalidated registration accepted")
	}
	digest := strings.Repeat("a", 64)
	intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{Origin: domainplugin.DevelopmentSourceOriginV1, SourceRegistrationSHA256: digest, Target: domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}, PluginName: "analytix-documents", PluginVersion: "1.0.0", SourceRoot: "/source", SourceTreeSHA256: strings.Repeat("b", 64), SourceTreeFileCount: 4, ManifestSHA256: strings.Repeat("c", 64), RequestedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	formal, err := NewService(store, authority, FormalPackageBindingV1{AuthoritySHA256: digest, Target: intent.Target, PackageIdentity: domainpackage.PackageIdentityV1{PackageID: intent.PluginName, PackageVersion: intent.PluginVersion}, DeclarationRawSHA256: digest, DeclarationCanonicalSHA256: digest, SourceTreeSHA256: intent.SourceTreeSHA256, SourceTreeFileCount: intent.SourceTreeFileCount, ManifestSHA256: intent.ManifestSHA256, EntrypointSHA256: digest}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := formal.Materialize(context.Background(), intent); !errors.Is(err, ErrPackageAuthority) || store.calls != 0 {
		t.Fatal("source intent reached formal store", err)
	}
}
