//go:build darwin || linux || windows

package finalauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func originalCreateResidueFixtureV1(t *testing.T, complete bool) (string, []string, *PreparedSecurePrivateCASOriginalCreateResiduesV1) {
	t.Helper()
	data := t.TempDir()
	owner := filepath.Join(data, "private", "attachment-authority")
	if err := os.MkdirAll(filepath.Dir(owner), 0o700); err != nil {
		t.Fatal(err)
	}
	residues := []string{filepath.Join(filepath.Dir(owner), domainprivatecas.CreateDirectoryResidueNameV1("attachment-authority"))}
	if complete {
		for _, leaf := range []string{"owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"} {
			if err := os.Mkdir(filepath.Join(owner, leaf), 0o700); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Join(owner, leaf), 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		}
		residues = append(residues, filepath.Join(owner, domainprivatecas.CreateDirectoryResidueNameV1("owners")), filepath.Join(owner, "owners", domainprivatecas.CreateDirectoryResidueNameV1("ab")))
	}
	for _, residue := range residues {
		if err := os.Mkdir(residue, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Join(data, "private"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(context.Background(), data, access)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := prepared.OriginalCreateResiduesV1(context.Background(), "private/attachment-authority")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.OriginalCreateResiduesV1(context.Background(), "private/unregistered-owner"); err == nil {
		t.Fatal("unknown catalog owner acquired original residue proof")
	}
	return data, residues, proof
}

func TestPrivateCASOriginalCreateOwnerSetPreservesOnlySelectedCatalogRoots(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	private := filepath.Join(data, "private")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	owners := []string{"attachment-authority", "pii-authorization", "report-publication"}
	states := map[string]os.FileInfo{}
	for _, owner := range owners {
		residue := filepath.Join(private, domainprivatecas.CreateDirectoryResidueNameV1(owner))
		if err := os.Mkdir(residue, 0o700); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(residue)
		if err != nil {
			t.Fatal(err)
		}
		states[owner] = info
	}
	access, err := privatecastest.NewAccessAuthority(private)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, data, access)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := plan.OriginalCreateResiduesForOwnersV1(ctx, []string{"private/pii-authorization", "private/attachment-authority"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.OwnerRootsV1()) != 2 || proof.OwnerRootV1() != "" || len(proof.RelativePathsV1()) != 2 {
		t.Fatal("set invented one root or lost the exact owner denominator")
	}
	if _, err := proof.ProjectOwnerV1(ctx, "private/report-publication"); err == nil {
		t.Fatal("set acquired an unselected owner")
	}
	if _, err := plan.OriginalCreateResiduesForOwnersV1(ctx, []string{"private/pii-authorization", "private/pii-authorization"}); err == nil {
		t.Fatal("set accepted duplicate owners")
	}
	if err := plan.ApplyPreservingOriginalCreateResiduesV1(ctx, proof, proof.Revalidate); err != nil {
		t.Fatal(err)
	}
	selected, err := proof.SelectOwnersV1(ctx, []string{"private/pii-authorization"})
	if err != nil || len(selected.RelativePathsV1()) != 1 || selected.OwnerRootV1() != filepath.Join(private, "pii-authorization") {
		t.Fatalf("selection after independent cleanup lost its frozen origin: %v", err)
	}
	for _, invalid := range [][]string{nil, {"private/report-publication"}, {"private/pii-authorization", "private/pii-authorization"}} {
		if _, err := proof.SelectOwnersV1(ctx, invalid); err == nil {
			t.Fatal("selection accepted empty, unobserved or duplicate owners")
		}
	}
	for _, owner := range owners {
		residue := filepath.Join(private, domainprivatecas.CreateDirectoryResidueNameV1(owner))
		info, err := os.Stat(residue)
		if owner == "report-publication" {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatal("independent owner was not recovered")
			}
			continue
		}
		if err != nil || !os.SameFile(states[owner], info) || states[owner].Mode() != info.Mode() {
			t.Fatal("selected original creation identity changed")
		}
	}
	if err := proof.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
}

type originalCreateDeniedWriteAuthorityV1 struct {
	SecurePrivateCASRecoveryAccessAuthority
	cause error
}

func (authority originalCreateDeniedWriteAuthorityV1) WithPrivateCASAccess(context.Context, string, func(privatecasport.RootBinding) error) error {
	return authority.cause
}

func TestPrivateCASOriginalCreateResidueLeafUsesExactAccessAuthority(t *testing.T) {
	for _, denyWrite := range []bool{false, true} {
		ctx := context.Background()
		data, residues, proof := originalCreateResidueFixtureV1(t, true)
		root := filepath.Join(data, "private", "attachment-authority", "owners")
		access := proof.prepared.access
		cause := errors.New("synthetic exact leaf write denied")
		if denyWrite {
			access = originalCreateDeniedWriteAuthorityV1{SecurePrivateCASRecoveryAccessAuthority: access, cause: cause}
		}
		if _, err := PrepareSecurePrivateCASRecoveryIfPresent(ctx, root, 4096, access); err == nil {
			t.Fatal("ordinary recovery borrowed original creation residue permission")
		}
		prepared, err := PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx, root, 4096, access, proof)
		if err != nil {
			t.Fatal(err)
		}
		store, err := prepared.OpenPreservingOriginalResiduesV1(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		before, err := os.Stat(residues[2])
		if err != nil {
			t.Fatal(err)
		}
		digest := strings.Repeat("c", 64)
		body := []byte(`{"independent":true}`)
		token, err := store.PutIfAbsentWithAdditionReceipt(ctx, digest, body)
		if denyWrite {
			if !errors.Is(err, cause) {
				t.Fatalf("parent observation bypassed exact leaf write refusal: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "cc")); !os.IsNotExist(err) {
				t.Fatalf("denied exact leaf write created topology: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		finalized, err := store.FinalizeCommittedAdditions(ctx, []SecurePrivateCASAdditionReceiptV2{token})
		if err != nil || len(finalized) != 1 {
			t.Fatalf("original creation residue blocked independent receipt finalization: %v", err)
		}
		if err := store.VerifyCommittedAddition(ctx, finalized[0]); err != nil {
			t.Fatal(err)
		}
		if got, err := store.Read(ctx, digest); err != nil || string(got) != string(body) {
			t.Fatalf("original creation residue blocked committed read: %v", err)
		}
		if files, err := store.List(ctx); err != nil || len(files) != 1 {
			t.Fatalf("original creation residue became a committed record: %v", err)
		}
		visited := 0
		if err := store.Visit(ctx, func(SecurePrivateCASFile) error { visited++; return nil }); err != nil || visited != 1 {
			t.Fatalf("original creation residue visitor lost complete inventory: %v", err)
		}
		after, err := os.Stat(residues[2])
		if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
			t.Fatalf("independent CAS addition changed original creation residue: %v", err)
		}
		if err := proof.Revalidate(ctx); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(residues[2]); err != nil {
			t.Fatal(err)
		}
		if files, err := store.List(ctx); err == nil || files != nil {
			t.Fatalf("live generation accepted missing original creation residue: %v", err)
		}
		called := false
		if err := store.Visit(ctx, func(SecurePrivateCASFile) error { called = true; return nil }); err == nil || called {
			t.Fatalf("live visitor crossed original directory drift: %v", err)
		}
	}
}

func TestPrivateCASOriginalCreateResidueAllowsIndependentTargetShard(t *testing.T) {
	ctx := context.Background()
	data, residues, proof := originalCreateResidueFixtureV1(t, true)
	root := filepath.Join(data, "private", "attachment-authority", "owners")
	prepared, err := PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx, root, 4096, proof.prepared.access, proof)
	if err != nil {
		t.Fatal(err)
	}
	store, err := prepared.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	before, err := os.Stat(residues[2])
	if err != nil {
		t.Fatal(err)
	}
	digest := "ab" + strings.Repeat("c", 62)
	body := []byte(`{"independentTargetShard":true}`)
	token, err := store.PutIfAbsentWithAdditionReceipt(ctx, digest, body)
	if err != nil {
		t.Fatalf("original creation residue blocked its independent target shard: %v", err)
	}
	if !token.ShardCreatedByThisCall || token.ShardExistedBeforeCommit {
		t.Fatal("independent target shard has no exact current-call creation provenance")
	}
	finalized, err := store.FinalizeCommittedAdditions(ctx, []SecurePrivateCASAdditionReceiptV2{token})
	if err != nil || len(finalized) != 1 {
		t.Fatalf("independent target shard receipt finalization: %v", err)
	}
	if err := store.VerifyCommittedAddition(ctx, finalized[0]); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(residues[2])
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("target shard write consumed or changed original creation residue: %v", err)
	}
	if got, err := store.Read(ctx, digest); err != nil || string(got) != string(body) {
		t.Fatalf("independent target shard readback: %v", err)
	}
	if err := proof.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateCASOriginalCreateResidueResumesEmptyIndependentShard(t *testing.T) {
	ctx := context.Background()
	data, residues, proof := originalCreateResidueFixtureV1(t, true)
	root := filepath.Join(data, "private", "attachment-authority", "owners")
	access := proof.prepared.access
	prepared, err := PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx, root, 4096, access, proof)
	if err != nil {
		t.Fatal(err)
	}
	store, err := prepared.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(residues[2])
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("synthetic interruption after canonical shard creation")
	store.generation.beforeOriginalRecordStage = func() error { return cause }
	digest := "ab" + strings.Repeat("c", 62)
	body := []byte(`{"afterInterruption":true}`)
	if err := store.PutIfAbsent(ctx, digest, body); !errors.Is(err, cause) {
		t.Fatalf("record-stage interruption lost its cause: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "ab")); err != nil || !info.IsDir() {
		t.Fatalf("fixture did not reach canonical empty shard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "ab", digest+".json")); !os.IsNotExist(err) {
		t.Fatalf("interrupted record stage created a committed record: %v", err)
	}
	physical, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, data, access)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := physical.OriginalCreateResiduesV1(ctx, "private/attachment-authority")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err = PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx, root, 4096, access, fresh)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := prepared.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	token, err := resumed.PutIfAbsentWithAdditionReceipt(ctx, digest, body)
	if err != nil {
		t.Fatal(err)
	}
	if token.ShardCreatedByThisCall || !token.ShardExistedBeforeCommit {
		t.Fatal("resumed call claimed creation of the previously empty shard")
	}
	finalized, err := resumed.FinalizeCommittedAdditions(ctx, []SecurePrivateCASAdditionReceiptV2{token})
	if err != nil || len(finalized) != 1 {
		t.Fatalf("resumed addition finalization: %v", err)
	}
	if err := resumed.VerifyCommittedAddition(ctx, finalized[0]); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(residues[2])
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("interruption or resume changed the original creation residue: %v", err)
	}
}

func TestPrivateCASOriginalCreateResidueRejectsUnpreparedTargetShard(t *testing.T) {
	ctx := context.Background()
	data, _, proof := originalCreateResidueFixtureV1(t, true)
	root := filepath.Join(data, "private", "attachment-authority", "owners")
	prepared, err := PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx, root, 4096, proof.prepared.access, proof)
	if err != nil {
		t.Fatal(err)
	}
	store, err := prepared.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := os.Mkdir(filepath.Join(root, "ab"), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := "ab" + strings.Repeat("c", 62)
	if _, err := store.PutIfAbsentWithAdditionReceipt(ctx, digest, []byte(`{"forbidden":true}`)); err == nil {
		t.Fatal("unprepared canonical target acquired creation provenance")
	}
	if _, err := os.Stat(filepath.Join(root, "ab", digest+".json")); !os.IsNotExist(err) {
		t.Fatalf("unprepared canonical target was written: %v", err)
	}
}

func TestPrivateCASOriginalCreateResidueProofBindsExactOwnerSet(t *testing.T) {
	for _, complete := range []bool{false, true} {
		data, residues, proof := originalCreateResidueFixtureV1(t, complete)
		paths := proof.RelativePathsV1()
		if len(paths) != len(residues) || proof.DataRootV1() != data {
			t.Fatal("original proof omitted a catalog-bound residue")
		}
		for _, residue := range residues {
			relative, err := filepath.Rel(data, residue)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, name := range paths {
				found = found || name == filepath.ToSlash(relative)
			}
			if !found {
				t.Fatal("original proof omitted physical residue name")
			}
		}
		paths[0] = "caller-mutated"
		if reflect.DeepEqual(paths, proof.RelativePathsV1()) {
			t.Fatal("caller modified original residue membership")
		}
		independent := filepath.Join(data, "private", domainprivatecas.CreateDirectoryResidueNameV1("case-thread-authority"))
		if err := os.Mkdir(independent, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := proof.Revalidate(context.Background()); err != nil {
			t.Fatalf("independent original-safe topology blocked proof: %v", err)
		}
		if err := os.Remove(independent); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(data, "private", "attachment-authority", "owners", "cd"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := proof.Revalidate(context.Background()); err != nil {
			t.Fatalf("canonical parent growth changed original residue proof: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := proof.Revalidate(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("original proof lost cancellation: %v", err)
		}
	}
}

func TestPrivateCASOriginalCreateResidueProofRejectsDrift(t *testing.T) {
	for _, kind := range []string{"removed", "replacement", "nonempty", "additional", "mode", "ancestor replacement"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && kind == "mode" {
				t.Skip("Windows validates native DACL rather than Unix mode")
			}
			data, residues, proof := originalCreateResidueFixtureV1(t, true)
			var err error
			switch kind {
			case "removed":
				err = os.Remove(residues[0])
			case "replacement":
				if err = os.Rename(residues[0], filepath.Join(data, "saved-original-residue")); err == nil {
					err = os.Mkdir(residues[0], 0o700)
				}
			case "nonempty":
				err = os.WriteFile(filepath.Join(residues[1], "unexpected"), []byte("unobserved"), 0o600)
			case "additional":
				err = os.Mkdir(filepath.Join(data, "private", "attachment-authority", "owners", domainprivatecas.CreateDirectoryResidueNameV1("ef")), 0o700)
			case "mode":
				err = os.Chmod(residues[1], 0o500)
			case "ancestor replacement":
				parent := filepath.Join(data, "private", "attachment-authority", "use-receipts")
				if err = os.Rename(parent, filepath.Join(data, "saved-original-parent")); err == nil {
					err = os.Mkdir(parent, 0o700)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := proof.Revalidate(context.Background()); err == nil {
				t.Fatal("original directory residue proof adopted drift")
			}
		})
	}
}

func TestOriginalCreateOrphanRecoveryRetainsAncestorsOfPartialOwner(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	leaf := filepath.Join(data, "private", "attachment-authority", "owners")
	residue := filepath.Join(leaf, domainprivatecas.CreateDirectoryResidueNameV1("aa"))
	independent := filepath.Join(leaf, "bb")
	for _, name := range []string{residue, independent} {
		if err := os.MkdirAll(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Join(data, "private"))
	if err != nil {
		t.Fatal(err)
	}
	creates, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, data, access)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := creates.OriginalCreateResiduesV1(ctx, "private/attachment-authority")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(residue)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := PrepareSecurePrivateCASOrphanTopologyWithOriginalCreateResiduesV1(ctx, data, access, proof)
	if err != nil {
		t.Fatal(err)
	}
	if err := orphan.Apply(ctx); err == nil {
		t.Fatal("unguarded original orphan plan acquired cleanup authority")
	}
	if err := orphan.ApplyPreservingOriginalDirectoriesV1(ctx, nil, proof.Revalidate); err != nil {
		t.Fatalf("independent shard cleanup beside original residue failed: %v", err)
	}
	if _, err := os.Stat(independent); !os.IsNotExist(err) {
		t.Fatalf("independent empty shard was not removed: %v", err)
	}
	now, err := os.Stat(residue)
	if err != nil || !os.SameFile(original, now) || original.Mode() != now.Mode() || !original.ModTime().Equal(now.ModTime()) {
		t.Fatalf("partial owner original residue changed: %v", err)
	}
	if err := proof.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
}
