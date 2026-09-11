package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestStartupSnapshotReaderProducesValidatedDomainSnapshot(t *testing.T) {
	roots := testRootSet(t)
	mustMkdirAll(t, filepath.Join(roots.DataDir, "attachments", "content"))
	mustWrite(t, filepath.Join(roots.DataDir, "attachments", "content", "payload.bin"), "payload")
	snapshot, err := NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(context.Background())
	if err != nil || snapshot.SnapshotDigest == "" || snapshot.RootBindingDigest == "" || len(snapshot.Entries) == 0 {
		t.Fatalf("domain startup snapshot was not captured: snapshot=%#v err=%v", snapshot, err)
	}
}

func TestStartupSnapshotExcludesDedicatedLargeRawArtifactRoot(t *testing.T) {
	roots := testRootSet(t)
	rawRoot := filepath.Join(roots.DataDir, "raw-artifact-chunks-v1", "aa")
	mustMkdirAll(t, rawRoot)
	chunk := filepath.Join(rawRoot, strings.Repeat("a", 64)+".blob")
	file, err := os.OpenFile(chunk, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o400)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(65 << 20); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(context.Background())
	if err != nil {
		t.Fatalf("dedicated raw root was incorrectly subjected to managed snapshot limits: %v", err)
	}
	for _, entry := range snapshot.Entries {
		if strings.Contains(entry.Path, "raw-artifact-chunks-v1") {
			t.Fatalf("dedicated raw root leaked into managed snapshot: %s", entry.Path)
		}
	}
}

func TestStartupRejectsDuplicateKeysRecursively(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_duplicate")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_duplicate","nested":{"claim":"a","claim":"b"}}`)

	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "invalid_json")
}

func TestManagedPreRecoveryBoundaryValidatesPublicStateAndDefersPrivateCAS(t *testing.T) {
	t.Run("public corruption", func(t *testing.T) {
		roots := testRootSet(t)
		threadDir := filepath.Join(roots.DurableDir, "threads", "thr_duplicate")
		mustMkdirAll(t, threadDir)
		mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_duplicate","nested":{"claim":"a","claim":"b"}}`)

		err := ValidateManagedPreRecoveryBoundaryV1(context.Background(), roots)
		assertIntegrityCode(t, err, "invalid_json")
	})
	t.Run("private CAS residue", func(t *testing.T) {
		roots := testRootSet(t)
		digest := strings.Repeat("a", 64)
		directory := filepath.Join(roots.DataDir, "private", "accepted-finals", "records", digest[:2])
		mustMkdirAll(t, directory)
		mustWrite(t, filepath.Join(directory, "."+digest+".json-crash.tmp"), `{"schemaVersion":2}`)

		if err := ValidateManagedPreRecoveryBoundaryV1(context.Background(), roots); err != nil {
			t.Fatalf("pre-recovery public boundary consumed private CAS authority: %v", err)
		}
		_, err := CaptureStrict(roots)
		assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
	})
}

func TestIntegrityErrorDoesNotRenderUntrustedPathOrJSONKey(t *testing.T) {
	roots := testRootSet(t)
	privateDir := filepath.Join(roots.DataDir, "private")
	mustMkdirAll(t, privateDir)
	secretName := "account-6222020000000000.json"
	secretKey := "identity-320000000000000000"
	mustWrite(t, filepath.Join(privateDir, secretName), `{"`+secretKey+`":1,"`+secretKey+`":2}`)
	_, err := CaptureStrict(roots)
	if err == nil {
		t.Fatal("duplicate key fixture passed strict capture")
	}
	message := err.Error()
	for _, secret := range []string{roots.DataDir, roots.DurableDir, secretName, secretKey} {
		if strings.Contains(message, secret) {
			t.Fatalf("integrity error rendered untrusted content %q: %s", secret, message)
		}
	}
}

func TestStartupRejectsTornAuthorityJSONLWithoutFinalNewline(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_torn")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_torn","turns":[]}`)
	mustWrite(t, filepath.Join(threadDir, "events.jsonl"), `{"kind":"heartbeat","threadId":"thr_torn","seq":1}`)

	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "jsonl_missing_final_newline")
}

func TestStartupAcceptsLegacyMessageJSONLWithoutFinalNewline(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_legacy_line")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_legacy_line","turns":[]}`)
	mustWrite(t, filepath.Join(threadDir, "messages.jsonl"), `{"kind":"user_message","threadId":"thr_legacy_line","text":"legacy"}`)
	if _, err := CaptureStrict(roots); err != nil {
		t.Fatalf("legacy non-authority JSONL framing should remain migratable: %v", err)
	}
}

func TestStartupAcceptsPositiveContiguousEventSequenceBase(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_compacted")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_compacted","turns":[]}`)
	mustWrite(t, filepath.Join(threadDir, "events.jsonl"),
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_compacted\",\"seq\":7}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_compacted\",\"seq\":8}\n")
	if _, err := CaptureStrict(roots); err != nil {
		t.Fatalf("positive contiguous event base should remain readable: %v", err)
	}
}

func TestStartupAcceptsLegacyOutOfOrderButContiguousEventsForSemanticCanonicalization(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_legacy_event_order")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "metadata.jsonl"), "{\"kind\":\"thread_metadata\",\"thread\":{\"id\":\"thr_legacy_event_order\",\"turns\":[]}}\n")
	mustWrite(t, filepath.Join(threadDir, "events.jsonl"),
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_legacy_event_order\",\"seq\":7}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_legacy_event_order\",\"seq\":9}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_legacy_event_order\",\"seq\":8}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_legacy_event_order\",\"seq\":10}\n")
	if _, err := CaptureStrict(roots); err != nil {
		t.Fatalf("contiguous legacy event set should remain migratable: %v", err)
	}
}

func TestStartupAcceptsPrimaryOutOfOrderContiguousEventsForSemanticCanonicalization(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_primary_event_order")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_primary_event_order","turns":[]}`)
	mustWrite(t, filepath.Join(threadDir, "events.jsonl"),
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_primary_event_order\",\"seq\":7}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_primary_event_order\",\"seq\":9}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_primary_event_order\",\"seq\":8}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_primary_event_order\",\"seq\":10}\n")
	if _, err := CaptureStrict(roots); err != nil {
		t.Fatalf("contiguous primary event set should remain migratable: %v", err)
	}
}

func TestStartupRejectsOutOfOrderEventsForCurrentThreadAuthority(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_current_event_order")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"),
		`{"id":"thr_current_event_order","securityState":{"contextDigest":"host-authority"},"turns":[]}`)
	mustWrite(t, filepath.Join(threadDir, "events.jsonl"),
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_current_event_order\",\"seq\":7}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_current_event_order\",\"seq\":9}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_current_event_order\",\"seq\":8}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_current_event_order\",\"seq\":10}\n")
	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "thread_record_identity")
}

func TestStartupRejectsLegacyOrderWhenEventCarriesTerminalAuthority(t *testing.T) {
	for _, marker := range []string{
		`"acceptedFinal":null`,
		`"generalTerminalCommitId":"forged"`,
		`"executionGrantId":"forged"`,
	} {
		roots := testRootSet(t)
		threadDir := filepath.Join(roots.DurableDir, "threads", "thr_event_authority_order")
		mustMkdirAll(t, threadDir)
		mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_event_authority_order","turns":[]}`)
		mustWrite(t, filepath.Join(threadDir, "events.jsonl"),
			"{\"kind\":\"tool_progress\",\"threadId\":\"thr_event_authority_order\",\"seq\":7}\n"+
				"{\"threadId\":\"thr_event_authority_order\",\"seq\":9,"+marker+"}\n"+
				"{\"kind\":\"tool_progress\",\"threadId\":\"thr_event_authority_order\",\"seq\":8}\n")
		_, err := CaptureStrict(roots)
		assertIntegrityCode(t, err, "thread_record_identity")
	}
}

func TestStartupRejectsLegacyOrderWithEventBundleTransactionResidue(t *testing.T) {
	for _, residue := range []string{
		".events-bundle-v1.tmp",
		".events-bundle-journal-v1.tmp",
		".events-bundle-journal-v1.json",
		".events-bundle-future-v9.candidate",
	} {
		t.Run(residue, func(t *testing.T) {
			roots := testRootSet(t)
			threadDir := filepath.Join(roots.DurableDir, "threads", "thr_event_residue_order")
			mustMkdirAll(t, threadDir)
			mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_event_residue_order","turns":[]}`)
			mustWrite(t, filepath.Join(threadDir, residue), "residue")
			eventsPath := filepath.Join(threadDir, "events.jsonl")
			before := "{\"kind\":\"tool_progress\",\"threadId\":\"thr_event_residue_order\",\"seq\":7}\n" +
				"{\"kind\":\"tool_progress\",\"threadId\":\"thr_event_residue_order\",\"seq\":9}\n" +
				"{\"kind\":\"tool_progress\",\"threadId\":\"thr_event_residue_order\",\"seq\":8}\n"
			mustWrite(t, eventsPath, before)
			if _, err := CaptureStrict(roots); err == nil {
				t.Fatal("event bundle transaction residue was accepted")
			}
			after, err := os.ReadFile(eventsPath)
			if err != nil || string(after) != before {
				t.Fatalf("read-only startup preflight mutated rejected events: err=%v", err)
			}
		})
	}
}

func TestStartupRejectsThreadAndEventIdentityMismatch(t *testing.T) {
	tests := []struct {
		name       string
		threadJSON string
		events     string
		code       string
	}{
		{
			name:       "thread id",
			threadJSON: `{"id":"thr_other","turns":[]}`,
			code:       "thread_id_mismatch",
		},
		{
			name:       "event thread id",
			threadJSON: `{"id":"thr_bound","turns":[]}`,
			events:     "{\"kind\":\"heartbeat\",\"threadId\":\"thr_other\",\"seq\":1}\n",
			code:       "thread_record_identity",
		},
		{
			name:       "event sequence",
			threadJSON: `{"id":"thr_bound","turns":[]}`,
			events: "{\"kind\":\"heartbeat\",\"threadId\":\"thr_bound\",\"seq\":7}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_bound\",\"seq\":9}\n",
			code: "thread_record_identity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roots := testRootSet(t)
			threadDir := filepath.Join(roots.DurableDir, "threads", "thr_bound")
			mustMkdirAll(t, threadDir)
			mustWrite(t, filepath.Join(threadDir, "thread.json"), test.threadJSON)
			if test.events != "" {
				mustWrite(t, filepath.Join(threadDir, "events.jsonl"), test.events)
			}
			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, test.code)
		})
	}
}

func TestStrictSnapshotRejectsSameSizeRewriteAndDirectoryChange(t *testing.T) {
	t.Run("same-size rewrite", func(t *testing.T) {
		roots := testRootSet(t)
		privateDir := filepath.Join(roots.DataDir, "private")
		mustMkdirAll(t, privateDir)
		path := filepath.Join(privateDir, "record.json")
		mustWrite(t, path, `{"value":"first"}`)
		_, err := captureStrictWithHook(roots, func() {
			mustWrite(t, path, `{"value":"other"}`)
		})
		assertIntegrityCode(t, err, "snapshot_changed")
	})

	t.Run("directory entry", func(t *testing.T) {
		roots := testRootSet(t)
		privateDir := filepath.Join(roots.DataDir, "private")
		mustMkdirAll(t, privateDir)
		mustWrite(t, filepath.Join(privateDir, "first.json"), `{"value":1}`)
		_, err := captureStrictWithHook(roots, func() {
			mustWrite(t, filepath.Join(privateDir, "second.json"), `{"value":2}`)
		})
		assertIntegrityCode(t, err, "snapshot_changed")
	})
}

func TestLegacyThreadBindingsMayBeAbsentButNeverConflict(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_legacy")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"turns":[]}`)
	mustWrite(t, filepath.Join(threadDir, "messages.jsonl"), "{\"kind\":\"user_message\",\"turnId\":\"turn_1\",\"text\":\"legacy\"}\n")
	if _, err := CaptureStrict(roots); err != nil {
		t.Fatalf("legacy missing binding should remain migratable: %v", err)
	}
	mustWrite(t, filepath.Join(threadDir, "messages.jsonl"), "{\"kind\":\"user_message\",\"threadId\":\"thr_other\",\"turnId\":\"turn_1\"}\n")
	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "thread_record_identity")
}

func TestStartupRejectsManagedSymlinkAndHardlink(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		roots := testRootSet(t)
		outside := filepath.Join(t.TempDir(), "outside.json")
		mustWrite(t, outside, `{"safe":true}`)
		mustMkdirAll(t, filepath.Join(roots.DataDir, "private"))
		link := filepath.Join(roots.DataDir, "private", "authority.json")
		if err := os.Symlink(outside, link); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink unavailable: %v", err)
			}
			t.Fatal(err)
		}
		_, err := CaptureStrict(roots)
		assertIntegrityCode(t, err, "managed_symlink")
	})

	t.Run("hardlink", func(t *testing.T) {
		roots := testRootSet(t)
		privateDir := filepath.Join(roots.DataDir, "private")
		mustMkdirAll(t, privateDir)
		first := filepath.Join(privateDir, "first.json")
		mustWrite(t, first, `{"safe":true}`)
		if err := os.Link(first, filepath.Join(privateDir, "second.json")); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
		_, err := CaptureStrict(roots)
		assertIntegrityCode(t, err, "managed_hardlink")
	})
}

func TestStrictSnapshotRejectsFinalAuthorityExclusiveWriteCrashResidueUntilCASRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink crash-residue fixture is Unix-only")
	}
	roots := testRootSet(t)
	digest := strings.Repeat("a", 64)
	directory := filepath.Join(roots.DataDir, "private", "accepted-finals", "records", digest[:2])
	mustMkdirAll(t, directory)
	tempPath := filepath.Join(directory, "."+digest+".json-crash.tmp")
	finalPath := filepath.Join(directory, digest+".json")
	mustWrite(t, tempPath, `{"schemaVersion":2}`)
	if err := os.Link(tempPath, finalPath); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
}

func TestStrictSnapshotRejectsPendingWorkExclusiveWriteCrashResidueUntilCASRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink crash-residue fixture is Unix-only")
	}
	roots := testRootSet(t)
	digest := strings.Repeat("b", 64)
	directory := filepath.Join(roots.DataDir, "private", "pending-work", "receipts", digest[:2])
	mustMkdirAll(t, directory)
	tempPath := filepath.Join(directory, "."+digest+".json-crash.tmp")
	finalPath := filepath.Join(directory, digest+".json")
	mustWrite(t, tempPath, `{"schemaVersion":1}`)
	if err := os.Link(tempPath, finalPath); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
}

func TestStrictSnapshotRejectsContinuationExclusiveWriteCrashResidueUntilCASRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink crash-residue fixture is Unix-only")
	}
	roots := testRootSet(t)
	digest := strings.Repeat("f", 64)
	directory := filepath.Join(roots.DataDir, "private", "gate-continuations", "receipts-v2", digest[:2])
	mustMkdirAll(t, directory)
	tempPath := filepath.Join(directory, "."+digest+".json-crash.tmp")
	finalPath := filepath.Join(directory, digest+".json")
	mustWrite(t, tempPath, `{"schemaVersion":2}`)
	if err := os.Link(tempPath, finalPath); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
}

func TestStrictSnapshotRejectsProviderCacheTelemetryCrashResidueUntilCASRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink crash-residue fixture is Unix-only")
	}
	roots := testRootSet(t)
	digest := strings.Repeat("d", 64)
	directory := filepath.Join(roots.DataDir, "private", "provider-cache-telemetry", "attempts", digest[:2])
	mustMkdirAll(t, directory)
	tempPath := filepath.Join(directory, "."+digest+".json-crash.tmp")
	finalPath := filepath.Join(directory, digest+".json")
	mustWrite(t, tempPath, `{"schemaVersion":"provider-attempt-intent.v1"}`)
	if err := os.Link(tempPath, finalPath); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
}

func TestProviderCacheTelemetryTempResidueRequiresExactKnownPartition(t *testing.T) {
	digest := strings.Repeat("e", 64)
	valid := []string{
		"data/private/provider-cache-telemetry/attempts/ee/." + digest + ".json-crash.tmp",
		"data/private/provider-cache-telemetry/settlements/ee/." + digest + ".json-crash.tmp",
		"data/private/provider-cache-telemetry/turn-closures/ee/." + digest + ".json-crash.tmp",
	}
	for _, label := range valid {
		if !privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("known provider cache telemetry residue was not recognized: %s", label)
		}
	}
	for _, label := range []string{
		"data/private/provider-cache-telemetry/unknown/ee/." + digest + ".json-crash.tmp",
		"data/private/provider-cache-telemetry/attempts/zz/." + digest + ".json-crash.tmp",
		"data/private/provider-cache-telemetry/attempts/ee/" + digest + ".json",
	} {
		if privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("unknown provider cache telemetry residue was accepted: %s", label)
		}
	}
}

func TestControlledArtifactAccessTempResidueRequiresExactKnownPartition(t *testing.T) {
	digest := strings.Repeat("c", 64)
	for _, label := range []string{
		"data/private/controlled-artifact-access/access-receipts/cc/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/controlled-artifact-access/access-dispositions/cc/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/controlled-artifact-access-v2/access-receipts/cc/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/controlled-artifact-access-v2/access-dispositions/cc/." + digest + ".json-000000000000000000000001.tmp",
	} {
		if !privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("known controlled artifact access residue was not recognized: %s", label)
		}
	}
	for _, label := range []string{
		"data/private/controlled-artifact-access/unknown/cc/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/controlled-artifact-access/access-receipts/zz/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/Controlled-Artifact-Access/access-receipts/cc/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/controlled-artifact-access/access-receipts/cc/" + digest + ".json",
		"data/private/controlled-artifact-access/access-receipts/extra/cc/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/controlled-artifact-access-v3/access-receipts/cc/." + digest + ".json-000000000000000000000001.tmp",
	} {
		if privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("unknown controlled artifact access residue was accepted: %s", label)
		}
	}
}

func TestStrictSnapshotRejectsControlledArtifactAccessSingleLinkResidueUntilCASRecovery(t *testing.T) {
	digest := strings.Repeat("c", 64)
	for _, owner := range []string{"controlled-artifact-access", "controlled-artifact-access-v2"} {
		for _, partition := range []string{"access-receipts", "access-dispositions"} {
			t.Run(owner+"/"+partition, func(t *testing.T) {
				roots := testRootSet(t)
				directory := filepath.Join(
					roots.DataDir,
					"private",
					owner,
					partition,
					digest[:2],
				)
				mustMkdirAll(t, directory)
				mustWrite(
					t,
					filepath.Join(directory, "."+digest+".json-000000000000000000000001.tmp"),
					`{"schemaVersion":"controlled-artifact-access-crash-residue.v1"}`,
				)

				_, err := CaptureStrict(roots)
				assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
			})
		}
	}
}

func TestStrictSnapshotHashesExactReportPublicationArtifactAsOpaque(t *testing.T) {
	roots := testRootSet(t)
	targetIdentityDigest := strings.Repeat("a", 64)
	body := "%PDF-1.7\ncontrolled artifact bytes are not JSON\n%%EOF"
	directory := filepath.Join(
		roots.DataDir,
		"private",
		"report-publication",
		"artifacts",
		targetIdentityDigest[:2],
	)
	mustMkdirAll(t, directory)
	mustWrite(t, filepath.Join(directory, targetIdentityDigest+".json"), body)

	snapshot, err := CaptureStrict(roots)
	if err != nil {
		t.Fatalf("capture exact report publication artifact: %v", err)
	}
	label := "data/private/report-publication/artifacts/" +
		targetIdentityDigest[:2] + "/" + targetIdentityDigest + ".json"
	entry, found := snapshotEntry(snapshot, label)
	if !found || entry.Type != "file" || entry.SHA256 != domainsecurity.SHA256Hex([]byte(body)) ||
		entry.Size != int64(len(body)) || entry.RecordCount != 1 {
		t.Fatalf("opaque report publication artifact snapshot mismatch: %#v", entry)
	}
}

func TestStrictSnapshotHashesEvidenceRegistryCASBodiesAsOpaque(t *testing.T) {
	for _, leaf := range []string{"indexes", "capsules"} {
		t.Run(leaf, func(t *testing.T) {
			roots := testRootSet(t)
			body := []byte("canonical opaque evidence-registry " + leaf + " body, not JSON")
			digest := domainsecurity.SHA256Hex(body)
			directory := filepath.Join(roots.DataDir, "private", "evidence-registry", leaf, digest[:2])
			mustMkdirAll(t, directory)
			mustWrite(t, filepath.Join(directory, digest+".json"), string(body))

			snapshot, err := CaptureStrict(roots)
			if err != nil {
				t.Fatalf("capture opaque evidence-registry %s: %v", leaf, err)
			}
			label := "data/private/evidence-registry/" + leaf + "/" + digest[:2] + "/" + digest + ".json"
			entry, found := snapshotEntry(snapshot, label)
			if !found || entry.Type != "file" || entry.SHA256 != digest || entry.Size != int64(len(body)) || entry.RecordCount != 1 {
				t.Fatalf("opaque evidence-registry snapshot mismatch: %#v", entry)
			}
		})
	}
}

func TestReportPublicationJSONArtifactKeepsOneSnapshotRecord(t *testing.T) {
	roots := testRootSet(t)
	targetIdentityDigest := strings.Repeat("b", 64)
	body := `{"controlledArtifact":"still opaque at the snapshot layer"}`
	directory := filepath.Join(
		roots.DataDir,
		"private",
		"report-publication",
		"artifacts",
		targetIdentityDigest[:2],
	)
	mustMkdirAll(t, directory)
	mustWrite(t, filepath.Join(directory, targetIdentityDigest+".json"), body)

	snapshot, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	label := "data/private/report-publication/artifacts/" +
		targetIdentityDigest[:2] + "/" + targetIdentityDigest + ".json"
	entry, found := snapshotEntry(snapshot, label)
	if !found || entry.RecordCount != 1 || entry.SHA256 != domainsecurity.SHA256Hex([]byte(body)) {
		t.Fatalf("JSON artifact snapshot compatibility changed: %#v", entry)
	}
}

func TestReportPublicationArtifactSnapshotRejectsEmptyAndSameSizeMutation(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		roots := testRootSet(t)
		digest := strings.Repeat("c", 64)
		directory := filepath.Join(
			roots.DataDir,
			"private",
			"report-publication",
			"artifacts",
			digest[:2],
		)
		mustMkdirAll(t, directory)
		mustWrite(t, filepath.Join(directory, digest+".json"), "")
		_, err := CaptureStrict(roots)
		assertIntegrityCode(t, err, "invalid_opaque_size")
	})

	t.Run("same-size mutation", func(t *testing.T) {
		roots := testRootSet(t)
		digest := strings.Repeat("d", 64)
		directory := filepath.Join(
			roots.DataDir,
			"private",
			"report-publication",
			"artifacts",
			digest[:2],
		)
		mustMkdirAll(t, directory)
		path := filepath.Join(directory, digest+".json")
		mustWrite(t, path, "%PDF-one")
		_, err := captureStrictWithHook(roots, func() {
			mustWrite(t, path, "%PDF-two")
		})
		assertIntegrityCode(t, err, "snapshot_changed")
	})
}

func TestReportPublicationSemanticLeavesRemainStrictJSON(t *testing.T) {
	digest := strings.Repeat("e", 64)
	for _, partition := range []string{
		"receipts",
		"commit-receipts",
		"indexes",
		"claim-ledgers",
		"pii-projections",
		"render-inspections",
	} {
		t.Run(partition, func(t *testing.T) {
			roots := testRootSet(t)
			directory := filepath.Join(
				roots.DataDir,
				"private",
				"report-publication",
				partition,
				digest[:2],
			)
			mustMkdirAll(t, directory)
			mustWrite(t, filepath.Join(directory, digest+".json"), "%PDF-not-semantic-JSON")
			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, "invalid_json")
		})
	}
}

func TestReportPublicationArtifactOpaqueLabelRequiresExactCASPath(t *testing.T) {
	digest := strings.Repeat("a", 64)
	valid := "data/private/report-publication/artifacts/aa/" + digest + ".json"
	if !privateOpaqueCASRecordLabel(valid) {
		t.Fatalf("exact report publication artifact was not recognized: %s", valid)
	}
	for _, label := range []string{
		"data/private/report-publication/receipts/aa/" + digest + ".json",
		"data/private/report-publication/artifacts/bb/" + digest + ".json",
		"data/private/report-publication/artifacts/aa/" + strings.ToUpper(digest) + ".json",
		"data/private/report-publication/artifacts/aa/extra/" + digest + ".json",
		"data/private/report-publication/artifacts/aa/" + digest + ".bin",
	} {
		if privateOpaqueCASRecordLabel(label) {
			t.Fatalf("noncanonical report publication artifact was accepted: %s", label)
		}
	}
}

func TestEveryRuntimePrivateCASRootClassifiesWriteResidueByRecoveryBoundary(t *testing.T) {
	digest := strings.Repeat("a", 64)
	name := "." + digest + ".json-000000000000000000000001.tmp"
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		t.Run(strings.ReplaceAll(spec.RootID, "/", "_"), func(t *testing.T) {
			label := "data/private/" + spec.RelativeCASRoot + "/" + digest[:2] + "/" + name
			if !privateAuthorityTempLabelCandidate(label) ||
				!privateAuthorityJournalRemovableResidue(label) {
				t.Fatalf("runtime private CAS write residue was not exactly classified: %s", label)
			}
			roots := testRootSet(t)
			directory := filepath.Join(
				roots.DataDir,
				"private",
				filepath.FromSlash(spec.RelativeCASRoot),
				digest[:2],
			)
			mustMkdirAll(t, directory)
			mustWrite(t, filepath.Join(directory, name), `{"schemaVersion":"private-cas-write-residue.v1"}`)
			snapshot, err := CaptureStrict(roots)
			if spec.RecoveryGroupID == domainprivatecas.EvidenceRegistryRecoveryGroupID {
				if err != nil {
					t.Fatalf("deferred evidence-registry residue was not frozen in the strict snapshot: %v", err)
				}
				found := false
				for _, entry := range snapshot.Entries {
					if entry.Path == label && entry.Type == "file" && entry.Size > 0 && entry.SHA256 != "" {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("deferred evidence-registry residue was omitted from the strict snapshot: %s", label)
				}
				return
			}
			assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
		})
	}
}

func TestEveryRuntimePrivateCASRootRejectsRecoveryTransactionResidue(t *testing.T) {
	digest := strings.Repeat("b", 64)
	transactionID := strings.Repeat("c", 64)
	original := "." + digest + ".json-000000000000000000000001.tmp"
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		for _, kind := range []domainprivatecas.ResidueKindV1{
			domainprivatecas.ResidueRecoveryStageV1,
			domainprivatecas.ResidueRecoveryCommitV1,
		} {
			name, ok := domainprivatecas.RecoveryQuarantineNameV1(
				kind,
				transactionID,
				original,
				digest[:2],
			)
			if !ok {
				t.Fatalf("build %s residue for %s", kind, spec.RootID)
			}
			t.Run(strings.ReplaceAll(spec.RootID+"_"+string(kind), "/", "_"), func(t *testing.T) {
				label := "data/private/" + spec.RelativeCASRoot + "/" + digest[:2] + "/" + name
				classified := classifyPrivateAuthorityResidueLabel(label)
				if classified.state != privateAuthorityResidueKnown || classified.journalRemovable {
					t.Fatalf("recovery residue classification = %#v", classified)
				}
				roots := testRootSet(t)
				directory := filepath.Join(
					roots.DataDir,
					"private",
					filepath.FromSlash(spec.RelativeCASRoot),
					digest[:2],
				)
				mustMkdirAll(t, directory)
				mustWrite(t, filepath.Join(directory, name), "quarantined-private-CAS-residue")
				_, err := CaptureStrict(roots)
				assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
			})
		}
	}
}

func TestMalformedRuntimePrivateCASResidueFailsClosedInStrictSnapshot(t *testing.T) {
	digest := strings.Repeat("d", 64)
	for _, relative := range []string{
		"report-publication/unknown/dd/." + digest + ".json-000000000000000000000001.tmp",
		"Report-Publication/artifacts/dd/." + digest + ".json-000000000000000000000001.tmp",
		"report-publication/artifacts/zz/." + digest + ".json-000000000000000000000001.tmp",
		"report-publication/artifacts/extra/dd/." + digest + ".json-000000000000000000000001.tmp",
		"report-publication/artifacts/dd/.not-a-digest.json-000000000000000000000001.tmp",
		"evidence-registry/Indexes/dd/." + digest + ".json-000000000000000000000001.tmp",
		"evidence-registry/indexes/zz/." + digest + ".json-000000000000000000000001.tmp",
		"evidence-registry/capsules/extra/dd/." + digest + ".json-000000000000000000000001.tmp",
	} {
		t.Run(strings.ReplaceAll(relative, "/", "_"), func(t *testing.T) {
			label := "data/private/" + relative
			if classified := classifyPrivateAuthorityResidueLabel(label); classified.state != privateAuthorityResidueMalformed {
				t.Fatalf("malformed residue classification = %#v", classified)
			}
			roots := testRootSet(t)
			path := filepath.Join(roots.DataDir, "private", filepath.FromSlash(relative))
			mustMkdirAll(t, filepath.Dir(path))
			mustWrite(t, path, "malformed-private-CAS-residue")
			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
		})
	}
}

func TestPrivateCASCreateResidueRequiresExactKnownComponentOrShard(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		relative string
	}{
		{
			name: "known owner leaf",
			relative: "report-publication/" +
				domainprivatecas.CreateDirectoryResidueNameV1("artifacts"),
		},
		{
			name: "known shard",
			relative: "report-publication/artifacts/" +
				domainprivatecas.CreateDirectoryResidueNameV1("af"),
		},
		{
			name: "wrong component hash",
			relative: "report-publication/" +
				domainprivatecas.CreateDirectoryResidueNameV1("not-a-runtime-root"),
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			label := "data/private/" + fixture.relative
			classified := classifyPrivateAuthorityResidueLabel(label)
			if fixture.name == "wrong component hash" {
				if classified.state != privateAuthorityResidueMalformed {
					t.Fatalf("wrong create residue classification = %#v", classified)
				}
			} else if classified.state != privateAuthorityResidueKnown ||
				classified.kind != domainprivatecas.ResidueCreateDirectoryV1 {
				t.Fatalf("known create residue classification = %#v", classified)
			}
			roots := testRootSet(t)
			path := filepath.Join(roots.DataDir, "private", filepath.FromSlash(fixture.relative))
			mustMkdirAll(t, filepath.Dir(path))
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
		})
	}
}

func TestStrictSnapshotRejectsPrivateRootCreateResidueAndMalformedLookalikes(t *testing.T) {
	for _, name := range []string{
		domainprivatecas.CreateDirectoryResidueNameV1("private"),
		strings.ToUpper(domainprivatecas.CreateDirectoryResidueNameV1("private")),
		".analytix-cas-create-not-a-digest.tmp",
	} {
		t.Run(name, func(t *testing.T) {
			roots := testRootSet(t)
			mustMkdirAll(t, roots.DataDir)
			path := filepath.Join(roots.DataDir, name)
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
		})
	}
}

func TestStrictSnapshotRejectsMalformedCreateResidueInsidePrivateTopology(t *testing.T) {
	for _, name := range []string{
		strings.ToUpper(domainprivatecas.CreateDirectoryResidueNameV1("artifacts")),
		".analytix-cas-create-not-a-digest.tmp",
	} {
		t.Run(name, func(t *testing.T) {
			roots := testRootSet(t)
			path := filepath.Join(roots.DataDir, "private", "report-publication", name)
			mustMkdirAll(t, filepath.Dir(path))
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
		})
	}
}

func TestReportPublicationTempResidueRequiresExactKnownPartition(t *testing.T) {
	digest := strings.Repeat("b", 64)
	partitions := []string{
		"receipts",
		"commit-receipts",
		"indexes",
		"claim-ledgers",
		"pii-projections",
		"render-inspections",
		"artifacts",
	}
	for _, partition := range partitions {
		label := "data/private/report-publication/" + partition + "/bb/." +
			digest + ".json-000000000000000000000001.tmp"
		if !privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("known report publication residue was not recognized: %s", label)
		}
	}
	for _, label := range []string{
		"data/private/report-publication/unknown/bb/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/report-publication/artifacts/zz/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/Report-Publication/artifacts/bb/." + digest + ".json-000000000000000000000001.tmp",
		"data/private/report-publication/artifacts/bb/" + digest + ".json",
		"data/private/report-publication/artifacts/extra/bb/." + digest + ".json-000000000000000000000001.tmp",
	} {
		if privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("unknown report publication residue was accepted: %s", label)
		}
	}
}

func TestStrictSnapshotRejectsReportPublicationSingleLinkResidueUntilCASRecovery(t *testing.T) {
	digest := strings.Repeat("b", 64)
	for _, partition := range []string{
		"receipts",
		"commit-receipts",
		"indexes",
		"claim-ledgers",
		"pii-projections",
		"render-inspections",
		"artifacts",
	} {
		t.Run(partition, func(t *testing.T) {
			roots := testRootSet(t)
			directory := filepath.Join(
				roots.DataDir,
				"private",
				"report-publication",
				partition,
				digest[:2],
			)
			mustMkdirAll(t, directory)
			mustWrite(
				t,
				filepath.Join(directory, "."+digest+".json-000000000000000000000001.tmp"),
				`{"schemaVersion":"report-publication-crash-residue.v1"}`,
			)

			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
		})
	}
}

func TestContinuationTempResidueRequiresExactKnownPartition(t *testing.T) {
	digest := strings.Repeat("f", 64)
	for _, label := range []string{
		"data/private/gate-continuations/receipts-v2/ff/." + digest + ".json-crash.tmp",
		"data/private/gate-continuations/dispositions-v2/ff/." + digest + ".json-pre-rename.tmp",
	} {
		if !privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("known continuation residue was not recognized: %s", label)
		}
	}
	for _, label := range []string{
		"data/private/gate-continuations/receipts/ff/." + digest + ".json-crash.tmp",
		"data/private/gate-continuations/unknown/ff/." + digest + ".json-crash.tmp",
		"data/private/gate-continuations/receipts-v2/zz/." + digest + ".json-crash.tmp",
		"data/private/gate-continuations/receipts-v2/ff/" + digest + ".json",
	} {
		if privateAuthorityTempLabelCandidate(label) {
			t.Fatalf("unknown continuation residue was accepted: %s", label)
		}
	}
}

func TestStrictSnapshotRejectsSingleLinkFinalAuthorityCrashResidueUntilCASRecovery(t *testing.T) {
	roots := testRootSet(t)
	digest := strings.Repeat("c", 64)
	fixtures := []struct {
		name      string
		directory string
		tempName  string
		finalName string
		winner    bool
	}{
		{
			name:      "accepted-final-pre-rename",
			directory: filepath.Join(roots.DataDir, "private", "accepted-finals", "records", digest[:2]),
			tempName:  "." + digest + ".json-pre-rename.tmp",
			finalName: digest + ".json",
		},
		{
			name:      "pending-work-lost-no-replace-race",
			directory: filepath.Join(roots.DataDir, "private", "pending-work", "receipts", digest[:2]),
			tempName:  "." + digest + ".json-loser.tmp",
			finalName: digest + ".json",
			winner:    true,
		},
		{
			name:      "continuation-receipt-pre-rename",
			directory: filepath.Join(roots.DataDir, "private", "gate-continuations", "receipts-v2", digest[:2]),
			tempName:  "." + digest + ".json-pre-rename.tmp",
			finalName: digest + ".json",
		},
		{
			name:      "continuation-disposition-lost-no-replace-race",
			directory: filepath.Join(roots.DataDir, "private", "gate-continuations", "dispositions-v2", digest[:2]),
			tempName:  "." + digest + ".json-loser.tmp",
			finalName: digest + ".json",
			winner:    true,
		},
		{
			name:      "provider-cache-settlement-pre-rename",
			directory: filepath.Join(roots.DataDir, "private", "provider-cache-telemetry", "settlements", digest[:2]),
			tempName:  "." + digest + ".json-pre-rename.tmp",
			finalName: digest + ".json",
		},
		{
			name:      "case-thread-authority-pre-rename",
			directory: filepath.Join(roots.DataDir, "private", "case-thread-authority", digest[:2]),
			tempName:  "." + digest + ".json-pre-rename.tmp",
			finalName: digest + ".json",
		},
		{
			name:      "authority-key-pre-rename",
			directory: filepath.Join(roots.DataDir, "private", "authority"),
			tempName:  ".final-answer-ed25519-v1.json-pre-rename.tmp",
			finalName: "final-answer-ed25519-v1.json",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			mustMkdirAll(t, fixture.directory)
			tempPath := filepath.Join(fixture.directory, fixture.tempName)
			mustWrite(t, tempPath, `{"schemaVersion":2}`)
			if fixture.winner {
				mustWrite(t, filepath.Join(fixture.directory, fixture.finalName), `{"schemaVersion":2,"winner":true}`)
			}
			_, err := CaptureStrict(roots)
			assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
			if err := os.Remove(tempPath); err != nil {
				t.Fatal(err)
			}
			if fixture.winner {
				if err := os.Remove(filepath.Join(fixture.directory, fixture.finalName)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestStrictSnapshotRejectsNoncanonicalFinalAuthorityTemp(t *testing.T) {
	roots := testRootSet(t)
	digest := strings.Repeat("d", 64)
	directory := filepath.Join(roots.DataDir, "private", "accepted-finals", "records", digest[:2])
	mustMkdirAll(t, directory)
	mustWrite(t, filepath.Join(directory, ".not-a-content-address.json-crash.tmp"), `{"schemaVersion":2}`)

	_, err := CaptureStrict(roots)
	assertIntegrityCode(t, err, "unknown_final_authority_crash_temp")
}

func TestStrictSnapshotIsDeterministicForValidAuthorityData(t *testing.T) {
	roots := testRootSet(t)
	threadDir := filepath.Join(roots.DurableDir, "threads", "thr_valid")
	mustMkdirAll(t, threadDir)
	mustWrite(t, filepath.Join(threadDir, "thread.json"), `{"id":"thr_valid","turns":[]}`)
	mustWrite(t, filepath.Join(threadDir, "events.jsonl"), "{\"kind\":\"heartbeat\",\"threadId\":\"thr_valid\",\"seq\":1}\n")
	mustMkdirAll(t, filepath.Join(roots.DataDir, "private", "authority"))
	mustWrite(t, filepath.Join(roots.DataDir, "private", "authority", "key.json"), `{"schemaVersion":1}`)

	first, err := CaptureStrict(roots)
	if err != nil {
		t.Fatalf("capture first snapshot: %v", err)
	}
	second, err := CaptureStrict(roots)
	if err != nil {
		t.Fatalf("capture second snapshot: %v", err)
	}
	if first.SHA256 == "" || first.SHA256 != second.SHA256 || first.FileCount != 3 {
		t.Fatalf("snapshot mismatch: first=%#v second=%#v", first, second)
	}
}

func TestStrictSnapshotBindsManagedDirectoryAbsenceAndMode(t *testing.T) {
	roots := testRootSet(t)
	absent, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := snapshotEntry(absent, "data/attachments"); !ok || entry.Type != "absent" {
		t.Fatalf("absent managed directory was not bound: %#v", absent.Entries)
	}
	attachments := filepath.Join(roots.DataDir, "attachments")
	if err := os.MkdirAll(attachments, 0o700); err != nil {
		t.Fatal(err)
	}
	empty, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := snapshotEntry(empty, "data/attachments"); !ok || entry.Type != "directory" || absent.SHA256 == empty.SHA256 {
		t.Fatalf("empty managed directory was indistinguishable from absence: entry=%#v", entry)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(attachments, 0o750); err != nil {
			t.Fatal(err)
		}
		modeChanged, err := CaptureStrict(roots)
		if err != nil {
			t.Fatal(err)
		}
		if modeChanged.SHA256 == empty.SHA256 {
			t.Fatal("managed directory mode change did not alter the snapshot digest")
		}
	}
}

func TestStrictSnapshotIncludesAttachmentsAndMCPSchemaCache(t *testing.T) {
	roots := testRootSet(t)
	attachment := filepath.Join(roots.DataDir, "attachments", "content", "payload.bin")
	mustMkdirAll(t, filepath.Dir(attachment))
	mustWrite(t, attachment, "first")
	cachePath := filepath.Join(roots.DataDir, "mcp-schema-cache", "docs.json")
	mustMkdirAll(t, filepath.Dir(cachePath))
	mustWrite(t, cachePath, `{"schemaVersion":5,"tools":[]}`)
	first, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, attachment, "other")
	second, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 == second.SHA256 {
		t.Fatal("attachment content change was outside the managed snapshot")
	}
	mustWrite(t, cachePath, `{"schemaVersion":5,"schemaVersion":4}`)
	_, err = CaptureStrict(roots)
	assertIntegrityCode(t, err, "invalid_json")
}

func TestResolveRootSetCanonicalizesExistingSymlinkWithoutCreatingRoots(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	mustMkdirAll(t, realRoot)
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(realRoot, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	roots, err := ResolveRootSet(filepath.Join(alias, "data"), filepath.Join(alias, "durable"))
	if err != nil {
		t.Fatalf("resolve roots: %v", err)
	}
	canonicalRealRoot, err := filepath.EvalSymlinks(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	if roots.DataDir != filepath.Join(canonicalRealRoot, "data") || roots.DurableDir != filepath.Join(canonicalRealRoot, "durable") {
		t.Fatalf("canonical roots mismatch: %#v", roots)
	}
	if _, err := os.Stat(roots.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("root resolution must not create data directory: %v", err)
	}
}

func TestResolveRootSetRejectsOverlappingNonEquivalentRoots(t *testing.T) {
	base := t.TempDir()
	if _, err := ResolveRootSet(base, filepath.Join(base, "private", "durable")); err == nil {
		t.Fatal("nested persistence roots should fail before first-start writes")
	}
	if _, err := ResolveRootSet(base, base); err != nil {
		t.Fatalf("equal persistence roots are a supported topology: %v", err)
	}
}

func testRootSet(t *testing.T) RootSet {
	t.Helper()
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatalf("resolve test roots: %v", err)
	}
	return roots
}

func snapshotEntry(snapshot RawSnapshot, path string) (EntryRecord, bool) {
	for _, entry := range snapshot.Entries {
		if entry.Path == path {
			return entry, true
		}
	}
	return EntryRecord{}, false
}

func assertIntegrityCode(t *testing.T, err error, code string) {
	t.Helper()
	var integrityErr IntegrityError
	if !errors.As(err, &integrityErr) || integrityErr.Code != code {
		t.Fatalf("expected integrity code %q, got %v", code, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
