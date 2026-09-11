package turn

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

const CaseCompactionOperationBindingPurposeV1 = "analytix.case-compaction-operation-binding/v1"

// CaseCompactionOperationBindingV1 is the non-secret manifest needed to
// verify a compacted task continuation against the existing installation-
// signed ContextEpoch recovery digest. It is integrity metadata only: the
// continuation remains unverified for case facts.
type CaseCompactionOperationBindingV1 struct {
	SchemaVersion                  int      `json:"schemaVersion"`
	Purpose                        string   `json:"purpose"`
	ThreadIDHash                   string   `json:"threadIdHash"`
	SourceContextDigest            string   `json:"sourceContextDigest"`
	CompactedTurnsDigest           string   `json:"compactedTurnsDigest"`
	ContinuationDigest             string   `json:"continuationDigest"`
	AuthorityTurnIDs               []string `json:"authorityTurnIds"`
	OperationStamp                 string   `json:"operationStamp"`
	PreviousCompactionSourceDigest string   `json:"previousCompactionSourceDigest,omitempty"`
}

func NewCaseCompactionOperationBindingV1(
	threadID string,
	compactedTurns []any,
	historyTurns []any,
	securityContext any,
	stamp int64,
	continuation threaddomain.TaskContinuationSnapshotV1,
	authorityTurnIDs []string,
) (CaseCompactionOperationBindingV1, error) {
	threadID = strings.TrimSpace(threadID)
	current, err := domainsecurity.ParseTurnSecurityContext(securityContext)
	if err != nil || threadID == "" || current.ThreadID != threadID || stamp <= 0 {
		return CaseCompactionOperationBindingV1{}, errors.New("case compaction operation binding input is invalid")
	}
	parsedContinuation, err := threaddomain.ParseTaskContinuationSnapshotV1(
		threaddomain.TaskContinuationSnapshotMapV1(continuation),
	)
	if err != nil {
		return CaseCompactionOperationBindingV1{}, errors.New("case compaction continuation binding is invalid")
	}
	turnBody, err := json.Marshal(compactedTurns)
	if err != nil {
		return CaseCompactionOperationBindingV1{}, err
	}
	binding := CaseCompactionOperationBindingV1{
		SchemaVersion:                  1,
		Purpose:                        CaseCompactionOperationBindingPurposeV1,
		ThreadIDHash:                   domainsecurity.SHA256Hex([]byte(threadID)),
		SourceContextDigest:            current.ContextDigest,
		CompactedTurnsDigest:           domainsecurity.CanonicalJSONHash(turnBody),
		ContinuationDigest:             parsedContinuation.StateDigest,
		AuthorityTurnIDs:               canonicalCaseCompactionTurnIDs(authorityTurnIDs),
		OperationStamp:                 strconv.FormatInt(stamp, 10),
		PreviousCompactionSourceDigest: latestCaseCompactionSourceDigestV1(historyTurns),
	}
	if err := ValidateCaseCompactionOperationBindingV1(binding); err != nil {
		return CaseCompactionOperationBindingV1{}, err
	}
	return binding, nil
}

func ParseCaseCompactionOperationBindingV1(value any) (CaseCompactionOperationBindingV1, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) == 0 {
		return CaseCompactionOperationBindingV1{}, errors.New("case compaction operation binding is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var binding CaseCompactionOperationBindingV1
	if err := decoder.Decode(&binding); err != nil {
		return CaseCompactionOperationBindingV1{}, errors.New("case compaction operation binding is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return CaseCompactionOperationBindingV1{}, errors.New("case compaction operation binding has trailing content")
	}
	if err := ValidateCaseCompactionOperationBindingV1(binding); err != nil {
		return CaseCompactionOperationBindingV1{}, err
	}
	return binding, nil
}

func ValidateCaseCompactionOperationBindingV1(binding CaseCompactionOperationBindingV1) error {
	stamp, stampErr := strconv.ParseInt(strings.TrimSpace(binding.OperationStamp), 10, 64)
	canonicalTurnIDs := canonicalCaseCompactionTurnIDs(binding.AuthorityTurnIDs)
	if binding.SchemaVersion != 1 || binding.Purpose != CaseCompactionOperationBindingPurposeV1 ||
		!domainsecurity.IsSHA256Hex(binding.ThreadIDHash) ||
		!domainsecurity.IsSHA256Hex(binding.SourceContextDigest) ||
		!domainsecurity.IsSHA256Hex(binding.CompactedTurnsDigest) ||
		!domainsecurity.IsSHA256Hex(binding.ContinuationDigest) ||
		len(canonicalTurnIDs) == 0 || len(canonicalTurnIDs) != len(binding.AuthorityTurnIDs) ||
		stampErr != nil || stamp <= 0 || strconv.FormatInt(stamp, 10) != binding.OperationStamp ||
		(binding.PreviousCompactionSourceDigest != "" &&
			!domainsecurity.IsSHA256Hex(binding.PreviousCompactionSourceDigest)) {
		return errors.New("case compaction operation binding is invalid")
	}
	for index, turnID := range binding.AuthorityTurnIDs {
		if strings.TrimSpace(turnID) != turnID || turnID != canonicalTurnIDs[index] {
			return errors.New("case compaction operation authority inventory is not canonical")
		}
	}
	return nil
}

func CaseCompactionOperationBindingMapV1(binding CaseCompactionOperationBindingV1) map[string]any {
	if ValidateCaseCompactionOperationBindingV1(binding) != nil {
		return nil
	}
	body, err := json.Marshal(binding)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

func CaseCompactionOperationDigestV1(binding CaseCompactionOperationBindingV1) (string, error) {
	if err := ValidateCaseCompactionOperationBindingV1(binding); err != nil {
		return "", err
	}
	body, err := json.Marshal(binding)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(body), nil
}

func latestCaseCompactionSourceDigestV1(turns []any) string {
	for index := len(turns) - 1; index >= 0; index-- {
		turn, _ := turns[index].(map[string]any)
		if strings.TrimSpace(stringField(turn, "caseHistoryProjection")) != "compaction_authority_v1" {
			continue
		}
		items := listAny(turn["items"])
		if len(items) != 1 {
			continue
		}
		item, _ := items[0].(map[string]any)
		digest := strings.TrimSpace(stringField(item, "sourceDigest"))
		if domainsecurity.IsSHA256Hex(digest) {
			return digest
		}
	}
	return ""
}
