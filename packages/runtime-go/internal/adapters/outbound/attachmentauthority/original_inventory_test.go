package attachmentauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type originalAttachmentVerifierV1 struct {
	fixture attachmentAuthorityFixture
	cause   error
}

func TestOriginalAttachmentPartialObservationHasEveryLeafPresenceProof(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3, 4} {
		t.Run(fmt.Sprintf("present-%d", count), func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "private", "attachment-authority")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			names := []string{"owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"}
			for _, name := range names[:count] {
				if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareOriginalInventoryV1(ctx, root, access)
			if err != nil {
				t.Fatal(err)
			}
			if len(prepared.leaves) != 5 {
				t.Fatal("partial observation omitted absent leaf plans")
			}
			for i, name := range names {
				plan := prepared.leaves[name]
				if plan == nil || plan.RootPath() != filepath.Join(root, name) || plan.Present() != (i < count) {
					t.Fatalf("unproved leaf presence at %s", name)
				}
			}
			files, err := prepared.SnapshotOriginalFilesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseOriginalInventoryV1(ctx, files, nil); err == nil {
				t.Fatal("ordinary parser admitted partial topology")
			}
			if _, err := prepared.ParseOriginalEndpointV1(ctx, files, nil); err != nil {
				t.Fatalf("proved original endpoint rejected: %v", err)
			}
			saved := prepared.leaves[names[count]]
			delete(prepared.leaves, names[count])
			if _, err := prepared.ParseOriginalEndpointV1(ctx, files, nil); err == nil {
				t.Fatal("missing map entry became physical absence proof")
			}
			prepared.leaves[names[count]] = saved
			if err := os.Mkdir(filepath.Join(root, names[count]), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := prepared.ParseOriginalEndpointV1(ctx, files, nil); err == nil {
				t.Fatal("stale absent leaf plan admitted a new directory")
			}
		})
	}
}

func (v originalAttachmentVerifierV1) KeyID() string { return v.fixture.keyID }
func (v originalAttachmentVerifierV1) PublicKey() []byte {
	return append([]byte(nil), v.fixture.publicKey...)
}
func (v originalAttachmentVerifierV1) VerifyTrusted(ctx context.Context, keyID string, key, body, signature []byte) error {
	if v.cause != nil {
		return v.cause
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if keyID != v.fixture.keyID || !bytes.Equal(key, v.fixture.publicKey) || !ed25519.Verify(v.fixture.publicKey, body, signature) {
		return errors.New("synthetic original attachment key mismatch")
	}
	return nil
}

func TestOriginalAttachmentInventoryKeepsAllFiveLeavesAndRawResidue(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "attachment-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newAttachmentAuthorityFixture(t)
	if err := store.PutOwnerIfAbsent(ctx, fixture.owner); err != nil {
		t.Fatal(err)
	}
	intent, err := domainattachment.NewUploadIntentV1(fixture.owner, domainsecurity.SHA256Hex([]byte("synthetic-original-metadata")))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadIntentIfAbsent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	uploadDisposition, err := domainattachment.NewUploadDispositionV1(intent, domainattachment.UploadDispositionCommittedV1, "upload_committed", intent.MetadataSHA256, intent.Owner.BlobSHA256, fixture.disposedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadDispositionIfAbsent(ctx, uploadDisposition); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUseReceiptIfAbsent(ctx, fixture.receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUseDispositionIfAbsent(ctx, fixture.disposition); err != nil {
		t.Fatal(err)
	}
	residue := "owners/" + fixture.owner.OwnerDigest[:2] + "/." + fixture.owner.OwnerDigest + ".json-original.tmp"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(residue)), []byte("opaque original unfinished bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	files, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files[residue].Body, []byte("opaque original unfinished bytes")) || files[residue].Mode != 0o600 {
		t.Fatal("original residue was dropped or normalized")
	}
	inventory, err := ParseOriginalInventoryV1(ctx, files, originalAttachmentVerifierV1{fixture: fixture})
	if err != nil || len(inventory.Owners) != 1 || len(inventory.UploadIntents) != 1 || len(inventory.UploadDispositions) != 1 || len(inventory.UseReceipts) != 1 || len(inventory.UseDispositions) != 1 {
		t.Fatalf("original attachment denominator is incomplete: err=%v", err)
	}
	for _, leaf := range []string{"owners", "upload-intents", "upload-dispositions", "use-receipts", "use-dispositions"} {
		if !files[leaf].Directory || files[leaf].Mode != 0o700 {
			t.Fatalf("original leaf mode or presence changed: %s", leaf)
		}
	}
	again, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil || !reflect.DeepEqual(files, again) {
		t.Fatalf("observation changed original attachment files: %v", err)
	}
	cause := errors.New("synthetic current key observation I/O failure")
	failed, err := ParseOriginalInventoryV1(ctx, files, originalAttachmentVerifierV1{fixture: fixture, cause: cause})
	if !errors.Is(err, cause) || !reflect.DeepEqual(failed, OriginalInventoryV1{}) {
		t.Fatalf("original key failure became partial inventory: %v", err)
	}
	for _, change := range []string{"owner_removed", "foreign_key", "address_changed", "unsafe_mode"} {
		t.Run(change, func(t *testing.T) {
			candidate := make(map[string]OriginalEntryV1, len(files))
			for name, value := range files {
				value.Body = append([]byte(nil), value.Body...)
				candidate[name] = value
			}
			verifier := originalAttachmentVerifierV1{fixture: fixture}
			ownerPath := "owners/" + fixture.owner.OwnerDigest[:2] + "/" + fixture.owner.OwnerDigest + ".json"
			switch change {
			case "owner_removed":
				delete(candidate, ownerPath)
			case "foreign_key":
				verifier.fixture = newAttachmentAuthorityFixture(t)
			case "address_changed":
				candidate[ownerPath+".bad"] = candidate[ownerPath]
				delete(candidate, ownerPath)
			case "unsafe_mode":
				value := candidate[ownerPath]
				value.Mode = 0o644
				if runtime.GOOS == "windows" {
					value.Mode = 0o1000
				}
				candidate[ownerPath] = value
			}
			if _, err := ParseOriginalInventoryV1(ctx, candidate, verifier); err == nil {
				t.Fatal("corrupt full original graph was accepted")
			}
		})
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if failed, err := prepared.SnapshotOriginalFilesV1(cancelled); !errors.Is(err, context.Canceled) || failed != nil {
		t.Fatalf("cancelled raw read lost its cause: %v", err)
	}
	ownerPath := filepath.Join(root, "owners", fixture.owner.OwnerDigest[:2], fixture.owner.OwnerDigest+".json")
	if err := os.Chmod(ownerPath, 0o400); err != nil {
		t.Fatal(err)
	}
	if failed, err := prepared.SnapshotOriginalFilesV1(ctx); err == nil || failed != nil {
		t.Fatal("stale original mode was accepted")
	}
}

func TestOriginalAttachmentInventoryLeavesAbsentOwnerAbsent(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "absent-attachment-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	files, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil || len(files) != 0 {
		t.Fatalf("absent original observation failed: %v", err)
	}
	if _, err := ParseOriginalInventoryV1(ctx, files, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("original observation created an absent owner")
	}
}

func TestOriginalAttachmentInventoryRejectsIncompleteCompleteGraph(t *testing.T) {
	for _, change := range []string{"root_only", "missing_empty_leaf", "root_body"} {
		t.Run(change, func(t *testing.T) {
			files := map[string]OriginalEntryV1{".": {Directory: true, Mode: 0o700}}
			if change != "root_only" {
				for _, leaf := range []string{"owners", "upload-intents", "upload-dispositions", "use-receipts", "use-dispositions"} {
					files[leaf] = OriginalEntryV1{Directory: true, Mode: 0o700}
				}
			}
			if change == "missing_empty_leaf" {
				delete(files, "use-dispositions")
			}
			if change == "root_body" {
				files["."] = OriginalEntryV1{Directory: true, Mode: 0o700, Body: []byte("unexpected directory bytes")}
			}
			if _, err := ParseOriginalInventoryV1(context.Background(), files, nil); err == nil {
				t.Fatal("incomplete or invalid complete original graph was accepted")
			}
		})
	}
}
