package toolcatalog

import (
	"errors"
	"strings"
)

// HostSideEffectDuplicateOutputV1 is an in-process authority type. Untrusted
// MCP/JSON maps cannot become this value merely by copying its fields.
type HostSideEffectDuplicateOutputV1 struct {
	Code              string `json:"code"`
	Executed          bool   `json:"executed"`
	IntentStatus      string `json:"intentStatus"`
	FactAnswerAllowed bool   `json:"factAnswerAllowed"`
	EvidenceAuthority bool   `json:"evidenceAuthority"`
}

func NewHostSideEffectDuplicateOutputV1(intentStatus string) (HostSideEffectDuplicateOutputV1, error) {
	intentStatus = strings.TrimSpace(intentStatus)
	if intentStatus != "open" && intentStatus != "closed" {
		return HostSideEffectDuplicateOutputV1{}, errors.New("side-effect duplicate status is invalid")
	}
	return HostSideEffectDuplicateOutputV1{
		Code: "side_effect_duplicate", IntentStatus: intentStatus,
		Executed: false, FactAnswerAllowed: false, EvidenceAuthority: false,
	}, nil
}

func validHostSideEffectDuplicateOutputV1(output HostSideEffectDuplicateOutputV1) bool {
	return output.Code == "side_effect_duplicate" && !output.Executed &&
		(output.IntentStatus == "open" || output.IntentStatus == "closed") &&
		!output.FactAnswerAllowed && !output.EvidenceAuthority
}
