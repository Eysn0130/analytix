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
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const fixturePluginVersionV1 = "0.16.16"

type serviceTestStore struct{ calls int }

func (store *serviceTestStore) Materialize(context.Context, domainplugin.IntentV1, pluginport.InstallationAuthority, time.Time) (pluginport.ResultV1, error) {
	store.calls++
	return pluginport.ResultV1{}, errors.New("stop after admission")
}
func (store *serviceTestStore) ResolveActive(context.Context, pluginport.InstallationAuthority) (pluginport.ResultV1, error) {
	return pluginport.ResultV1{}, pluginport.ErrUnavailable
}

type serviceTestAuthority struct{ private ed25519.PrivateKey }

func (authority serviceTestAuthority) KeyID() string {
	value := sha256.Sum256(authority.PublicKey())
	return strings.ToLower(strings.TrimSpace(strings.Repeat("0", 0) + fmtHex(value[:])))
}
func (authority serviceTestAuthority) PublicKey() []byte {
	return authority.private.Public().(ed25519.PublicKey)
}
func (authority serviceTestAuthority) Sign(_ context.Context, body []byte) ([]byte, error) {
	return ed25519.Sign(authority.private, body), nil
}

func TestServiceRejectsIntentOutsideFrozenFormalPackageBinding(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	digest := strings.Repeat("a", 64)
	intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		PackageAuthoritySHA256: digest, Target: domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"},
		PluginName: domainplugin.PluginNameV1, PluginVersion: fixturePluginVersionV1,
		SourceRoot: "/formal/plugin", SourceTreeSHA256: strings.Repeat("1", 64), SourceTreeFileCount: 3,
		ManifestSHA256: strings.Repeat("2", 64), EntrypointSHA256: strings.Repeat("3", 64), RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256([]byte("service-authority"))
	store := &serviceTestStore{}
	authority := serviceTestAuthority{private: ed25519.NewKeyFromSeed(seed[:])}
	binding := FormalPackageBindingV1{
		AuthoritySHA256: digest, Target: intent.Target,
		PackageIdentity:      domainpluginpackage.PackageIdentityV1{PackageID: intent.PluginName, PackageVersion: intent.PluginVersion},
		DeclarationRawSHA256: strings.Repeat("b", 64), DeclarationCanonicalSHA256: strings.Repeat("c", 64),
		SourceTreeSHA256:    intent.SourceTreeSHA256,
		SourceTreeFileCount: intent.SourceTreeFileCount, ManifestSHA256: intent.ManifestSHA256, EntrypointSHA256: intent.EntrypointSHA256,
	}
	service, err := NewService(store, authority, binding, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	readOnlyBinding := binding
	readOnlyBinding.PackageIdentity = domainpluginpackage.PackageIdentityV1{}
	readOnlyBinding.DeclarationRawSHA256 = ""
	readOnlyBinding.DeclarationCanonicalSHA256 = ""
	readOnlyService, err := NewService(store, authority, readOnlyBinding, func() time.Time { return now })
	if err != nil {
		t.Fatalf("signed-generation reader binding was rejected: %v", err)
	}
	if _, err := readOnlyService.Materialize(context.Background(), intent); !errors.Is(err, ErrPackageAuthority) || store.calls != 0 {
		t.Fatalf("identity-unbound reader minted materialization: calls=%d err=%v", store.calls, err)
	}
	changed := intent
	changed.SourceTreeSHA256 = strings.Repeat("4", 64)
	changed.IntentID = ""
	changed, err = domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		PackageAuthoritySHA256: changed.PackageAuthoritySHA256, Target: changed.Target,
		PluginName: changed.PluginName, PluginVersion: changed.PluginVersion, SourceRoot: changed.SourceRoot,
		SourceTreeSHA256: changed.SourceTreeSHA256, SourceTreeFileCount: changed.SourceTreeFileCount,
		ManifestSHA256: changed.ManifestSHA256, EntrypointSHA256: changed.EntrypointSHA256, RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Materialize(context.Background(), changed); !errors.Is(err, ErrPackageAuthority) {
		t.Fatalf("changed source escaped formal package binding: %v", err)
	}
	if store.calls != 0 {
		t.Fatal("rejected package reached materialization store")
	}
	if _, err := service.Materialize(context.Background(), intent); err == nil || store.calls != 1 {
		t.Fatalf("exact package was not admitted to store: calls=%d err=%v", store.calls, err)
	}
}

func fmtHex(body []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(body)*2)
	for index, value := range body {
		out[index*2] = alphabet[value>>4]
		out[index*2+1] = alphabet[value&15]
	}
	return string(out)
}
