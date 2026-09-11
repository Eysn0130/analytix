package job

import (
	"encoding/json"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestNormalizePersistedRecordV1RemovesReasoningAndPreservesOrdinaryDiagnostics(t *testing.T) {
	record := Record{
		Name:                    "worker<think>PRIVATE_NAME</think>",
		Label:                   "ordinary label",
		Prompt:                  `{"reasoning_content":"PRIVATE_PROMPT"}`,
		ProfileDescription:      "reviewer<think>PRIVATE_PROFILE</think>",
		DiffSummary:             "2 files changed<think>PRIVATE_DIFF</think>",
		Output:                  "ordinary diagnostic<think>PRIVATE_OUTPUT</think>",
		Error:                   "ordinary error<think>PRIVATE_ERROR</think>",
		AutoContinueError:       "PRIVATE_AUTO",
		CompletionDeliveryError: "PRIVATE_DELIVERY",
		ArtifactPath:            "/private/legacy-job.log",
	}
	projected := NormalizePersistedRecordV1(record)
	if projected.Label != "ordinary label" || projected.Output != "ordinary diagnostic" || projected.Error != "" ||
		projected.FailureCode != FailureChildUnknown ||
		projected.Name != "worker" || projected.ProfileDescription != "reviewer" || projected.DiffSummary != "2 files changed" ||
		projected.Prompt != "" || projected.AutoContinueError != "" || projected.CompletionDeliveryError != "" ||
		projected.ArtifactPath != "" {
		t.Fatalf("persisted projection=%#v", projected)
	}
	if strings.Contains(strings.ToLower(projected.Name+projected.Prompt+projected.Output), "private") {
		t.Fatal("persisted projection retained a private sentinel")
	}
	if ValidatePersistedProjectionV1(record) == nil || ValidatePersistedProjectionV1(projected) != nil {
		t.Fatal("persisted projection validation did not distinguish raw and normalized records")
	}
}

func TestNormalizePersistedRecordV1RecursivelyRemovesNestedPrivateContent(t *testing.T) {
	const sentinel = "PRIVATE_NESTED_REASONING"
	record := Record{
		ChangedFiles:    []ChangedFile{{Path: "safe.go<think>" + sentinel + "</think>", Status: "modified"}},
		MergeDecisions:  []MergeDecision{{Reason: sentinel}},
		CleanupReceipts: []CleanupReceipt{{RetainedReason: sentinel}},
		ConflictReports: []ConflictReport{{
			ConflictSummary: "conflict<think>" + sentinel + "</think>",
			ConflictFiles:   []ChangedFile{{Path: "account-6222020202020202020.csv", Status: "modified"}},
		}},
		RepairPatchReviews: []RepairPatchReview{{
			RepairPatch:    "diff --git a/x b/x\n+<think>" + sentinel + "</think>",
			RejectedReason: sentinel,
		}},
		ChildTodoLists: []ChildTodoList{{
			Scope: "scope<think>" + sentinel + "</think>",
			Items: []ChildTodoItem{{Content: "todo<think>" + sentinel + "</think> account 6222020202020202020"}},
		}},
		ChildTodoProjections: []ChildTodoProjection{{
			Summary:        "summary<think>" + sentinel + "</think>",
			ProjectedItems: []ChildTodoProjectionItem{{Content: "projected<think>" + sentinel + "</think>"}},
		}},
		ProjectionDecisions: []ProjectionDecision{{
			SkippedItems: []SkippedProjectionItem{{Reason: sentinel}},
		}},
	}

	projected := NormalizePersistedRecordV1(record)
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{sentinel, "<think>", "6222020202020202020", "diff --git"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("nested persisted projection retained %q: %s", forbidden, text)
		}
	}
	if len(projected.RepairPatchReviews) != 1 || projected.RepairPatchReviews[0].RepairPatch != "" ||
		projected.ChildTodoLists[0].Items[0].Content != "todo account [ACCOUNT]" ||
		projected.ConflictReports[0].ConflictSummary != "conflict" {
		t.Fatalf("nested persisted projection mismatch: %#v", projected)
	}
	if ValidatePersistedProjectionV1(record) == nil || ValidatePersistedProjectionV1(projected) != nil {
		t.Fatal("nested persisted projection validation did not distinguish raw and normalized records")
	}
}

func TestNormalizePersistedRecordV1DowngradesOnlyExactUnboundLegacySteerV0(t *testing.T) {
	const sentinel = "PRIVATE_LEGACY_STEER"
	record := Record{
		ID: "job-1", ParentThreadID: "thread-1",
		Steers: []SteerMessage{{
			ID: "steer-1", ParentThreadID: "thread-1", ChildRunID: "job-1", JobID: "job-1",
			Text: sentinel, SourceTurnID: "turn-private", Status: "admitted", AdmittedAt: "2026-07-01T00:00:01Z",
			CreatedAt: "2026-07-01T00:00:00Z",
		}},
	}
	projected := NormalizePersistedRecordV1(record)
	steer := projected.Steers[0]
	if steer.Status != "expired" || steer.Text != "" || steer.SourceTurnID != "" || steer.AdmittedAt != "" ||
		steer.ProjectionVersion != SteerMessageProjectionVersionV1 || steer.ContentDigest != SteerMessageContentDigestV1(steer) ||
		!domainsecurity.IsSHA256Hex(steer.QueueAuthorityDigest) || steer.RejectedReason != "legacy_private_steer_removed" {
		t.Fatalf("legacy unbound steer was not reduced to a closed tombstone: %#v", steer)
	}
	encoded, err := json.Marshal(projected)
	if err != nil || strings.Contains(string(encoded), sentinel) || strings.Contains(string(encoded), "turn-private") {
		t.Fatalf("legacy steer tombstone retained private bytes: body=%s err=%v", encoded, err)
	}

	partial := record
	partial.Steers = append([]SteerMessage(nil), record.Steers...)
	partial.Steers[0].ProjectionVersion = SteerMessageProjectionVersionV1
	partialProjected := NormalizePersistedRecordV1(partial)
	if partialProjected.Steers[0].Status == "expired" && partialProjected.Steers[0].RejectedReason == "legacy_private_steer_removed" {
		t.Fatal("partially forged current steer was accepted as the exact legacy-v0 shape")
	}
}

func TestNormalizePersistedRecordV1RemovesConsumedResumeToken(t *testing.T) {
	record := Record{PauseRequests: []PauseRequest{{
		ID: "pause-1", ParentThreadID: "thread-1", ChildRunID: "job-1", JobID: "job-1", Status: "resumed",
		RequestedAt: "2026-07-01T00:00:00Z", PausedAt: "2026-07-01T00:00:01Z", ResumedAt: "2026-07-01T00:00:02Z",
		ResumeToken: "PRIVATE_CONSUMED_RESUME_TOKEN", ResumeTokenIssuedAt: "2026-07-01T00:00:01Z",
		ResumeTokenExpiresAt: "2026-07-02T00:00:01Z",
	}}}
	projected := NormalizePersistedRecordV1(record)
	request := projected.PauseRequests[0]
	if request.ResumeToken != "" || request.ResumeTokenIssuedAt != "2026-07-01T00:00:01Z" ||
		request.ResumeTokenExpiresAt != "2026-07-02T00:00:01Z" || request.ResumedAt != "2026-07-01T00:00:02Z" {
		t.Fatalf("settled pause projection changed audit times or retained the consumed capability: %#v", request)
	}
}

func TestNormalizePersistedBackgroundShellDropsCommandLabelAndBoundContent(t *testing.T) {
	const secret = "background-secret-123456"
	projected := NormalizePersistedRecordV1(Record{
		Kind: "background-shell", Name: "curl", Label: "TOKEN=" + secret + " curl https://example.test",
		Output: "stdout=" + secret, Error: "Authorization: Bearer " + secret,
		SecurityBinding: &SecurityBinding{},
	})
	if projected.Name != "bash" || projected.Label != "background shell" || projected.Output != "" || projected.Error != "" {
		t.Fatalf("security-bound background projection retained content: %#v", projected)
	}
	if strings.Contains(projected.Label+projected.Output+projected.Error, secret) {
		t.Fatalf("security-bound background projection retained credential: %#v", projected)
	}
}
