package attachmentauthority

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestOriginalObservationRetainsProvedCreationResidues(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	root := filepath.Join(data, "private", "attachment-authority")
	for leaf := range originalAttachmentLeafLimitsV1 {
		if err := os.MkdirAll(filepath.Join(root, leaf), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	residues := []string{
		filepath.Join(data, "private", domainprivatecas.CreateDirectoryResidueNameV1("attachment-authority")),
		filepath.Join(root, domainprivatecas.CreateDirectoryResidueNameV1("owners")),
		filepath.Join(root, "owners", domainprivatecas.CreateDirectoryResidueNameV1("ab")),
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
	physical, err := finalauthority.PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, data, access)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := physical.OriginalCreateResiduesV1(ctx, "private/attachment-authority")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareOriginalInventoryV1(ctx, root, access); err == nil {
		t.Fatal("ordinary owner observation accepted creation residues without explicit proof")
	}
	prepared, err := PrepareOriginalInventoryWithCreateResiduesV1(ctx, root, access, proof)
	if err != nil {
		t.Fatal(err)
	}
	files, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 || len(proof.RelativePathsV1()) != 3 {
		t.Fatalf("original directory denominator incomplete: owner=%d outerProof=%d", len(files), len(proof.RelativePathsV1()))
	}
	if _, err := ParseOriginalInventoryV1(ctx, files, nil); err != nil {
		t.Fatal(err)
	}
	leafResidue := domainprivatecas.CreateDirectoryResidueNameV1("owners")
	files[leafResidue+"/foreign"] = OriginalEntryV1{Mode: 0o600}
	if _, err := ParseOriginalInventoryV1(ctx, files, nil); err == nil {
		t.Fatal("original parser accepted a nonempty creation residue")
	}
	if err := os.Remove(residues[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.SnapshotOriginalFilesV1(ctx); err == nil {
		t.Fatal("owner snapshot ignored missing original residue outside owner root")
	}
}

func TestOriginalObservationSeparatesPhysicalPartialFromLogicalAbsence(t *testing.T) {
	for _, state := range []string{"absent", "root only", "partial", "complete"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "attachment-authority")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			if state != "absent" {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if state == "partial" {
				if err := os.Mkdir(filepath.Join(root, "owners"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if state == "complete" {
				for leaf := range originalAttachmentLeafLimitsV1 {
					if err := os.Mkdir(filepath.Join(root, leaf), 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}
			prepared, err := PrepareOriginalInventoryV1(ctx, root, access)
			if err != nil {
				t.Fatal(err)
			}
			files, err := prepared.SnapshotOriginalFilesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Present() != (state != "absent") || (len(files) == 0) != (state == "absent") {
				t.Fatalf("physical state %s misreported as absence: present=%v entries=%d", state, prepared.Present(), len(files))
			}
			if _, err := ParseOriginalInventoryV1(ctx, files, nil); (err == nil) != (state == "absent" || state == "complete") {
				t.Fatalf("physical observation granted partial logical authority: state=%s err=%v", state, err)
			}
			if state == "absent" {
				if _, err := os.Lstat(root); !os.IsNotExist(err) {
					t.Fatalf("physical absence observation created root: %v", err)
				}
			}
		})
	}
}

func TestOriginalObservationRejectsUnknownAndStaleLeafMembership(t *testing.T) {
	for _, kind := range []string{"unknown", "regular", "new leaf", "removed leaf"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "attachment-authority")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "owners"), 0o700); err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareOriginalInventoryV1(ctx, root, access)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "unknown":
				err = os.Mkdir(filepath.Join(root, "unobserved-owner"), 0o700)
			case "regular":
				err = os.WriteFile(filepath.Join(root, "use-receipts"), nil, 0o600)
			case "new leaf":
				err = os.Mkdir(filepath.Join(root, "use-receipts"), 0o700)
			case "removed leaf":
				err = os.Remove(filepath.Join(root, "owners"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if files, err := prepared.SnapshotOriginalFilesV1(ctx); err == nil || files != nil {
				t.Fatalf("stale full physical membership accepted %s: %v", kind, err)
			}
			if kind == "unknown" || kind == "regular" {
				if _, err := PrepareOriginalInventoryV1(ctx, root, access); err == nil {
					t.Fatalf("physical preparation accepted %s", kind)
				}
			}
		})
	}
}
