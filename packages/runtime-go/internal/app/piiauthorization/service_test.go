package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const (
	serviceTestAccount      = "00123456789012345678"
	serviceTestAccountExact = "0012-3456789012345678"
)

func TestPrepareControlledPIIApprovalReturnsOnlyExactHashBoundArguments(t *testing.T) {
	fixture := newServiceFixture(t)
	prepared, err := fixture.service.PrepareApproval(context.Background(), prepareApprovalInputFromIssueV1(fixture.input))
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := ParseControlledPIIApprovalArgumentsV1(prepared.PrivateArguments)
	if err != nil || arguments.ApprovalScopeDigest != prepared.ApprovalScopeDigest || arguments.ExpiresAt != prepared.ExpiresAt ||
		arguments.ProjectedContentSHA256 != fixture.input.ProjectedContentSHA256 || arguments.TargetIdentityDigest != fixture.input.TargetIdentityDigest {
		t.Fatalf("prepared approval lost exact bindings: prepared=%#v arguments=%#v err=%v", prepared, arguments, err)
	}
	if bytes.Contains(prepared.PrivateArguments, []byte(serviceTestAccountExact)) {
		t.Fatalf("prepared approval exposed the raw account: %s", prepared.PrivateArguments)
	}
	changed := prepareApprovalInputFromIssueV1(fixture.input)
	changed.ProjectedContentSHA256 = domainsecurity.SHA256Hex([]byte("other-controlled-report"))
	other, err := fixture.service.PrepareApproval(context.Background(), changed)
	if err != nil || other.ApprovalScopeDigest == prepared.ApprovalScopeDigest {
		t.Fatalf("changed report retained approval scope: other=%#v err=%v", other, err)
	}
	fixture.evidence.reject = true
	if _, err := fixture.service.PrepareApproval(context.Background(), prepareApprovalInputFromIssueV1(fixture.input)); err == nil {
		t.Fatal("approval was prepared after evidence revocation")
	}
}

func TestPIIAuthorizationRequiresCurrentApprovalEvidenceAndExactVerifiedClaim(t *testing.T) {
	fixture := newServiceFixture(t)
	grant, err := fixture.service.Issue(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	projection := fixture.controlledProjection(t, grant.RecordDigest, fixture.input.ProjectedContentSHA256)
	if err := fixture.service.ValidateCurrent(context.Background(), fixture.context, projection, fixture.input.TargetIdentityDigest); err != nil {
		t.Fatal(err)
	}
	if fixture.approval.calls < 2 || fixture.evidence.calls < 2 || fixture.currentCalls < 4 {
		t.Fatalf("current authorities were not revalidated: approval=%d evidence=%d context=%d", fixture.approval.calls, fixture.evidence.calls, fixture.currentCalls)
	}
	body, _ := domainpii.PIIProjectionGrantV1Bytes(grant)
	if bytes.Contains(body, []byte(serviceTestAccountExact)) {
		t.Fatalf("protected grant store leaked the raw account: %s", body)
	}
	if grant.RequesterUserID != fixture.context.UserID || grant.Context.ContextDigest != fixture.context.ContextDigest ||
		grant.FieldBindings[0].ValueSHA256 != domainsecurity.SHA256Hex([]byte(serviceTestAccountExact)) {
		t.Fatalf("issued grant lost its host bindings: %#v", grant)
	}
}

func TestControlledArtifactAuthorizationBindsCanonicalMetadataAndLeadingZeroValue(t *testing.T) {
	fixture := newServiceFixture(t)
	binding := fixture.input.FieldBindings[0]
	artifact, err := domainpii.NewControlledPIIArtifactV1(domainpii.ControlledPIIArtifactInputV1{
		SecurityContext: fixture.context, ClaimLedgerDigest: fixture.input.ClaimLedger.LedgerDigest,
		ProjectionRulesetHash: fixture.input.ProjectionRulesetHash,
		TargetIdentityDigest:  fixture.input.TargetIdentityDigest,
		Fields: []domainpii.ControlledPIIFieldV1{{
			PIIClass: binding.PIIClass, ClaimID: binding.ClaimID, ClaimRecordDigest: binding.ClaimRecordDigest,
			ClaimType: binding.ClaimType, FieldName: binding.FieldName, ExactValue: serviceTestAccountExact,
			ValueSHA256: binding.ValueSHA256, EvidenceReceiptIDs: append([]string(nil), binding.EvidenceReceiptIDs...),
		}},
		RenderedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainpii.ControlledPIIArtifactV1Bytes(artifact)
	if err != nil || !bytes.Contains(body, []byte(`"exactValue":"`+serviceTestAccountExact+`"`)) {
		t.Fatalf("canonical artifact did not preserve the authorized leading-zero account: err=%v", err)
	}
	fixture.input.ProjectedContentSHA256 = domainsecurity.SHA256Hex(body)
	grant, err := fixture.service.Issue(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	projection := fixture.controlledProjection(t, grant.RecordDigest, fixture.input.ProjectedContentSHA256)
	metadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := registryport.WitnessedSnapshot{Context: fixture.context}
	if err := fixture.service.ValidateControlledArtifactCurrentWithinSnapshot(
		context.Background(), fixture.context, projection, metadata,
		snapshot, &witnessedPIICapabilityStub{active: true, snapshot: snapshot},
	); err != nil {
		t.Fatal(err)
	}
	tampered := metadata
	tampered.ClaimLedgerDigest = domainsecurity.SHA256Hex([]byte("other-ledger"))
	if fixture.service.ValidateControlledArtifactCurrent(
		context.Background(), fixture.context, projection, tampered,
	) == nil {
		t.Fatal("controlled grant authorized artifact metadata for another claim ledger")
	}
}

func TestPIIAuthorizationRejectsFakeAuditCrossContextContentTargetExpiryAndRevokedAuthorities(t *testing.T) {
	fixture := newServiceFixture(t)
	grant, err := fixture.service.Issue(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	valid := fixture.controlledProjection(t, grant.RecordDigest, fixture.input.ProjectedContentSHA256)

	t.Run("fake audit", func(t *testing.T) {
		projection := fixture.controlledProjection(t, domainsecurity.SHA256Hex([]byte("fake-audit")), fixture.input.ProjectedContentSHA256)
		if fixture.service.ValidateCurrent(context.Background(), fixture.context, projection, fixture.input.TargetIdentityDigest) == nil {
			t.Fatal("nonempty fake audit digest became authorization")
		}
	})
	t.Run("cross context", func(t *testing.T) {
		other, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: "other-thread", TurnID: "other-turn", WorkspaceRealPath: "/workspace/other", CaseID: "other-case",
			CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")), ContextEpoch: 1, IssuedAt: fixture.now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if fixture.service.ValidateCurrent(context.Background(), other, valid, fixture.input.TargetIdentityDigest) == nil {
			t.Fatal("PII grant crossed a case context")
		}
	})
	t.Run("content", func(t *testing.T) {
		projection := fixture.controlledProjection(t, grant.RecordDigest, domainsecurity.SHA256Hex([]byte("different-report")))
		if fixture.service.ValidateCurrent(context.Background(), fixture.context, projection, fixture.input.TargetIdentityDigest) == nil {
			t.Fatal("PII grant authorized different report bytes")
		}
	})
	t.Run("target", func(t *testing.T) {
		if fixture.service.ValidateCurrent(context.Background(), fixture.context, valid, domainsecurity.SHA256Hex([]byte("different-target"))) == nil {
			t.Fatal("PII grant authorized a different target")
		}
	})
	t.Run("expired", func(t *testing.T) {
		fixture.clock = fixture.now.Add(20 * time.Minute)
		if !errors.Is(fixture.service.ValidateCurrent(context.Background(), fixture.context, valid, fixture.input.TargetIdentityDigest), ErrExpired) {
			t.Fatal("expired PII grant remained current")
		}
		fixture.clock = fixture.now.Add(time.Minute)
	})
	t.Run("approval revoked", func(t *testing.T) {
		fixture.approval.reject = true
		if fixture.service.ValidateCurrent(context.Background(), fixture.context, valid, fixture.input.TargetIdentityDigest) == nil {
			t.Fatal("revoked human approval remained current")
		}
		fixture.approval.reject = false
	})
	t.Run("evidence revoked", func(t *testing.T) {
		fixture.evidence.reject = true
		if fixture.service.ValidateCurrent(context.Background(), fixture.context, valid, fixture.input.TargetIdentityDigest) == nil {
			t.Fatal("revoked evidence remained current")
		}
		fixture.evidence.reject = false
	})
	t.Run("context stale", func(t *testing.T) {
		fixture.currentReject = true
		if fixture.service.ValidateCurrent(context.Background(), fixture.context, valid, fixture.input.TargetIdentityDigest) == nil {
			t.Fatal("stale context remained current")
		}
		fixture.currentReject = false
	})
}

func TestPIIAuthorizationRejectsWrongClaimEvidenceAndPartialClaimBeforePersist(t *testing.T) {
	fixture := newServiceFixture(t)
	wrongHash := fixture.input
	wrongHash.FieldBindings = append([]domainpii.FieldBindingV1(nil), fixture.input.FieldBindings...)
	wrongHash.FieldBindings[0].ValueSHA256 = domainsecurity.SHA256Hex([]byte("different-account"))
	if _, err := fixture.service.Issue(context.Background(), wrongHash); !errors.Is(err, ErrMismatch) {
		t.Fatalf("wrong field hash was not rejected: %v", err)
	}
	wrongEvidence := fixture.input
	wrongEvidence.FieldBindings = append([]domainpii.FieldBindingV1(nil), fixture.input.FieldBindings...)
	wrongEvidence.FieldBindings[0].EvidenceReceiptIDs = []string{"evr_" + domainsecurity.SHA256Hex([]byte("other-evidence"))}
	if _, err := fixture.service.Issue(context.Background(), wrongEvidence); !errors.Is(err, ErrMismatch) {
		t.Fatalf("mismatched claim citation was not rejected: %v", err)
	}
	partial := fixture.input
	partial.ClaimLedger = fixture.input.ClaimLedger
	partial.ClaimLedger.Claims = append([]domainevidence.ClaimRecord(nil), fixture.input.ClaimLedger.Claims...)
	partial.ClaimLedger.Claims[0].SupportState = domainevidence.ClaimPartial
	if _, err := fixture.service.Issue(context.Background(), partial); !errors.Is(err, ErrMismatch) {
		t.Fatalf("partial claim authorized full PII: %v", err)
	}
	if len(fixture.grants.records) != 0 || len(fixture.ledgers.records) != 0 {
		t.Fatalf("rejected issuance persisted authority: grants=%d ledgers=%d", len(fixture.grants.records), len(fixture.ledgers.records))
	}
}

type serviceFixture struct {
	service       *Service
	context       domainsecurity.TurnSecurityContext
	input         IssueInputV1
	now           time.Time
	clock         time.Time
	grants        *memoryGrantStore
	ledgers       *memoryLedgerStore
	authority     *testAuthority
	approval      *approvalStub
	evidence      *evidenceStub
	currentCalls  int
	currentReject bool
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pii-service", TurnID: "turn-pii-service", WorkspaceRealPath: "/workspace/pii-service",
		CaseID: "case-pii-service", CaseBindingHash: domainsecurity.SHA256Hex([]byte("pii-service-binding")), ContextEpoch: 3, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := "evr_" + domainsecurity.SHA256Hex([]byte("pii-service-evidence"))
	payload := domainevidence.NormalizedClaimPayload{SubjectID: "entity-a", AccountID: serviceTestAccount}
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-account", ClaimType: domainevidence.ClaimAccount,
		NormalizedPayload: payload, EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{},
	}
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: "claim-account", Proposal: proposal, SupportState: domainevidence.ClaimVerified,
		EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{},
		SupportedScope: &domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"entity-a"}, AccountIDs: []string{serviceTestAccount}, Directions: []string{}, SourceIDs: []string{"bank-flow"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("pii-service-scope")),
		},
		AllowedWording: []string{"exact account is verified"}, ProhibitedUpgrades: []string{"do not infer ownership"},
		VerifierReceiptID:  domainevidence.VerifierReceiptDigest("claim-account", domainevidence.ClaimAccount, payload, []string{evidenceID}, []string{}, domainevidence.ClaimVerified),
		VerificationReason: "exact account field match", VerifiedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: []domainevidence.ClaimRecord{claim}, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x76}, ed25519.SeedSize))
	authority := &testAuthority{privateKey: privateKey, publicKey: privateKey.Public().(ed25519.PublicKey)}
	fixture := &serviceFixture{
		context: securityContext, now: now, clock: now.Add(time.Minute), grants: &memoryGrantStore{records: map[string]domainpii.PIIProjectionGrantV1{}},
		ledgers: &memoryLedgerStore{records: map[string]domainpublication.ClaimLedgerV1{}}, authority: authority,
		approval: &approvalStub{}, evidence: &evidenceStub{},
	}
	sourceField, err := domainevidence.NewSourceFieldBindingV2(domainevidence.SourceFieldBindingInputV2{
		FactID: "fact-account", ClaimType: claim.ClaimType, CanonicalEntityID: "entity-a", CanonicalAccountID: serviceTestAccount,
		SourceRecordID: "row-account", RawArtifactSHA256: domainsecurity.SHA256Hex([]byte("raw-account-artifact")),
		SourceRecordSHA256: domainsecurity.SHA256Hex([]byte("row-account")), SourceRecordPath: "/record",
		SourceRecordIDPath: "/record/sourceRecordId", SourceEntityIDPath: "/record/entityId", SourceFieldPath: "/record/accountId",
		SourceScalarKind: domainevidence.SourceFieldBindingScalarTextV2,
		SourceExactValue: serviceTestAccountExact,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.evidence.sourceFields = []domainevidence.SourceFieldBindingV2{sourceField}
	service, err := New(Config{
		Authority: authority, Store: fixture.grants, Ledgers: fixture.ledgers, Approval: fixture.approval, Evidence: fixture.evidence,
		ValidateCurrent: func(_ context.Context, candidate domainsecurity.TurnSecurityContext) error {
			fixture.currentCalls++
			if fixture.currentReject || candidate.ContextDigest != securityContext.ContextDigest {
				return errors.New("context is stale")
			}
			return nil
		},
		Now: func() time.Time { return fixture.clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.service = service
	fixture.input = IssueInputV1{
		SecurityContext: securityContext, ClaimLedger: ledger,
		FieldBindings: []domainpii.FieldBindingV1{{
			PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: claim.ClaimID, ClaimRecordDigest: claim.RecordDigest,
			ClaimType: claim.ClaimType, FieldName: "accountId", ValueSHA256: domainsecurity.SHA256Hex([]byte(serviceTestAccountExact)),
			EvidenceReceiptIDs: append([]string(nil), claim.EvidenceIDs...),
		}},
		ProjectionRulesetHash:  domainsecurity.SHA256Hex([]byte("pii-service-rules")),
		ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("controlled report exact account")), PreservedControlledFieldCount: 1,
		TargetIdentityDigest:  domainsecurity.SHA256Hex([]byte("pii-service-target")),
		AllowedAccessActions:  []string{domainpii.ControlledArtifactAccessActionDisplayV1, domainpii.ControlledArtifactAccessActionExportV1},
		AccessPolicyDigest:    domainsecurity.SHA256Hex([]byte("pii-service-access-policy")),
		RetentionPolicyDigest: domainsecurity.SHA256Hex([]byte("pii-service-retention-policy")),
		RetentionUntil:        fixture.clock.Add(24 * time.Hour),
		ApprovalID:            "appr_pii_service_12345678", ApprovalRecordDigest: domainsecurity.SHA256Hex([]byte("pii-service-approval")),
		ExpiresAt: fixture.clock.Add(10 * time.Minute),
	}
	return fixture
}

func (fixture *serviceFixture) controlledProjection(t *testing.T, auditDigest, contentHash string) domainpublication.PIIProjectionV1 {
	t.Helper()
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{
		ProjectionClass: domainpublication.PIIProjectionControlledFull, RulesetHash: fixture.input.ProjectionRulesetHash,
		ProjectedContentSHA256: contentHash, RestrictedFieldCount: 1, PreservedControlledFieldCount: 1,
		AuthorizationAuditDigest: auditDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

type testAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

func (authority *testAuthority) KeyID() string { return domainsecurity.SHA256Hex(authority.publicKey) }
func (authority *testAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *testAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *testAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(publicKey, message, signature) {
		return errors.New("untrusted authority")
	}
	return nil
}

type memoryGrantStore struct {
	records map[string]domainpii.PIIProjectionGrantV1
}

func (store *memoryGrantStore) PutGrantIfAbsent(_ context.Context, grant domainpii.PIIProjectionGrantV1) error {
	if current, ok := store.records[grant.RecordDigest]; ok && !reflect.DeepEqual(current, grant) {
		return piiauthorizationport.ErrConflict
	}
	store.records[grant.RecordDigest] = grant
	return nil
}
func (store *memoryGrantStore) ResolveGrant(_ context.Context, digest string) (domainpii.PIIProjectionGrantV1, error) {
	grant, ok := store.records[digest]
	if !ok {
		return domainpii.PIIProjectionGrantV1{}, piiauthorizationport.ErrNotFound
	}
	return grant, nil
}

type memoryLedgerStore struct {
	records map[string]domainpublication.ClaimLedgerV1
}

func (store *memoryLedgerStore) PutIfAbsent(_ context.Context, ledger domainpublication.ClaimLedgerV1) error {
	if current, ok := store.records[ledger.LedgerDigest]; ok && !reflect.DeepEqual(current, ledger) {
		return errors.New("ledger conflict")
	}
	store.records[ledger.LedgerDigest] = ledger
	return nil
}
func (store *memoryLedgerStore) Resolve(_ context.Context, digest string) (domainpublication.ClaimLedgerV1, error) {
	ledger, ok := store.records[digest]
	if !ok {
		return domainpublication.ClaimLedgerV1{}, errors.New("ledger missing")
	}
	return ledger, nil
}

type approvalStub struct {
	calls  int
	reject bool
	last   piiauthorizationport.ApprovalValidationV1
}

func (stub *approvalStub) ValidateCurrent(_ context.Context, input piiauthorizationport.ApprovalValidationV1) error {
	stub.calls++
	stub.last = input
	if stub.reject || !strings.HasPrefix(input.ApprovalID, "appr_") || !domainsecurity.IsSHA256Hex(input.ApprovalScopeDigest) ||
		input.Context.ContextDigest == "" || input.ProjectedContentHash == "" {
		return errors.New("approval unavailable")
	}
	return nil
}

type evidenceStub struct {
	calls          int
	witnessedCalls int
	reject         bool
	rejectOnCall   int
	sourceFields   []domainevidence.SourceFieldBindingV2
}

func (stub *evidenceStub) ValidateWitnessed(
	_ context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	_ registryport.WitnessedSnapshot,
	_ registryport.WitnessedSnapshotCapability,
) error {
	stub.witnessedCalls++
	if stub.reject || input.Context.ContextDigest != input.ClaimLedger.ContextDigest || len(input.FieldBindings) == 0 {
		return errors.New("witnessed evidence unavailable")
	}
	return nil
}

func (stub *evidenceStub) ValidateWitnessedSourceFieldsV2(
	_ context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	_ registryport.WitnessedSnapshot,
	capability registryport.WitnessedSnapshotCapability,
) error {
	stub.witnessedCalls++
	if stub.reject || capability == nil || input.Context.ContextDigest != input.ClaimLedger.ContextDigest || len(input.FieldBindings) == 0 {
		return errors.New("witnessed source field evidence unavailable")
	}
	fields, err := controlledPIIFieldsFromEvidenceV1(input, stub.sourceFields)
	clearControlledPIIFieldsV1(fields)
	return err
}

func (stub *evidenceStub) ValidateCurrent(_ context.Context, input piiauthorizationport.EvidenceValidationV1) error {
	stub.calls++
	if stub.reject || stub.rejectOnCall > 0 && stub.calls >= stub.rejectOnCall ||
		input.Context.ContextDigest != input.ClaimLedger.ContextDigest || len(input.FieldBindings) == 0 {
		return errors.New("evidence unavailable")
	}
	return nil
}

func (stub *evidenceStub) ValidateCurrentSourceFieldsV2(
	_ context.Context,
	input piiauthorizationport.EvidenceValidationV1,
) error {
	stub.calls++
	if stub.reject || stub.rejectOnCall > 0 && stub.calls >= stub.rejectOnCall ||
		input.Context.ContextDigest != input.ClaimLedger.ContextDigest || len(input.FieldBindings) == 0 {
		return errors.New("source field evidence unavailable")
	}
	fields, err := controlledPIIFieldsFromEvidenceV1(input, stub.sourceFields)
	clearControlledPIIFieldsV1(fields)
	return err
}

func (stub *evidenceStub) RenderCurrentControlledPIIArtifactV2(
	_ context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	render piiauthorizationport.ControlledPIIArtifactRenderInputV2,
) ([]byte, error) {
	stub.calls++
	if stub.reject || stub.rejectOnCall > 0 && stub.calls >= stub.rejectOnCall ||
		input.Context.ContextDigest != input.ClaimLedger.ContextDigest || len(input.FieldBindings) == 0 {
		return nil, errors.New("source field evidence unavailable")
	}
	fields, err := controlledPIIFieldsFromEvidenceV1(input, stub.sourceFields)
	if err != nil {
		return nil, err
	}
	defer clearControlledPIIFieldsV1(fields)
	return renderControlledPIIArtifactBytesV2(input, render, fields)
}
