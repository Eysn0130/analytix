package turnterminal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

const (
	GeneralTerminalCASBindingV1SchemaVersion = "general-terminal-cas-binding.v1"
	GeneralTerminalCASBindingV1Purpose       = "analytix.general-terminal-cas-binding/v1"
)

var generalTerminalCASBindingDigestDomain = []byte("analytix/general-terminal-cas-binding/v1\x00")

// GeneralTerminalCASBindingV1 is an internal compare-and-swap guard. It is
// deliberately not an AcceptedFinal, evidence receipt, signature, citation,
// or publication authority. Its only purpose is to make the durable terminal
// write reject a provider result produced for a stale general-turn context.
type GeneralTerminalCASBindingV1 struct {
	SchemaVersion      string `json:"schemaVersion"`
	Purpose            string `json:"purpose"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	ContextDigest      string `json:"contextDigest"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	TerminalReason     string `json:"terminalReason"`
	TerminalStatus     string `json:"terminalStatus"`
	RenderedTextSHA256 string `json:"renderedTextSha256"`
	BindingDigest      string `json:"bindingDigest"`
}

func NewGeneralTerminalCASBindingV1(context domainsecurity.TurnSecurityContext, renderedText string) (GeneralTerminalCASBindingV1, error) {
	return NewGeneralTerminalCASBindingForOutcomeV1(context, "success", "completed", renderedText)
}

func NewGeneralTerminalCASBindingForOutcomeV1(
	context domainsecurity.TurnSecurityContext,
	terminalReason string,
	terminalStatus string,
	renderedText string,
) (GeneralTerminalCASBindingV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil || !domainsecurity.TurnSecurityContextIsGeneral(context) {
		return GeneralTerminalCASBindingV1{}, errors.New("general terminal CAS context is invalid")
	}
	terminalReason = strings.TrimSpace(terminalReason)
	terminalStatus = strings.TrimSpace(terminalStatus)
	if !validGeneralTerminalOutcomeV1(terminalReason, terminalStatus) {
		return GeneralTerminalCASBindingV1{}, errors.New("general terminal CAS outcome is invalid")
	}
	binding := GeneralTerminalCASBindingV1{
		SchemaVersion: GeneralTerminalCASBindingV1SchemaVersion,
		Purpose:       GeneralTerminalCASBindingV1Purpose,
		ThreadID:      context.ThreadID, TurnID: context.TurnID, ContextDigest: context.ContextDigest,
		ContextEpoch: context.ContextEpoch, DatasetSnapshotID: context.DatasetSnapshotID,
		TerminalReason: terminalReason, TerminalStatus: terminalStatus,
		RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(renderedText)),
	}
	binding.BindingDigest = generalTerminalCASBindingDigestV1(binding)
	if err := ValidateGeneralTerminalCASBindingV1(binding); err != nil {
		return GeneralTerminalCASBindingV1{}, err
	}
	return binding, nil
}

func ValidateGeneralTerminalCASBindingV1(binding GeneralTerminalCASBindingV1) error {
	if binding.SchemaVersion != GeneralTerminalCASBindingV1SchemaVersion || binding.Purpose != GeneralTerminalCASBindingV1Purpose ||
		strings.TrimSpace(binding.ThreadID) == "" || strings.TrimSpace(binding.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(binding.ContextDigest) || binding.ContextEpoch == 0 ||
		strings.TrimSpace(binding.DatasetSnapshotID) == "" || !validGeneralTerminalOutcomeV1(binding.TerminalReason, binding.TerminalStatus) ||
		!domainsecurity.IsSHA256Hex(binding.RenderedTextSHA256) ||
		!domainsecurity.IsSHA256Hex(binding.BindingDigest) || binding.BindingDigest != generalTerminalCASBindingDigestV1(binding) {
		return errors.New("general terminal CAS binding is invalid")
	}
	return nil
}

func ValidateGeneralTerminalCASBindingForContextV1(binding GeneralTerminalCASBindingV1, context domainsecurity.TurnSecurityContext, renderedText, status string) error {
	return ValidateGeneralTerminalCASBindingForOutcomeV1(binding, context, renderedText, binding.TerminalReason, status)
}

func ValidateGeneralTerminalCASBindingForOutcomeV1(
	binding GeneralTerminalCASBindingV1,
	context domainsecurity.TurnSecurityContext,
	renderedText string,
	terminalReason string,
	terminalStatus string,
) error {
	if ValidateGeneralTerminalCASBindingV1(binding) != nil || domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(context) || binding.ThreadID != context.ThreadID || binding.TurnID != context.TurnID ||
		binding.ContextDigest != context.ContextDigest || binding.ContextEpoch != context.ContextEpoch ||
		binding.DatasetSnapshotID != context.DatasetSnapshotID || binding.TerminalReason != strings.TrimSpace(terminalReason) ||
		binding.TerminalStatus != strings.TrimSpace(terminalStatus) ||
		binding.RenderedTextSHA256 != domainsecurity.SHA256Hex([]byte(renderedText)) {
		return errors.New("general terminal CAS binding does not match the frozen context")
	}
	return nil
}

func validGeneralTerminalOutcomeV1(reason, status string) bool {
	reason = strings.TrimSpace(reason)
	status = strings.TrimSpace(status)
	expected, ok := GeneralTerminalStatusForReasonV1(reason)
	return ok && status == expected
}

func GeneralTerminalStatusForReasonV1(reason string) (string, bool) {
	return domainterminal.StatusForReasonV1(reason)
}

func ParseGeneralTerminalCASBindingV1(value any) (GeneralTerminalCASBindingV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return GeneralTerminalCASBindingV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var binding GeneralTerminalCASBindingV1
	if err := decoder.Decode(&binding); err != nil {
		return GeneralTerminalCASBindingV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return GeneralTerminalCASBindingV1{}, errors.New("general terminal CAS binding contains trailing JSON")
	}
	if err := ValidateGeneralTerminalCASBindingV1(binding); err != nil {
		return GeneralTerminalCASBindingV1{}, err
	}
	return binding, nil
}

func GeneralTerminalCASBindingV1Map(binding GeneralTerminalCASBindingV1) map[string]any {
	body, _ := json.Marshal(binding)
	value := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&value)
	return value
}

func generalTerminalCASBindingDigestV1(binding GeneralTerminalCASBindingV1) string {
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	digest := sha256.New()
	_, _ = digest.Write(generalTerminalCASBindingDigestDomain)
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}
