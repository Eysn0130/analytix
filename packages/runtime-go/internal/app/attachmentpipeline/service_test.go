package attachmentpipeline

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	attachmentpublicationapp "analytix.local/runtime-go/internal/app/attachmentpublication"
	attachmentuseapp "analytix.local/runtime-go/internal/app/attachmentuse"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	appturn "analytix.local/runtime-go/internal/app/turn"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestAttachmentProviderAttemptIssuesReceiptBeforeContentAndSettlesBeforeRelease(t *testing.T) {
	fixture := newAttachmentPipelineFixture(t)
	prepared, err := fixture.service.PrepareProviderAttempt(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	receiptIndex, contentIndex := pipelineEventIndex(fixture.store.events, "receipt"), pipelineEventIndex(fixture.store.events, "content")
	if receiptIndex < 0 || contentIndex < 0 || receiptIndex >= contentIndex || pipelineEventIndex(fixture.store.events, "disposition") >= 0 {
		t.Fatalf("content was read without an open unsettled receipt: %#v", fixture.store.events)
	}
	if len(prepared.Request.Messages) != 1 || len(prepared.Request.Messages[0].Parts) != 1 ||
		!strings.Contains(prepared.Request.Messages[0].Parts[0].Text, "6222020000000000000") {
		t.Fatalf("attempt-local provider projection mismatch: %#v", prepared.Request.Messages)
	}
	tokenJSON, err := json.Marshal(prepared.Token)
	if err != nil || string(tokenJSON) != "{}" || strings.Contains(string(tokenJSON), "6222020000000000000") {
		t.Fatalf("settlement token retained or exposed attempt payload: json=%s err=%v", tokenJSON, err)
	}
	if err := prepared.Request.BeforeSend(1); err != nil {
		t.Fatalf("physical send revalidation failed: %v", err)
	}
	if err := fixture.service.SettleProviderAttempt(context.Background(), prepared.Token, domainattachment.AttachmentUseDispositionConsumedV1, "provider_attempt_completed"); err != nil {
		t.Fatal(err)
	}
	if len(fixture.store.dispositions) != 1 || fixture.store.events[len(fixture.store.events)-1] != "disposition_resolve" {
		t.Fatalf("provider result was not durably settled: events=%#v dispositions=%#v", fixture.store.events, fixture.store.dispositions)
	}
	if err := fixture.uses.RequireNoOpenForFrozenPublicationContext(context.Background(), fixture.securityContext); err != nil {
		t.Fatalf("settled provider attachment still blocked final publication: %v", err)
	}
}

func pipelineEventIndex(events []string, expected string) int {
	for index, event := range events {
		if event == expected {
			return index
		}
	}
	return -1
}

func TestAttachmentProviderAttemptRevalidatesCurrentContextAtPhysicalSend(t *testing.T) {
	fixture := newAttachmentPipelineFixture(t)
	prepared, err := fixture.service.PrepareProviderAttempt(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	fixture.current = false
	if err := prepared.Request.BeforeSend(1); err == nil {
		t.Fatal("stale attachment attempt reached a physical provider send")
	}
	if err := fixture.service.SettleProviderAttempt(context.Background(), prepared.Token, domainattachment.AttachmentUseDispositionFailedV1, "provider_attempt_failed"); err != nil {
		t.Fatal(err)
	}
	fixture.current = true
	if err := fixture.uses.RequireNoOpenForFrozenPublicationContext(context.Background(), fixture.securityContext); err != nil {
		t.Fatalf("stale provider attachment use remained open: %v", err)
	}
}

func TestImageProjectionUnavailableKeepsPrimaryTextAttemptSettleable(t *testing.T) {
	fixture, bridgeProvider := newAttachmentPipelineImageFixture(t)
	prepared, err := fixture.service.PrepareProviderAttempt(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if bridgeProvider.calls != 0 {
		t.Fatalf("missing trusted projector reached bridge provider: calls=%d", bridgeProvider.calls)
	}
	if len(prepared.Request.Messages) != 1 || len(prepared.Request.Messages[0].Parts) != 1 ||
		prepared.Request.Messages[0].Parts[0].Type != "text" {
		t.Fatalf("image-unavailable attempt did not continue as text-only primary work: %#v", prepared.Request.Messages)
	}
	providerText := prepared.Request.Messages[0].Parts[0].Text
	for _, forbidden := range []string{fixture.store.dataBase64, "PRIVATE_IMAGE_SOURCE_NAME_731.png", "data:image/", "image_url"} {
		if strings.Contains(providerText, forbidden) {
			t.Fatalf("text-only primary continuation leaked %q: %s", forbidden, providerText)
		}
	}
	if !strings.Contains(providerText, "trusted local image privacy projection is unavailable") {
		t.Fatalf("text-only primary continuation omitted image-effect unavailable result: %s", providerText)
	}
	if err := prepared.Request.BeforeSend(1); err != nil {
		t.Fatalf("independent text-only primary send lost attachment authority: %v", err)
	}
	if err := fixture.service.SettleProviderAttempt(
		context.Background(), prepared.Token, domainattachment.AttachmentUseDispositionConsumedV1, "provider_attempt_completed",
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.uses.RequireNoOpenForFrozenPublicationContext(context.Background(), fixture.securityContext); err != nil {
		t.Fatalf("image-unavailable text continuation left an open receipt: %v", err)
	}
}

func TestCaseFinalGateRejectsOpenAttachmentUse(t *testing.T) {
	fixture := newAttachmentPipelineFixture(t)
	prepared, err := fixture.service.PrepareProviderAttempt(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	inner := &attachmentPublicationFinalizerSpy{}
	guard := attachmentpublicationapp.WithUseGuard(inner, fixture.uses)
	if _, err := guard.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{Context: fixture.securityContext}); !errors.Is(err, attachmentuseapp.ErrOpen) || inner.calls != 0 {
		t.Fatalf("open attachment effect reached case finalizer: calls=%d err=%v", inner.calls, err)
	}
	if err := fixture.service.SettleProviderAttempt(context.Background(), prepared.Token, domainattachment.AttachmentUseDispositionConsumedV1, "provider_attempt_completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{Context: fixture.securityContext}); err != nil || inner.calls != 1 {
		t.Fatalf("settled attachment effect did not release final gate: calls=%d err=%v", inner.calls, err)
	}
}

type attachmentPublicationFinalizerSpy struct{ calls int }

func (spy *attachmentPublicationFinalizerSpy) PersistBoundary(context.Context, evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
	spy.calls++
	return evidenceapp.PersistCaseBoundaryResult{}, nil
}

type attachmentPipelineFixture struct {
	service         Service
	uses            *attachmentuseapp.Service
	store           *attachmentPipelineMemoryStore
	securityContext domainsecurity.TurnSecurityContext
	input           ProviderAttemptInput
	current         bool
}

func newAttachmentPipelineFixture(t *testing.T) *attachmentPipelineFixture {
	return newAttachmentPipelineFixtureForContent(t, "statement.txt", "text/plain", []byte("account 6222020000000000000"))
}

type attachmentPipelineVisionProvider struct{ calls int }

func (provider *attachmentPipelineVisionProvider) Stream(context.Context, domainmodel.Request) (domainmodel.Result, error) {
	provider.calls++
	return domainmodel.Result{}, nil
}

type attachmentPipelineVisionExecutionResolver struct{ config domainmodel.TurnConfig }

func (resolver attachmentPipelineVisionExecutionResolver) ResolveVisionExecution(context.Context) (visionbridgeapp.ExecutionLease, error) {
	return visionbridgeapp.NewExecutionLease(
		resolver.config,
		func(context.Context) error { return nil },
		func() {},
	), nil
}

func newAttachmentPipelineImageFixture(t *testing.T) (*attachmentPipelineFixture, *attachmentPipelineVisionProvider) {
	t.Helper()
	body := []byte("\x89PNG\r\n\x1a\nPRIVATE_IMAGE_BYTES_SENTINEL_731")
	fixture := newAttachmentPipelineFixtureForContent(t, "PRIVATE_IMAGE_SOURCE_NAME_731.png", "image/png", body)
	bridgeProvider := &attachmentPipelineVisionProvider{}
	visionConfig := visionbridgeapp.Service{
		Provider: bridgeProvider,
		ExecutionResolver: attachmentPipelineVisionExecutionResolver{config: domainmodel.TurnConfig{
			ProviderID: "bridge", Family: "openai", EndpointFormat: "chat_completions",
			BaseURL: "https://bridge.invalid", Model: "vision-model",
		}},
		DefaultConfig: runtimeVisionBridgeConfigForAttachmentPipelineTest(),
	}
	fixture.service.Vision = visionConfig
	fixture.input.Request.PrivateProviderTelemetry = &domainmodel.ProviderTelemetryBindingV1{
		SecurityContext: fixture.securityContext, UsageSource: domaincache.ProviderUsageSourceTurn,
		Channel: domaincache.ProviderChannelPrimary, LogicalSequence: 1, OuterAttempt: 1,
	}
	return fixture, bridgeProvider
}

func runtimeVisionBridgeConfigForAttachmentPipelineTest() runtimeinfoapp.VisionBridgeConfig {
	return runtimeinfoapp.VisionBridgeConfig{
		Enabled: true, Mode: "auto", ProviderID: "bridge", BaseURL: "https://bridge.invalid",
		APIKey: "bridge-key", EndpointFormat: "chat_completions", Model: "vision-model",
		SemanticProbeStatus: "supported", FallbackWhenPrimaryImageUnsupported: true,
		MaxImageBytes: 1024, MaxScreenshotsPerTurn: 1,
	}
}

func newAttachmentPipelineFixtureForContent(t *testing.T, name, mimeType string, body []byte) *attachmentPipelineFixture {
	t.Helper()
	issuedAt := time.Date(2026, 7, 14, 6, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/cases/a",
		CaseID: "case-a", ContextEpoch: 9, IssuedAt: issuedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: securityContext.WorkspaceRealPath, State: domainsecurity.CaseBindingStateValid,
		CaseID: securityContext.CaseID, BindingSHA256: domainsecurity.SHA256Hex([]byte("binding-document")),
		CaseBindingHash: securityContext.CaseBindingHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	metadataProjection := map[string]any{
		"name": name, "kind": "document", "mimeType": mimeType, "byteSize": float64(len(body)),
		"hash": domainsecurity.SHA256Hex(body), "scope": "thread", "threadIds": []any{securityContext.ThreadID},
		"workspaces": []any{securityContext.WorkspaceRealPath},
	}
	projectionBody, _ := json.Marshal(metadataProjection)
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{
		OwnerNonce: strings.Repeat("a", 32), BlobSHA256: domainsecurity.SHA256Hex(body), ByteSize: int64(len(body)),
		MIMEType: mimeType, ThreadID: securityContext.ThreadID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
		CaseBindingObservation: &observation, ProjectionSHA256: domainsecurity.SHA256Hex(projectionBody), CreatedAt: issuedAt.Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerBody, _ := domainattachment.OwnerRecordV1Bytes(owner)
	var ownerMap map[string]any
	if err := json.Unmarshal(ownerBody, &ownerMap); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{}
	for key, value := range metadataProjection {
		metadata[key] = value
	}
	metadata["id"] = owner.AttachmentID
	metadata["ownerRecord"] = ownerMap
	store := newAttachmentPipelineMemoryStore(metadata, base64.StdEncoding.EncodeToString(body), owner)
	fixture := &attachmentPipelineFixture{store: store, securityContext: securityContext, current: true}
	authority := newAttachmentPipelineAuthority(31)
	fixture.uses = attachmentuseapp.NewService(authority, store, func(context.Context, domainsecurity.TurnSecurityContext) error {
		if !fixture.current {
			return errors.New("context changed")
		}
		return nil
	})
	planner := appturn.AttachmentPlanner{Store: store, Owners: store}
	plan, err := planner.Plan(context.Background(), appturn.AttachmentPlanInput{
		IDs: []string{owner.AttachmentID}, SecurityContext: securityContext,
		ModelInputModalities: []string{"text"}, ModelMessageParts: []string{"text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.events = nil
	fixture.service = Service{Planner: planner, Uses: fixture.uses, Vision: visionbridgeapp.Service{}}
	manifest := domainsecurity.SHA256Hex([]byte("tool-manifest"))
	fixture.input = ProviderAttemptInput{
		SecurityContext: securityContext, Plan: plan,
		Request: domainmodel.Request{
			ProviderID: "provider-a", Family: "openai", EndpointFormat: "chat_completions",
			BaseURL: "https://provider.invalid", Model: "model-a", Route: "agent",
			Messages: []domainmodel.Message{{
				Role: "user", Content: "analyze", PrivateAttachmentPlanDigest: plan.PlanDigest,
			}},
			PrivateAttachmentPlanDigest: plan.PlanDigest,
		},
		Primary: domainmodel.TurnConfig{
			ProviderID: "provider-a", Family: "openai", EndpointFormat: "chat_completions",
			BaseURL: "https://provider.invalid", Model: "model-a", InputModalities: []string{"text"}, MessageParts: []string{"text"},
		},
		PrimaryProvider: "provider-a", PrimaryModel: "model-a", PromptRoute: "agent",
		ToolManifestHash: manifest, Sequence: 1, Attempt: 1,
	}
	return fixture
}

type attachmentPipelineAuthority struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	keyID   string
}

func newAttachmentPipelineAuthority(seed byte) *attachmentPipelineAuthority {
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	public := append(ed25519.PublicKey(nil), private.Public().(ed25519.PublicKey)...)
	return &attachmentPipelineAuthority{private: private, public: public, keyID: domainsecurity.SHA256Hex(public)}
}

func (authority *attachmentPipelineAuthority) KeyID() string { return authority.keyID }
func (authority *attachmentPipelineAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}
func (authority *attachmentPipelineAuthority) Sign(_ context.Context, body []byte) ([]byte, error) {
	return ed25519.Sign(authority.private, body), nil
}
func (authority *attachmentPipelineAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, body, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.public) || !ed25519.Verify(publicKey, body, signature) {
		return errors.New("untrusted authority")
	}
	return nil
}

type attachmentPipelineMemoryStore struct {
	metadata     map[string]any
	dataBase64   string
	owners       map[string]domainattachment.OwnerRecordV1
	receipts     map[string]domainattachment.AttachmentUseReceiptV1
	dispositions map[string]domainattachment.AttachmentUseDispositionV1
	events       []string
}

func newAttachmentPipelineMemoryStore(metadata map[string]any, dataBase64 string, owner domainattachment.OwnerRecordV1) *attachmentPipelineMemoryStore {
	return &attachmentPipelineMemoryStore{
		metadata: metadata, dataBase64: dataBase64,
		owners:       map[string]domainattachment.OwnerRecordV1{owner.OwnerDigest: owner},
		receipts:     map[string]domainattachment.AttachmentUseReceiptV1{},
		dispositions: map[string]domainattachment.AttachmentUseDispositionV1{},
	}
}

func (store *attachmentPipelineMemoryStore) Metadata(id string) (map[string]any, bool, error) {
	store.events = append(store.events, "metadata")
	return store.metadata, store.metadata["id"] == id, nil
}
func (store *attachmentPipelineMemoryStore) Content(id string) (map[string]any, string, bool, error) {
	store.events = append(store.events, "content")
	return store.metadata, store.dataBase64, store.metadata["id"] == id, nil
}
func (store *attachmentPipelineMemoryStore) PutOwnerIfAbsent(_ context.Context, owner domainattachment.OwnerRecordV1) error {
	store.owners[owner.OwnerDigest] = owner
	return nil
}
func (store *attachmentPipelineMemoryStore) ResolveOwner(_ context.Context, digest string) (domainattachment.OwnerRecordV1, error) {
	store.events = append(store.events, "owner_resolve")
	owner, ok := store.owners[digest]
	if !ok {
		return domainattachment.OwnerRecordV1{}, storeport.ErrNotFound
	}
	return owner, nil
}
func (store *attachmentPipelineMemoryStore) PutUseReceiptIfAbsent(_ context.Context, receipt domainattachment.AttachmentUseReceiptV1) error {
	store.events = append(store.events, "receipt")
	store.receipts[receipt.UseID] = receipt
	return nil
}
func (store *attachmentPipelineMemoryStore) ResolveUseReceipt(_ context.Context, useID string) (domainattachment.AttachmentUseReceiptV1, error) {
	store.events = append(store.events, "receipt_resolve")
	receipt, ok := store.receipts[useID]
	if !ok {
		return domainattachment.AttachmentUseReceiptV1{}, storeport.ErrNotFound
	}
	return receipt, nil
}
func (store *attachmentPipelineMemoryStore) PutUseDispositionIfAbsent(_ context.Context, disposition domainattachment.AttachmentUseDispositionV1) error {
	store.events = append(store.events, "disposition")
	store.dispositions[disposition.UseID] = disposition
	return nil
}
func (store *attachmentPipelineMemoryStore) ResolveUseDisposition(_ context.Context, useID string) (domainattachment.AttachmentUseDispositionV1, error) {
	store.events = append(store.events, "disposition_resolve")
	disposition, ok := store.dispositions[useID]
	if !ok {
		return domainattachment.AttachmentUseDispositionV1{}, storeport.ErrNotFound
	}
	return disposition, nil
}
func (store *attachmentPipelineMemoryStore) VisitOwners(_ context.Context, visit func(domainattachment.OwnerRecordV1) error) error {
	for _, key := range sortedPipelineKeys(store.owners) {
		if err := visit(store.owners[key]); err != nil {
			return err
		}
	}
	return nil
}
func (store *attachmentPipelineMemoryStore) VisitUseReceipts(_ context.Context, visit func(domainattachment.AttachmentUseReceiptV1) error) error {
	for _, key := range sortedPipelineKeys(store.receipts) {
		if err := visit(store.receipts[key]); err != nil {
			return err
		}
	}
	return nil
}
func (store *attachmentPipelineMemoryStore) VisitUseDispositions(_ context.Context, visit func(domainattachment.AttachmentUseDispositionV1) error) error {
	for _, key := range sortedPipelineKeys(store.dispositions) {
		if err := visit(store.dispositions[key]); err != nil {
			return err
		}
	}
	return nil
}
func (store *attachmentPipelineMemoryStore) HasRecords(context.Context) (bool, error) {
	return len(store.owners)+len(store.receipts)+len(store.dispositions) > 0, nil
}
func sortedPipelineKeys[T any](records map[string]T) []string {
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
