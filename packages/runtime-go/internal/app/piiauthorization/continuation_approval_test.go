package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestContinuationApprovalAuthorityRequiresExactAllowedHostRecord(t *testing.T) {
	fixture := newContinuationApprovalFixture(t, continuationApprovalFixtureOptions{})
	if err := fixture.authority.ValidateCurrent(context.Background(), fixture.validation); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(fixture.receipt.Payload.Arguments, []byte("6222020202020202020")) {
		t.Fatal("controlled PII approval receipt contained the raw account")
	}

	tests := map[string]func(*piiauthorizationport.ApprovalValidationV1){
		"fake disposition": func(input *piiauthorizationport.ApprovalValidationV1) {
			input.ApprovalRecordDigest = domainsecurity.SHA256Hex([]byte("fake-disposition"))
		},
		"scope": func(input *piiauthorizationport.ApprovalValidationV1) {
			input.ApprovalScopeDigest = domainsecurity.SHA256Hex([]byte("other-scope"))
		},
		"target": func(input *piiauthorizationport.ApprovalValidationV1) {
			input.TargetIdentityDigest = domainsecurity.SHA256Hex([]byte("other-target"))
		},
		"content": func(input *piiauthorizationport.ApprovalValidationV1) {
			input.ProjectedContentHash = domainsecurity.SHA256Hex([]byte("other-report"))
		},
		"requester": func(input *piiauthorizationport.ApprovalValidationV1) { input.RequesterUserID = "other-user" },
		"purpose":   func(input *piiauthorizationport.ApprovalValidationV1) { input.DisclosurePurpose = "chat" },
		"expiry": func(input *piiauthorizationport.ApprovalValidationV1) {
			input.ExpiresAt = fixture.expiresAt.Add(-time.Minute).Format(time.RFC3339Nano)
		},
		"case": func(input *piiauthorizationport.ApprovalValidationV1) {
			other, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
				ThreadID: "other-thread", TurnID: "other-turn", WorkspaceRealPath: "/workspace/other", CaseID: "other-case",
				CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")), ContextEpoch: 1, IssuedAt: fixture.now.Add(-time.Minute),
			})
			if err != nil {
				t.Fatal(err)
			}
			input.Context = other
			input.RequesterUserID = other.UserID
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := fixture.validation
			mutate(&input)
			if fixture.authority.ValidateCurrent(context.Background(), input) == nil {
				t.Fatalf("%s mismatch retained disclosure authority", name)
			}
		})
	}
}

func TestContinuationApprovalAuthorityRejectsDeniedExpiredWrongToolServerAndScope(t *testing.T) {
	tests := map[string]continuationApprovalFixtureOptions{
		"denied":           {status: domaincontinuation.StatusDenied, reason: "approval_denied"},
		"wrong tool":       {toolName: "write_file"},
		"wrong server":     {serverIdentity: "mcp"},
		"broader scope":    {extraToolScope: "write_file"},
		"approval expired": {clockOffset: 11 * time.Minute},
		"outlives grant":   {grantExpiryOffset: 5 * time.Minute},
	}
	for name, options := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newContinuationApprovalFixture(t, options)
			if fixture.authority.ValidateCurrent(context.Background(), fixture.validation) == nil {
				t.Fatalf("%s continuation authorized controlled PII", name)
			}
		})
	}
}

func TestControlledPIIApprovalArgumentsAreClosedCanonicalAndPIIFree(t *testing.T) {
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	actions := []string{domainpii.ControlledArtifactAccessActionDisplayV1, domainpii.ControlledArtifactAccessActionExportV1}
	accessPolicy := domainsecurity.SHA256Hex([]byte("approval-access-policy"))
	retentionPolicy := domainsecurity.SHA256Hex([]byte("approval-retention-policy"))
	retentionUntil := now.Add(24 * time.Hour)
	body, err := CanonicalControlledPIIApprovalArgumentsV1(ControlledPIIApprovalArgumentsInputV1{
		ApprovalScopeDigest: domainsecurity.SHA256Hex([]byte("scope")), TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("target")),
		ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("report")),
		AllowedAccessActions:   actions, AccessPolicyDigest: accessPolicy, RetentionPolicyDigest: retentionPolicy,
		RetentionUntil: retentionUntil, ExpiresAt: now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseControlledPIIApprovalArgumentsV1(body); err != nil {
		t.Fatal(err)
	}
	unknown := append([]byte(nil), body[:len(body)-1]...)
	unknown = append(unknown, []byte(`,"account":"6222020202020202020"}`)...)
	if _, err := ParseControlledPIIApprovalArgumentsV1(unknown); err == nil {
		t.Fatal("unknown raw PII field was accepted in approval arguments")
	}
	nonCanonical := append([]byte(" "), body...)
	if _, err := ParseControlledPIIApprovalArgumentsV1(nonCanonical); err == nil {
		t.Fatal("non-canonical approval arguments were accepted")
	}
	duplicate := []byte(strings.Replace(string(body), `"purpose":`, `"purpose":"analytix.controlled-pii-approval/v1","purpose":`, 1))
	if _, err := ParseControlledPIIApprovalArgumentsV1(duplicate); err == nil {
		t.Fatal("duplicate approval argument key was accepted")
	}
}

type continuationApprovalFixtureOptions struct {
	status            string
	reason            string
	toolName          string
	serverIdentity    string
	extraToolScope    string
	clockOffset       time.Duration
	grantExpiryOffset time.Duration
}

type continuationApprovalFixture struct {
	authority  *ContinuationApprovalAuthority
	validation piiauthorizationport.ApprovalValidationV1
	receipt    domaincontinuation.Receipt
	expiresAt  time.Time
	now        time.Time
}

func newContinuationApprovalFixture(t *testing.T, options continuationApprovalFixtureOptions) continuationApprovalFixture {
	t.Helper()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	contextValue, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-controlled-pii", TurnID: "turn-controlled-pii", WorkspaceRealPath: "/workspace/controlled-pii",
		CaseID: "case-controlled-pii", CaseBindingHash: domainsecurity.SHA256Hex([]byte("controlled-pii-binding")),
		ContextEpoch: 4, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := now.Add(10 * time.Minute)
	actions := []string{domainpii.ControlledArtifactAccessActionDisplayV1, domainpii.ControlledArtifactAccessActionExportV1}
	accessPolicy := domainsecurity.SHA256Hex([]byte("controlled-pii-access-policy"))
	retentionPolicy := domainsecurity.SHA256Hex([]byte("controlled-pii-retention-policy"))
	retentionUntil := now.Add(24 * time.Hour)
	binding := domainpii.FieldBindingV1{
		PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: "claim-account", ClaimRecordDigest: domainsecurity.SHA256Hex([]byte("claim-record")),
		ClaimType: domainevidence.ClaimAccount, FieldName: "accountId", ValueSHA256: domainsecurity.SHA256Hex([]byte("6222020202020202020")),
		EvidenceReceiptIDs: []string{"evr_" + domainsecurity.SHA256Hex([]byte("evidence"))},
	}
	scopeDigest, err := domainpii.ApprovalScopeDigestV1(domainpii.ApprovalScopeInputV1{
		SecurityContext: contextValue, RequesterUserID: contextValue.UserID, DisclosurePurpose: domainpii.DisclosurePurposeCaseReportV1,
		ClaimLedgerDigest: domainsecurity.SHA256Hex([]byte("claim-ledger")), FieldBindings: []domainpii.FieldBindingV1{binding},
		ProjectionRulesetHash: domainsecurity.SHA256Hex([]byte("rules")), ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("controlled-report")),
		PreservedControlledFieldCount: 1, TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("target")),
		AllowedAccessActions: actions, AccessPolicyDigest: accessPolicy, RetentionPolicyDigest: retentionPolicy,
		RetentionUntil: retentionUntil, ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := CanonicalControlledPIIApprovalArgumentsV1(ControlledPIIApprovalArgumentsInputV1{
		ApprovalScopeDigest: scopeDigest, TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("target")),
		ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("controlled-report")),
		AllowedAccessActions:   actions, AccessPolicyDigest: accessPolicy, RetentionPolicyDigest: retentionPolicy,
		RetentionUntil: retentionUntil, ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolName := options.toolName
	if toolName == "" {
		toolName = pendingworkapp.ReportStageToolName
	}
	serverIdentity := options.serverIdentity
	connectionEpoch := uint64(0)
	if serverIdentity == "" {
		serverIdentity = "host:builtin"
	} else if serverIdentity == "mcp" {
		serverIdentity, err = domainsecurity.NewVerifiedMCPServerIdentity(
			"funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("controlled-pii-mcp-instance")), 1,
		)
		if err != nil {
			t.Fatal(err)
		}
		connectionEpoch = 1
	}
	toolScope := []string{toolName}
	if options.extraToolScope != "" {
		toolScope = append(toolScope, options.extraToolScope)
	}
	grantExpiry := expiresAt
	if options.grantExpiryOffset != 0 {
		grantExpiry = now.Add(options.grantExpiryOffset)
	}
	toolCallID := piiTestHostToolCallID(t, "controlled-pii")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextValue, Provider: "provider", ServerIdentity: serverIdentity, ToolName: toolName, ToolCallID: toolCallID,
		ConnectionEpoch: connectionEpoch,
		ArgsHash:        domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("controlled-pii-schema")),
		ScopeHash: executiongrantapp.ScopeHash(toolScope), ReadOnly: false, ApprovalState: "pending", IssuedAt: now, ExpiresAt: grantExpiry,
	})
	gateID := domaincontinuation.GateID(domaincontinuation.KindApproval, contextValue.ThreadID, contextValue.TurnID, contextValue.ContextDigest, grant.GrantID, grant.ToolCallID)
	payload := domaincontinuation.Payload{
		Version: domaincontinuation.ContractVersion, Kind: domaincontinuation.KindApproval, GateID: gateID,
		ThreadID: contextValue.ThreadID, TurnID: contextValue.TurnID, ItemID: "item_" + gateID, CallID: grant.ToolCallID,
		ToolName: toolName, ToolCallItemID: domaintoolcall.ToolCallItemIDV1(contextValue.TurnID, grant.ToolCallID), Arguments: arguments, ProviderID: grant.Provider, Model: "model",
		ProviderRouteHash: domainsecurity.SHA256Hex([]byte("provider-route")), ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		ToolScope: toolScope, LogicalEffect: domainsecurity.LogicalEffectCaseData, OrdinaryWork: false, ProviderStepExact: true,
		CaseSourceUnavailable: false, OrdinaryResultInputIsolated: false,
		ProviderStepPromptSHA256: domaincontinuation.CanonicalProviderStepPromptSHA256("continue controlled PII report publication"),
		ProviderNamespace:        domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		SecurityContext:          contextValue, ExecutionGrant: grant, IssuedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x63}, ed25519.SeedSize))
	hostAuthority := &continuationApprovalTestAuthority{privateKey: privateKey, publicKey: privateKey.Public().(ed25519.PublicKey)}
	store := &continuationApprovalMemoryStore{}
	continuations := continuationapp.NewService(hostAuthority, store)
	receipt, err := continuations.Issue(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	status, reason := options.status, options.reason
	if status == "" {
		status, reason = domaincontinuation.StatusAllowed, "approval_allowed"
	}
	var disposition domaincontinuation.Disposition
	if status == domaincontinuation.StatusAllowed {
		disposition, err = continuations.Consume(context.Background(), gateID, status, reason, now.Add(2*time.Minute))
	} else {
		disposition, err = continuations.Close(context.Background(), gateID, status, reason, now.Add(2*time.Minute))
	}
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewContinuationApprovalAuthority(continuations, func() time.Time { return now.Add(3*time.Minute + options.clockOffset) })
	if err != nil {
		t.Fatal(err)
	}
	return continuationApprovalFixture{
		authority: authority, receipt: receipt, expiresAt: expiresAt, now: now,
		validation: piiauthorizationport.ApprovalValidationV1{
			Context: contextValue, ApprovalID: gateID, ApprovalRecordDigest: disposition.DispositionID, ApprovalScopeDigest: scopeDigest,
			RequesterUserID: contextValue.UserID, DisclosurePurpose: domainpii.DisclosurePurposeCaseReportV1,
			TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("target")), ProjectedContentHash: domainsecurity.SHA256Hex([]byte("controlled-report")),
			AllowedAccessActions: actions, AccessPolicyDigest: accessPolicy, RetentionPolicyDigest: retentionPolicy,
			RetentionUntil: retentionUntil.Format(time.RFC3339Nano),
			ExpiresAt:      expiresAt.Format(time.RFC3339Nano),
		},
	}
}

type continuationApprovalTestAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

func (authority *continuationApprovalTestAuthority) KeyID() string {
	return domainsecurity.SHA256Hex(authority.publicKey)
}
func (authority *continuationApprovalTestAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *continuationApprovalTestAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *continuationApprovalTestAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(publicKey, message, signature) {
		return errors.New("untrusted continuation authority")
	}
	return nil
}

type continuationApprovalMemoryStore struct {
	receipt     domaincontinuation.Receipt
	disposition domaincontinuation.Disposition
}

func (store *continuationApprovalMemoryStore) PutReceiptIfAbsent(_ context.Context, receipt domaincontinuation.Receipt) error {
	if store.receipt.ReceiptID != "" && !reflect.DeepEqual(store.receipt, receipt) {
		return errors.New("receipt conflict")
	}
	store.receipt = receipt
	return nil
}
func (store *continuationApprovalMemoryStore) ResolveReceipt(_ context.Context, gateID string) (domaincontinuation.Receipt, error) {
	if store.receipt.Payload.GateID != gateID {
		return domaincontinuation.Receipt{}, continuationstoreport.ErrNotFound
	}
	return store.receipt, nil
}
func (store *continuationApprovalMemoryStore) PutDispositionIfAbsent(_ context.Context, disposition domaincontinuation.Disposition) error {
	if store.disposition.DispositionID != "" && !reflect.DeepEqual(store.disposition, disposition) {
		return errors.New("disposition conflict")
	}
	store.disposition = disposition
	return nil
}
func (store *continuationApprovalMemoryStore) ResolveDisposition(_ context.Context, gateID string) (domaincontinuation.Disposition, error) {
	if store.disposition.GateID != gateID {
		return domaincontinuation.Disposition{}, continuationstoreport.ErrNotFound
	}
	return store.disposition, nil
}
func (store *continuationApprovalMemoryStore) HasRecords(context.Context) (bool, error) {
	return store.receipt.ReceiptID != "" || store.disposition.DispositionID != "", nil
}

func TestControlledPIIApprovalArgumentsMarshalShape(t *testing.T) {
	body, err := json.Marshal(ControlledPIIApprovalArgumentsV1{})
	if err != nil || !bytes.Contains(body, []byte(`"schemaVersion"`)) {
		t.Fatalf("approval argument JSON shape changed unexpectedly: body=%s err=%v", body, err)
	}
}
