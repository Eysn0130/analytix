package security

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	GeneralOnlyRiskPolicySchemaVersion = 1
	GeneralOnlyRiskPolicyPurpose       = "analytix.general-only-risk-policy/v1"
	GeneralOnlyRiskHostPolicyPurpose   = "analytix.host-general-only-authority/v1"
	GeneralOnlyRiskToolAuthority       = "read_only"
)

// GeneralOnlyRiskPolicyV1 is a deterministic, non-elevating host policy. It
// is deliberately not a monotonic case witness and carries no signing key or
// local enrollment marker. Its authority is limited to a currently observed
// workspace with an exactly missing case binding and is re-derived at every
// provider or tool execution boundary.
type GeneralOnlyRiskPolicyV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	ThreadID                 string `json:"threadId"`
	WorkspaceRealPath        string `json:"workspaceRealPath"`
	BindingObservationDigest string `json:"bindingObservationDigest"`
	RiskClass                string `json:"riskClass"`
	Origin                   string `json:"origin"`
	Disposition              string `json:"disposition"`
	ToolAuthority            string `json:"toolAuthority"`
	HostPolicyDigest         string `json:"hostPolicyDigest"`
	PolicyDigest             string `json:"policyDigest"`
}

type generalOnlyRiskHostPolicyV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	RiskClass                string `json:"riskClass"`
	Origin                   string `json:"origin"`
	Disposition              string `json:"disposition"`
	RequiredCaseBindingState string `json:"requiredCaseBindingState"`
	ToolAuthority            string `json:"toolAuthority"`
	CaseUpgradeAllowed       bool   `json:"caseUpgradeAllowed"`
	CaseEvidenceAllowed      bool   `json:"caseEvidenceAllowed"`
	ReportPublicationAllowed bool   `json:"reportPublicationAllowed"`
}

func GeneralOnlyRiskHostPolicyDigestV1() string {
	body, _ := json.Marshal(generalOnlyRiskHostPolicyV1{
		SchemaVersion: GeneralOnlyRiskPolicySchemaVersion, Purpose: GeneralOnlyRiskHostPolicyPurpose,
		RiskClass: RiskClassGeneral, Origin: RiskPolicyOriginGeneralWorkspace,
		Disposition: PublicationDispositionGeneralOutput, RequiredCaseBindingState: CaseBindingStateMissing,
		ToolAuthority: GeneralOnlyRiskToolAuthority,
	})
	return SHA256Hex(body)
}

func NewGeneralOnlyRiskPolicyV1(threadID, workspaceRealPath, bindingObservationDigest string) (GeneralOnlyRiskPolicyV1, error) {
	policy := GeneralOnlyRiskPolicyV1{
		SchemaVersion: GeneralOnlyRiskPolicySchemaVersion, Purpose: GeneralOnlyRiskPolicyPurpose,
		ThreadID: strings.TrimSpace(threadID), WorkspaceRealPath: strings.TrimSpace(workspaceRealPath),
		BindingObservationDigest: strings.TrimSpace(bindingObservationDigest),
		RiskClass:                RiskClassGeneral, Origin: RiskPolicyOriginGeneralWorkspace,
		Disposition: PublicationDispositionGeneralOutput, ToolAuthority: GeneralOnlyRiskToolAuthority,
		HostPolicyDigest: GeneralOnlyRiskHostPolicyDigestV1(),
	}
	policy.PolicyDigest = generalOnlyRiskPolicyDigestV1(policy)
	if err := ValidateGeneralOnlyRiskPolicyV1(policy); err != nil {
		return GeneralOnlyRiskPolicyV1{}, err
	}
	return policy, nil
}

func ValidateGeneralOnlyRiskPolicyV1(policy GeneralOnlyRiskPolicyV1) error {
	if policy.SchemaVersion != GeneralOnlyRiskPolicySchemaVersion || policy.Purpose != GeneralOnlyRiskPolicyPurpose ||
		policy.ThreadID == "" || policy.ThreadID != strings.TrimSpace(policy.ThreadID) ||
		policy.WorkspaceRealPath == "" || policy.WorkspaceRealPath != strings.TrimSpace(policy.WorkspaceRealPath) ||
		!isCanonicalSHA256Hex(policy.BindingObservationDigest) || policy.RiskClass != RiskClassGeneral ||
		policy.Origin != RiskPolicyOriginGeneralWorkspace || policy.Disposition != PublicationDispositionGeneralOutput ||
		policy.ToolAuthority != GeneralOnlyRiskToolAuthority || policy.HostPolicyDigest != GeneralOnlyRiskHostPolicyDigestV1() ||
		!isCanonicalSHA256Hex(policy.PolicyDigest) || policy.PolicyDigest != generalOnlyRiskPolicyDigestV1(policy) {
		return errors.New("general-only risk policy integrity is invalid")
	}
	return nil
}

func ValidateTurnPublicationPolicyForGeneralOnlyRiskPolicyV1(publication TurnPublicationPolicyV1, policy GeneralOnlyRiskPolicyV1) error {
	if err := ValidateTurnPublicationPolicyV1(publication); err != nil {
		return err
	}
	if err := ValidateGeneralOnlyRiskPolicyV1(policy); err != nil {
		return err
	}
	if publication.ThreadRiskPolicyDigest != policy.PolicyDigest || publication.RiskClass != policy.RiskClass ||
		publication.Disposition != policy.Disposition || publication.CaseBindingState != CaseBindingStateMissing ||
		publication.BindingObservationDigest != policy.BindingObservationDigest || publication.BlockerCode != PublicationBlockerNone {
		return errors.New("turn publication policy general-only binding is invalid")
	}
	return nil
}

func generalOnlyRiskPolicyDigestV1(policy GeneralOnlyRiskPolicyV1) string {
	policy.PolicyDigest = ""
	body, _ := json.Marshal(policy)
	return SHA256Hex(body)
}
