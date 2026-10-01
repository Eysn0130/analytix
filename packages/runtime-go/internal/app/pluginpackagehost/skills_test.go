package pluginpackagehost

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
	materializationport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	hostport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type testSkillReader struct {
	snapshot domainskill.PackageSnapshot
	after    func()
}

func (r *testSkillReader) ReadSkill(context.Context, hostport.Binding) (domainskill.PackageSnapshot, error) {
	if r.after != nil {
		r.after()
	}
	return r.snapshot, nil
}
func skillFixture(t *testing.T) (hostFixture, *testSkillReader) {
	t.Helper()
	f := fixture(t)
	snapshot, err := domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: []byte("---\nname: documents\n---\nCreate a synthetic report.")}})
	if err != nil {
		t.Fatal(err)
	}
	reader := &testSkillReader{snapshot: snapshot}
	registration := f.registration
	registration.SkillReader = reader
	f.host.registrations[registration.Identity.PackageID] = registration
	return f, reader
}

func TestHostedSkillDiscoveryDistinguishesEmptyAndIncomplete(t *testing.T) {
	for _, scenario := range []string{"unset", "disabled", "available", "materialization", "activation", "snapshot", "identity", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			f, reader := skillFixture(t)
			ctx := context.Background()
			if scenario != "unset" {
				f.enable(t)
			}
			wantSkills, wantErrors := 0, 0
			switch scenario {
			case "available":
				wantSkills = 1
			case "disabled":
				if _, err := f.host.SetDesiredState(ctx, f.setRequest(domainplugin.DesiredDisabledV1)); err != nil {
					t.Fatal(err)
				}
			case "materialization":
				f.registration.Materialization.(*fakeMaterialization).err = errors.New("PRIVATE materialization failure")
				wantErrors = 1
			case "activation":
				f.state.readErr = errors.New("PRIVATE activation failure")
				wantErrors = 1
			case "snapshot":
				reader.snapshot = domainskill.PackageSnapshot{}
				wantErrors = 1
			case "identity":
				f.identity.invalid = true
				f.state.afterRead = func() { t.Fatal("discovery read state before principal validation") }
				wantErrors = 1
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
				wantErrors = 1
			}
			got := f.host.DiscoverSkills(ctx)
			if len(got.Skills) != wantSkills || got.ValidationErrorCount != wantErrors {
				t.Fatalf("skills=%d errors=%d; want %d/%d", len(got.Skills), got.ValidationErrorCount, wantSkills, wantErrors)
			}
		})
	}
}

func TestHostedSkillDiscoveryRetainsVerifiedSiblingAndClearsRecoveredError(t *testing.T) {
	f, reader := skillFixture(t)
	f.enable(t)
	other := fixtureForPackage(t, "analytix-spreadsheets")
	other.enable(t)
	registration := other.registration
	registration.SkillReader = &testSkillReader{snapshot: reader.snapshot}
	f.host.registrations[registration.Identity.PackageID] = registration
	f.host.packageIDs = append(f.host.packageIDs, registration.Identity.PackageID)
	original := reader.snapshot
	reader.snapshot = domainskill.PackageSnapshot{}
	partial := f.host.DiscoverSkills(context.Background())
	if len(partial.Skills) != 1 || partial.Skills[0].Binding.PackageID != registration.Identity.PackageID || partial.ValidationErrorCount != 1 {
		t.Fatal("valid sibling lost or failed contribution treated as empty", partial)
	}
	reader.snapshot = original
	recovered := f.host.DiscoverSkills(context.Background())
	if len(recovered.Skills) != 2 || recovered.ValidationErrorCount != 0 {
		t.Fatal("discovery retained stale failure", recovered)
	}
}

func TestHostedSkillRequiresActivationAndRejectsPreviousRevision(t *testing.T) {
	f, _ := skillFixture(t)
	ctx := context.Background()
	if len(f.host.Skills(ctx)) != 0 {
		t.Fatal("unset skill advertised")
	}
	f.enable(t)
	skills := f.host.Skills(ctx)
	if len(skills) != 1 {
		t.Fatal("enabled skill missing")
	}
	loaded := skills[0]
	called := 0
	consume := func(domainskill.PackageSnapshot) error { called++; return nil }
	if err := f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); err != nil || called != 1 {
		t.Fatal(err)
	}
	if _, err := f.host.SetDesiredState(ctx, f.setRequest(domainplugin.DesiredDisabledV1)); err != nil {
		t.Fatal(err)
	}
	if len(f.host.Skills(ctx)) != 0 {
		t.Fatal("disabled skill advertised")
	}
	if err := f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	f.enable(t)
	if err := f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), consume); !errors.Is(err, ErrConflict) || called != 1 {
		t.Fatal("old activation consumed", err)
	}
}

func TestHostedSkillWithdrawsChangedSnapshotAndIdentity(t *testing.T) {
	f, reader := skillFixture(t)
	ctx := context.Background()
	f.enable(t)
	loaded := f.host.Skills(ctx)[0]
	reader.snapshot, _ = domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: []byte("Changed")}})
	if err := f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(domainskill.PackageSnapshot) error { t.Fatal("changed body executed"); return nil }); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	reader.after = func() { f.identity.invalid = true }
	if len(f.host.Skills(ctx)) != 0 {
		t.Fatal("revoked identity advertised")
	}
}

func TestHostedSkillConsumptionSerializesDisable(t *testing.T) {
	f, _ := skillFixture(t)
	ctx := context.Background()
	f.enable(t)
	loaded := f.host.Skills(ctx)[0]
	entered, release, completed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	request := f.setRequest(domainplugin.DesiredDisabledV1)
	go func() {
		completed <- f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(domainskill.PackageSnapshot) error { close(entered); <-release; return nil })
	}()
	<-entered
	disabled := make(chan error, 1)
	go func() { _, err := f.host.SetDesiredState(ctx, request); disabled <- err }()
	select {
	case err := <-disabled:
		t.Fatal("disable crossed active consumption", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	if err := <-disabled; err != nil {
		t.Fatal(err)
	}
	if len(f.host.Skills(ctx)) != 0 {
		t.Fatal("disable was lost")
	}
}

func TestHostedSkillRejectsRevocationDuringConsumption(t *testing.T) {
	for _, change := range []string{"principal", "principal-replacement", "context"} {
		t.Run(change, func(t *testing.T) {
			f, _ := skillFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.enable(t)
			loaded := f.host.Skills(ctx)[0]
			called := 0
			err := f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(domainskill.PackageSnapshot) error {
				called++
				if change == "principal" {
					f.identity.invalid = true
				} else if change == "principal-replacement" {
					var err error
					f.identity.current, err = identitydomain.NewPrincipalV1(strings.Repeat("2", 64), "local", "local")
					if err != nil {
						t.Fatal(err)
					}
				} else {
					cancel()
				}
				return nil
			})
			if !errors.Is(err, ErrIdentity) || called != 1 {
				t.Fatalf("late authority loss reported success: err=%v calls=%d", err, called)
			}
		})
	}
}

func TestHostedSkillPreservesConsumptionError(t *testing.T) {
	f, _ := skillFixture(t)
	ctx := context.Background()
	f.enable(t)
	loaded := f.host.Skills(ctx)[0]
	want := errors.New("synthetic consumption failure")
	err := f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(domainskill.PackageSnapshot) error {
		f.identity.invalid = true
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal("consumption error was replaced", err)
	}
}

func TestHostedSkillRejectsInstalledStateChangesDuringConsumption(t *testing.T) {
	for _, change := range []string{"snapshot", "generation", "activation"} {
		t.Run(change, func(t *testing.T) {
			f, reader := skillFixture(t)
			ctx := context.Background()
			f.enable(t)
			loaded := f.host.Skills(ctx)[0]
			called := 0
			err := f.host.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(domainskill.PackageSnapshot) error {
				called++
				switch change {
				case "snapshot":
					var err error
					reader.snapshot, err = domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: []byte("Changed while consuming")}})
					if err != nil {
						t.Fatal(err)
					}
				case "generation":
					before := f.state.current.Receipt
					intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{Origin: before.Origin, SourceRegistrationSHA256: before.SourceRegistrationSHA256, Target: before.Target, PluginName: before.PluginName, PluginVersion: before.PluginVersion, SourceRoot: "/private/synthetic/source", SourceTreeSHA256: before.SourceTreeSHA256, SourceTreeFileCount: before.SourceTreeFileCount, ManifestSHA256: before.ManifestSHA256, RequestedAt: f.now})
					if err != nil {
						t.Fatal(err)
					}
					receipt, err := domainplugin.NewReceiptV1(intent, strings.Repeat("e", 64), before.ActiveRelativePath, f.now, f.authority.KeyID(), f.authority.PublicKey(), func(body []byte) ([]byte, error) { return f.authority.Sign(ctx, body) })
					if err != nil {
						t.Fatal(err)
					}
					index, err := domainplugin.NewIndexV1(receipt, f.now)
					if err != nil {
						t.Fatal(err)
					}
					activation, err := domainplugin.NewActivationV1(receipt, 1, domainplugin.DesiredEnabledV1, f.now, f.authority.KeyID(), f.authority.PublicKey(), func(body []byte) ([]byte, error) { return f.authority.Sign(ctx, body) })
					if err != nil {
						t.Fatal(err)
					}
					f.state.current = materializationport.ResultV1{Receipt: receipt, Index: index}
					f.state.activation = activation
				case "activation":
					_, err := f.state.SetDesiredState(ctx, materializationport.SetDesiredStateRequestV1{GenerationID: loaded.Binding.GenerationID, ExpectedRevision: loaded.Binding.ActivationRevision, DesiredState: domainplugin.DesiredDisabledV1}, f.authority, f.now.Add(time.Second))
					if err != nil {
						t.Fatal(err)
					}
				}
				return nil
			})
			want := ErrConflict
			if change == "activation" {
				want = ErrDisabled
			}
			if !errors.Is(err, want) || called != 1 {
				t.Fatalf("late %s change reported success: err=%v calls=%d", change, err, called)
			}
		})
	}
}
