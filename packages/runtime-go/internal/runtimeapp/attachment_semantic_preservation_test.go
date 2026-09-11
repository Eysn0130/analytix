//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	attachmentstore "analytix.local/runtime-go/internal/adapters/outbound/attachmentauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func runtimeHeldAttachmentIntentFixtureV1(t *testing.T) (*runtimeChildIdentityStartupV1, domainattachment.UploadIntentV1, string) {
	t.Helper()
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	frozen := scope.Contexts()[0]
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{
		OwnerNonce: "00000000000000000000000000000001", BlobSHA256: domainsecurity.SHA256Hex([]byte("held blob")),
		ByteSize: 9, MIMEType: "application/pdf", ThreadID: frozen.ThreadID, WorkspaceRealPath: frozen.WorkspaceRealPath,
		ProjectionSHA256: domainsecurity.SHA256Hex([]byte("held projection")), CreatedAt: time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainattachment.NewUploadIntentV1(owner, domainsecurity.SHA256Hex([]byte("held metadata")))
	if err != nil {
		t.Fatal(err)
	}
	store, err := attachmentstore.NewStoreContext(ctx, filepath.Join(core.roots.DataDir, "private", "attachment-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadIntentIfAbsent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	fresh, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	return fresh, intent, "data/private/attachment-authority/upload-intents/" + intent.UploadID[:2] + "/" + intent.UploadID + ".json"
}

func TestRuntimeAttachmentSemanticAllowsIndependentTransactions(t *testing.T) {
	ctx := context.Background()
	core, held, _ := runtimeHeldAttachmentIntentFixtureV1(t)
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{OwnerNonce: "00000000000000000000000000000002", BlobSHA256: held.Owner.BlobSHA256, ByteSize: held.Owner.ByteSize, MIMEType: held.Owner.MIMEType, ThreadID: "thread-attachment-independent", WorkspaceRealPath: held.Owner.WorkspaceRealPath, ProjectionSHA256: held.Owner.ProjectionSHA256, CreatedAt: time.Unix(1, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainattachment.NewUploadIntentV1(owner, held.MetadataSHA256)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainattachment.NewUploadDispositionV1(intent, domainattachment.UploadDispositionQuarantinedV1, "restart_missing_upload_files", "", "", time.Unix(2, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	intentBody, err := domainattachment.UploadIntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := domainattachment.UploadDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	originalHeld := preserved.attachments.held
	rootAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("independent-attachment-transactions"))
	for _, remove := range []bool{false, true} {
		snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
		if err != nil {
			t.Fatal(err)
		}
		baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
		if err != nil {
			t.Fatal(err)
		}
		journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
		if err != nil {
			t.Fatal(err)
		}
		builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, rootAuthority, journal, preserved)
		plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
			for leaf, body := range map[string][]byte{"upload-intents": intentBody, "upload-dispositions": dispositionBody} {
				name := filepath.Join(stage.DataDir, "private", "attachment-authority", leaf, intent.UploadID[:2], intent.UploadID+".json")
				if remove {
					if err := os.Remove(name); err != nil {
						return err
					}
					continue
				}
				if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(name, body, 0o600); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := plan.Apply(ctx); err != nil {
			_ = plan.Close()
			t.Fatalf("independent attachment transaction: %v", err)
		}
		if err := plan.Close(); err != nil {
			t.Fatal(err)
		}
		if err := preserved.validateAttachmentSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(originalHeld, preserved.attachments.held) {
			t.Fatal("independent transaction changed held attachment scope")
		}
	}
}

func TestRuntimeAttachmentOriginalObservationAcceptsAuthenticatedDirectoryPrefix(t *testing.T) {
	for _, installed := range []string{"private/attachment-authority", "private/attachment-authority/owners"} {
		t.Run(filepath.Base(installed), func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			leaves := []string{"owners", "upload-intents", "upload-dispositions", "use-receipts", "use-dispositions"}
			runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				for _, leaf := range leaves {
					if err := os.MkdirAll(filepath.Join(stage.DataDir, "private", "attachment-authority", leaf), 0o700); err != nil {
						return err
					}
				}
				return os.Mkdir(filepath.Join(stage.DataDir, "private", "attachment-authority", "owners", "ab"), 0o700)
			}, installed, "private/attachment-authority/use-receipts", preserved)
			fresh, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			restarted, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh)
			if err != nil {
				t.Fatalf("authenticated partial attachment topology rejected before original/Final reconstruction: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("original observation changed signed physical prefix")
			}
			root, err := persistencefs.FreezeRootAuthority(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, restarted)
			if err := builder.RecoverAuthenticatedExisting(ctx); err != nil {
				t.Fatalf("recover authenticated attachment directory prefix: %v", err)
			}
			for _, leaf := range leaves {
				if info, err := os.Stat(filepath.Join(core.roots.DataDir, "private", "attachment-authority", leaf)); err != nil || !info.IsDir() {
					t.Fatalf("recovery did not finish complete attachment owner: %v", err)
				}
			}
			if err := recoverRuntimePrivateCASOrphanTopology(ctx, core.roots.DataDir, core.access, restarted); err != nil {
				t.Fatalf("cleanup independent signed After directory: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(core.roots.DataDir, "private", "attachment-authority", "owners", "ab")); !os.IsNotExist(err) {
				t.Fatalf("orphan preservation adopted a directory absent from the original scope: %v", err)
			}
		})
	}
}

func TestRuntimeAttachmentOriginalFilesRemainBoundToHeldIntent(t *testing.T) {
	for _, scenario := range []string{"unchanged", "metadata_mode", "content_mode", "content_replaced", "metadata_removed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
			scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			store, err := filestore.NewPersistentAttachmentStore(core.roots.DataDir)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := store.Prepare(ctx, map[string]any{"name": "held.txt", "mimeType": "text/plain", "dataBase64": base64.StdEncoding.EncodeToString([]byte("original held attachment")), "threadId": scope.Contexts()[0].ThreadID, "workspace": scope.Contexts()[0].WorkspaceRealPath})
			if err != nil {
				t.Fatal(err)
			}
			intent, err := domainattachment.NewUploadIntentV1(prepared.Owner(), prepared.MetadataSHA256())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := prepared.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			authority, err := attachmentstore.NewStoreContext(ctx, filepath.Join(core.roots.DataDir, "private", "attachment-authority"), core.access)
			if err != nil {
				t.Fatal(err)
			}
			if err := authority.PutUploadIntentIfAbsent(ctx, intent); err != nil {
				t.Fatal(err)
			}
			core, err = runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			metadata := filepath.Join(core.roots.DataDir, "attachments", "metadata", intent.Owner.AttachmentID+".json")
			content := filepath.Join(core.roots.DataDir, "attachments", "content", intent.Owner.AttachmentID+".bin")
			switch scenario {
			case "metadata_mode":
				err = os.Chmod(metadata, 0o400)
			case "content_mode":
				err = os.Chmod(content, 0o400)
			case "content_replaced":
				err = os.WriteFile(content, []byte("different content"), 0o600)
			case "metadata_removed":
				err = os.Remove(metadata)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.validateAttachmentSemanticOperationsV1(ctx, nil, nil, "")
			if (err == nil) != (scenario == "unchanged") {
				t.Fatalf("original attachment files binding: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("original attachment observation changed files")
			}
		})
	}
}

func TestRuntimeAttachmentSemanticPreservesOriginalHeldIntent(t *testing.T) {
	for _, scenario := range []string{"remove_intent", "intent_mode", "add_owner", "add_disposition", "shared_mode", "remove_residue"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, intent, name := runtimeHeldAttachmentIntentFixtureV1(t)
			body, err := domainattachment.UploadIntentV1Bytes(intent)
			if err != nil {
				t.Fatal(err)
			}
			before := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			after := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
			kind := domainstartup.SemanticOperationRemoveFile
			switch scenario {
			case "intent_mode":
				kind, after = domainstartup.SemanticOperationSetMode, before
				after.Mode = 0o400
			case "add_owner", "add_disposition":
				kind = domainstartup.SemanticOperationInstallFile
				before = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				name = "data/private/attachment-authority/owners/" + intent.Owner.OwnerDigest[:2] + "/" + intent.Owner.OwnerDigest + ".json"
				body, err = domainattachment.OwnerRecordV1Bytes(intent.Owner)
				if scenario == "add_disposition" {
					disposition, err := domainattachment.NewUploadDispositionV1(intent, domainattachment.UploadDispositionQuarantinedV1, "restart_missing_upload_files", "", "", time.Unix(2, 0).UTC())
					if err != nil {
						t.Fatal(err)
					}
					body, err = domainattachment.UploadDispositionV1Bytes(disposition)
					name = "data/private/attachment-authority/upload-dispositions/" + intent.UploadID[:2] + "/" + intent.UploadID + ".json"
				}
				if err != nil {
					t.Fatal(err)
				}
				after = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			case "shared_mode":
				kind = domainstartup.SemanticOperationSetMode
				name = "data/private/attachment-authority"
				before = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeDirectory, Mode: uint32(os.ModeDir) | 0o700}
				after = before
				after.Mode = uint32(os.ModeDir) | 0o500
			case "remove_residue":
				name = "data/private/attachment-authority/upload-intents/" + intent.UploadID[:2] + "/." + intent.UploadID + ".json-held.tmp"
				body = []byte("opaque held upload prefix")
				if err := os.WriteFile(filepath.Join(core.roots.DataDir, filepath.FromSlash(name[len("data/"):])), body, 0o600); err != nil {
					t.Fatal(err)
				}
				before = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("attachment-original-semantic"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, []domainstartup.SemanticStartupOperationV1{{Kind: kind, Path: name, Before: before, After: after}})
			if err != nil {
				t.Fatal(err)
			}
			files := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			if err := preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }, ""); err == nil {
				t.Fatal("semantic operation changed original held attachment inventory")
			}
			if !reflect.DeepEqual(files, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("attachment rejection mutated files")
			}
		})
	}
}

func TestRuntimeAttachmentOriginalGraphRejectsCorruptPrivateRecord(t *testing.T) {
	core, _, name := runtimeHeldAttachmentIntentFixtureV1(t)
	if err := os.WriteFile(filepath.Join(core.roots.DataDir, filepath.FromSlash(name[len("data/"):])), json.RawMessage(`{"corrupt":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareRuntimeReportRestartPreservationV1(context.Background(), core); err == nil {
		t.Fatal("corrupt attachment original graph was ignored")
	}
}

func TestRuntimeAttachmentOriginalRejectsUnknownOrdinaryFileAddress(t *testing.T) {
	for _, relative := range []string{"metadata/unexpected.bin", "content/not-an-attachment.bin"} {
		t.Run(relative, func(t *testing.T) {
			core, _, _ := runtimeHeldAttachmentIntentFixtureV1(t)
			name := filepath.Join(core.roots.DataDir, "attachments", filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte("unrecognized ordinary attachment file"), 0o600); err != nil {
				t.Fatal(err)
			}
			core, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := prepareRuntimeReportRestartPreservationV1(context.Background(), core); err == nil {
				t.Fatal("unknown original attachment address was deferred until live preflight")
			}
		})
	}
}
