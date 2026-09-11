package security

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGeneralOnlyRiskPolicyIsDeterministicAndNonElevating(t *testing.T) {
	observation := SHA256Hex([]byte("missing-case-binding"))
	first, err := NewGeneralOnlyRiskPolicyV1("thread-general", "/workspace/general", observation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewGeneralOnlyRiskPolicyV1("thread-general", "/workspace/general", observation)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.RiskClass != RiskClassGeneral || first.ToolAuthority != GeneralOnlyRiskToolAuthority {
		t.Fatalf("general-only policy is not deterministic and fixed: first=%#v second=%#v", first, second)
	}
	binding, err := NewHostGeneralOnlyRiskAuthorityBindingV1(first)
	if err != nil {
		t.Fatal(err)
	}
	if binding.State != RiskAuthorityBindingStateHostGeneralOnly || binding.IndexDigest != "" || binding.Generation != 0 ||
		binding.CheckpointDigest != "" || binding.ObservationDigest != "" {
		t.Fatalf("general-only binding simulated witness authority: %#v", binding)
	}
	body, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(body)
	if strings.Contains(encoded, "indexDigest") || strings.Contains(encoded, "checkpointDigest") || strings.Contains(encoded, "authoritySignature") {
		t.Fatalf("general-only binding serialized witness/signing material: %s", encoded)
	}
}

func TestGeneralOnlyRiskPolicyAndBindingRejectForgery(t *testing.T) {
	policy, err := NewGeneralOnlyRiskPolicyV1("thread-general", "/workspace/general", SHA256Hex([]byte("observation")))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewHostGeneralOnlyRiskAuthorityBindingV1(policy)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*GeneralOnlyRiskPolicyV1){
		"thread":      func(value *GeneralOnlyRiskPolicyV1) { value.ThreadID = "thread-other" },
		"workspace":   func(value *GeneralOnlyRiskPolicyV1) { value.WorkspaceRealPath = "/workspace/other" },
		"observation": func(value *GeneralOnlyRiskPolicyV1) { value.BindingObservationDigest = SHA256Hex([]byte("other")) },
		"host policy": func(value *GeneralOnlyRiskPolicyV1) { value.HostPolicyDigest = SHA256Hex([]byte("fake-host-policy")) },
		"risk":        func(value *GeneralOnlyRiskPolicyV1) { value.RiskClass = RiskClassCase },
		"tool ceiling": func(value *GeneralOnlyRiskPolicyV1) {
			value.ToolAuthority = "write"
		},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := policy
			mutate(&tampered)
			if ValidateGeneralOnlyRiskPolicyV1(tampered) == nil {
				t.Fatalf("forged general-only policy was accepted: %#v", tampered)
			}
		})
	}
	for name, mutate := range map[string]func(*RiskAuthorityBindingV1){
		"general policy": func(value *RiskAuthorityBindingV1) { value.GeneralPolicyDigest = SHA256Hex([]byte("fake")) },
		"host policy":    func(value *RiskAuthorityBindingV1) { value.HostPolicyDigest = SHA256Hex([]byte("fake")) },
		"observation":    func(value *RiskAuthorityBindingV1) { value.BindingObservationDigest = SHA256Hex([]byte("fake")) },
		"witness field":  func(value *RiskAuthorityBindingV1) { value.Generation = 1 },
	} {
		t.Run("binding "+name, func(t *testing.T) {
			tampered := binding
			mutate(&tampered)
			if ValidateHostGeneralOnlyRiskAuthorityBindingV1(tampered, policy) == nil {
				t.Fatalf("forged general-only binding was accepted: %#v", tampered)
			}
		})
	}
}

func TestHostGeneralOnlyTurnCannotCarryCaseOrSnapshotAuthority(t *testing.T) {
	workspace := "/workspace/general"
	observation := SHA256Hex([]byte("missing-case-binding"))
	policy, err := NewGeneralOnlyRiskPolicyV1("thread-general", workspace, observation)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewHostGeneralOnlyRiskAuthorityBindingV1(policy)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := NewTurnPublicationPolicyV1(TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policy.PolicyDigest, RiskClass: RiskClassGeneral,
		Disposition: PublicationDispositionGeneralOutput, CaseBindingState: CaseBindingStateMissing,
		BindingObservationDigest: observation, BlockerCode: PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := TurnSecurityContextInput{
		ThreadID: "thread-general", TurnID: "turn-general", WorkspaceRealPath: workspace,
		TenantID: LocalTenantID, UserID: LocalUserID, CaseID: UnboundCaseID,
		CaseBindingHash: UnboundCaseBindingHash(workspace), DatasetSnapshotID: NoDatasetSnapshotID,
		SourceManifestHash: EmptySourceManifestHash, ContextEpoch: 1, IssuedAt: time.Unix(1_700_000_000, 0).UTC(),
		PublicationPolicy: publication, RiskAuthorityBinding: binding,
	}
	securityContext, err := NewTurnSecurityContextV2(base)
	if err != nil || ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!TurnSecurityContextUsesHostGeneralOnlyRisk(securityContext) || TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("valid host general-only context failed closed incorrectly: context=%#v err=%v", securityContext, err)
	}
	for name, mutate := range map[string]func(*TurnSecurityContextInput){
		"case id": func(value *TurnSecurityContextInput) { value.CaseID = "case-forged" },
		"dsv1":    func(value *TurnSecurityContextInput) { value.DatasetSnapshotID = "dsv1_" + SHA256Hex([]byte("forged")) },
		"dsv2": func(value *TurnSecurityContextInput) {
			value.DatasetSnapshotID = DatasetSnapshotIDPrefixV2 + SHA256Hex([]byte("forged"))
		},
		"manifest": func(value *TurnSecurityContextInput) {
			value.SourceManifestHash = SHA256Hex([]byte("forged-manifest"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			forged := base
			mutate(&forged)
			if _, err := NewTurnSecurityContextV2(forged); err == nil {
				t.Fatalf("general-only context accepted case/snapshot authority: %#v", forged)
			}
		})
	}
}
