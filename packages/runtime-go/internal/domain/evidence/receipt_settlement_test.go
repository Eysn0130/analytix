package evidence

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestHistoricalHostSettlementContentBindingCompatibility(t *testing.T) {
	input, key := preparedEvidenceSettlementFixture(t)
	binding, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: input.Context.WorkspaceRealPath, CaseID: input.Context.CaseID,
		State: domainsecurity.CaseBindingStateValid, CaseBindingHash: input.Context.CaseBindingHash,
		BindingSHA256: domainsecurity.SHA256Hex([]byte("historical-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	input.HostAuthority = &PreparedEvidenceHostAuthorityV1{Binding: binding, SelectionDigest: strings.Repeat("a", 64)}
	// This is the exact pre-extension wire type, with the original field order.
	legacyBody, _ := json.Marshal(struct {
		Binding         domainsecurity.CaseBindingObservationV1 `json:"binding"`
		SelectionDigest string                                  `json:"selectionDigest"`
	}{binding, input.HostAuthority.SelectionDigest})
	currentBody, _ := json.Marshal(input.HostAuthority)
	if !bytes.Equal(legacyBody, currentBody) {
		t.Fatal("absent content binding changed historical signing material")
	}
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(key, message), nil }
	makeRecord := func() PreparedEvidenceSettlement {
		input.ReceiptDraft.ReceiptID = EvidenceSettlementReceiptID(ComputeEvidenceSettlementID(input))
		input.ReceiptDraft.ReceiptDigest = evidenceReceiptDigest(input.ReceiptDraft)
		record, err := NewPreparedEvidenceSettlement(input, sign)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	legacy := makeRecord()
	body, err := PreparedEvidenceSettlementBytes(legacy)
	if err != nil || bytes.Contains(body, []byte("selectionContentDigest")) {
		t.Fatal("historical record acquired a content binding")
	}
	parsed, err := ParsePreparedEvidenceSettlement(body)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, _ := PreparedEvidenceSettlementBytes(parsed)
	if !bytes.Equal(body, roundTrip) || !bytes.Equal(EvidenceSettlementSigningBytes(legacy), EvidenceSettlementSigningBytes(parsed)) ||
		legacy.SettlementID != parsed.SettlementID || legacy.AuthoritySignature != parsed.AuthoritySignature || legacy.RecordDigest != parsed.RecordDigest {
		t.Fatal("historical host serialization changed bytes, signature or identity")
	}
	host := *input.HostAuthority
	host.SelectionContentDigest = strings.Repeat("b", 64)
	input.HostAuthority = &host
	current := makeRecord()
	if current.SettlementID == legacy.SettlementID {
		t.Fatal("new signed binding did not enter settlement identity")
	}
	for name, value := range map[string]string{"removed": "", "malformed": "bad", "tampered": strings.Repeat("c", 64)} {
		t.Run(name, func(t *testing.T) {
			candidate := current
			host := *current.HostAuthority
			host.SelectionContentDigest = value
			candidate.HostAuthority = &host
			if ValidatePreparedEvidenceSettlement(candidate) == nil {
				t.Fatal("unsigned content binding edit retained authority")
			}
		})
	}
}

func TestPreparedEvidenceSettlementIsDeterministicSignedAndNotRegistryAuthority(t *testing.T) {
	input, privateKey := preparedEvidenceSettlementFixture(t)
	record, err := NewPreparedEvidenceSettlement(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ReceiptID != "evr_"+record.SettlementID || record.ReceiptDraft.RegistrySequence != 0 || record.ReceiptDraft.RegistryIntegrityProof != "" {
		t.Fatalf("prepared record acquired registry authority: %#v", record)
	}
	marker, err := NewHostEvidenceSettlementMarker(record)
	if err != nil {
		t.Fatal(err)
	}
	if marker.PreparedRecordDigest != record.RecordDigest || marker.ReceiptID != record.ReceiptID {
		t.Fatalf("host settlement marker mismatch: %#v", marker)
	}
	if _, err := ParseHostEvidenceSettlementMarker(map[string]any{
		"schemaVersion": marker.SchemaVersion, "purpose": marker.Purpose, "settlementId": marker.SettlementID,
		"preparedRecordDigest": marker.PreparedRecordDigest, "receiptId": marker.ReceiptID, "markerDigest": marker.MarkerDigest,
		"providerExtra": true,
	}); err == nil {
		t.Fatal("provider-extended marker passed strict parsing")
	}
	retry := input
	retry.PreparedAt = input.PreparedAt.Add(time.Minute)
	retry.ReceiptDraft.IssuedAt = retry.PreparedAt.Format(time.RFC3339Nano)
	retry.ReceiptDraft.ReceiptDigest = evidenceReceiptDigest(retry.ReceiptDraft)
	if ComputeEvidenceSettlementID(retry) != record.SettlementID {
		t.Fatal("trusted retry time changed deterministic settlement identity")
	}
}

func TestHistoricalPreparedSettlementOmitsOptionalHostAuthority(t *testing.T) {
	input, privateKey := preparedEvidenceSettlementFixture(t)
	record, err := NewPreparedEvidenceSettlement(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.HostAuthority != nil {
		t.Fatal("historical settlement unexpectedly acquired host authority")
	}
	body, err := PreparedEvidenceSettlementBytes(record)
	if err != nil || strings.Contains(string(body), `"hostAuthority"`) {
		t.Fatalf("nil host authority was not omitted from historical bytes: err=%v body=%s", err, body)
	}
	parsed, err := ParsePreparedEvidenceSettlement(body)
	if err != nil || parsed.HostAuthority != nil || parsed.SettlementID != record.SettlementID {
		t.Fatalf("historical settlement roundtrip changed identity: parsed=%#v err=%v", parsed.HostAuthority, err)
	}
}

func TestPreparedEvidenceSettlementRejectsTamperAndInvalidTimeOrder(t *testing.T) {
	input, privateKey := preparedEvidenceSettlementFixture(t)
	record, err := NewPreparedEvidenceSettlement(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := record
	tampered.CanonicalEvidence = json.RawMessage(`{"schemaVersion":1,"facts":[]}`)
	if err := ValidatePreparedEvidenceSettlement(tampered); err == nil {
		t.Fatal("canonical evidence tamper passed prepared settlement validation")
	}
	badTime := input
	badTime.PreparedAt, _ = time.Parse(time.RFC3339Nano, input.Grant.ExpiresAt)
	badTime.ReceiptDraft.IssuedAt = badTime.PreparedAt.Format(time.RFC3339Nano)
	badTime.ReceiptDraft.ReceiptDigest = evidenceReceiptDigest(badTime.ReceiptDraft)
	if _, err := NewPreparedEvidenceSettlement(badTime, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil {
		t.Fatal("prepared settlement at grant expiry was accepted")
	}
	mismatchedIdentity, mismatchKey := preparedEvidenceSettlementFixtureWithIdentities(t,
		domainEvidenceTestIdentity(t, "analytix_funds", "legacy", "0.16.15", 4),
		domainEvidenceTestIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 4))
	if _, err := NewPreparedEvidenceSettlement(mismatchedIdentity, func(message []byte) ([]byte, error) {
		return ed25519.Sign(mismatchKey, message), nil
	}); err == nil {
		t.Fatal("grant/outcome and source-probe/receipt identities diverged inside a prepared settlement")
	}
}

func TestPreparedEvidenceSettlementNewWritesRequireHostAndExactResultIdentity(t *testing.T) {
	input, privateKey := preparedEvidenceSettlementFixture(t)
	rawProviderID := "provider_call_6222020202020202020"

	rawGrant := input.Grant
	rawGrant.ToolCallID = rawProviderID
	if record, err := NewPreparedEvidenceSettlement(inputWithSettlementGrant(input, rawGrant), func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil || record.RecordDigest != "" || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("raw provider identity reached a new prepared settlement: record=%#v err=%v", record, err)
	}

	wrongResult := input
	wrongResult.ResultItemID = "item_result_turn-settlement_" + rawProviderID
	if record, err := NewPreparedEvidenceSettlement(wrongResult, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil || record.RecordDigest != "" || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("non-canonical result identity reached a new prepared settlement: record=%#v err=%v", record, err)
	}
}

func TestHistoricalRawIdentitySettlementParsesButCannotRegainExecutionAuthority(t *testing.T) {
	input, privateKey := preparedEvidenceSettlementFixture(t)
	record, err := NewPreparedEvidenceSettlement(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rawProviderID := "provider_call_6222020202020202020"
	issuedAt, _ := time.Parse(time.RFC3339Nano, record.ExecutionGrant.IssuedAt)
	expiresAt, _ := time.Parse(time.RFC3339Nano, record.ExecutionGrant.ExpiresAt)
	rawGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: record.SecurityContext, Provider: record.ExecutionGrant.Provider, ServerIdentity: record.ExecutionGrant.ServerIdentity,
		ToolName: record.ExecutionGrant.ToolName, ToolCallID: rawProviderID, ConnectionEpoch: record.ExecutionGrant.ConnectionEpoch,
		ArgsHash: record.ExecutionGrant.ArgsHash, SchemaHash: record.ExecutionGrant.SchemaHash, ScopeHash: record.ExecutionGrant.ScopeHash,
		ReadOnly: record.ExecutionGrant.ReadOnly, ApprovalState: record.ExecutionGrant.ApprovalState, IssuedAt: issuedAt, ExpiresAt: expiresAt,
	})
	outcomeIssuedAt, _ := time.Parse(time.RFC3339Nano, record.ToolOutcome.IssuedAt)
	record.ExecutionGrant = rawGrant
	record.ToolOutcome = NewToolOutcome(ToolOutcomeInput{
		ToolName: rawGrant.ToolName, ToolCallID: rawProviderID, ContextDigest: rawGrant.ContextDigest,
		ExecutionGrantID: rawGrant.GrantID, CaseID: record.SecurityContext.CaseID, ContextEpoch: record.SecurityContext.ContextEpoch,
		DatasetSnapshotID: record.SecurityContext.DatasetSnapshotID, ServerIdentity: rawGrant.ServerIdentity,
		TransportStatus: record.ToolOutcome.TransportStatus, SemanticStatus: record.ToolOutcome.SemanticStatus,
		IsError: record.ToolOutcome.IsError, Blocker: record.ToolOutcome.Blocker, PartialCoverage: record.ToolOutcome.PartialCoverage,
		Data: record.ToolOutcome.Data, CandidateEvidenceReceipts: record.ToolOutcome.CandidateEvidenceReceipts,
		ReportedSemanticStatus: record.ToolOutcome.ReportedSemanticStatus, ReportedSafeToAnswer: record.ToolOutcome.ReportedSafeToAnswer,
		ReportedCaseID: record.ToolOutcome.ReportedCaseID, ReportedContextEpoch: record.ToolOutcome.ReportedContextEpoch,
		ReportedDatasetSnapshotID: record.ToolOutcome.ReportedDatasetSnapshotID, ReportedServerIdentity: record.ToolOutcome.ReportedServerIdentity,
		UntrustedMeta: record.ToolOutcome.UntrustedMeta, IssuedAt: outcomeIssuedAt,
	})
	record.ReceiptDraft.ExecutionGrantID = rawGrant.GrantID
	record.ReceiptDraft.ToolCallID = rawProviderID
	record.ResultItemID = "item_result_" + record.SecurityContext.TurnID + "_" + rawProviderID
	record.SettlementID = ComputeEvidenceSettlementID(preparedEvidenceSettlementInputFromRecord(record))
	record.ReceiptID = EvidenceSettlementReceiptID(record.SettlementID)
	record.ReceiptDraft.ReceiptID = record.ReceiptID
	record.ReceiptDraft.ReceiptDigest = evidenceReceiptDigest(record.ReceiptDraft)
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, EvidenceSettlementSigningBytes(record)))
	record.RecordDigest = preparedEvidenceSettlementDigest(record)

	if err := ValidatePreparedEvidenceSettlement(record); err != nil {
		t.Fatalf("historical raw-ID settlement lost audit readability: %v", err)
	}
	if err := ValidatePreparedEvidenceSettlementForExecution(record); err == nil || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("historical raw-ID settlement regained execution authority: %v", err)
	}
}

func inputWithSettlementGrant(input PreparedEvidenceSettlementInput, grant domainsecurity.ExecutionGrant) PreparedEvidenceSettlementInput {
	input.Grant = grant
	return input
}

func preparedEvidenceSettlementInputFromRecord(record PreparedEvidenceSettlement) PreparedEvidenceSettlementInput {
	return PreparedEvidenceSettlementInput{
		Context: record.SecurityContext, Grant: record.ExecutionGrant,
		ActiveGrantRegistrySequence: record.ActiveGrantRegistrySequence, ActiveGrantRegistryDigest: record.ActiveGrantRegistryDigest,
		SourceProbe: record.SourceProbe, ToolOutcome: record.ToolOutcome, CanonicalEvidence: record.CanonicalEvidence,
		ReceiptDraft: record.ReceiptDraft, QueryHash: record.QueryHash, ResultItemID: record.ResultItemID,
	}
}

func preparedEvidenceSettlementFixture(t *testing.T) (PreparedEvidenceSettlementInput, ed25519.PrivateKey) {
	return preparedEvidenceSettlementFixtureWithIdentities(t,
		domainEvidenceTestIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 4),
		domainEvidenceTestIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 4))
}

func domainEvidenceTestIdentity(t *testing.T, serverID, observedName, observedVersion string, epoch uint64) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(serverID, observedName, observedVersion, domainsecurity.SHA256Hex([]byte("domain-evidence-test-instance")), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func preparedEvidenceSettlementFixtureWithIdentities(t *testing.T, grantIdentity, probeIdentity string) (PreparedEvidenceSettlementInput, ed25519.PrivateKey) {
	t.Helper()
	base := evidenceReceiptTestTime()
	securityContext := evidenceReceiptTestContext(
		t, "thread-settlement", "turn-settlement", "case-a",
		domainsecurity.DatasetSnapshotIDPrefixV1+domainsecurity.SHA256Hex([]byte("snapshot-a")), 8,
	)
	toolName := "mcp__analytix_funds__query"
	toolCallID := evidenceTestHostToolCallID(t, "settlement")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: grantIdentity,
		ToolName: toolName, ToolCallID: toolCallID, ConnectionEpoch: 4,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: base.Add(time.Minute), ExpiresAt: base.Add(10 * time.Minute),
	})
	registry, err := domainsecurity.RegisterExecutionGrant(domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID), securityContext.ThreadID, grant, base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: probeIdentity, ConnectionEpoch: 4,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog")), SpecFingerprint: domainsecurity.SHA256Hex([]byte("spec")),
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		CheckedAt: base, Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.16",
			CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash, DatasetSnapshotID: securityContext.DatasetSnapshotID,
			Ready: true, ReadOnly: true, CheckedAt: base.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome := NewToolOutcome(ToolOutcomeInput{
		ToolName: toolName, ToolCallID: grant.ToolCallID, ContextDigest: securityContext.ContextDigest,
		ExecutionGrantID: grant.GrantID, CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ServerIdentity: grant.ServerIdentity,
		TransportStatus: TransportSuccess, SemanticStatus: SemanticSuccess, IssuedAt: base.Add(2 * time.Minute),
	})
	canonical, err := json.Marshal(CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersion,
		Facts: []CanonicalEvidenceFact{{
			FactID: "fact-a", ClaimType: ClaimAccount,
			NormalizedPayload: NormalizedClaimPayload{SubjectID: "entity-a", AccountID: "00123456789012345678"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	preparedAt := base.Add(3 * time.Minute)
	queryHash := domainsecurity.SHA256Hex([]byte("query"))
	raw := []byte(`{"result":"raw"}`)
	draftInput := EvidenceReceiptInput{
		ReceiptID: "pending", Context: securityContext, ExecutionGrantID: grant.GrantID, ToolCallID: grant.ToolCallID,
		ServerIdentity: probe.ServerIdentity, ServerVersion: "0.16.16", ConnectionEpoch: 4, ToolName: toolName,
		ArgsHash: grant.ArgsHash, ResultHash: domainsecurity.CanonicalJSONHash(canonical), SourceType: "transactions",
		DatasetSnapshotID: securityContext.DatasetSnapshotID, QueryHash: queryHash,
		QueryRange: EvidenceQueryRange{
			EntityIDs: []string{"entity-a"}, AccountIDs: []string{"00123456789012345678"}, Directions: []string{"out"},
			StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", SourceIDs: []string{"flow-a"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("filters")),
		},
		Granularity: "transaction", Currency: "CNY", Timezone: "Asia/Shanghai", PaginationCompleteness: PaginationComplete,
		SourceRecordIDs: []string{"row-a"}, RawSHA256: domainsecurity.SHA256Hex(raw),
		TransformationLineage: []TransformationLineageStep{}, PIIClassification: PIIMasked, IssuedAt: preparedAt,
	}
	provisional, err := NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	input := PreparedEvidenceSettlementInput{
		Context: securityContext, Grant: grant, ActiveGrantRegistrySequence: registry.Sequence,
		ActiveGrantRegistryDigest: registry.StateDigest, SourceProbe: probe, ToolOutcome: outcome,
		RawResult: raw, CanonicalEvidence: canonical, ReceiptDraft: provisional, QueryHash: queryHash,
		ResultItemID: domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, grant.ToolCallID), PreparedAt: preparedAt,
	}
	settlementID := ComputeEvidenceSettlementID(input)
	draftInput.ReceiptID = EvidenceSettlementReceiptID(settlementID)
	input.ReceiptDraft, err = NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	input.AuthorityKeyID = domainsecurity.SHA256Hex(publicKey)
	input.AuthorityPublicKey = publicKey
	return input, privateKey
}
