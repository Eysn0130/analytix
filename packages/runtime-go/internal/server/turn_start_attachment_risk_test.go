package server

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	attachmentauthorityport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestTurnStartAttachmentRiskMissingMetadataFailsBeforeProvider(t *testing.T) {
	handler, threadID, provider, _ := newTurnStartAttachmentRiskHandler(t, workspacetest.New(t))

	_, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt:        "Summarize the supplied attachment.",
		AttachmentIDs: []string{"att_000000000000000000000000"},
	})
	if !errors.Is(err, appturn.ErrAttachmentRiskMetadataMissing) {
		t.Fatalf("missing attachment metadata did not fail closed: %v", err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("missing attachment metadata reached provider: calls=%d", provider.calls.Load())
	}
}

func TestTurnStartAttachmentRiskCorruptMetadataFailsBeforeProvider(t *testing.T) {
	handler, threadID, provider, dataDir := newTurnStartAttachmentRiskHandler(t, workspacetest.New(t))
	metadata := createTurnStartRiskAttachment(t, handler, threadID, t.TempDir(), "corrupt metadata")
	id := stringField(metadata, "id")
	if err := os.WriteFile(turnStartRiskMetadataPath(dataDir, id), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize the supplied attachment.", AttachmentIDs: []string{id},
	})
	if !errors.Is(err, appturn.ErrAttachmentRiskMetadataInvalid) {
		t.Fatalf("corrupt attachment metadata did not fail closed: %v", err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("corrupt attachment metadata reached provider: calls=%d", provider.calls.Load())
	}
}

func TestTurnStartAttachmentRiskPrivateOwnerMismatchFailsBeforeProvider(t *testing.T) {
	workspace := workspacetest.New(t)
	handler, threadID, provider, _ := newTurnStartAttachmentRiskHandler(t, workspace)
	first := createTurnStartRiskAttachment(t, handler, threadID, workspace, "first owner")
	second := createTurnStartRiskAttachment(t, handler, threadID, workspace, "second owner")
	firstOwner := turnStartRiskOwner(t, first)
	secondOwner := turnStartRiskOwner(t, second)
	owners := &turnStartAttachmentRiskOwnerStore{
		owners: map[string]domainattachment.OwnerRecordV1{
			firstOwner.OwnerDigest: secondOwner,
		},
	}
	access := *handler.attachmentAccess
	access.Owners = owners
	handler.attachmentAccess = &access

	_, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize the supplied attachment.", AttachmentIDs: []string{firstOwner.AttachmentID},
	})
	if !errors.Is(err, appturn.ErrAttachmentRiskOwnerMismatch) {
		t.Fatalf("private attachment owner mismatch did not fail closed: %v", err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("private attachment owner mismatch reached provider: calls=%d", provider.calls.Load())
	}
	if owners.resolveCalls != 1 {
		t.Fatalf("private owner authority was not checked exactly once: calls=%d", owners.resolveCalls)
	}
}

func TestTurnStartAttachmentRiskExactOwnersClassifyWithoutContent(t *testing.T) {
	for _, test := range []struct {
		name      string
		workspace func(*testing.T) string
		wantCase  bool
	}{
		{name: "ordinary exact owner", workspace: func(t *testing.T) string { return workspacetest.New(t) }},
		{name: "case-bound exact owner", workspace: writeThreadMutationCaseBinding, wantCase: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := test.workspace(t)
			handler, threadID, _, dataDir := newTurnStartAttachmentRiskHandler(t, workspace)
			metadata := createTurnStartRiskAttachment(t, handler, threadID, workspace, test.name)
			id := stringField(metadata, "id")
			if err := handler.attachmentAccess.CommitUpload(context.Background(), metadata); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(turnStartRiskContentPath(dataDir, id)); err != nil {
				t.Fatal(err)
			}

			got, err := (appturn.AttachmentPlanner{
				Store: handler.attachments, Owners: handler.attachmentAccess.Owners,
			}).PreflightCaseRisk(
				context.Background(), []string{id}, false,
			)
			if err != nil || got != test.wantCase {
				t.Fatalf("attachment risk classification mismatch: got=%v want=%v err=%v", got, test.wantCase, err)
			}
		})
	}
}

func TestTurnStartAttachmentRiskExactGenericOwnerRemainsOrdinary(t *testing.T) {
	workspace := workspacetest.New(t)
	handler, threadID, _, _ := newTurnStartAttachmentRiskHandler(t, workspace)
	metadata := createTurnStartRiskAttachment(t, handler, threadID, workspace, "ordinary attachment")
	if err := handler.attachmentAccess.CommitUpload(context.Background(), metadata); err != nil {
		t.Fatal(err)
	}
	provider := &receiptBoundAttachmentProvider{}
	handler.provider = provider

	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize the supplied attachment.", AttachmentIDs: []string{stringField(metadata, "id")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("exact generic attachment owner did not retain ordinary provider execution: calls=%d", provider.calls)
	}
	thread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	var turn map[string]any
	for _, raw := range listAny(thread["turns"]) {
		candidate, _ := raw.(map[string]any)
		if stringField(candidate, "id") == stringField(response, "turnId") {
			turn = candidate
			break
		}
	}
	securityContext, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if turn == nil || parseErr != nil || !domainsecurity.TurnSecurityContextIsGeneral(securityContext) ||
		turn["acceptedFinal"] != nil {
		t.Fatalf("exact generic attachment owner was upgraded to case risk: turn=%#v context=%#v err=%v", turn, securityContext, parseErr)
	}
}

func TestTurnStartCaseBoundAttachmentWithoutAuthorityUsesHostBoundary(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	handler, threadID, provider, _ := newTurnStartAttachmentRiskHandler(t, workspace)
	configureSteerTestCaseAuthorities(t, handler, handler.store.root)
	metadata := createTurnStartRiskAttachment(t, handler, threadID, workspace, "case attachment")
	if err := handler.attachmentAccess.CommitUpload(context.Background(), metadata); err != nil {
		t.Fatal(err)
	}

	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize the supplied attachment.", AttachmentIDs: []string{stringField(metadata, "id")},
	})
	if err != nil || stringField(response, "status") != "completed" {
		t.Fatalf("case attachment did not close at the host boundary: response=%#v err=%v", response, err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("case attachment without authority reached provider: calls=%d", provider.calls.Load())
	}
	thread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	var turn map[string]any
	for _, raw := range listAny(thread["turns"]) {
		candidate, _ := raw.(map[string]any)
		if stringField(candidate, "id") == stringField(response, "turnId") {
			turn = candidate
			break
		}
	}
	securityContext, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	accepted, _ := turn["acceptedFinal"].(map[string]any)
	if parseErr != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
		stringField(accepted, "variant") != string(domainevidence.SourceUnavailableAnswer) ||
		stringField(accepted, "terminalReason") != "source_unavailable" {
		t.Fatalf("case attachment lost its source-unavailable boundary: turn=%#v context=%#v err=%v", turn, securityContext, parseErr)
	}
}

func TestTurnStartCaseBoundAttachmentWithoutAuthorityKeepsIndependentOrdinaryWork(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	handler, threadID, _, _ := newTurnStartAttachmentRiskHandler(t, workspace)
	configureSteerTestCaseAuthorities(t, handler, handler.store.root)
	metadata := createTurnStartRiskAttachment(t, handler, threadID, workspace, "case attachment")
	if err := handler.attachmentAccess.CommitUpload(context.Background(), metadata); err != nil {
		t.Fatal(err)
	}
	provider := &providerStepRecordingProvider{}
	handler.provider = provider

	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt:        "update ordinary code; inspect current-case funds",
		AttachmentIDs: []string{stringField(metadata, "id")},
	})
	requests := provider.Requests()
	if err != nil || stringField(response, "status") != "completed" || len(requests) != 1 {
		t.Fatalf("case attachment blocked independent ordinary work: response=%#v requests=%d err=%v", response, len(requests), err)
	}
	request := requests[0]
	if request.PrivateAttachmentPlanDigest != "" || len(request.Messages) == 0 || request.Messages[len(request.Messages)-1].Content != "update ordinary code" {
		t.Fatalf("ordinary retry retained protected attachment material: %#v", request)
	}
}

func TestTurnStartExplicitCaseResearchWithoutLexicalCueUsesHostBoundary(t *testing.T) {
	workspace := workspacetest.New(t)
	handler, threadID, provider, _ := newTurnStartAttachmentRiskHandler(t, workspace)
	configureSteerTestCaseAuthorities(t, handler, handler.store.root)
	prompt := "/goal --research Crash before resolving gates."
	if apploop.PromptRequiresCaseRiskAdmission(prompt) {
		t.Fatalf("test prompt unexpectedly acquired lexical case admission: %q", prompt)
	}

	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: prompt, RiskIntent: domainsecurity.RiskClassCase,
	})
	if err != nil || stringField(response, "status") != "completed" {
		t.Fatalf("explicit case risk did not close at the host boundary: response=%#v err=%v", response, err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("explicit case risk without evidence authority reached provider: calls=%d", provider.calls.Load())
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytix", "autoresearch", threadID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("case boundary created AutoResearch state from the blocked prompt: %v", err)
	}
	thread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	var turn map[string]any
	for _, raw := range listAny(thread["turns"]) {
		candidate, _ := raw.(map[string]any)
		if stringField(candidate, "id") == stringField(response, "turnId") {
			turn = candidate
			break
		}
	}
	accepted, _ := turn["acceptedFinal"].(map[string]any)
	if stringField(accepted, "variant") != string(domainevidence.SourceUnavailableAnswer) ||
		stringField(accepted, "terminalReason") != "source_unavailable" {
		t.Fatalf("explicit case risk lost its source-unavailable boundary: turn=%#v", turn)
	}
}

func TestTurnStartPureCaseRiskSkipsAttachmentMetadataAndProvider(t *testing.T) {
	workspace := workspacetest.New(t)
	handler, threadID, provider, dataDir := newTurnStartAttachmentRiskHandler(t, workspace)
	configureSteerTestCaseAuthorities(t, handler, handler.store.root)
	metadata := createTurnStartRiskAttachment(t, handler, threadID, workspace, "known case risk")
	id := stringField(metadata, "id")
	if err := handler.attachmentAccess.CommitUpload(context.Background(), metadata); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(turnStartRiskMetadataPath(dataDir, id), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt := "请分析本案件的资金流入、流出和净额。"
	if !apploop.PromptRequiresCaseRiskAdmission(prompt) {
		t.Fatalf("test prompt did not establish pure case risk: %q", prompt)
	}

	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: prompt, AttachmentIDs: []string{id},
	})
	if err != nil {
		t.Fatalf("pure case boundary read corrupt attachment metadata: %v", err)
	}
	if stringField(response, "status") != "completed" {
		t.Fatalf("pure case boundary did not complete: %#v", response)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("pure case boundary reached provider: calls=%d", provider.calls.Load())
	}
}

func newTurnStartAttachmentRiskHandler(
	t *testing.T,
	workspace string,
) (*runtimeServerHandler, string, *admissionFailureCountingProvider, string) {
	t.Helper()
	durableRoot := t.TempDir()
	dataDir := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "attachment-risk-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "attachment-risk-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &admissionFailureCountingProvider{}
	handler.provider = provider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "attachment risk preflight", "workspace": workspace,
		"providerId": "attachment-risk-provider", "model": "attachment-risk-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	return handler, stringField(thread, "id"), provider, dataDir
}

func createTurnStartRiskAttachment(
	t *testing.T,
	handler *runtimeServerHandler,
	threadID string,
	workspace string,
	content string,
) map[string]any {
	t.Helper()
	metadata, err := handler.attachments.Create(map[string]any{
		"name": "risk.txt", "mimeType": "text/plain",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte(content)),
		"threadId":   threadID, "workspace": workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func turnStartRiskOwner(t *testing.T, metadata map[string]any) domainattachment.OwnerRecordV1 {
	t.Helper()
	owner, err := appturn.AttachmentOwnerAuthorityRecord(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func turnStartRiskMetadataPath(dataDir string, id string) string {
	return filepath.Join(dataDir, "attachments", "metadata", id+".json")
}

func turnStartRiskContentPath(dataDir string, id string) string {
	return filepath.Join(dataDir, "attachments", "content", id+".bin")
}

type turnStartAttachmentRiskOwnerStore struct {
	owners       map[string]domainattachment.OwnerRecordV1
	resolveCalls int
}

func (store *turnStartAttachmentRiskOwnerStore) PutOwnerIfAbsent(
	_ context.Context,
	owner domainattachment.OwnerRecordV1,
) error {
	if store.owners == nil {
		store.owners = map[string]domainattachment.OwnerRecordV1{}
	}
	store.owners[owner.OwnerDigest] = owner
	return nil
}

func (store *turnStartAttachmentRiskOwnerStore) ResolveOwner(
	_ context.Context,
	digest string,
) (domainattachment.OwnerRecordV1, error) {
	store.resolveCalls++
	owner, ok := store.owners[digest]
	if !ok {
		return domainattachment.OwnerRecordV1{}, attachmentauthorityport.ErrNotFound
	}
	return owner, nil
}

func (*turnStartAttachmentRiskOwnerStore) PutUseReceiptIfAbsent(
	context.Context,
	domainattachment.AttachmentUseReceiptV1,
) error {
	return errors.New("unsupported")
}

func (*turnStartAttachmentRiskOwnerStore) ResolveUseReceipt(
	context.Context,
	string,
) (domainattachment.AttachmentUseReceiptV1, error) {
	return domainattachment.AttachmentUseReceiptV1{}, attachmentauthorityport.ErrNotFound
}

func (*turnStartAttachmentRiskOwnerStore) PutUseDispositionIfAbsent(
	context.Context,
	domainattachment.AttachmentUseDispositionV1,
) error {
	return errors.New("unsupported")
}

func (*turnStartAttachmentRiskOwnerStore) ResolveUseDisposition(
	context.Context,
	string,
) (domainattachment.AttachmentUseDispositionV1, error) {
	return domainattachment.AttachmentUseDispositionV1{}, attachmentauthorityport.ErrNotFound
}

var _ attachmentauthorityport.Store = (*turnStartAttachmentRiskOwnerStore)(nil)
