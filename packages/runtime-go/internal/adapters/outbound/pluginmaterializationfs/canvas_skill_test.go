package pluginmaterializationfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

type canvasSkillIdentity struct {
	principal identitydomain.PrincipalV1
	invalid   bool
}

func (i *canvasSkillIdentity) ResolveCurrent(context.Context) (identitydomain.PrincipalV1, error) {
	return i.principal, nil
}
func (i *canvasSkillIdentity) ValidateCurrent(_ context.Context, p identitydomain.PrincipalV1) error {
	if i.invalid || !identitydomain.SamePrincipalV1(i.principal, p) {
		return identityport.ErrMismatch
	}
	return nil
}

func TestCanvasInstalledSkillHostRevokesDisableUpgradeAndTamper(t *testing.T) {
	ctx := context.Background()
	source, instructions := writeCanvasSkillSourceV1(t)
	home := realTempDir(t)
	store, err := NewPackageStoreV1(home, "analytix-canvas", nil)
	if err != nil {
		t.Fatal(err)
	}
	authority := newTestAuthority()
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	principal, err := identitydomain.NewPrincipalV1(strings.Repeat("1", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	identity := &canvasSkillIdentity{principal: principal}
	compose := func(st *Store, at time.Time) (*hostapp.Service, pluginapp.DevelopmentSourceBindingV1, *pluginapp.DevelopmentSourceServiceV1) {
		t.Helper()
		observed, err := InspectDevelopmentSourceTreeV1(ctx, source)
		if err != nil {
			t.Fatal(err)
		}
		registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
		if err != nil {
			t.Fatal(err)
		}
		binding := developmentBindingV1(t, source)
		service, err := pluginapp.NewDevelopmentSourceServiceV1(st, authority, binding, func() time.Time { return at })
		if err != nil {
			t.Fatal(err)
		}
		host, err := hostapp.New(identity, authority, []hostapp.Registration{{Identity: registration.Identity, SourceRegistrationSHA256: observed.SourceRegistrationSHA256, Materialization: service, State: st, SkillReader: StaticEditorSkillReader{Store: st, Authority: authority, SHA256: registration.CanvasSkillSHA256}}}, func() time.Time { return at })
		if err != nil {
			t.Fatal(err)
		}
		return host, binding, service
	}
	host, binding, service := compose(store, now)
	intent, err := binding.NewIntentV1(now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Materialize(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(host.Skills(ctx)) != 0 {
		t.Fatal("unset skill exposed")
	}
	set := func(h *hostapp.Service, generation string, revision uint64, state domainplugin.DesiredStateV1) uint64 {
		t.Helper()
		view, err := h.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: "analytix-canvas", GenerationID: generation, ExpectedRevision: revision, DesiredState: state})
		if err != nil {
			t.Fatal(err)
		}
		return view.ActivationRevision
	}
	revision := set(host, first.Receipt.GenerationID, 0, domainplugin.DesiredEnabledV1)
	skills := host.Skills(ctx)
	if len(skills) != 1 {
		t.Fatal("enabled Canvas absent")
	}
	loaded := skills[0]
	consumed := 0
	consume := func(snapshot domainskill.PackageSnapshot) error {
		body, ok := snapshot.File("SKILL.md")
		if !ok || !bytes.Equal(body, instructions) {
			t.Fatal("wrong installed bytes")
		}
		consumed++
		return nil
	}
	if err := host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); err != nil || consumed != 1 {
		t.Fatal(err)
	}
	// Restart recovers the signed activation and installed body.
	reopened, err := OpenExistingPackageStoreV1(home, "analytix-canvas")
	if err != nil {
		t.Fatal(err)
	}
	restarted, _, _ := compose(reopened, now)
	if got := restarted.Skills(ctx); len(got) != 1 || got[0].Binding != loaded.Binding || got[0].Snapshot.Digest() != loaded.Snapshot.Digest() {
		t.Fatal("restart changed skill authority")
	}
	revision = set(restarted, first.Receipt.GenerationID, revision, domainplugin.DesiredDisabledV1)
	if len(host.Skills(ctx)) != 0 {
		t.Fatal("disabled installed skill exposed")
	}
	if err := host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); !errors.Is(err, hostapp.ErrDisabled) || consumed != 1 {
		t.Fatal("disable failed to revoke", err)
	}
	revision = set(restarted, first.Receipt.GenerationID, revision, domainplugin.DesiredEnabledV1)
	if err := host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); !errors.Is(err, hostapp.ErrConflict) || consumed != 1 {
		t.Fatal("old activation regained authority", err)
	}
	loaded = host.Skills(ctx)[0]
	identity.invalid = true
	if len(host.Skills(ctx)) != 0 {
		t.Fatal("revoked principal retained skill")
	}
	identity.invalid = false
	// Source changes alone must never become runtime instruction bytes.
	changed := append(append([]byte{}, instructions...), []byte("\nRevised synthetic Canvas instructions.\n")...)
	if err := os.WriteFile(filepath.Join(source, domainpackage.CanvasSkillRelativePathV1), changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); err != nil || consumed != 2 {
		t.Fatal("raw source became runtime authority", err)
	}
	if _, err := service.Materialize(ctx, intent); err == nil {
		t.Fatal("changed source retained frozen registration")
	}
	upgraded, nextBinding, nextService := compose(store, now.Add(time.Second))
	nextIntent, err := nextBinding.NewIntentV1(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	next, err := nextService.Materialize(ctx, nextIntent)
	if err != nil {
		t.Fatal(err)
	}
	if next.Receipt.GenerationID == first.Receipt.GenerationID {
		t.Fatal("upgrade did not rotate generation")
	}
	if len(host.Skills(ctx)) != 0 || len(upgraded.Skills(ctx)) != 0 {
		t.Fatal("upgrade inherited activation")
	}
	if err := host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); err == nil || consumed != 2 {
		t.Fatal("old host consumed upgraded bytes")
	}
	set(upgraded, next.Receipt.GenerationID, 0, domainplugin.DesiredEnabledV1)
	nextLoaded := upgraded.Skills(ctx)
	if len(nextLoaded) != 1 || nextLoaded[0].Snapshot.Digest() == loaded.Snapshot.Digest() {
		t.Fatal("new enabled snapshot missing")
	}
	if err := upgraded.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); err == nil || consumed != 2 {
		t.Fatal("old binding executed on upgraded host")
	}
	installed := filepath.Join(store.absolute(next.Receipt.ActiveRelativePath), domainpackage.CanvasSkillRelativePathV1)
	if err := os.WriteFile(installed, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(upgraded.Skills(ctx)) != 0 {
		t.Fatal("tampered installed snapshot retained authority")
	}
	if err := upgraded.WithSkill(ctx, nextLoaded[0].Binding, nextLoaded[0].Snapshot.Digest(), consume); err == nil || consumed != 2 {
		t.Fatal("tampered bytes executed")
	}
}
