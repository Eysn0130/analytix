package pluginpackagehost

import (
	"context"
	"errors"
	"testing"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
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
