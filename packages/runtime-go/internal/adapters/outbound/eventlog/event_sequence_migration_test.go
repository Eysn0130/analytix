package eventlog

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	turnterminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

func TestValidateCurrentTerminalEventGroupsV1RejectsResignedMalformedHistoricalV1Semantics(t *testing.T) {
	fixture, err := turnterminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := appturn.BuildAcceptedFinalPublicationPlan(
		fixture.PrivateFinal.AcceptedFinal,
		fixture.PrivateFinal.RenderedText,
		fixture.PrivateFinal.PublicationIntent,
	)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]map[string]any, 0, len(plan.Events))
	for index, planned := range plan.Events {
		event := contracts.CloneMap(planned.Draft)
		event["seq"] = float64(index + 1)
		events = append(events, event)
	}
	privateItem := contracts.CloneMap(plan.Completion.AssistantItem)
	events[0]["item"] = privateItem
	events[0]["itemId"] = contracts.StringField(privateItem, "id")
	delete(events[0], "publicationPayloadDigest")
	events[0]["publicationPayloadDigest"] = appturn.AcceptedFinalPublicationPayloadDigest(events[0])
	if err := domainevidence.ValidateHistoricalAcceptedFinalDeliveryEventsV1(events); err != nil {
		t.Fatalf("valid frozen historical V1 fixture was rejected: %v", err)
	}

	record := fixture.PrivateFinal.AcceptedFinal
	core := *record.PublicView
	core.ReceiptMetadata.Projection = "count_only"
	record.PublicView = &core
	coreBody, _ := json.Marshal(core)
	coreDigest := sha256.Sum256(append([]byte("analytix.accepted-final-public-view/v2\x00"), coreBody...))
	record.PublicViewDigest = hex.EncodeToString(coreDigest[:])
	record.AuthoritySignature = ""
	record.RecordDigest = ""
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(fixture.PrivateKey, domainevidence.AcceptedFinalSigningBytes(record)),
	)
	recordBody, _ := json.Marshal(record)
	recordDigest := sha256.Sum256(recordBody)
	record.RecordDigest = hex.EncodeToString(recordDigest[:])
	privateItem["acceptedFinal"] = domainevidence.AcceptedFinalRecordMap(record)
	view := privateItem["acceptedFinalView"].(map[string]any)
	view["publicViewDigest"] = record.PublicViewDigest
	view["acceptedFinalDigest"] = record.RecordDigest
	view["receiptMetadata"].(map[string]any)["projection"] = "count_only"

	for _, event := range events {
		event["acceptedFinalDigest"] = record.RecordDigest
		event["publicationCommitId"] = record.RecordDigest
		slot := contracts.StringField(event, "publicationSlot")
		eventID := sha256.Sum256([]byte("analytix.accepted-final-event/v1\x00" + record.RecordDigest + "\x00" + slot))
		event["publicationEventId"] = hex.EncodeToString(eventID[:])
		delete(event, "publicationPayloadDigest")
		event["publicationPayloadDigest"] = appturn.AcceptedFinalPublicationPayloadDigest(event)
	}
	if err := domainevent.ValidateAcceptedFinalDeliveryEventsV1(events); err != nil {
		t.Fatalf("framing-only validator did not isolate the semantic counterexample: %v", err)
	}
	if err := domainevidence.ValidateHistoricalAcceptedFinalDeliveryEventsV1(events); err == nil {
		t.Fatal("evidence owner accepted a resigned malformed historical V1 public core")
	}
	if err := validateCurrentTerminalEventGroupsV1(events); err == nil {
		t.Fatal("startup migration accepted a resigned malformed historical V1 authority group")
	}
}

func TestMigrateLegacyEventSequenceOrderV1SortsRawRecordsWithoutRenumbering(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_legacy_sequence"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "metadata.jsonl"), []byte("{\"kind\":\"thread_metadata\",\"thread\":{\"id\":\"thr_legacy_sequence\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "events.jsonl")
	line7 := []byte(`{"kind":"heartbeat","threadId":"thr_legacy_sequence","seq":7,"rawSpacing":"kept"}` + "\n")
	line8 := []byte(`{"seq":8,"threadId":"thr_legacy_sequence","kind":"heartbeat"}` + "\n")
	line9 := []byte(`{"threadId":"thr_legacy_sequence","kind":"heartbeat","seq":9}` + "\n")
	line10 := []byte(`{"kind":"heartbeat","seq":10,"threadId":"thr_legacy_sequence"}` + "\n")
	legacy := bytes.Join([][]byte{line7, line9, line8, line10}, nil)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	input := legacyEventSequenceMigrationInput{Root: root}
	if err := migrateLegacyEventSequenceOrderV1(input); err != nil {
		t.Fatal(err)
	}
	want := bytes.Join([][]byte{line7, line8, line9, line10}, nil)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("event records were not reordered byte-for-byte\nwant=%s\ngot=%s", want, got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("canonicalized event file mode = %o, want 600", info.Mode().Perm())
	}
	if err := migrateLegacyEventSequenceOrderV1(input); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if !bytes.Equal(again, want) {
		t.Fatal("repeat event sequence migration was not byte-stable")
	}
}

func TestMigrateLegacyEventSequenceOrderV1SortsPrimaryThreadAuthority(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_primary_sequence"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thr_primary_sequence","turns":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	line41 := []byte(`{"kind":"tool_call_started","threadId":"thr_primary_sequence","turnId":"turn_durable_102","callId":"call_redacted","seq":1041}` + "\n")
	line42 := []byte(`{"kind":"tool_progress","threadId":"thr_primary_sequence","turnId":"turn_durable_102","callId":"call_redacted","message":"progress-redacted-1","seq":1042}` + "\n")
	line43 := []byte(`{"kind":"tool_progress","threadId":"thr_primary_sequence","turnId":"turn_durable_102","callId":"call_redacted","message":"progress-redacted-2","seq":1043}` + "\n")
	line44 := []byte(`{"kind":"tool_progress","threadId":"thr_primary_sequence","turnId":"turn_durable_102","callId":"call_redacted","message":"progress-redacted-3","seq":1044}` + "\n")
	path := filepath.Join(threadDir, "events.jsonl")
	before := bytes.Join([][]byte{line41, line43, line42, line44}, nil)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeHashes := eventLineHashesForTest(before)

	if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	want := bytes.Join([][]byte{line41, line42, line43, line44}, nil)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("primary event records were not canonicalized byte-for-byte\nwant=%s\ngot=%s", want, got)
	}
	if len(got) != len(before) || !equalStringsForTest(eventLineHashesForTest(got), beforeHashes) {
		t.Fatal("event migration changed record bytes, count, or total size")
	}
}

func TestMigrateLegacyEventSequenceOrderV1RejectsCurrentAuthorityMarkersAtAnyDepth(t *testing.T) {
	markers := []string{
		`"acceptedFinal":null`,
		`"nested":{"publicationCommitId":"forged"}`,
		`"nested":{"generalTerminalCommitId":"forged"}`,
		`"kind":"accepted_final_batch"`,
		`"approvalItemId":null`,
		`"approvalId":"approval-current","continuationReceiptId":"receipt-current"`,
		`"datasetSnapshotId":"snapshot-current"`,
	}
	for index, marker := range markers {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			root := t.TempDir()
			threadID := "thr_marker_sequence"
			threadDir := filepath.Join(root, "threads", threadID)
			if err := os.MkdirAll(threadDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thr_marker_sequence","turns":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			before := []byte("{\"kind\":\"tool_progress\",\"threadId\":\"thr_marker_sequence\",\"seq\":1}\n" +
				"{\"threadId\":\"thr_marker_sequence\",\"seq\":3," + marker + "}\n" +
				"{\"kind\":\"tool_progress\",\"threadId\":\"thr_marker_sequence\",\"seq\":2}\n")
			path := filepath.Join(threadDir, "events.jsonl")
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err == nil {
				t.Fatal("current authority marker was reordered")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("rejected marker mutated source: err=%v", err)
			}
		})
	}
}

func TestMigrateLegacyEventSequenceOrderV1DoesNotPromoteNestedContentKeysToAuthority(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_nested_content_sequence"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thr_nested_content_sequence","turns":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	line1 := []byte(`{"kind":"tool_progress","threadId":"thr_nested_content_sequence","seq":1}` + "\n")
	line2 := []byte(`{"kind":"tool_progress","threadId":"thr_nested_content_sequence","seq":2,"details":{"caseId":"sk-untrusted","workspaceRealPath":"/tmp/sk-untrusted","approvalId":"approval-untrusted"}}` + "\n")
	line3 := []byte(`{"kind":"tool_progress","threadId":"thr_nested_content_sequence","seq":3}` + "\n")
	path := filepath.Join(threadDir, "events.jsonl")
	if err := os.WriteFile(path, bytes.Join([][]byte{line1, line3, line2}, nil), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, bytes.Join([][]byte{line1, line2, line3}, nil)) {
		t.Fatalf("nested ordinary content was promoted to authority or rewritten: err=%v body=%s", err, after)
	}
}

func TestMigrateLegacyEventSequenceOrderV1LeavesOrderedCurrentExecutionMetadataUnchanged(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_ordered_execution_metadata"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thr_ordered_execution_metadata","turns":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "events.jsonl")
	before := []byte("{\"kind\":\"turn_started\",\"threadId\":\"thr_ordered_execution_metadata\",\"seq\":1}\n" +
		"{\"kind\":\"approval_requested\",\"threadId\":\"thr_ordered_execution_metadata\",\"approvalId\":\"legacy-handle\",\"seq\":2}\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err != nil {
		t.Fatalf("ordered historical execution metadata was rejected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("ordered execution metadata was mutated: err=%v", err)
	}
}

func TestMigrateLegacyEventSequenceOrderV1RejectsAtomicBundleResidue(t *testing.T) {
	for _, residue := range []string{
		".events-bundle-v1.tmp",
		".events-bundle-journal-v1.tmp",
		".events-bundle-journal-v1.json",
		".events-bundle-unknown-v9.candidate",
	} {
		t.Run(residue, func(t *testing.T) {
			root := t.TempDir()
			threadID := "thr_residue_sequence"
			threadDir := filepath.Join(root, "threads", threadID)
			if err := os.MkdirAll(threadDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thr_residue_sequence","turns":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(threadDir, residue), []byte("transaction-residue"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(threadDir, "events.jsonl")
			before := []byte("{\"kind\":\"tool_progress\",\"threadId\":\"thr_residue_sequence\",\"seq\":1}\n" +
				"{\"kind\":\"tool_progress\",\"threadId\":\"thr_residue_sequence\",\"seq\":3}\n" +
				"{\"kind\":\"tool_progress\",\"threadId\":\"thr_residue_sequence\",\"seq\":2}\n")
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err == nil {
				t.Fatal("event order was repaired while an atomic bundle transaction remained")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("residue rejection mutated source: err=%v", err)
			}
		})
	}
}

func TestLegacyEventSequenceOrderV1RunsInsideSignedSemanticStartupPlan(t *testing.T) {
	base := t.TempDir()
	roots, err := persistencefs.ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	threadID := "thr_semantic_sequence"
	threadDir := filepath.Join(roots.DurableDir, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thr_semantic_sequence","turns":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	line1041 := []byte(`{"kind":"tool_call_started","threadId":"thr_semantic_sequence","turnId":"turn_redacted","seq":1041}` + "\n")
	line1042 := []byte(`{"kind":"tool_progress","threadId":"thr_semantic_sequence","turnId":"turn_redacted","seq":1042}` + "\n")
	line1043 := []byte(`{"kind":"tool_progress","threadId":"thr_semantic_sequence","turnId":"turn_redacted","seq":1043}` + "\n")
	line1044 := []byte(`{"kind":"tool_progress","threadId":"thr_semantic_sequence","turnId":"turn_redacted","seq":1044}` + "\n")
	before := bytes.Join([][]byte{line1041, line1043, line1042, line1044}, nil)
	after := bytes.Join([][]byte{line1041, line1042, line1043, line1044}, nil)
	eventsPath := filepath.Join(threadDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, before, 0o600); err != nil {
		t.Fatal(err)
	}

	reader := persistencefs.NewStartupSnapshotReader(roots)
	snapshot, err := reader.CaptureManagedSnapshotV1(context.Background())
	if err != nil {
		t.Fatalf("legacy baseline capture: %v", err)
	}
	configurationDigest := domainsecurity.SHA256Hex([]byte("legacy-event-order-integration"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, configurationDigest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	builder := persistencefs.NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configurationDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		return MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{
			Root: stage.DurableDir, ThreadSummaryIndexPath: filepath.Join(stage.DurableDir, "thread_summaries.jsonl"),
		})
	})
	if err != nil {
		t.Fatalf("prepare signed semantic migration: %v", err)
	}
	defer prepared.Close()
	plan := prepared.Plan()
	if domainstartup.ValidateSemanticStartupPlanV1(plan) != nil {
		t.Fatal("event migration plan is not valid")
	}
	var install *domainstartup.SemanticStartupOperationV1
	for index := range plan.Operations {
		candidate := &plan.Operations[index]
		if candidate.Path == filepath.ToSlash(filepath.Join("durable", "threads", threadID, "events.jsonl")) {
			install = candidate
			break
		}
	}
	if install == nil || install.Kind != domainstartup.SemanticOperationInstallFile ||
		install.Before.SHA256 != digestBytesForTest(before) || install.After.SHA256 != digestBytesForTest(after) ||
		install.Before.Size != int64(len(before)) || install.After.Size != int64(len(after)) {
		t.Fatalf("event install operation is not bound to exact before/after bytes: %#v", install)
	}
	liveBefore, err := os.ReadFile(eventsPath)
	if err != nil || !bytes.Equal(liveBefore, before) {
		t.Fatalf("planning mutated the live source: err=%v", err)
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatalf("apply signed semantic migration: %v", err)
	}
	liveAfter, err := os.ReadFile(eventsPath)
	if err != nil || !bytes.Equal(liveAfter, after) {
		t.Fatalf("signed migration did not publish exact sorted bytes: err=%v", err)
	}
	if _, err := persistencefs.CaptureStrict(roots); err != nil {
		t.Fatalf("strict readback rejected sorted fixed point: %v", err)
	}
	if err := persistencefs.NewSemanticPlanBuilder(roots).Recover(context.Background(), configurationDigest); err != nil {
		t.Fatalf("restart recovery rejected committed fixed point: %v", err)
	}
	restarted, err := os.ReadFile(eventsPath)
	if err != nil || !bytes.Equal(restarted, after) {
		t.Fatalf("restart changed committed event bytes: err=%v", err)
	}
}

func TestMigrateLegacyEventSequenceOrderV1RejectsCurrentPrimaryAuthority(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_current_sequence"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(
		`{"id":"thr_current_sequence","securityState":{"contextDigest":"host-authority"},"turns":[]}`,
	), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "events.jsonl")
	before := []byte("{\"kind\":\"heartbeat\",\"threadId\":\"thr_current_sequence\",\"seq\":1}\n" +
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_current_sequence\",\"seq\":3}\n" +
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_current_sequence\",\"seq\":2}\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err == nil {
		t.Fatal("current primary authority was reordered")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("rejected current primary event file was mutated")
	}
}

func TestMigrateLegacyEventSequenceOrderV1RejectsUnownedEventOnlyDirectory(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_unowned_sequence"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "events.jsonl")
	before := []byte("{\"kind\":\"heartbeat\",\"threadId\":\"thr_unowned_sequence\",\"seq\":1}\n" +
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_unowned_sequence\",\"seq\":3}\n" +
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_unowned_sequence\",\"seq\":2}\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err == nil {
		t.Fatal("unowned event-only directory was reordered")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("rejected unowned event file was mutated")
	}
}

func TestMigrateLegacyEventSequenceOrderV1RejectsInvalidSequenceSetsWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "gap",
			body: "{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":7}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":9}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":8}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":11}\n",
		},
		{
			name: "ordered_gap",
			body: "{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":7}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":9}\n",
		},
		{
			name: "duplicate",
			body: "{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":7}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":9}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":8}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":8}\n",
		},
		{
			name: "identity_mismatch",
			body: "{\"kind\":\"heartbeat\",\"threadId\":\"other_thread\",\"seq\":8}\n" +
				"{\"kind\":\"heartbeat\",\"threadId\":\"thr_invalid_sequence\",\"seq\":7}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			threadDir := filepath.Join(root, "threads", "thr_invalid_sequence")
			if err := os.MkdirAll(threadDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(threadDir, "metadata.jsonl"), []byte("{\"kind\":\"thread_metadata\",\"thread\":{\"id\":\"thr_invalid_sequence\"}}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(threadDir, "events.jsonl")
			before := []byte(test.body)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err == nil {
				t.Fatal("invalid event sequence set was accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("rejected event sequence migration mutated its source")
			}
		})
	}
}

func TestMigrateLegacyEventSequenceOrderV1DoesNotCreateMissingEventFile(t *testing.T) {
	root := t.TempDir()
	threadDir := filepath.Join(root, "threads", "thr_without_events")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "events.jsonl")
	if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("missing events file was created: %v", err)
	}
}

func eventLineHashesForTest(body []byte) []string {
	lines := bytes.Split(body, []byte{'\n'})
	hashes := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		sum := sha256.Sum256(append(append([]byte(nil), line...), '\n'))
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	sort.Strings(hashes)
	return hashes
}

func digestBytesForTest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func equalStringsForTest(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
