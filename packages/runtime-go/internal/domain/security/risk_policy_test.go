package security

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestThreadRiskPolicyV1AuthorityRoundTripAndTamperRejection(t *testing.T) {
	privateKey, publicKey := riskPolicyTestKey(31)
	policy := mustThreadRiskPolicyV1(t, privateKey, publicKey, ThreadRiskPolicyInputV1{
		ThreadID: "thread-a", WorkspaceRealPath: "/workspace/a", RiskClass: RiskClassCase,
		Origin: RiskPolicyOriginDesktopCaseEntry, SignalsDigest: SHA256Hex([]byte("desktop-case-entry")),
		IssuedAt: time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC), AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	})
	body, err := ThreadRiskPolicyV1Bytes(policy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseThreadRiskPolicyV1(body)
	if err != nil || parsed != policy {
		t.Fatalf("thread risk policy round trip failed: parsed=%#v err=%v", parsed, err)
	}
	if err := ValidateThreadRiskPolicyV1ForInstallation(policy, SHA256Hex(publicKey), publicKey); err != nil {
		t.Fatalf("trusted installation policy failed authority anchoring: %v", err)
	}
	attackerPrivateKey, attackerPublicKey := riskPolicyTestKey(33)
	attackerPolicy := mustThreadRiskPolicyV1(t, attackerPrivateKey, attackerPublicKey, ThreadRiskPolicyInputV1{
		ThreadID: policy.ThreadID, WorkspaceRealPath: policy.WorkspaceRealPath, RiskClass: policy.RiskClass,
		Origin: policy.Origin, SignalsDigest: policy.SignalsDigest, IssuedAt: time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC),
		AuthorityKeyID: SHA256Hex(attackerPublicKey), AuthorityPublicKey: attackerPublicKey,
	})
	if ValidateThreadRiskPolicyV1(attackerPolicy) != nil {
		t.Fatal("attacker policy fixture must be internally valid")
	}
	if err := ValidateThreadRiskPolicyV1ForInstallation(attackerPolicy, SHA256Hex(publicKey), publicKey); err == nil {
		t.Fatal("self-signed policy from an untrusted key received installation authority")
	}
	for name, mutate := range map[string]func(*ThreadRiskPolicyV1){
		"class":     func(value *ThreadRiskPolicyV1) { value.RiskClass = RiskClassGeneral },
		"workspace": func(value *ThreadRiskPolicyV1) { value.WorkspaceRealPath = "/workspace/other" },
		"signals":   func(value *ThreadRiskPolicyV1) { value.SignalsDigest = SHA256Hex([]byte("other")) },
		"signature": func(value *ThreadRiskPolicyV1) {
			value.AuthoritySignature = strings.Repeat("A", len(value.AuthoritySignature))
		},
		"digest": func(value *ThreadRiskPolicyV1) { value.PolicyDigest = SHA256Hex([]byte("forged")) },
	} {
		t.Run(name, func(t *testing.T) {
			tampered := policy
			mutate(&tampered)
			if ValidateThreadRiskPolicyV1(tampered) == nil {
				t.Fatal("tampered thread risk policy validated")
			}
		})
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	raw["safeToAnswer"] = true
	unknown, _ := json.Marshal(raw)
	if _, err := ParseThreadRiskPolicyV1(unknown); err == nil {
		t.Fatal("unknown thread risk policy property was accepted")
	}
	duplicate := bytes.Replace(body, []byte(`"threadId":"thread-a"`), []byte(`"threadId":"thread-a","threadId":"thread-b"`), 1)
	if _, err := ParseThreadRiskPolicyV1(duplicate); err == nil {
		t.Fatal("duplicate thread risk policy property was accepted")
	}
}

func TestThreadRiskPolicyV1TransitionIsMonotonic(t *testing.T) {
	privateKey, publicKey := riskPolicyTestKey(32)
	at := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
	general := mustThreadRiskPolicyV1(t, privateKey, publicKey, ThreadRiskPolicyInputV1{
		ThreadID: "thread-transition", WorkspaceRealPath: "/workspace/transition", RiskClass: RiskClassGeneral,
		Origin: RiskPolicyOriginGeneralWorkspace, SignalsDigest: SHA256Hex([]byte("general")), IssuedAt: at,
		AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	})
	casePolicy := mustThreadRiskPolicyV1(t, privateKey, publicKey, ThreadRiskPolicyInputV1{
		ThreadID: general.ThreadID, WorkspaceRealPath: general.WorkspaceRealPath, RiskClass: RiskClassCase,
		Origin: RiskPolicyOriginBindingMarkerPresent, SignalsDigest: SHA256Hex([]byte("marker")),
		PredecessorPolicyDigest: general.PolicyDigest, IssuedAt: at.Add(time.Second),
		AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	})
	if err := ValidateThreadRiskPolicyTransitionV1(general, casePolicy); err != nil {
		t.Fatalf("general-to-case transition failed: %v", err)
	}
	publication := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: casePolicy.PolicyDigest, RiskClass: casePolicy.RiskClass,
		Disposition: PublicationDispositionCaseEvidenceGate, CaseBindingState: CaseBindingStateValid,
		BindingObservationDigest: SHA256Hex([]byte("valid-binding")), BlockerCode: PublicationBlockerNone,
	})
	if err := ValidateTurnPublicationPolicyForThreadRiskPolicyV1(publication, casePolicy); err != nil {
		t.Fatalf("publication did not bind its signed thread policy: %v", err)
	}
	mismatchedPublication := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: general.PolicyDigest, RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseEvidenceGate, CaseBindingState: CaseBindingStateValid,
		BindingObservationDigest: SHA256Hex([]byte("valid-binding")), BlockerCode: PublicationBlockerNone,
	})
	if err := ValidateTurnPublicationPolicyForThreadRiskPolicyV1(mismatchedPublication, casePolicy); err == nil {
		t.Fatal("publication accepted a different thread policy digest")
	}
	downgrade := mustThreadRiskPolicyV1(t, privateKey, publicKey, ThreadRiskPolicyInputV1{
		ThreadID: general.ThreadID, WorkspaceRealPath: general.WorkspaceRealPath, RiskClass: RiskClassGeneral,
		Origin: RiskPolicyOriginGeneralWorkspace, SignalsDigest: SHA256Hex([]byte("downgrade")),
		PredecessorPolicyDigest: casePolicy.PolicyDigest, IssuedAt: at.Add(2 * time.Second),
		AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	})
	if err := ValidateThreadRiskPolicyTransitionV1(casePolicy, downgrade); err == nil {
		t.Fatal("case risk policy downgraded to general")
	}
	wrongPredecessor := mustThreadRiskPolicyV1(t, privateKey, publicKey, ThreadRiskPolicyInputV1{
		ThreadID: general.ThreadID, WorkspaceRealPath: general.WorkspaceRealPath, RiskClass: RiskClassCase,
		Origin: RiskPolicyOriginBindingMarkerPresent, SignalsDigest: SHA256Hex([]byte("wrong-lineage")),
		PredecessorPolicyDigest: SHA256Hex([]byte("wrong")), IssuedAt: at.Add(time.Second),
		AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	})
	if err := ValidateThreadRiskPolicyTransitionV1(general, wrongPredecessor); err == nil {
		t.Fatal("thread risk policy transition accepted wrong predecessor")
	}
	workspaceSuccessor := mustThreadRiskPolicyV1(t, privateKey, publicKey, ThreadRiskPolicyInputV1{
		ThreadID: general.ThreadID, WorkspaceRealPath: "/workspace/transition-next", RiskClass: RiskClassGeneral,
		Origin: RiskPolicyOriginGeneralWorkspace, SignalsDigest: SHA256Hex([]byte("workspace-rebind")),
		PredecessorPolicyDigest: general.PolicyDigest, IssuedAt: at.Add(time.Second),
		AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	})
	if err := ValidateThreadRiskPolicyTransitionV1(general, workspaceSuccessor); err != nil {
		t.Fatalf("signed general workspace successor failed: %v", err)
	}
}

func TestTurnPublicationPolicyV1RejectsInvalidCombinations(t *testing.T) {
	threadPolicyDigest := SHA256Hex([]byte("thread-policy"))
	observationDigest := SHA256Hex([]byte("binding-observation"))
	tests := []TurnPublicationPolicyInputV1{
		{ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: RiskClassGeneral, Disposition: PublicationDispositionGeneralOutput,
			CaseBindingState: CaseBindingStateMissing, BindingObservationDigest: observationDigest, BlockerCode: PublicationBlockerNone},
		{ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: RiskClassCase, Disposition: PublicationDispositionCaseEvidenceGate,
			CaseBindingState: CaseBindingStateValid, BindingObservationDigest: observationDigest, BlockerCode: PublicationBlockerNone},
		{ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: RiskClassCase, Disposition: PublicationDispositionCaseBoundaryOnly,
			CaseBindingState: CaseBindingStateInvalid, BindingObservationDigest: observationDigest, BlockerCode: PublicationBlockerCaseBindingInvalid},
	}
	for _, input := range tests {
		policy, err := NewTurnPublicationPolicyV1(input)
		if err != nil || ValidateTurnPublicationPolicyV1(policy) != nil {
			t.Fatalf("valid publication policy failed: input=%#v policy=%#v err=%v", input, policy, err)
		}
		body, _ := TurnPublicationPolicyV1Bytes(policy)
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		raw["modelRiskClass"] = "general"
		if _, err := ParseTurnPublicationPolicyV1(raw); err == nil {
			t.Fatal("unknown publication policy field was accepted")
		}
	}
	invalid := []TurnPublicationPolicyInputV1{
		{ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: RiskClassGeneral, Disposition: PublicationDispositionGeneralOutput,
			CaseBindingState: CaseBindingStateNotApplicable, BindingObservationDigest: observationDigest, BlockerCode: PublicationBlockerNone},
		{ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: RiskClassGeneral, Disposition: PublicationDispositionCaseBoundaryOnly,
			CaseBindingState: CaseBindingStateMissing, BindingObservationDigest: observationDigest, BlockerCode: PublicationBlockerCaseBindingMissing},
		{ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: RiskClassCase, Disposition: PublicationDispositionCaseEvidenceGate,
			CaseBindingState: CaseBindingStateMissing, BindingObservationDigest: observationDigest, BlockerCode: PublicationBlockerNone},
		{ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: RiskClassCase, Disposition: PublicationDispositionCaseBoundaryOnly,
			CaseBindingState: CaseBindingStateInvalid, BindingObservationDigest: observationDigest, BlockerCode: PublicationBlockerCaseBindingMissing},
	}
	for _, input := range invalid {
		if policy, err := NewTurnPublicationPolicyV1(input); err == nil {
			t.Fatalf("invalid publication combination was accepted: %#v", policy)
		}
	}
}

func TestLegacyGeneralRiskPolicyIsAuditOnlyForPublication(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x51}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	threadPolicy := mustThreadRiskPolicyV1(t, privateKey, publicKey, ThreadRiskPolicyInputV1{
		ThreadID: "thread-legacy-general", WorkspaceRealPath: "/workspace/legacy-general",
		RiskClass: RiskClassGeneral, Origin: RiskPolicyOriginLegacyMigration,
		SignalsDigest:  SHA256Hex([]byte("legacy-general-signals")),
		IssuedAt:       time.Date(2026, 7, 17, 11, 0, 0, 0, time.UTC),
		AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	})
	publication := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: threadPolicy.PolicyDigest, RiskClass: RiskClassGeneral,
		Disposition: PublicationDispositionGeneralOutput, CaseBindingState: CaseBindingStateMissing,
		BindingObservationDigest: SHA256Hex([]byte("missing-binding")), BlockerCode: PublicationBlockerNone,
	})
	if ValidateThreadRiskPolicyV1(threadPolicy) != nil {
		t.Fatal("legacy general policy must remain parseable for audit")
	}
	if err := ValidateTurnPublicationPolicyForThreadRiskPolicyV1(publication, threadPolicy); err == nil {
		t.Fatal("legacy general risk policy authorized live general publication")
	}
}

func TestCaseBoundaryPolicyAllowsOnlyTypedAuthorityAndSnapshotBlockers(t *testing.T) {
	threadDigest := SHA256Hex([]byte("case-boundary-thread-policy"))
	observationDigest := SHA256Hex([]byte("case-boundary-observation"))
	for _, test := range []struct {
		name    string
		state   string
		blocker string
	}{
		{"valid-snapshot-unavailable", CaseBindingStateValid, PublicationBlockerDatasetSnapshotUnavailable},
		{"valid-snapshot-corrupt", CaseBindingStateValid, PublicationBlockerDatasetSnapshotCorrupt},
		{"valid-risk-unavailable", CaseBindingStateValid, PublicationBlockerRiskAuthorityUnavailable},
		{"missing-risk-indeterminate", CaseBindingStateMissing, PublicationBlockerRiskAuthorityIndeterminate},
		{"invalid-risk-inconsistent", CaseBindingStateInvalid, PublicationBlockerRiskAuthorityInconsistent},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewTurnPublicationPolicyV1(TurnPublicationPolicyInputV1{
				ThreadRiskPolicyDigest: threadDigest, RiskClass: RiskClassCase,
				Disposition: PublicationDispositionCaseBoundaryOnly, CaseBindingState: test.state,
				BindingObservationDigest: observationDigest, BlockerCode: test.blocker,
			})
			if err != nil || ValidateTurnPublicationPolicyV1(policy) != nil {
				t.Fatalf("typed boundary blocker rejected: policy=%#v err=%v", policy, err)
			}
		})
	}
	for _, test := range []struct {
		state   string
		blocker string
	}{
		{CaseBindingStateMissing, PublicationBlockerDatasetSnapshotUnavailable},
		{CaseBindingStateValid, PublicationBlockerCaseBindingMissing},
		{CaseBindingStateNotApplicable, PublicationBlockerRiskAuthorityUnavailable},
		{CaseBindingStateValid, "model_selected_blocker"},
	} {
		if _, err := NewTurnPublicationPolicyV1(TurnPublicationPolicyInputV1{
			ThreadRiskPolicyDigest: threadDigest, RiskClass: RiskClassCase,
			Disposition: PublicationDispositionCaseBoundaryOnly, CaseBindingState: test.state,
			BindingObservationDigest: observationDigest, BlockerCode: test.blocker,
		}); err == nil {
			t.Fatalf("invalid boundary combination accepted: state=%s blocker=%s", test.state, test.blocker)
		}
	}
}

func TestTurnSecurityContextV2BindsPublicationPolicyAndSeparatesAuditAuthority(t *testing.T) {
	at := time.Date(2026, 7, 12, 3, 0, 0, 0, time.UTC)
	casePolicy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("case-thread-policy")), RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseEvidenceGate, CaseBindingState: CaseBindingStateValid,
		BindingObservationDigest: SHA256Hex([]byte("valid-binding")), BlockerCode: PublicationBlockerNone,
	})
	input := TurnSecurityContextInput{
		ThreadID: "thread-v2", TurnID: "turn-v2", WorkspaceRealPath: "/workspace/v2", TenantID: "local", UserID: "local",
		CaseID: "case-v2", CaseBindingHash: SHA256Hex([]byte("case-binding")),
		DatasetSnapshotID:  DatasetSnapshotIDPrefixV2 + SHA256Hex([]byte("snapshot-v2")),
		SourceManifestHash: SHA256Hex([]byte("manifest-v2")), ContextEpoch: 4, IssuedAt: at, PublicationPolicy: casePolicy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(t, "thread-v2", "/workspace/v2", RiskClassCase, casePolicy.ThreadRiskPolicyDigest),
	}
	context, err := NewTurnSecurityContextV2(input)
	if err != nil || ValidateTurnSecurityContextForExecution(context) != nil || !TurnSecurityContextAllowsCaseEvidence(context) ||
		!TurnSecurityContextRequiresFinalEvidenceGate(context) {
		t.Fatalf("valid V2 case context failed: context=%#v err=%v", context, err)
	}
	changedPolicy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: casePolicy.ThreadRiskPolicyDigest, RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseEvidenceGate, CaseBindingState: CaseBindingStateValid,
		BindingObservationDigest: SHA256Hex([]byte("other-valid-binding")), BlockerCode: PublicationBlockerNone,
	})
	changedInput := input
	changedInput.PublicationPolicy = changedPolicy
	changed, err := NewTurnSecurityContextV2(changedInput)
	if err != nil || changed.ContextDigest == context.ContextDigest {
		t.Fatalf("publication policy did not change context digest: changed=%#v err=%v", changed, err)
	}
	body, err := json.Marshal(context)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseTurnSecurityContext(json.RawMessage(body))
	if err != nil || parsed != context {
		t.Fatalf("V2 context round trip failed: parsed=%#v err=%v body=%s", parsed, err, body)
	}
	reencoded, err := json.Marshal(parsed)
	if err != nil || !bytes.Equal(reencoded, body) {
		t.Fatalf("V2 context encoding was not canonical: got=%s want=%s err=%v", reencoded, body, err)
	}
	legacy := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: input.ThreadID, TurnID: input.TurnID, WorkspaceRealPath: input.WorkspaceRealPath,
		CaseID: input.CaseID, CaseBindingHash: input.CaseBindingHash, DatasetSnapshotID: input.DatasetSnapshotID,
		SourceManifestHash: input.SourceManifestHash, ContextEpoch: input.ContextEpoch, IssuedAt: input.IssuedAt,
	})
	if ValidateTurnSecurityContext(legacy) != nil {
		t.Fatal("legacy V1 audit context must remain parseable")
	}
	if err := ValidateTurnSecurityContextForExecution(legacy); err == nil {
		t.Fatal("legacy V1 context was treated as execution authority")
	}
	grant := NewExecutionGrant(ExecutionGrantInput{
		Context: context, Provider: "test", ServerIdentity: "host:builtin", ToolName: "read_file", ToolCallID: securityTestHostToolCallID("risk-v2"),
		ArgsHash: SHA256Hex([]byte("args")), SchemaHash: SHA256Hex([]byte("schema")), ScopeHash: SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: at, ExpiresAt: at.Add(time.Minute),
	})
	if err := ValidateExecutionGrantForContext(grant, context); err != nil {
		t.Fatalf("V2-bound execution grant failed validation: %v", err)
	}
	legacyGrant := NewExecutionGrant(ExecutionGrantInput{
		Context: legacy, Provider: "test", ServerIdentity: "host:builtin", ToolName: "read_file", ToolCallID: securityTestHostToolCallID("risk-v1"),
		ArgsHash: SHA256Hex([]byte("args")), SchemaHash: SHA256Hex([]byte("schema")), ScopeHash: SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: at, ExpiresAt: at.Add(time.Minute),
	})
	if ValidateExecutionGrant(legacyGrant) != nil {
		t.Fatal("legacy grant must remain parseable for audit")
	}
	if err := ValidateExecutionGrantForContext(legacyGrant, legacy); err == nil {
		t.Fatal("legacy V1 context and grant were treated as execution authority")
	}
	replayed := grant
	replayed.TurnID = "turn-other"
	replayed.GrantID = executionGrantHash(replayed)
	if err := ValidateExecutionGrantForContext(replayed, context); err == nil {
		t.Fatal("execution grant replayed against a mismatched turn")
	}
	if _, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: "thread", TurnID: "turn", WorkspaceRealPath: "/workspace", TenantID: "local", UserID: "local",
		ContextEpoch: 1, IssuedAt: at,
	}); err == nil {
		t.Fatal("V2 constructor silently inferred a general publication policy")
	}
}

func TestTurnSecurityContextV2RequiresCanonicalUTCIssuedAt(t *testing.T) {
	context := mustGeneralTurnSecurityContextV2(t, "thread-time", "turn-time", "/workspace/time")
	if !TurnSecurityContextIsGeneral(context) || TurnSecurityContextRequiresFinalEvidenceGate(context) ||
		TurnSecurityContextIsBoundaryOnly(context) {
		t.Fatal("general V2 context classification is invalid")
	}
	nonCanonical := context
	issuedAt, err := time.Parse(time.RFC3339Nano, context.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	nonCanonical.IssuedAt = issuedAt.In(time.FixedZone("offset", 8*60*60)).Format(time.RFC3339Nano)
	nonCanonical.ContextDigest = ""
	nonCanonical.ContextDigest = hashContract(nonCanonical)
	if ValidateTurnSecurityContext(nonCanonical) == nil {
		t.Fatal("V2 context accepted a non-canonical timestamp after digest recomputation")
	}
}

func TestTurnSecurityContextV2BoundaryCannotAuthorizeExecution(t *testing.T) {
	workspace := "/workspace/boundary"
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("case-thread-policy")), RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseBoundaryOnly, CaseBindingState: CaseBindingStateInvalid,
		BindingObservationDigest: SHA256Hex([]byte("invalid-binding")), BlockerCode: PublicationBlockerCaseBindingInvalid,
	})
	context, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: "thread-boundary", TurnID: "turn-boundary", WorkspaceRealPath: workspace, TenantID: "local", UserID: "local",
		CaseID: UnboundCaseID, CaseBindingHash: UnboundCaseBindingHash(workspace), DatasetSnapshotID: NoDatasetSnapshotID,
		SourceManifestHash: EmptySourceManifestHash, ContextEpoch: 2, IssuedAt: time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC),
		PublicationPolicy: policy, RiskAuthorityBinding: NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil || ValidateTurnSecurityContext(context) != nil || !TurnSecurityContextRequiresFinalEvidenceGate(context) {
		t.Fatalf("valid boundary context failed: context=%#v err=%v", context, err)
	}
	if err := ValidateTurnSecurityContextForExecution(context); err == nil || TurnSecurityContextAllowsCaseEvidence(context) ||
		!TurnSecurityContextIsBoundaryOnly(context) || TurnSecurityContextIsGeneral(context) {
		t.Fatal("case boundary context authorized execution or evidence")
	}
	tampered := context
	tampered.PublicationPolicy.BlockerCode = PublicationBlockerNone
	if ValidateTurnSecurityContext(tampered) == nil {
		t.Fatal("tampered nested publication policy validated")
	}
	badInput := TurnSecurityContextInput{
		ThreadID: context.ThreadID, TurnID: context.TurnID, WorkspaceRealPath: workspace, TenantID: "local", UserID: "local",
		CaseID: "case-forged", CaseBindingHash: SHA256Hex([]byte("forged")), DatasetSnapshotID: "snapshot-forged",
		SourceManifestHash: EmptySourceManifestHash, ContextEpoch: context.ContextEpoch, IssuedAt: time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC),
		PublicationPolicy: policy, RiskAuthorityBinding: NewQuarantinedRiskAuthorityBindingV1(),
	}
	if _, err := NewTurnSecurityContextV2(badInput); err == nil {
		t.Fatal("boundary policy accepted a fact-capable case binding")
	}
}

func TestTurnSecurityContextV2QuarantinedAuthorityIsBoundaryOnly(t *testing.T) {
	general := mustGeneralTurnSecurityContextV2(t, "thread-quarantine-general", "turn-quarantine-general", "/workspace/quarantine-general")
	generalInput := TurnSecurityContextInput{
		ThreadID: general.ThreadID, TurnID: general.TurnID, WorkspaceRealPath: general.WorkspaceRealPath,
		TenantID: general.TenantID, UserID: general.UserID, CaseID: general.CaseID, CaseBindingHash: general.CaseBindingHash,
		DatasetSnapshotID: general.DatasetSnapshotID, SourceManifestHash: general.SourceManifestHash,
		ContextEpoch: general.ContextEpoch, IssuedAt: time.Date(2026, 7, 12, 5, 0, 0, 0, time.UTC),
		PublicationPolicy: general.PublicationPolicy, RiskAuthorityBinding: NewQuarantinedRiskAuthorityBindingV1(),
	}
	if _, err := NewTurnSecurityContextV2(generalInput); err == nil {
		t.Fatal("quarantined risk authority authorized a general context")
	}

	caseContext := mustCaseTurnSecurityContextV2(
		t, "thread-quarantine-case", "turn-quarantine-case", "/workspace/quarantine-case", "case-quarantine", 2,
		time.Date(2026, 7, 12, 5, 1, 0, 0, time.UTC),
	)
	caseInput := TurnSecurityContextInput{
		ThreadID: caseContext.ThreadID, TurnID: caseContext.TurnID, WorkspaceRealPath: caseContext.WorkspaceRealPath,
		TenantID: caseContext.TenantID, UserID: caseContext.UserID, CaseID: caseContext.CaseID,
		CaseBindingHash: caseContext.CaseBindingHash, DatasetSnapshotID: caseContext.DatasetSnapshotID,
		SourceManifestHash: caseContext.SourceManifestHash, ContextEpoch: caseContext.ContextEpoch,
		IssuedAt: time.Date(2026, 7, 12, 5, 1, 0, 0, time.UTC), PublicationPolicy: caseContext.PublicationPolicy,
		RiskAuthorityBinding: NewQuarantinedRiskAuthorityBindingV1(),
	}
	if _, err := NewTurnSecurityContextV2(caseInput); err == nil {
		t.Fatal("quarantined risk authority authorized a case evidence context")
	}
}

func TestTurnSecurityContextV2DigestBindsRiskAuthorityObservation(t *testing.T) {
	context := mustGeneralTurnSecurityContextV2(t, "thread-risk-digest", "turn-risk-digest", "/workspace/risk-digest")
	for name, mutate := range map[string]func(*RiskAuthorityBindingV1){
		"index digest": func(binding *RiskAuthorityBindingV1) { binding.IndexDigest = SHA256Hex([]byte("other-index")) },
		"generation":   func(binding *RiskAuthorityBindingV1) { binding.Generation++ },
		"checkpoint digest": func(binding *RiskAuthorityBindingV1) {
			binding.CheckpointDigest = SHA256Hex([]byte("other-checkpoint"))
		},
		"observation digest": func(binding *RiskAuthorityBindingV1) {
			binding.ObservationDigest = SHA256Hex([]byte("other-observation"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := context
			mutate(&tampered.RiskAuthorityBinding)
			if ValidateTurnSecurityContext(tampered) == nil {
				t.Fatal("risk authority field changed without invalidating context digest")
			}
		})
	}

	input := TurnSecurityContextInput{
		ThreadID: context.ThreadID, TurnID: context.TurnID, WorkspaceRealPath: context.WorkspaceRealPath,
		TenantID: context.TenantID, UserID: context.UserID, CaseID: context.CaseID, CaseBindingHash: context.CaseBindingHash,
		DatasetSnapshotID: context.DatasetSnapshotID, SourceManifestHash: context.SourceManifestHash,
		ContextEpoch: context.ContextEpoch, IssuedAt: time.Date(2026, 7, 12, 5, 0, 0, 0, time.UTC),
		PublicationPolicy: context.PublicationPolicy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(
			t, context.ThreadID, context.WorkspaceRealPath, RiskClassGeneral, context.PublicationPolicy.ThreadRiskPolicyDigest,
		),
	}
	rebound, err := NewTurnSecurityContextV2(input)
	if err != nil || rebound.ContextDigest == context.ContextDigest {
		t.Fatalf("distinct witnessed observation was not bound into context digest: digest=%s err=%v", rebound.ContextDigest, err)
	}
}

func TestUnresolvedSnapshotCannotAuthorizeExecution(t *testing.T) {
	threadID, workspace, caseID := "thread-unresolved-snapshot", "/workspace/unresolved-snapshot", "case-unresolved"
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("case-policy:" + threadID)), RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseEvidenceGate, CaseBindingState: CaseBindingStateValid,
		BindingObservationDigest: SHA256Hex([]byte("binding-observation:" + caseID)), BlockerCode: PublicationBlockerNone,
	})
	bindingHash := SHA256Hex([]byte("binding:" + caseID))
	_, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-unresolved-snapshot", WorkspaceRealPath: workspace,
		TenantID: LocalTenantID, UserID: LocalUserID, CaseID: caseID, CaseBindingHash: bindingHash,
		DatasetSnapshotID: UnresolvedSnapshotMark + bindingHash, SourceManifestHash: SHA256Hex([]byte("manifest:" + caseID)),
		ContextEpoch: 2, IssuedAt: time.Date(2026, 7, 12, 5, 2, 0, 0, time.UTC), PublicationPolicy: policy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(t, threadID, workspace, RiskClassCase, policy.ThreadRiskPolicyDigest),
	})
	if err == nil {
		t.Fatal("unresolved dataset placeholder authorized case evidence execution")
	}
}

func TestCaseEvidenceRequiresConcreteDatasetSnapshotIDV2(t *testing.T) {
	threadID, workspace, caseID := "thread-snapshot-format", "/workspace/snapshot-format", "case-snapshot-format"
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("case-policy:" + threadID)), RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseEvidenceGate, CaseBindingState: CaseBindingStateValid,
		BindingObservationDigest: SHA256Hex([]byte("binding-observation:" + caseID)), BlockerCode: PublicationBlockerNone,
	})
	bindingHash := SHA256Hex([]byte("binding:" + caseID))
	base := TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-snapshot-format", WorkspaceRealPath: workspace,
		TenantID: LocalTenantID, UserID: LocalUserID, CaseID: caseID, CaseBindingHash: bindingHash,
		SourceManifestHash: SHA256Hex([]byte("manifest:" + caseID)), ContextEpoch: 2,
		IssuedAt: time.Date(2026, 7, 12, 5, 4, 0, 0, time.UTC), PublicationPolicy: policy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(t, threadID, workspace, RiskClassCase, policy.ThreadRiskPolicyDigest),
	}
	for _, invalid := range []string{
		NoDatasetSnapshotID,
		UnresolvedSnapshotMark + bindingHash,
		"snapshot-free-form",
		DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("legacy-v1-audit-only")),
		DatasetSnapshotIDPrefixV1 + strings.ToUpper(SHA256Hex([]byte("uppercase"))),
		DatasetSnapshotIDPrefixV1 + strings.Repeat("a", 63),
		DatasetSnapshotIDPrefixV2 + strings.ToUpper(SHA256Hex([]byte("uppercase"))),
		DatasetSnapshotIDPrefixV2 + strings.Repeat("a", 63),
	} {
		input := base
		input.DatasetSnapshotID = invalid
		if _, err := NewTurnSecurityContextV2(input); err == nil {
			t.Fatalf("non-concrete dataset snapshot authorized case evidence: %q", invalid)
		}
	}
	valid := base
	valid.DatasetSnapshotID = DatasetSnapshotIDPrefixV2 + SHA256Hex([]byte("concrete-snapshot"))
	context, err := NewTurnSecurityContextV2(valid)
	if err != nil || ValidateTurnSecurityContextForExecution(context) != nil {
		t.Fatalf("concrete dataset snapshot failed execution validation: context=%#v err=%v", context, err)
	}
}

func TestTurnSecurityContextV2StrictJSONRejectsUnknownAndDuplicateFields(t *testing.T) {
	context := mustGeneralTurnSecurityContextV2(t, "thread-json", "turn-json", "/workspace/json")
	body, _ := json.Marshal(context)
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	raw["safeToAnswer"] = true
	if _, err := ParseTurnSecurityContext(raw); err == nil {
		t.Fatal("unknown V2 context property was accepted")
	}
	duplicate := bytes.Replace(body, []byte(`"turnId":"turn-json"`), []byte(`"turnId":"turn-json","turnId":"turn-forged"`), 1)
	var parsed TurnSecurityContext
	if err := json.Unmarshal(duplicate, &parsed); err == nil {
		t.Fatal("duplicate V2 context property was accepted")
	}
	var nested map[string]any
	_ = json.Unmarshal(body, &nested)
	policy := nested["publicationPolicy"].(map[string]any)
	policy["supportStatus"] = "verified"
	if _, err := ParseTurnSecurityContext(nested); err == nil {
		t.Fatal("unknown nested publication policy property was accepted")
	}
	var nestedRisk map[string]any
	_ = json.Unmarshal(body, &nestedRisk)
	riskBinding := nestedRisk["riskAuthorityBinding"].(map[string]any)
	riskBinding["safeToAnswer"] = true
	if _, err := ParseTurnSecurityContext(nestedRisk); err == nil {
		t.Fatal("unknown nested risk authority property was accepted")
	}
	legacy := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread-v1-json", TurnID: "turn-v1-json", WorkspaceRealPath: "/workspace/v1-json",
		IssuedAt: time.Date(2026, 7, 12, 5, 3, 0, 0, time.UTC),
	})
	legacyBody, _ := json.Marshal(legacy)
	legacyWithRisk := bytes.Replace(legacyBody, []byte(`"contextDigest"`), []byte(`"riskAuthorityBinding":{"schemaVersion":1},"contextDigest"`), 1)
	if err := json.Unmarshal(legacyWithRisk, &parsed); err == nil {
		t.Fatal("V1 context accepted a V2 risk authority field")
	}
}

func mustGeneralTurnSecurityContextV2(t *testing.T, threadID, turnID, workspace string) TurnSecurityContext {
	t.Helper()
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("general-policy:" + threadID)), RiskClass: RiskClassGeneral,
		Disposition: PublicationDispositionGeneralOutput, CaseBindingState: CaseBindingStateMissing,
		BindingObservationDigest: SHA256Hex([]byte("missing:" + workspace)), BlockerCode: PublicationBlockerNone,
	})
	context, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, TenantID: "local", UserID: "local",
		CaseID: UnboundCaseID, CaseBindingHash: UnboundCaseBindingHash(workspace), DatasetSnapshotID: NoDatasetSnapshotID,
		SourceManifestHash: EmptySourceManifestHash, ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 12, 5, 0, 0, 0, time.UTC),
		PublicationPolicy:    policy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(t, threadID, workspace, RiskClassGeneral, policy.ThreadRiskPolicyDigest),
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func mustCaseTurnSecurityContextV2(t *testing.T, threadID, turnID, workspace, caseID string, epoch uint64, issuedAt time.Time) TurnSecurityContext {
	t.Helper()
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("case-policy:" + threadID)), RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseEvidenceGate, CaseBindingState: CaseBindingStateValid,
		BindingObservationDigest: SHA256Hex([]byte("binding-observation:" + caseID)), BlockerCode: PublicationBlockerNone,
	})
	context, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, TenantID: "local", UserID: "local",
		CaseID: caseID, CaseBindingHash: SHA256Hex([]byte("binding:" + caseID)),
		DatasetSnapshotID:  DatasetSnapshotIDPrefixV2 + SHA256Hex([]byte("snapshot:"+caseID)),
		SourceManifestHash: SHA256Hex([]byte("manifest:" + caseID)), ContextEpoch: epoch, IssuedAt: issuedAt,
		PublicationPolicy:    policy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(t, threadID, workspace, RiskClassCase, policy.ThreadRiskPolicyDigest),
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func mustTurnPublicationPolicyV1(t *testing.T, input TurnPublicationPolicyInputV1) TurnPublicationPolicyV1 {
	t.Helper()
	policy, err := NewTurnPublicationPolicyV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func mustThreadRiskPolicyV1(t *testing.T, privateKey ed25519.PrivateKey, _ ed25519.PublicKey, input ThreadRiskPolicyInputV1) ThreadRiskPolicyV1 {
	t.Helper()
	policy, err := NewThreadRiskPolicyV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func riskPolicyTestKey(seed byte) (ed25519.PrivateKey, ed25519.PublicKey) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	return privateKey, privateKey.Public().(ed25519.PublicKey)
}
