package checkpoint

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestCheckpointIDsAndAuditOnlyPlan(t *testing.T) {
	checkpointID := RuntimeCheckpointIDFromWorkspaceCheckpointID("abc/def")
	if !strings.HasPrefix(checkpointID, "axcp_v2_") || len(checkpointID) != len("axcp_v2_")+64 ||
		RuntimeCheckpointIDFromWorkspaceCheckpointID("abc/def") != checkpointID {
		t.Fatalf("runtime checkpoint id mismatch: %q", checkpointID)
	}
	for _, collidingLegacyValue := range []string{"abc\\def", "abc:def", "abc_def"} {
		if RuntimeCheckpointIDFromWorkspaceCheckpointID(collidingLegacyValue) == checkpointID {
			t.Fatalf("lossy legacy checkpoint collision remains for %q", collidingLegacyValue)
		}
	}
	if RuntimeCheckpointIDFromWorkspaceCheckpointID(checkpointID) != checkpointID {
		t.Fatalf("canonical checkpoint id should be idempotent")
	}
	if RuntimeCheckpointIDFromWorkspaceCheckpointID("axcp_existing") == "axcp_existing" {
		t.Fatalf("legacy checkpoint id must not bypass the collision-resistant mapping")
	}
	plan := BuildAuditOnlyPlan(AuditPlanInput{
		ThreadID:     "thr_1",
		CheckpointID: "cp_1",
		Workspace:    "/workspace",
		Scope:        "combined",
		CreatedAt:    "now",
	})
	if plan["planId"] != PlanID("cp_1") || plan["destructive"] != false {
		t.Fatalf("audit plan identity mismatch: %#v", plan)
	}
	conversation, _ := plan["conversation"].(map[string]any)
	if conversation["status"] != "blocked" {
		t.Fatalf("audit conversation should be blocked: %#v", conversation)
	}
}

func TestBuildPlanCountsFileStatusesAndConversation(t *testing.T) {
	plan := BuildPlan(PlanInput{
		ThreadID:     "thr_1",
		CheckpointID: "cp_1",
		Workspace:    "/workspace",
		Scope:        "code",
		CreatedAt:    "now",
		Checkpoint:   map[string]any{"checkpointId": "cp_1"},
		Files: []any{
			map[string]any{"status": "ready"},
			map[string]any{"status": "manual_review"},
			map[string]any{"status": "blocked"},
		},
		Conversation: map[string]any{"status": "ready"},
	})
	summary, _ := plan["summary"].(map[string]any)
	if summary["fileCount"] != float64(3) || summary["readyFileCount"] != float64(1) || summary["manualReviewFileCount"] != float64(1) || summary["blockedFileCount"] != float64(1) {
		t.Fatalf("summary mismatch: %#v", summary)
	}
	if _, ok := plan["conversation"].(map[string]any); !ok {
		t.Fatalf("conversation missing: %#v", plan)
	}
}

func TestPlanDigestIsDeterministicAndBindsAuthoritativePlan(t *testing.T) {
	left := map[string]any{
		"threadId":     "thr_1",
		"checkpointId": "axcp_1",
		"files": []any{
			map[string]any{"relativePath": "a.txt", "status": "ready"},
		},
		"summary": map[string]any{"fileCount": float64(1)},
	}
	right := map[string]any{
		"summary":      map[string]any{"fileCount": float64(1)},
		"files":        []any{map[string]any{"status": "ready", "relativePath": "a.txt"}},
		"checkpointId": "axcp_1",
		"threadId":     "thr_1",
	}
	leftDigest := PlanDigest(left)
	if len(leftDigest) != 64 || leftDigest != PlanDigest(right) {
		t.Fatalf("canonical plan digest mismatch: left=%q right=%q", leftDigest, PlanDigest(right))
	}
	right["planDigest"] = strings.Repeat("f", 64)
	if PlanDigest(right) != leftDigest {
		t.Fatal("embedded planDigest must not make the digest self-referential")
	}
	right["threadId"] = "thr_2"
	if PlanDigest(right) == leftDigest {
		t.Fatal("plan digest must bind authoritative identity fields")
	}
}

func TestRewindContextAllowsLaterTurnOnlyAcrossSameStableBoundary(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	base := domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", WorkspaceRealPath: "/workspace", TenantID: "tenant_1", UserID: "user_1",
		CaseID: "case_1", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding_1")),
		DatasetSnapshotID:  securitytest.DatasetSnapshotID("snapshot_1"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest_1")), ContextEpoch: 7,
	}
	base.TurnID, base.IssuedAt = "turn_capture", now
	frozen, err := securitytest.CaseExecutionContextV2(base)
	if err != nil {
		t.Fatal(err)
	}
	base.TurnID, base.IssuedAt = "turn_apply", now.Add(time.Minute)
	current, err := securitytest.CaseExecutionContextV2(base)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.ContextDigest == current.ContextDigest || RewindContextMismatch(frozen, current) != "" {
		t.Fatalf("same-boundary later turn should be allowed: frozen=%q current=%q mismatch=%q", frozen.ContextDigest, current.ContextDigest, RewindContextMismatch(frozen, current))
	}

	cases := []struct {
		name   string
		mutate func(*domainsecurity.TurnSecurityContextInput)
	}{
		{"workspace", func(input *domainsecurity.TurnSecurityContextInput) { input.WorkspaceRealPath = "/other" }},
		{"principal", func(input *domainsecurity.TurnSecurityContextInput) { input.UserID = "user_2" }},
		{"case", func(input *domainsecurity.TurnSecurityContextInput) {
			input.CaseID = "case_2"
			input.CaseBindingHash = domainsecurity.SHA256Hex([]byte("binding_2"))
		}},
		{"epoch", func(input *domainsecurity.TurnSecurityContextInput) { input.ContextEpoch++ }},
		{"dataset", func(input *domainsecurity.TurnSecurityContextInput) {
			input.DatasetSnapshotID = securitytest.DatasetSnapshotID("snapshot_2")
		}},
		{"source manifest", func(input *domainsecurity.TurnSecurityContextInput) {
			input.SourceManifestHash = domainsecurity.SHA256Hex([]byte("manifest_2"))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changedInput := base
			test.mutate(&changedInput)
			changed, err := securitytest.CaseExecutionContextV2(changedInput)
			if err != nil {
				t.Fatal(err)
			}
			if mismatch := RewindContextMismatch(frozen, changed); mismatch == "" {
				t.Fatalf("changed %s boundary was accepted", test.name)
			}
		})
	}
}

func TestCheckpointScopeIsClosed(t *testing.T) {
	for _, scope := range []string{"code", "conversation", "combined"} {
		if !ValidScope(scope) {
			t.Fatalf("valid scope rejected: %q", scope)
		}
	}
	for _, scope := range []string{"", "files", " code ", "both", "CODE"} {
		if ValidScope(scope) {
			t.Fatalf("unknown or non-canonical scope accepted: %q", scope)
		}
	}
}

func TestConversationPlanFromEvents(t *testing.T) {
	plan := ConversationPlanFromEvents("cp_1", []map[string]any{
		{"kind": "turn_started", "turnId": "turn_before", "seq": float64(1)},
		{"kind": "turn_started", "turnId": "turn_cp", "seq": float64(2)},
		{"kind": "checkpoint_captured", "turnId": "turn_cp", "seq": float64(3), "checkpoint": map[string]any{"checkpointId": "cp_1", "turnId": "turn_cp"}},
		{"kind": "item_created", "turnId": "turn_after", "seq": float64(4)},
	})
	if plan["status"] != "ready" || plan["boundarySeq"] != float64(2) || plan["checkpointEventSeq"] != float64(3) {
		t.Fatalf("conversation plan mismatch: %#v", plan)
	}
	removed, _ := plan["removedTurnIds"].([]any)
	if len(removed) != 2 || removed[0] != "turn_cp" || removed[1] != "turn_after" {
		t.Fatalf("removed turns mismatch: %#v", removed)
	}
	blocked := ConversationPlanFromEvents("missing", nil)
	if blocked["status"] != "blocked" {
		t.Fatalf("missing checkpoint should block: %#v", blocked)
	}
}

func TestSnapshotEvidenceContentAndHash(t *testing.T) {
	hash := Hash("hello")
	content, ok, reason := SnapshotContent(map[string]any{
		"before": map[string]any{"encoding": "utf8", "content": "hello", "hash": hash},
	}, "before", DefaultSnapshotMaxBytes)
	if !ok || reason != "" || content.Hash != hash || content.Content != "hello" {
		t.Fatalf("snapshot content mismatch: content=%#v ok=%v reason=%q", content, ok, reason)
	}
	_, ok, reason = SnapshotContent(map[string]any{"before": map[string]any{"encoding": "utf8", "content": "hello", "hash": "bad"}}, "before", DefaultSnapshotMaxBytes)
	if ok || reason == "" {
		t.Fatalf("bad hash should fail: ok=%v reason=%q", ok, reason)
	}
	evidence := SnapshotEvidence([]any{map[string]any{"relativePath": "a.txt", "before": map[string]any{"content": "x"}}})
	if _, ok := evidence["a.txt"]; !ok {
		t.Fatalf("snapshot evidence missing: %#v", evidence)
	}
}

func TestRawSnapshotRoundTripsExactBytesAndPublicProjectionOmitsPrivateAuthority(t *testing.T) {
	raw := []byte{0xff, 0xfe, 'a', 0, '\n', 0}
	hash := HashBytes(raw)
	rootIdentity := "unix:private-root-identity"
	content, ok, reason := SnapshotContent(map[string]any{
		"before": map[string]any{
			"schemaVersion": float64(1), "encoding": "utf16le",
			"bytesBase64": base64.StdEncoding.EncodeToString(raw), "hash": hash,
		},
	}, "before", DefaultSnapshotMaxBytes)
	if !ok || reason != "" || content.Hash != hash || content.Encoding != "utf16le" || string(content.RawBytes) != string(raw) {
		t.Fatalf("raw snapshot mismatch: content=%#v ok=%v reason=%q", content, ok, reason)
	}

	record := map[string]any{
		"schemaVersion": float64(2), "checkpointId": "axcp_private", "threadId": "thr_private", "turnId": "turn_private",
		"workspace": "/workspace", "relativePath": "account.txt", "changeKind": "modified",
		"beforeHash": hash, "afterHash": HashBytes([]byte("after")), "createdAt": "created",
		"pathAuthoritySchemaVersion": float64(1), "authorityKind": "allow_write",
		"authorityRoot": "/private/authorized/account-root", "authorityRootIdentity": rootIdentity,
		"authorityRootHash": HashBytes([]byte("/private/authorized/account-root\x00" + rootIdentity)),
		"before": map[string]any{
			"schemaVersion": float64(1), "encoding": "utf16le",
			"bytesBase64": base64.StdEncoding.EncodeToString(raw), "hash": hash,
		},
	}
	metadata, ok := BuildCapturedCheckpointMetadata(CapturedCheckpointInput{
		ThreadID: "thr_private", TurnID: "turn_private", CheckpointID: "axcp_private", Records: []map[string]any{record},
	})
	if !ok {
		t.Fatal("private operation record did not produce public checkpoint metadata")
	}
	publicBody, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"bytesBase64", base64.StdEncoding.EncodeToString(raw), "/private/authorized/account-root", rootIdentity, `"before":`} {
		if strings.Contains(string(publicBody), forbidden) {
			t.Fatalf("public checkpoint metadata leaked private snapshot material %q: %s", forbidden, publicBody)
		}
	}
	if !strings.Contains(string(publicBody), `"authorityKind":"allow_write"`) || !strings.Contains(string(publicBody), `"authorityRootHash"`) {
		t.Fatalf("public checkpoint metadata omitted non-sensitive root identity: %s", publicBody)
	}
	event, ok := BuildCapturedCheckpointEvent(CapturedCheckpointInput{
		ThreadID: "thr_private", TurnID: "turn_private", CheckpointID: "axcp_private", Records: []map[string]any{record},
	})
	if !ok {
		t.Fatal("private operation record did not produce public checkpoint event")
	}
	eventBody, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"bytesBase64", base64.StdEncoding.EncodeToString(raw), "/private/authorized/account-root", rootIdentity, `"before":`, "account.txt"} {
		if strings.Contains(string(eventBody), forbidden) {
			t.Fatalf("public SSE/history checkpoint event leaked private snapshot material %q: %s", forbidden, eventBody)
		}
	}
}

func TestApplySummaryConversationAndConfirmation(t *testing.T) {
	if !ApplyConfirmationValid(map[string]any{"confirmed": true, "destructive": true, "phrase": "APPLY_CHECKPOINT_REWIND"}) {
		t.Fatal("confirmation should be valid")
	}
	if ApplyConfirmationValid(map[string]any{"confirmed": true}) {
		t.Fatal("partial confirmation should be invalid")
	}
	summary := ApplySummary([]map[string]any{{"status": "applied"}, {"status": "noop"}, {"status": "manual_review"}, {"status": "blocked"}, {"status": "failed"}})
	if summary["fileAppliedCount"] != float64(1) || summary["fileFailedCount"] != float64(1) {
		t.Fatalf("apply summary mismatch: %#v", summary)
	}
	conversation := ConversationApply(map[string]any{"conversation": map[string]any{"boundaryTurnId": "turn_1", "retainedEventCount": float64(2), "removedEventCount": float64(3), "removedTurnIds": []any{"turn_1"}}}, "applied")
	if conversation["status"] != "audit_recorded" || conversation["boundaryTurnId"] != "turn_1" {
		t.Fatalf("conversation apply mismatch: %#v", conversation)
	}
}

func TestRestoreFilePlanAndApplyResponse(t *testing.T) {
	blocked := BuildRestoreFilePlan(RestoreFilePlanInput{
		Changed:   map[string]any{"relativePath": "../secret.txt", "changeKind": "modified"},
		PathError: "checkpoint relativePath escapes workspace",
	})
	if blocked["status"] != "blocked" || blocked["reason"] != "checkpoint relativePath escapes workspace" {
		t.Fatalf("blocked restore plan mismatch: %#v", blocked)
	}
	ready := BuildRestoreFilePlan(RestoreFilePlanInput{
		Changed: map[string]any{"relativePath": "a.txt", "changeKind": "modified", "beforeHash": "before", "afterHash": "after"},
	})
	if ready["action"] != "restore_previous_version" || ready["status"] != "ready" {
		t.Fatalf("ready restore plan mismatch: %#v", ready)
	}
	response := BuildApplyResponse(ApplyResponseInput{
		ThreadID:     "thr_1",
		CheckpointID: "axcp_cp_1",
		Workspace:    "/workspace",
		PlanID:       PlanID("axcp_cp_1"),
		Scope:        "combined",
		CreatedAt:    "now",
		Status:       "applied",
		Plan: map[string]any{"conversation": map[string]any{
			"boundaryTurnId":     "turn_1",
			"retainedEventCount": float64(1),
			"removedEventCount":  float64(2),
			"removedTurnIds":     []any{"turn_1"},
		}},
		Files:    []map[string]any{{"relativePath": "a.txt", "status": "applied"}},
		AuditSeq: 7,
	})
	if response["status"] != "applied" || response["auditEventSeq"] != float64(7) {
		t.Fatalf("apply response mismatch: %#v", response)
	}
	conversation, _ := response["conversation"].(map[string]any)
	if conversation["status"] != "audit_recorded" || conversation["auditEventSeq"] != float64(7) {
		t.Fatalf("apply conversation audit mismatch: %#v", conversation)
	}
	applyEvent := BuildRewindAppliedEvent("thr_1", response)
	if applyEvent["kind"] != "checkpoint_rewind_applied" || applyEvent["threadId"] != "thr_1" {
		t.Fatalf("apply event mismatch: %#v", applyEvent)
	}
	if BuildRewindAppliedEvent(" thr_1 ", response) != nil {
		t.Fatal("apply event must reject normalized outer identity")
	}
	responseWithoutSeq := BuildApplyResponse(ApplyResponseInput{
		ThreadID:     "thr_1",
		CheckpointID: "axcp_cp_1",
		PlanID:       PlanID("axcp_cp_1"),
		Status:       "applied",
		Plan: map[string]any{"conversation": map[string]any{
			"boundaryTurnId":     "turn_1",
			"retainedEventCount": float64(1),
			"removedEventCount":  float64(2),
			"removedTurnIds":     []any{"turn_1"},
		}},
	})
	ApplyResponseWithAuditEventSeq(responseWithoutSeq, map[string]any{"seq": float64(9)})
	if responseWithoutSeq["auditEventSeq"] != float64(9) {
		t.Fatalf("audit seq not applied: %#v", responseWithoutSeq)
	}
	rescue := BuildRescueRecord(RescueRecordInput{
		ThreadID:     "thr_1",
		CheckpointID: "axcp_cp_1",
		PlanID:       PlanID("axcp_cp_1"),
		Workspace:    "/workspace",
		CreatedAt:    "now",
		Files: []RescueFile{
			{RelativePath: "a.txt", Existed: true, Hash: "hash", Content: "content"},
			{RelativePath: "missing.txt", Existed: false, Error: "read failed"},
		},
	})
	files, _ := rescue["files"].([]any)
	if rescue["rescueId"] != RescueID("axcp_cp_1") || len(files) != 2 {
		t.Fatalf("rescue record mismatch: %#v", rescue)
	}
	rescueEvent := BuildRescueCreatedEvent("thr_1", rescue)
	if rescueEvent["kind"] != "checkpoint_rewind_rescue_created" || rescueEvent["threadId"] != "thr_1" {
		t.Fatalf("rescue event mismatch: %#v", rescueEvent)
	}
	if BuildRescueCreatedEvent(" thr_1 ", rescue) != nil {
		t.Fatal("rescue event must reject normalized outer identity")
	}
	privateBody, _ := json.Marshal(rescueEvent)
	if strings.Contains(string(privateBody), "content") || strings.Contains(string(privateBody), "relativePath") || strings.Contains(string(privateBody), "/workspace") {
		t.Fatalf("rescue audit event leaked private payload: %s", privateBody)
	}
	responseWithRescue := BuildApplyResponse(ApplyResponseInput{
		ThreadID: "thr_1", CheckpointID: "axcp_cp_1", Workspace: "/workspace", PlanID: PlanID("axcp_cp_1"),
		Scope: "code", CreatedAt: "2026-07-14T00:00:00Z", Status: "applied", Rescue: rescue,
	})
	responseRescue, _ := responseWithRescue["rescue"].(map[string]any)
	if len(responseRescue) != 2 || responseRescue["rescueId"] != RescueID("axcp_cp_1") || responseRescue["fileCount"] != float64(2) {
		t.Fatalf("HTTP rescue summary is not exact: %#v", responseRescue)
	}
}

func TestApplyFilePreflight(t *testing.T) {
	before := "old"
	after := "new"
	snapshot := map[string]any{
		"relativePath": "a.txt",
		"before": map[string]any{
			"encoding": "utf8",
			"content":  before,
			"hash":     Hash(before),
		},
	}
	ready := BuildApplyFilePreflight(map[string]any{
		"relativePath": "a.txt",
		"action":       "restore_previous_version",
		"beforeHash":   Hash(before),
		"afterHash":    Hash(after),
	}, snapshot, ApplyFileState{
		AbsolutePath: "/workspace/a.txt",
		Exists:       true,
		Current:      FileContent{Hash: Hash(after), Content: after},
	})
	if ready.Status != "apply" || ready.TargetContent != before {
		t.Fatalf("restore preflight mismatch: %#v", ready)
	}
	result := ApplyFilePreflightResult(ready)
	if result["currentHash"] != Hash(after) || result["status"] != "apply" {
		t.Fatalf("preflight result mismatch: %#v", result)
	}
	blocked := BuildApplyFilePreflight(map[string]any{
		"relativePath": "../secret.txt",
		"action":       "restore_deleted_file",
	}, snapshot, ApplyFileState{PathError: "checkpoint relativePath escapes workspace"})
	if blocked.Status != "blocked" || blocked.Reason != "checkpoint relativePath escapes workspace" {
		t.Fatalf("path error preflight mismatch: %#v", blocked)
	}
	noop := BuildApplyFilePreflight(map[string]any{
		"relativePath": "created.txt",
		"action":       "delete_created_file",
		"afterHash":    Hash(after),
	}, nil, ApplyFileState{})
	if noop.Status != "noop" {
		t.Fatalf("delete-created missing file should noop: %#v", noop)
	}
	statBlocked := BuildApplyFilePreflight(map[string]any{
		"relativePath": "created.txt",
		"action":       "delete_created_file",
		"afterHash":    Hash(after),
	}, nil, ApplyFileState{StatError: "stat failed"})
	if statBlocked.Status != "blocked" || statBlocked.Reason != "stat failed" {
		t.Fatalf("stat error should block: %#v", statBlocked)
	}
}

func TestCapturedCheckpointEventAndEvidence(t *testing.T) {
	completed := map[string]any{
		"schemaVersion":               float64(1),
		"checkpointId":                "cp_1",
		"sourceWorkspaceCheckpointId": "git_1",
		"threadId":                    "thr_1",
		"turnId":                      "turn_1",
		"workspace":                   "/workspace",
		"relativePath":                "a.txt",
		"changeKind":                  "modified",
		"beforeHash":                  Hash("old"),
		"afterHash":                   Hash("new"),
		"createdAt":                   "created",
		"updatedAt":                   "updated",
	}
	event, ok := BuildCapturedCheckpointEvent(CapturedCheckpointInput{
		ThreadID:                    "thr_1",
		TurnID:                      "turn_1",
		CheckpointID:                "cp_1",
		SourceWorkspaceCheckpointID: "git_1",
		Records:                     []map[string]any{completed},
	})
	if !ok || event["kind"] != "checkpoint_captured" {
		t.Fatalf("captured checkpoint event mismatch: %#v ok=%v", event, ok)
	}
	evidence := SnapshotEvidenceFromRecords([]map[string]any{completed})
	if _, ok := evidence["a.txt"]; !ok {
		t.Fatalf("snapshot evidence mismatch: %#v", evidence)
	}
	latest, ok := LatestCapturedCheckpointFromEvents("cp_1", []map[string]any{
		event,
		{"kind": "checkpoint_captured", "checkpoint": map[string]any{"checkpointId": "cp_1", "workspace": "/newer"}},
	})
	if !ok || latest["workspace"] != "/newer" {
		t.Fatalf("latest checkpoint event mismatch: %#v ok=%v", latest, ok)
	}
}
