//go:build darwin || linux

package runtimeapp

import (
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	privatecasrecoverytest "analytix.local/runtime-go/internal/testsupport/privatecasrecovery"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRuntimeAttachmentOriginalCreateResidueSurvivesEarlyRecovery(t *testing.T) {
	for _, kind := range []string{"owner root", "leaf", "shard"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			core, _, _ := runtimeHeldAttachmentIntentFixtureV1(t)
			parent := filepath.Join(core.roots.DataDir, "private")
			component := "attachment-authority"
			if kind == "leaf" || kind == "shard" {
				parent = filepath.Join(parent, "attachment-authority")
				component = "owners"
			}
			if kind == "shard" {
				parent = filepath.Join(parent, "owners")
				component = "ab"
			}
			residue := filepath.Join(parent, domainprivatecas.CreateDirectoryResidueNameV1(component))
			if err := os.Mkdir(residue, 0o700); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(residue)
			if err != nil {
				t.Fatal(err)
			}
			// Match startup: freeze the original creation inventory before any
			// preservation consumer or recovery action receives the Core.
			core.originalCreates, err = prepareRuntimeOriginalCreateStartupV1(ctx, core.roots, core.access)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatalf("original create residue blocked complete held inventory observation: %v", err)
			}
			if err := recoverRuntimePrivateCASCreateResidues(ctx, core.roots.DataDir, core.access, preserved); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(residue)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("early recovery changed original held create residue: %v", err)
			}
			if err := preserved.validateAttachmentSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
				t.Fatalf("preserved create residue blocked later original observation: %v", err)
			}
		})
	}
}

func TestRuntimeAttachmentOriginalPartialCreationAllowsIndependentFirstWrite(t *testing.T) {
	for _, count := range []int{-1, 0, 1, 4} {
		t.Run(fmt.Sprintf("original-leaves-%d", count), func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
			scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(core.roots.DataDir, "private", "attachment-authority")
			parent, component := filepath.Dir(root), "attachment-authority"
			if count >= 0 {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				parent, component = root, "upload-intents"
				for _, leaf := range []string{"owners", "use-receipts", "use-dispositions", "upload-dispositions"}[:count] {
					if err := os.Mkdir(filepath.Join(root, leaf), 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}
			residue := filepath.Join(parent, domainprivatecas.CreateDirectoryResidueNameV1(component))
			if err := os.Mkdir(residue, 0o700); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(residue)
			if err != nil {
				t.Fatal(err)
			}
			original := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatalf("complete original partial observation failed: %v", err)
			}
			store, err := preserved.OpenAttachmentRestartStoreV1(ctx, root, core.access)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("read-only partial activation materialized an absent leaf")
			}
			frozen := scope.Contexts()[0]
			owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{OwnerNonce: "00000000000000000000000000000009", BlobSHA256: domainsecurity.SHA256Hex([]byte("independent original partial blob")), ByteSize: 9, MIMEType: "application/pdf", ThreadID: "thread-independent", WorkspaceRealPath: frozen.WorkspaceRealPath, ProjectionSHA256: domainsecurity.SHA256Hex([]byte("independent projection")), CreatedAt: time.Unix(1, 0).UTC()})
			if err != nil {
				t.Fatal(err)
			}
			intent, err := domainattachment.NewUploadIntentV1(owner, domainsecurity.SHA256Hex([]byte("independent metadata")))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PutUploadIntentIfAbsent(ctx, intent); err != nil {
				t.Fatalf("independent first write collided with original creation residue: %v", err)
			}
			if got, err := store.ResolveUploadIntent(ctx, intent.UploadID); err != nil || !reflect.DeepEqual(got, intent) {
				t.Fatalf("independent first record readback: %v", err)
			}
			after, err := os.Stat(residue)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("independent first write changed original creation residue: %v", err)
			}
			current := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for name, record := range original {
				if !semanticOriginalRecordUnchangedForTestV1(record, current[name]) {
					t.Fatalf("independent first write changed original %s", name)
				}
			}
		})
	}
}

func TestRuntimeAttachmentOriginalCreateResidueRecoveryAndIndependentWrite(t *testing.T) {
	for _, kind := range []string{"owner root", "leaf", "shard"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			core, held, _ := runtimeHeldAttachmentIntentFixtureV1(t)
			owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{OwnerNonce: "00000000000000000000000000000004", BlobSHA256: held.Owner.BlobSHA256, ByteSize: held.Owner.ByteSize, MIMEType: held.Owner.MIMEType, ThreadID: "thread-independent", WorkspaceRealPath: held.Owner.WorkspaceRealPath, ProjectionSHA256: held.Owner.ProjectionSHA256, CreatedAt: time.Unix(1, 0).UTC()})
			if err != nil {
				t.Fatal(err)
			}
			intent, err := domainattachment.NewUploadIntentV1(owner, held.MetadataSHA256)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(core.roots.DataDir, "private", "attachment-authority")
			parent, component := filepath.Dir(root), "attachment-authority"
			if kind == "leaf" {
				parent, component = root, "upload-intents"
			}
			if kind == "shard" {
				parent, component = filepath.Join(root, "upload-intents"), intent.UploadID[:2]
			}
			residue := filepath.Join(parent, domainprivatecas.CreateDirectoryResidueNameV1(component))
			if err := os.Mkdir(residue, 0o700); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(residue)
			if err != nil {
				t.Fatal(err)
			}
			core.originalCreates, err = prepareRuntimeOriginalCreateStartupV1(ctx, core.roots, core.access)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			if err := recoverRuntimePrivateCASCreateResidues(ctx, core.roots.DataDir, core.access, preserved); err != nil {
				t.Fatal(err)
			}
			if err := recoverRuntimePrivateCASOrphanTopology(ctx, core.roots.DataDir, core.access, preserved); err != nil {
				t.Fatalf("original creation residue blocked orphan recovery: %v", err)
			}
			var selected []runtimePrivateCASOwnerRecovery
			for _, owner := range runtimePrivateCASOwnerRecoveries(core.roots.DataDir, core.access) {
				if owner.name == "attachment-authority" {
					selected = append(selected, owner)
				}
			}
			prepared, err := prepareAndValidateRuntimePrivateCASOwners(ctx, selected, true, nil, preserved)
			if err != nil || len(prepared) != 1 {
				t.Fatalf("original creation residue blocked body recovery preparation: %v", err)
			}
			if err := privatecasrecoverytest.ApplyV4(ctx, "original-create-attachment-recovery", prepared[0]); err != nil {
				t.Fatal(err)
			}
			store, err := preserved.OpenAttachmentRestartStoreV1(ctx, root, core.access)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PutUploadIntentIfAbsent(ctx, intent); err != nil {
				t.Fatalf("original creation residue blocked independent append: %v", err)
			}
			if got, err := store.ResolveUploadIntent(ctx, intent.UploadID); err != nil || !reflect.DeepEqual(got, intent) {
				t.Fatalf("independent readback: %v", err)
			}
			if got, err := store.ResolveUploadIntent(ctx, held.UploadID); err != nil || !reflect.DeepEqual(got, held) {
				t.Fatalf("held readback: %v", err)
			}
			after, err := os.Stat(residue)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("startup or independent append changed original creation residue: %v", err)
			}
		})
	}
}
