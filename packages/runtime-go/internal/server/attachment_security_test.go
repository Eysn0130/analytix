package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

type receiptBoundAttachmentProvider struct {
	calls            int
	sawExactAccount  bool
	sawMaskedAccount bool
}

func (*receiptBoundAttachmentProvider) RequiresDurablePipelineStagesV1() {}

func (fake *receiptBoundAttachmentProvider) Stream(_ context.Context, request provider.Request) (provider.Result, error) {
	if request.BeforeSend == nil {
		return provider.Result{}, errors.New("attachment physical send authority is unavailable")
	}
	if err := request.BeforeSend(1); err != nil {
		return provider.Result{}, err
	}
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return provider.Result{}, err
	}
	fake.calls++
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if strings.Contains(part.Text, "6222020000000000000") {
				fake.sawExactAccount = true
			}
			if strings.Contains(part.Text, "[ACCOUNT]") {
				fake.sawMaskedAccount = true
			}
		}
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "attachment reviewed"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return provider.Result{}, err
		}
	}
	return provider.Result{Chunks: []provider.Chunk{chunk}, StreamCompleted: true}, nil
}

func TestAttachmentContentHashMismatchBlocksBeforeProvider(t *testing.T) {
	testAttachmentTamperBlocksBeforeProvider(t, "content", true, func(t *testing.T, dataDir string, attachmentID string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dataDir, "attachments", "content", attachmentID+".bin"), []byte("tampered-ledger"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
}

func TestAttachmentProjectionHashMismatchBlocksBeforeProvider(t *testing.T) {
	testAttachmentTamperBlocksBeforeProvider(t, "projection", false, func(t *testing.T, dataDir string, attachmentID string) {
		t.Helper()
		path := filepath.Join(dataDir, "attachments", "metadata", attachmentID+".json")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err := json.Unmarshal(body, &metadata); err != nil {
			t.Fatal(err)
		}
		metadata["documentText"] = "tampered projection must never reach a provider"
		body, err = json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	})
}

func testAttachmentTamperBlocksBeforeProvider(
	t *testing.T,
	kind string,
	wantTerminalTurn bool,
	tamper func(*testing.T, string, string),
) {
	t.Helper()
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	dataDir := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "attachment-security-provider", BaseURL: "https://provider.invalid/v1", APIKey: "test-key",
		Model: "attachment-security-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	configureServerCaseExecution(t, handler, workspace)
	provider := &admissionFailureCountingProvider{}
	handler.provider = provider

	thread, err := handler.store.CreateThread(map[string]any{
		"title": "attachment integrity", "workspace": workspace,
		"providerId": "attachment-security-provider", "model": "attachment-security-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	attachment, err := handler.attachments.Create(map[string]any{
		"name": "ledger.csv", "mimeType": "text/csv",
		"dataBase64":   base64.StdEncoding.EncodeToString([]byte("amount\n1")),
		"documentText": "amount\n1", "threadId": threadID, "workspace": workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID := stringField(attachment, "id")
	if err := handler.attachmentAccess.CommitUpload(context.Background(), attachment); err != nil {
		t.Fatal(err)
	}
	tamper(t, dataDir, attachmentID)

	_, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize the supplied attachment.", ProviderID: "attachment-security-provider",
		Model: "attachment-security-model", AttachmentIDs: []string{attachmentID},
	})
	if provider.calls.Load() != 0 {
		t.Fatalf("%s-tampered attachment reached provider: calls=%d", kind, provider.calls.Load())
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if !wantTerminalTurn {
		if startErr == nil || len(turns) != 0 {
			t.Fatalf("%s metadata preflight did not fail before turn admission: turns=%#v err=%v", kind, turns, startErr)
		}
		replay, replayErr := handler.store.LoadEventsSince(threadID, 0)
		if replayErr != nil {
			t.Fatal(replayErr)
		}
		publicBody, marshalErr := json.Marshal([]any{reloaded, replay.Events})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, forbidden := range []string{"amount\\n1", "tampered-ledger", "tampered projection must never reach a provider"} {
			if strings.Contains(string(publicBody), forbidden) {
				t.Fatalf("%s preflight leaked private attachment material into public state: %q", kind, forbidden)
			}
		}
		return
	}
	if len(turns) != 1 {
		t.Fatalf("%s tamper lost its terminal turn: %#v", kind, reloaded["turns"])
	}
	turn, _ := turns[0].(map[string]any)
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || !domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("%s tamper did not preserve exact case authority: context=%#v err=%v", kind, securityContext, err)
	}
	acceptedFinal, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	if err != nil || acceptedFinal.TerminalReason != string(evidenceapp.TerminalProviderFailure) ||
		acceptedFinal.Variant != domainevidence.NeedsEvidenceAnswer || acceptedFinal.RegistrySequence != 0 ||
		acceptedFinal.ContextDigest != securityContext.ContextDigest || acceptedFinal.FinalGateVersion != domainevidence.FinalEvidenceGateVersion {
		t.Fatalf("%s tamper bypassed Final Evidence Gate: accepted=%#v err=%v", kind, acceptedFinal, err)
	}
	durableBody, err := json.Marshal(turn)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"amount\\n1", "tampered-ledger", "tampered projection must never reach a provider"} {
		if strings.Contains(string(durableBody), forbidden) {
			t.Fatalf("%s tamper leaked private attachment material into durable turn: %q", kind, forbidden)
		}
	}
}

func TestFullAccountNeverEntersProviderRequestOrOrdinaryDurableState(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	dataDir := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "attachment-receipt-provider", BaseURL: "https://provider.invalid/v1", APIKey: "test-key",
		Model: "attachment-receipt-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	configureServerCaseExecution(t, handler, workspace)
	fake := &receiptBoundAttachmentProvider{}
	handler.provider = fake

	thread, err := handler.store.CreateThread(map[string]any{
		"title": "attachment receipt", "workspace": workspace,
		"providerId": "attachment-receipt-provider", "model": "attachment-receipt-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	attachment, err := handler.attachments.Create(map[string]any{
		"name": "account.txt", "mimeType": "text/plain",
		"dataBase64":   base64.StdEncoding.EncodeToString([]byte("account 6222020000000000000")),
		"documentText": "account 6222020000000000000", "threadId": threadID, "workspace": workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.attachmentAccess.CommitUpload(context.Background(), attachment); err != nil {
		t.Fatal(err)
	}
	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize the supplied authorized statement.", ProviderID: "attachment-receipt-provider",
		Model: "attachment-receipt-model", AttachmentIDs: []string{stringField(attachment, "id")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || fake.sawExactAccount || !fake.sawMaskedAccount {
		t.Fatalf("provider privacy projection failed: calls=%d exact=%v masked=%v", fake.calls, fake.sawExactAccount, fake.sawMaskedAccount)
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := appmodel.TurnByID(reloaded, stringField(response, "turnId"))
	if !ok {
		t.Fatalf("attachment turn is missing: %#v", reloaded["turns"])
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.attachmentUses.RequireNoOpenForFrozenPublicationContext(context.Background(), securityContext); err != nil {
		t.Fatalf("terminal turn retained an open attachment effect: %v", err)
	}
	durableBody, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(durableBody), "6222020000000000000") {
		t.Fatal("full bank account leaked into ordinary durable thread state")
	}
	accepted, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	if err != nil || accepted.FinalGateVersion != domainevidence.FinalEvidenceGateVersion {
		t.Fatalf("attachment turn bypassed Final Evidence Gate: accepted=%#v err=%v", accepted, err)
	}
}
