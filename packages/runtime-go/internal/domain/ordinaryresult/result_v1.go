package ordinaryresult

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainordinaryprojection "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	domainrestrictedevidence "analytix.local/runtime-go/internal/domain/restrictedevidence"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ResultSlotSchemaVersionV1              = 1
	ResultSlotPurposeV1                    = "analytix.ordinary-result/v1"
	ResultSlotProjectionV1                 = "analytix.ordinary-output-projection/v1"
	ResultSlotLogicalEffectV1              = "ordinary"
	ResultSlotOriginProviderOrdinaryOnlyV1 = "provider_ordinary_only"
	ResultSlotOriginHostFixedV1            = "host_fixed"
	resultSlotMaxJSONBytesV1               = 4 << 20

	HostFixedProviderResultWithheldTextV1 = "普通任务已执行，但模型结果正文未通过普通输出安全投影，因此未予发布。"
	HostFixedProtectedFactBlockedTextV1   = "本轮模型输出包含必须经过案件证据门核验的事实候选，宿主已阻止该草稿发布。请在已绑定的案件项目中重新发起核验。"
)

var resultSlotDigestDomainV1 = []byte("analytix/ordinary-result/v1\x00")

var resultSlotExactFieldsV1 = map[string]bool{
	"schemaVersion": true, "purpose": true, "projectionVersion": true,
	"logicalEffect": true, "ordinaryWork": true, "candidateOrigin": true, "evidenceAuthority": true,
	"citationAuthority": true, "factAnswerAllowed": true, "text": true,
	"textSha256": true, "resultDigest": true,
}

var (
	ErrResultSlotEmptyV1             = errors.New("ordinary result is empty")
	ErrResultSlotProjectionV1        = errors.New("ordinary result projection is invalid")
	ErrResultSlotProtectedFactV1     = errors.New("ordinary result contains a protected case fact candidate")
	ErrResultSlotInternalReferenceV1 = errors.New("ordinary result contains an internal case entity reference")
)

// ResultSlotV1 is a typed, non-evidentiary ordinary Agent result. It is not an
// authority: a caller must still bind it to the exact current turn through the
// existing general CAS or case accepted-final authority. The closed flags make
// its non-factual role explicit to durable/public validators.
type ResultSlotV1 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	ProjectionVersion string `json:"projectionVersion"`
	LogicalEffect     string `json:"logicalEffect"`
	OrdinaryWork      bool   `json:"ordinaryWork"`
	CandidateOrigin   string `json:"candidateOrigin"`
	EvidenceAuthority bool   `json:"evidenceAuthority"`
	CitationAuthority bool   `json:"citationAuthority"`
	FactAnswerAllowed bool   `json:"factAnswerAllowed"`
	Text              string `json:"text"`
	TextSHA256        string `json:"textSha256"`
	ResultDigest      string `json:"resultDigest"`
}

// NewResultSlotV1 compiles provider-originated ordinary prose through the
// reasoning, credential, and restricted-PII projections before admitting it.
// Fact-shaped case material and internal entity references are rejected in
// full; this constructor never upgrades prose into evidence authority.
func NewResultSlotV1(candidate string) (ResultSlotV1, error) {
	return newResultSlotV1(candidate, ResultSlotOriginProviderOrdinaryOnlyV1)
}

// NewHostFixedResultSlotV1 creates one of the closed host-authored fallback
// results. Callers cannot relabel arbitrary provider prose as host fixed.
func NewHostFixedResultSlotV1(text string) (ResultSlotV1, error) {
	if text != HostFixedProviderResultWithheldTextV1 && text != HostFixedProtectedFactBlockedTextV1 {
		return ResultSlotV1{}, ErrResultSlotProjectionV1
	}
	return newResultSlotV1(text, ResultSlotOriginHostFixedV1)
}

func newResultSlotV1(candidate, origin string) (ResultSlotV1, error) {
	filtered, err := domainreasoningmarkup.Filter(candidate)
	if err != nil {
		return ResultSlotV1{}, errors.Join(ErrResultSlotProjectionV1, err)
	}
	if domaincaseentity.ContainsInternalReferenceV1(filtered.PublicText) {
		return ResultSlotV1{}, ErrResultSlotInternalReferenceV1
	}
	if domainrestrictedevidence.ValidateCanonicalText(filtered.PublicText) != nil {
		return ResultSlotV1{}, ErrResultSlotProjectionV1
	}
	text := strings.TrimSpace(domainordinaryprojection.ProjectTextV1(filtered.PublicText))
	if text == "" {
		return ResultSlotV1{}, ErrResultSlotEmptyV1
	}
	if text == domainsecret.RedactedV1 || domainordinaryprojection.ProjectTextV1(text) != text ||
		domainprivacy.ValidateOrdinaryText(text) != nil || domainsecret.ValidateValueV1(text) != nil ||
		domainrestrictedevidence.ValidateCanonicalText(text) != nil {
		return ResultSlotV1{}, ErrResultSlotProjectionV1
	}
	if domainsecurity.ContainsProtectedCaseFactCandidate(text) {
		return ResultSlotV1{}, ErrResultSlotProtectedFactV1
	}
	if domaincaseentity.ContainsInternalReferenceV1(text) {
		return ResultSlotV1{}, ErrResultSlotInternalReferenceV1
	}
	slot := ResultSlotV1{
		SchemaVersion: ResultSlotSchemaVersionV1, Purpose: ResultSlotPurposeV1,
		ProjectionVersion: ResultSlotProjectionV1, LogicalEffect: ResultSlotLogicalEffectV1,
		OrdinaryWork: true, CandidateOrigin: origin, Text: text, TextSHA256: domainsecurity.SHA256Hex([]byte(text)),
	}
	slot.ResultDigest = resultSlotDigestV1(slot)
	if err := ValidateResultSlotV1(slot); err != nil {
		return ResultSlotV1{}, err
	}
	return slot, nil
}

func ValidateResultSlotV1(slot ResultSlotV1) error {
	if slot.SchemaVersion != ResultSlotSchemaVersionV1 || slot.Purpose != ResultSlotPurposeV1 ||
		slot.ProjectionVersion != ResultSlotProjectionV1 || slot.LogicalEffect != ResultSlotLogicalEffectV1 ||
		!slot.OrdinaryWork || slot.EvidenceAuthority || slot.CitationAuthority || slot.FactAnswerAllowed ||
		strings.TrimSpace(slot.Text) == "" || slot.Text != strings.TrimSpace(slot.Text) ||
		!domainsecurity.IsSHA256Hex(slot.TextSHA256) || slot.TextSHA256 != domainsecurity.SHA256Hex([]byte(slot.Text)) ||
		!domainsecurity.IsSHA256Hex(slot.ResultDigest) || slot.ResultDigest != resultSlotDigestV1(slot) {
		return errors.New("ordinary result slot is invalid")
	}
	if slot.CandidateOrigin != ResultSlotOriginProviderOrdinaryOnlyV1 &&
		slot.CandidateOrigin != ResultSlotOriginHostFixedV1 {
		return errors.New("ordinary result slot candidate origin is invalid")
	}
	if slot.CandidateOrigin == ResultSlotOriginHostFixedV1 &&
		slot.Text != HostFixedProviderResultWithheldTextV1 && slot.Text != HostFixedProtectedFactBlockedTextV1 {
		return errors.New("ordinary result slot host-fixed text is invalid")
	}
	filtered, err := domainreasoningmarkup.Filter(slot.Text)
	if err != nil || filtered.PublicText != slot.Text || domainordinaryprojection.ProjectTextV1(slot.Text) != slot.Text ||
		domainprivacy.ValidateOrdinaryText(slot.Text) != nil || domainsecret.ValidateValueV1(slot.Text) != nil ||
		domainrestrictedevidence.ValidateCanonicalText(slot.Text) != nil ||
		domainsecurity.ContainsProtectedCaseFactCandidate(slot.Text) || domaincaseentity.ContainsInternalReferenceV1(slot.Text) {
		return ErrResultSlotProjectionV1
	}
	return nil
}

// UnmarshalJSON keeps the closed false authority flags canonical even when a
// ResultSlotV1 is nested inside another signed structure. encoding/json would
// otherwise treat a missing false-valued field as equivalent to an explicitly
// present field and would accept duplicate object keys.
func (slot *ResultSlotV1) UnmarshalJSON(body []byte) error {
	if slot == nil {
		return errors.New("ordinary result slot destination is unavailable")
	}
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: resultSlotMaxJSONBytesV1, MaxDepth: 8,
		MaxTokens: 64, MaxStringBytes: resultSlotMaxJSONBytesV1,
	})
	if err != nil || len(object) != len(resultSlotExactFieldsV1) {
		return errors.New("ordinary result slot JSON shape is invalid")
	}
	for field := range resultSlotExactFieldsV1 {
		if _, present := object[field]; !present {
			return errors.New("ordinary result slot JSON shape is invalid")
		}
	}
	type plainResultSlotV1 ResultSlotV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var decoded plainResultSlotV1
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("ordinary result slot contains trailing JSON")
	}
	*slot = ResultSlotV1(decoded)
	return nil
}

func ParseResultSlotV1(value any) (ResultSlotV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ResultSlotV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var slot ResultSlotV1
	if err := decoder.Decode(&slot); err != nil {
		return ResultSlotV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ResultSlotV1{}, errors.New("ordinary result slot contains trailing JSON")
	}
	if err := ValidateResultSlotV1(slot); err != nil {
		return ResultSlotV1{}, err
	}
	return slot, nil
}

func ResultSlotV1Map(slot ResultSlotV1) map[string]any {
	body, _ := json.Marshal(slot)
	value := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&value)
	return value
}

func resultSlotDigestV1(slot ResultSlotV1) string {
	slot.ResultDigest = ""
	body, _ := json.Marshal(slot)
	digest := sha256.New()
	_, _ = digest.Write(resultSlotDigestDomainV1)
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}
