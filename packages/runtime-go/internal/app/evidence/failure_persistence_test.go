package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestSourceUnavailableFailurePrecedesReportPublicationGuidance(t *testing.T) {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-source-failure", TurnID: "turn-source-failure", WorkspaceRealPath: "/workspace/case",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	var boundary CaseBoundaryResult
	err = PersistRuntimeFailure(context.Background(), PersistRuntimeFailureInput{
		Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Prompt: "请生成正式案件资金分析报告",
		Cause:  apploop.TurnFailureError{Code: "tool_source_unavailable"},
		At:     time.Unix(2, 0),
		FinalizeCase: func(ctx context.Context, input PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error) {
			if !input.SourceUnavailable || !input.ReportRequested || input.TerminalReason != TerminalSourceUnavailable {
				t.Fatalf("failure boundary lost source/report semantics: %#v", input)
			}
			var finalizeErr error
			boundary, finalizeErr = FinalizeCaseBoundary(ctx, nil, CaseBoundaryInput{
				Context: input.Context, TerminalReason: input.TerminalReason,
				SourceUnavailable: input.SourceUnavailable, ReportRequested: input.ReportRequested, IssuedAt: input.AcceptedAt,
			})
			return PersistCaseBoundaryResult{}, finalizeErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if boundary.Envelope.Variant != domainevidence.SourceUnavailableAnswer ||
		len(boundary.Envelope.AcquisitionSteps) != 1 ||
		boundary.Envelope.AcquisitionSteps[0] != "reconnect_and_verify_current_case_source" {
		t.Fatalf("source outage incorrectly requested a PublicationReceipt: %#v", boundary.Envelope)
	}
}

func TestRuntimeCancellationPersistsExplicitGeneralInterruptMetadata(t *testing.T) {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-runtime-cancel", TurnID: "turn-runtime-cancel", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &continuationTerminalStoreStub{}
	if err := PersistRuntimeFailure(context.Background(), PersistRuntimeFailureInput{
		Store: store, Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Cause: context.Canceled, At: time.Unix(2, 0),
	}); err != nil {
		t.Fatal(err)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.fields["generalTerminalPublication"])
	if err != nil || store.status != "aborted" || store.fields["discard"] != false ||
		store.fields["cancelled"] != true || store.fields["cancelledPendingGates"] != 0 ||
		commit.TerminalEvent["discard"] != false || commit.TerminalEvent["cancelled"] != true ||
		commit.TerminalEvent["cancelledPendingGates"] != json.Number("0") {
		t.Fatalf("runtime cancellation lost explicit interrupt authority: status=%q fields=%#v commit=%#v err=%v", store.status, store.fields, commit, err)
	}
}

func TestRuntimeProviderToolArgumentsFailurePersistsExactClosedCode(t *testing.T) {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-invalid-tool-arguments", TurnID: "turn-invalid-tool-arguments", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &continuationTerminalStoreStub{}
	if err := PersistRuntimeFailure(context.Background(), PersistRuntimeFailureInput{
		Store: store, Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Cause: apploop.TurnFailureError{
			Message: "PRIVATE_INVALID_TOOL_ARGUMENTS_SENTINEL", Code: domainfailure.CodeProviderToolArgumentsInvalid,
			Details: map[string]any{"unsafe": "PRIVATE_INVALID_TOOL_ARGUMENTS_SENTINEL"}, Severity: "error",
		},
		At: time.Unix(2, 0),
	}); err != nil {
		t.Fatal(err)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.fields["generalTerminalPublication"])
	encoded, _ := json.Marshal(map[string]any{"items": store.items, "commit": commit})
	if err != nil || store.status != "failed" || commit.TerminalReason != "tool_failure" ||
		commit.TerminalEvent["code"] != domainfailure.CodeProviderToolArgumentsInvalid ||
		strings.Contains(string(encoded), "PRIVATE_INVALID_TOOL_ARGUMENTS_SENTINEL") ||
		strings.Contains(string(encoded), `"details"`) {
		t.Fatalf("runtime tool cause projection mismatch: status=%q commit=%#v body=%s err=%v", store.status, commit, encoded, err)
	}
}

func TestRuntimeHostPublicationFailurePersistsExactClosedPhase(t *testing.T) {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-host-publication-failure", TurnID: "turn-host-publication-failure", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &continuationTerminalStoreStub{}
	cause := apploop.WrapHostBoundaryFailure(
		apploop.HostCandidatePublicationFailure,
		errors.New("PRIVATE_HOST_PUBLICATION_SENTINEL"),
	)
	if err := PersistRuntimeFailure(context.Background(), PersistRuntimeFailureInput{
		Store: store, Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Cause: cause, At: time.Unix(2, 0),
	}); err != nil {
		t.Fatal(err)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.fields["generalTerminalPublication"])
	encoded, _ := json.Marshal(map[string]any{"items": store.items, "commit": commit})
	if err != nil || store.status != "failed" || commit.TerminalReason != "semantic_failure" ||
		commit.TerminalEvent["code"] != domainfailure.CodeHostCandidatePublicationFailed ||
		strings.Contains(string(encoded), "PRIVATE_HOST_PUBLICATION_SENTINEL") ||
		strings.Contains(string(encoded), `"details"`) {
		t.Fatalf("runtime host cause projection mismatch: status=%q commit=%#v body=%s err=%v", store.status, commit, encoded, err)
	}
}

func TestRuntimeProviderOutputPolicyFailurePersistsExactClosedCode(t *testing.T) {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-provider-output-policy", TurnID: "turn-provider-output-policy", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &continuationTerminalStoreStub{}
	if err := PersistRuntimeFailure(context.Background(), PersistRuntimeFailureInput{
		Store: store, Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Cause: domainfailure.NewError(
			domainfailure.CodeProviderReasoningMarkupInvalid,
			map[string]any{"status": 503, "unsafe": "PRIVATE_PROVIDER_BODY"},
		),
		At: time.Unix(2, 0),
	}); err != nil {
		t.Fatal(err)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.fields["generalTerminalPublication"])
	encoded, _ := json.Marshal(map[string]any{"items": store.items, "commit": commit})
	if err != nil || store.status != "failed" || commit.TerminalReason != "provider_failure" ||
		commit.TerminalEvent["code"] != domainfailure.CodeProviderReasoningMarkupInvalid ||
		strings.Contains(string(encoded), "PRIVATE_PROVIDER_BODY") ||
		strings.Contains(string(encoded), `"details"`) {
		t.Fatalf("runtime provider output policy projection mismatch: status=%q commit=%#v body=%s err=%v", store.status, commit, encoded, err)
	}
}
