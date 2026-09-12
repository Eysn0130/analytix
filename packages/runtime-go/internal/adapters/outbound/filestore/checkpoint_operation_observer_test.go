package filestore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
)

func TestCheckpointOperationObserverCapturesExactEncodedBytes(t *testing.T) {
	for _, encoding := range []string{
		filetoolsapp.TextEncodingUTF8,
		filetoolsapp.TextEncodingUTF8BOM,
		filetoolsapp.TextEncodingUTF16LE,
		filetoolsapp.TextEncodingUTF16BE,
		filetoolsapp.TextEncodingUTF16LENoBOM,
		filetoolsapp.TextEncodingUTF16BENoBOM,
	} {
		t.Run(encoding, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "evidence.txt")
			raw := filetoolsapp.EncodeTextBytes("账户 6222020200000000000\n", encoding)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			observer := CheckpointOperationObserver{}
			before, err := observer.CaptureBefore(context.Background(), workspace, path)
			if err != nil {
				t.Fatal(err)
			}
			if !before.Existed || !before.ContentAvailable || before.Encoding != encoding ||
				before.Hash != checkpointapp.HashBytes(raw) || !bytes.Equal(before.RawBytes, raw) ||
				before.PathAuthority.SchemaVersion != 1 || before.PathAuthority.Kind != "workspace" ||
				before.PathAuthority.RootIdentity == "" || before.PathAuthority.RelativePath != "evidence.txt" {
				t.Fatalf("exact encoded snapshot mismatch: %#v", before)
			}

			afterRaw := filetoolsapp.EncodeTextBytes("账户 6222020200000000001\n", encoding)
			if err := os.WriteFile(path, afterRaw, 0o600); err != nil {
				t.Fatal(err)
			}
			observed := observer.Observe(context.Background(), workspace, before.PathAuthority, path)
			if observed.ObservationStatus != "exact" || !observed.Existed || observed.Hash != checkpointapp.HashBytes(afterRaw) {
				t.Fatalf("encoded after observation mismatch: %#v", observed)
			}
		})
	}
}

func TestCheckpointOperationObserverBindsMissingAllowWriteTargetAndRejectsStaleRoot(t *testing.T) {
	workspace := t.TempDir()
	allowRoot := t.TempDir()
	target := filepath.Join(allowRoot, "new", "nested", "evidence.txt")
	observer := CheckpointOperationObserver{AllowWriteRoots: []string{allowRoot}}
	before, err := observer.CaptureBefore(context.Background(), workspace, target)
	if err != nil {
		t.Fatalf("missing allow_write target should bind before parent creation: %v", err)
	}
	if before.Existed || before.PathAuthority.Kind != "allow_write" ||
		before.PathAuthority.RelativePath != "new/nested/evidence.txt" || before.PathAuthority.Root == workspace {
		t.Fatalf("allow_write path authority mismatch: %#v", before.PathAuthority)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("created")
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	observed := observer.ObserveRelative(context.Background(), workspace, before.PathAuthority)
	if observed.ObservationStatus != "exact" || observed.Hash != checkpointapp.HashBytes(raw) {
		t.Fatalf("authorized external target observation mismatch: %#v", observed)
	}

	stale := (CheckpointOperationObserver{}).ObserveRelative(context.Background(), workspace, before.PathAuthority)
	if stale.ObservationStatus != "unavailable" || stale.BlockerCode != "path_unsafe" || stale.Existed || stale.Hash != "" {
		t.Fatalf("removed allow_write root did not fail closed: %#v", stale)
	}
	intent := domaincheckpoint.OperationGroupIntentV2{Paths: []domaincheckpoint.OperationPathV2{{
		PathAuthoritySchemaVersion: before.PathAuthority.SchemaVersion,
		AuthorityKind:              before.PathAuthority.Kind, AuthorityRootHash: before.PathAuthority.RootHash,
		RelativePath: before.PathAuthority.RelativePath, ExpectedAfterExisted: true,
		ExpectedAfterHash: checkpointapp.HashBytes(raw),
	}}}
	if status, ok := domaincheckpoint.ClassifyObservedOperationGroup(intent, []domaincheckpoint.ObservedOperationPathV2{stale}); !ok || status != "quarantined" {
		t.Fatalf("removed allow_write root did not classify as quarantined: status=%q ok=%t observed=%#v", status, ok, stale)
	}
	forged := before.PathAuthority
	forged.RootHash = checkpointAuthorityRootHash("/forged", forged.RootIdentity)
	changed := observer.ObserveRelative(context.Background(), workspace, forged)
	if changed.ObservationStatus != "unavailable" || changed.BlockerCode != "path_unsafe" {
		t.Fatalf("changed root identity did not fail closed: %#v", changed)
	}
	legacy := observer.ObserveRelative(context.Background(), workspace, checkpointfileport.PathAuthority{
		Root: workspace, RelativePath: "legacy.txt",
	})
	if legacy.ObservationStatus != "unavailable" || legacy.BlockerCode != "path_unsafe" {
		t.Fatalf("legacy path authority did not require quarantine: %#v", legacy)
	}

	oldRoot := allowRoot + "-old"
	if err := os.Rename(allowRoot, oldRoot); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(oldRoot)
	if err := os.Mkdir(allowRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	replaced := observer.ObserveRelative(context.Background(), workspace, before.PathAuthority)
	if replaced.ObservationStatus != "unavailable" || replaced.BlockerCode != "path_unsafe" || replaced.Existed || replaced.Hash != "" {
		t.Fatalf("same-path allow_write root replacement did not fail closed: %#v", replaced)
	}
	if status, ok := domaincheckpoint.ClassifyObservedOperationGroup(intent, []domaincheckpoint.ObservedOperationPathV2{replaced}); !ok || status != "quarantined" {
		t.Fatalf("same-path root replacement did not quarantine: status=%q ok=%t observed=%#v", status, ok, replaced)
	}
}

func TestCheckpointOperationResourceOverlapPreservesPhysicalAliases(t *testing.T) {
	observer := CheckpointOperationObserver{}
	first := checkpointfileport.PathAuthority{Root: filepath.Join(string(filepath.Separator), "workspace"), RootIdentity: "physical-root", RelativePath: "held/item"}
	for _, tc := range []struct {
		name                     string
		root, identity, relative string
		overlap                  bool
	}{
		{"same", first.Root, "", "held/item", true},
		{"parent", first.Root, "", "held", true},
		{"child", first.Root, "", "held/item/child", true},
		{"adjacent prefix", first.Root, "", "held/items", false},
		{"canonical relative", first.Root, "", "held/other/../item", true},
		{"physical alias", first.Root + "-alias", first.RootIdentity, "held/item", true},
		{"different physical owner", first.Root + "-other", "other-root", "held/item", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			second := checkpointfileport.PathAuthority{Root: tc.root, RootIdentity: tc.identity, RelativePath: tc.relative}
			if observer.ResourcesOverlap(first, second) != tc.overlap || observer.ResourcesOverlap(second, first) != tc.overlap {
				t.Fatal("resource overlap does not preserve symmetric ownership")
			}
		})
	}
}
