package piiauthorization

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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

func TestWitnessedEvidenceAuthorityReplaysExactClaimAndCurrentReceipt(t *testing.T) {
	contextValue, registry, ledger, binding := witnessedPIIEvidenceFixture(t, true)
	reader := &witnessedPIIReaderStub{snapshot: registryport.WitnessedSnapshot{Context: contextValue, Registry: registry}}
	authority, err := NewWitnessedEvidenceAuthority(reader)
	if err != nil {
		t.Fatal(err)
	}
	input := piiauthorizationport.EvidenceValidationV1{Context: contextValue, ClaimLedger: ledger, FieldBindings: []domainpii.FieldBindingV1{binding}}
	if err := authority.ValidateCurrent(context.Background(), input); err != nil || reader.calls != 1 {
		t.Fatalf("current witnessed evidence was rejected: calls=%d err=%v", reader.calls, err)
	}
	var leaked registryport.WitnessedSnapshotCapability
	if err := reader.WithWitnessedSnapshotAuthority(context.Background(), contextValue, func(
		snapshot registryport.WitnessedSnapshot,
		capability registryport.WitnessedSnapshotCapability,
	) error {
		leaked = capability
		return authority.ValidateWitnessed(context.Background(), input, snapshot, capability)
	}); err != nil {
		t.Fatalf("callback-scoped witnessed evidence was rejected: %v", err)
	}
	if authority.ValidateWitnessed(context.Background(), input, reader.snapshot, leaked) == nil {
		t.Fatal("held snapshot remained valid after its witness capability callback")
	}
	body, err := authority.RenderCurrentControlledPIIArtifactV2(context.Background(), input, piiauthorizationport.ControlledPIIArtifactRenderInputV2{
		ProjectionRulesetHash: domainsecurity.SHA256Hex([]byte("witnessed-pii-projection-rules")),
		TargetIdentityDigest:  domainsecurity.SHA256Hex([]byte("witnessed-pii-target")),
		RenderedAt:            time.Date(2026, 7, 16, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("witnessed exact source account was not rendered into a protected artifact: %v", err)
	}
	defer clearControlledArtifactBytesV1(body)
	artifact, err := domainpii.ParseControlledPIIArtifactV1(body)
	if err != nil || len(artifact.Fields) != 1 || artifact.Fields[0].ExactValue != "6222-0202-0202-0202-020" {
		t.Fatalf("protected artifact lost the witnessed exact account: artifact=%#v err=%v", artifact, err)
	}

	revoked, err := domainevidence.RevokeEvidenceReceipt(registry, ledger.EvidenceReceiptIDs[0], "authorization_revoked", time.Date(2026, 7, 16, 12, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	reader.snapshot.Registry = revoked
	if authority.ValidateCurrent(context.Background(), input) == nil {
		t.Fatal("revoked evidence remained controlled PII authority")
	}
	reader.snapshot.Registry = registry
	reader.snapshot.Context.ContextEpoch++
	if authority.ValidateCurrent(context.Background(), input) == nil {
		t.Fatal("mismatched witnessed snapshot context retained authority")
	}
	reader.err = errors.New("witness unavailable")
	if authority.ValidateCurrent(context.Background(), input) == nil {
		t.Fatal("unavailable witness failed open")
	}
}

func TestWitnessedEvidenceAuthorityRejectsV1CanonicalEvidenceAsSourceFieldAuthority(t *testing.T) {
	contextValue, registry, ledger, binding := witnessedPIIEvidenceFixture(t, false)
	reader := &witnessedPIIReaderStub{snapshot: registryport.WitnessedSnapshot{Context: contextValue, Registry: registry}}
	authority, err := NewWitnessedEvidenceAuthority(reader)
	if err != nil {
		t.Fatal(err)
	}
	input := piiauthorizationport.EvidenceValidationV1{
		Context: contextValue, ClaimLedger: ledger, FieldBindings: []domainpii.FieldBindingV1{binding},
	}
	if err := authority.ValidateCurrent(context.Background(), input); err != nil {
		t.Fatalf("V1 claim support unexpectedly failed before the source-field boundary: %v", err)
	}
	body, err := authority.RenderCurrentControlledPIIArtifactV2(context.Background(), input, piiauthorizationport.ControlledPIIArtifactRenderInputV2{
		ProjectionRulesetHash: domainsecurity.SHA256Hex([]byte("v1-rejected-projection-rules")),
		TargetIdentityDigest:  domainsecurity.SHA256Hex([]byte("v1-rejected-target")),
		RenderedAt:            time.Date(2026, 7, 16, 12, 1, 0, 0, time.UTC),
	})
	if err == nil || len(body) != 0 {
		t.Fatalf("V1 canonical evidence minted a protected exact-account artifact: bytes=%d err=%v", len(body), err)
	}
}

func TestBareReaderCannotAuthorizeCurrentSourceExactPII(t *testing.T) {
	contextValue, registry, _, _ := witnessedPIIEvidenceFixture(t, true)
	reader := bareWitnessedPIIReaderStub{snapshot: registryport.WitnessedSnapshot{Context: contextValue, Registry: registry}}
	authority, err := NewWitnessedEvidenceAuthority(reader)
	if err == nil || authority != nil {
		t.Fatalf("bare snapshot reader constructed source-exact PII authority: authority=%#v err=%v", authority, err)
	}
}

func witnessedPIIEvidenceFixture(t *testing.T, sourceBoundV2 bool) (
	domainsecurity.TurnSecurityContext,
	domainevidence.EvidenceReceiptRegistry,
	domainpublication.ClaimLedgerV1,
	domainpii.FieldBindingV1,
) {
	t.Helper()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	contextValue, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-witnessed-pii", TurnID: "turn-witnessed-pii", WorkspaceRealPath: "/workspace/witnessed-pii",
		CaseID: "case-witnessed-pii", CaseBindingHash: domainsecurity.SHA256Hex([]byte("witnessed-pii-binding")),
		ContextEpoch: 5, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	const (
		account      = "6222020202020202020"
		exactAccount = "6222-0202-0202-0202-020"
	)
	payload := domainevidence.NormalizedClaimPayload{SubjectID: "entity-a", EntityID: "entity-a", AccountID: account}
	rawHash := domainsecurity.SHA256Hex([]byte("immutable-raw-account-row"))
	material := domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts:         []domainevidence.CanonicalEvidenceFact{{FactID: "fact-account", ClaimType: domainevidence.ClaimAccount, NormalizedPayload: payload}},
	}
	if sourceBoundV2 {
		sourceField, err := domainevidence.NewSourceFieldBindingV2(domainevidence.SourceFieldBindingInputV2{
			FactID: "fact-account", ClaimType: domainevidence.ClaimAccount, CanonicalEntityID: "entity-a", CanonicalAccountID: account,
			SourceRecordID: "row-account", RawArtifactSHA256: rawHash,
			SourceRecordSHA256: domainsecurity.SHA256Hex([]byte("immutable-source-account-row")),
			SourceRecordPath:   "/record", SourceRecordIDPath: "/record/sourceRecordId", SourceEntityIDPath: "/record/entityId",
			SourceFieldPath:  "/record/accountId",
			SourceScalarKind: domainevidence.SourceFieldBindingScalarTextV2, SourceExactValue: exactAccount,
		})
		if err != nil {
			t.Fatal(err)
		}
		sourceFields, err := domainevidence.CanonicalSourceFieldBindingsV2([]domainevidence.SourceFieldBindingV2{sourceField})
		if err != nil {
			t.Fatal(err)
		}
		sourceFieldDigest, err := domainevidence.SourceFieldBindingSetDigestV2(sourceFields)
		if err != nil {
			t.Fatal(err)
		}
		material.SchemaVersion = domainevidence.CanonicalEvidenceVersionV2
		material.Purpose = domainevidence.CanonicalEvidencePurposeV2
		material.SourceFieldBindings = sourceFields
		material.SourceFieldBindingSetDigest = sourceFieldDigest
	}
	materialBody, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(materialBody)
	if err != nil {
		t.Fatal(err)
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("witnessed-pii-instance")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	settlementID := domainsecurity.SHA256Hex([]byte("witnessed-pii-settlement"))
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: domainevidence.EvidenceSettlementReceiptID(settlementID), Context: contextValue,
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("witnessed-pii-grant")), ToolCallID: piiTestHostToolCallID(t, "witnessed-pii"),
		ServerIdentity: serverIdentity, ServerVersion: "1.0.0", ConnectionEpoch: 1, ToolName: "mcp__funds__query",
		ArgsHash: domainsecurity.SHA256Hex([]byte("witnessed-pii-args")), ResultHash: domainsecurity.CanonicalJSONHash(canonical),
		SourceType: "transactions", DatasetSnapshotID: contextValue.DatasetSnapshotID, QueryHash: domainsecurity.SHA256Hex([]byte("witnessed-pii-query")),
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"entity-a"}, AccountIDs: []string{account}, Directions: []string{"out"},
			StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", SourceIDs: []string{"bank-flow"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("witnessed-pii-filter")),
		},
		Granularity: "transaction", Currency: "CNY", Timezone: "Asia/Shanghai", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{"row-account"}, RawSHA256: rawHash,
		TransformationLineage: []domainevidence.TransformationLineageStep{{
			StepID: "normalize-account", Transformer: "host-normalizer", TransformerVersion: "1.0.0",
			InputHash: rawHash, OutputHash: domainsecurity.CanonicalJSONHash(canonical),
		}},
		PIIClassification: domainevidence.PIIControlled, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(contextValue)
	if err != nil {
		t.Fatal(err)
	}
	registry, receipt, err := domainevidence.RegisterEvidenceReceipt(registry, draft, canonical, domainevidence.EvidenceSettlementProof{
		SettlementID: settlementID, PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("witnessed-pii-prepared-record")),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-account", ClaimType: domainevidence.ClaimAccount,
		NormalizedPayload: payload, EvidenceIDs: []string{receipt.ReceiptID}, CounterEvidenceIDs: []string{},
	}
	claimID := "claim-account"
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: claimID, Proposal: proposal, SupportState: domainevidence.ClaimVerified,
		EvidenceIDs: []string{receipt.ReceiptID}, CounterEvidenceIDs: []string{}, SupportedScope: &receipt.QueryRange,
		AllowedWording: []string{"exact_verified_fact"}, ProhibitedUpgrades: []string{"legal_characterization_without_review", "zero_or_nonexistence_upgrade"},
		VerifierReceiptID:  domainevidence.VerifierReceiptDigest(claimID, proposal.ClaimType, payload, []string{receipt.ReceiptID}, nil, domainevidence.ClaimVerified),
		VerificationReason: "exact_current_evidence_support", VerifiedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: contextValue, EvidenceReceiptIDs: []string{receipt.ReceiptID}, Claims: []domainevidence.ClaimRecord{claim}, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding := domainpii.FieldBindingV1{
		PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: claim.ClaimID, ClaimRecordDigest: claim.RecordDigest,
		ClaimType: claim.ClaimType, FieldName: "accountId", ValueSHA256: domainsecurity.SHA256Hex([]byte(exactAccount)),
		EvidenceReceiptIDs: []string{receipt.ReceiptID},
	}
	return contextValue, registry, ledger, binding
}

type witnessedPIIReaderStub struct {
	snapshot registryport.WitnessedSnapshot
	err      error
	calls    int
}

type bareWitnessedPIIReaderStub struct {
	snapshot registryport.WitnessedSnapshot
}

type witnessedPIICapabilityStub struct {
	active   bool
	snapshot registryport.WitnessedSnapshot
}

func (reader *witnessedPIIReaderStub) WithWitnessedSnapshot(_ context.Context, _ domainsecurity.TurnSecurityContext, callback func(registryport.WitnessedSnapshot) error) error {
	reader.calls++
	if reader.err != nil {
		return reader.err
	}
	return callback(reader.snapshot)
}

func (reader *witnessedPIIReaderStub) WithWitnessedSnapshotAuthority(
	ctx context.Context,
	_ domainsecurity.TurnSecurityContext,
	callback func(registryport.WitnessedSnapshot, registryport.WitnessedSnapshotCapability) error,
) error {
	reader.calls++
	if reader.err != nil {
		return reader.err
	}
	capability := &witnessedPIICapabilityStub{active: true, snapshot: reader.snapshot}
	defer func() { capability.active = false }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return callback(reader.snapshot, capability)
}

func (reader bareWitnessedPIIReaderStub) WithWitnessedSnapshot(
	_ context.Context,
	_ domainsecurity.TurnSecurityContext,
	callback func(registryport.WitnessedSnapshot) error,
) error {
	return callback(reader.snapshot)
}

func (capability *witnessedPIICapabilityStub) UseExact(snapshot registryport.WitnessedSnapshot, mutation func() error) error {
	if capability == nil || !capability.active || mutation == nil || !reflect.DeepEqual(snapshot, capability.snapshot) {
		return errors.New("inactive witnessed PII capability")
	}
	return mutation()
}
