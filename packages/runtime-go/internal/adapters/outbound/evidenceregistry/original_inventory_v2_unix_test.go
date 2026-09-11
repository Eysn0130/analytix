//go:build darwin || linux

package evidenceregistry

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

func TestOriginalRegistryV2RawObservationPreservesNativeLinkedWriteCut(t *testing.T) {
	ctx := context.Background()
	root, access := seedRecoveryV2IdentityLineage(t, []uint64{1})
	plan, err := PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	var digest string
	if err := plan.indexes.VisitCommittedFiles(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		digest = file.Digest
		return nil
	}); err != nil || digest == "" {
		t.Fatalf("fixture lacks committed index: %v", err)
	}
	committed := filepath.Join(root, "indexes", digest[:2], digest+".json")
	temporary := filepath.Join(filepath.Dir(committed), "."+digest+".json-native-cut.tmp")
	if err := os.Link(committed, temporary); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(temporary)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		t.Fatalf("native physical observation rejected exact producer pair: %v", err)
	}
	if err := plan.indexes.VisitCommittedFiles(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return nil }); err != nil {
		t.Fatalf("native committed observation rejected exact producer pair: %v", err)
	}
	files, err := plan.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		t.Fatalf("raw original reader rejected native authenticated linked cut: %v", err)
	}
	base := "indexes/" + digest[:2] + "/"
	if !reflect.DeepEqual(files[base+digest+".json"], files[base+filepath.Base(temporary)]) {
		t.Fatal("raw original inventory lost one member of exact linked pair")
	}
	after, err := os.Stat(temporary)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("original linked observation changed producer residue: %v", err)
	}
}

func TestOriginalRegistryV2RawObservationRetainsOpaqueResidueAndRejectsUnpairedLinkDrift(t *testing.T) {
	ctx := context.Background()
	root, access := seedRecoveryV2IdentityLineage(t, []uint64{1})
	plan, err := PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	var digest string
	if err := plan.indexes.VisitCommittedFiles(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		digest = file.Digest
		return nil
	}); err != nil || digest == "" {
		t.Fatalf("fixture lacks original index: %v", err)
	}
	committed := filepath.Join(root, "indexes", digest[:2], digest+".json")
	residue := filepath.Join(filepath.Dir(committed), "."+digest+".json-opaque-cut.tmp")
	opaque := []byte("synthetic incomplete write")
	if err := os.WriteFile(residue, opaque, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err = PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	files, err := plan.SnapshotOriginalFilesV1(ctx)
	if err != nil || string(files["indexes/"+digest[:2]+"/"+filepath.Base(residue)].Body) != string(opaque) {
		t.Fatalf("opaque original bytes were consumed or omitted: %v", err)
	}
	if err := os.Link(committed, filepath.Join(t.TempDir(), "unpaired-external-link")); err != nil {
		t.Fatal(err)
	}
	if files, err := plan.SnapshotOriginalFilesV1(ctx); err == nil || files != nil {
		t.Fatal("late unpaired hardlink was accepted or returned a partial original inventory")
	}
	if _, err := PrepareRecoveryV2(ctx, root, access); err == nil {
		t.Fatal("fresh native original observation adopted an unpaired hardlink")
	}
}
