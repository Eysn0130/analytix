package reportpublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ClaimLedgerSchemaVersion = 1
	ClaimLedgerPurpose       = "analytix.claim-ledger/v1"
)

type ClaimLedgerV1 struct {
	SchemaVersion        int                          `json:"schemaVersion"`
	Purpose              string                       `json:"purpose"`
	ThreadID             string                       `json:"threadId"`
	TurnID               string                       `json:"turnId"`
	ContextDigest        string                       `json:"contextDigest"`
	CaseID               string                       `json:"caseId"`
	CaseBindingHash      string                       `json:"caseBindingHash"`
	ContextEpoch         uint64                       `json:"contextEpoch"`
	DatasetSnapshotID    string                       `json:"datasetSnapshotId"`
	SourceManifestHash   string                       `json:"sourceManifestHash"`
	EvidenceReceiptIDs   []string                     `json:"evidenceReceiptIds"`
	Claims               []domainevidence.ClaimRecord `json:"claims"`
	VerifiedClaimCount   uint64                       `json:"verifiedClaimCount"`
	PartialClaimCount    uint64                       `json:"partialClaimCount"`
	UnresolvedClaimCount uint64                       `json:"unresolvedClaimCount"`
	RefutedClaimCount    uint64                       `json:"refutedClaimCount"`
	CreatedAt            string                       `json:"createdAt"`
	LedgerDigest         string                       `json:"ledgerDigest"`
}

type ClaimLedgerInputV1 struct {
	Context            domainsecurity.TurnSecurityContext
	EvidenceReceiptIDs []string
	Claims             []domainevidence.ClaimRecord
	CreatedAt          time.Time
}

func NewClaimLedgerV1(input ClaimLedgerInputV1) (ClaimLedgerV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil {
		return ClaimLedgerV1{}, errors.New("claim ledger requires current V2 case fact authority")
	}
	createdAt := input.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	claims := append([]domainevidence.ClaimRecord(nil), input.Claims...)
	sort.Slice(claims, func(i, j int) bool { return claims[i].ClaimID < claims[j].ClaimID })
	ledger := ClaimLedgerV1{
		SchemaVersion: ClaimLedgerSchemaVersion, Purpose: ClaimLedgerPurpose,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, ContextDigest: input.Context.ContextDigest,
		CaseID: input.Context.CaseID, CaseBindingHash: input.Context.CaseBindingHash, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, SourceManifestHash: input.Context.SourceManifestHash,
		EvidenceReceiptIDs: canonicalEvidenceReceiptIDs(input.EvidenceReceiptIDs), Claims: claims,
		CreatedAt: createdAt.Format(time.RFC3339Nano),
	}
	if ledger.EvidenceReceiptIDs == nil {
		ledger.EvidenceReceiptIDs = []string{}
	}
	if ledger.Claims == nil {
		ledger.Claims = []domainevidence.ClaimRecord{}
	}
	for _, claim := range ledger.Claims {
		switch claim.SupportState {
		case domainevidence.ClaimVerified:
			ledger.VerifiedClaimCount++
		case domainevidence.ClaimPartial:
			ledger.PartialClaimCount++
		case domainevidence.ClaimUnresolved:
			ledger.UnresolvedClaimCount++
		case domainevidence.ClaimRefuted:
			ledger.RefutedClaimCount++
		}
	}
	ledger.LedgerDigest = claimLedgerDigestV1(ledger)
	if err := ValidateClaimLedgerV1(ledger); err != nil {
		return ClaimLedgerV1{}, err
	}
	return ledger, nil
}

func ValidateClaimLedgerV1(ledger ClaimLedgerV1) error {
	createdAt, timeErr := time.Parse(time.RFC3339Nano, ledger.CreatedAt)
	if ledger.SchemaVersion != ClaimLedgerSchemaVersion || ledger.Purpose != ClaimLedgerPurpose ||
		strings.TrimSpace(ledger.ThreadID) == "" || strings.TrimSpace(ledger.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(ledger.ContextDigest) || strings.TrimSpace(ledger.CaseID) == "" || ledger.CaseID == domainsecurity.UnboundCaseID ||
		!domainsecurity.IsSHA256Hex(ledger.CaseBindingHash) || ledger.ContextEpoch == 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(ledger.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(ledger.SourceManifestHash) || ledger.EvidenceReceiptIDs == nil || ledger.Claims == nil ||
		timeErr != nil || createdAt.IsZero() || createdAt.UTC().Format(time.RFC3339Nano) != ledger.CreatedAt ||
		!domainsecurity.IsSHA256Hex(ledger.LedgerDigest) {
		return errors.New("claim ledger is incomplete")
	}
	if len(ledger.EvidenceReceiptIDs) == 0 {
		return errors.New("formal claim ledger requires at least one witnessed evidence receipt")
	}
	if !canonicalEvidenceReceiptIDSlice(ledger.EvidenceReceiptIDs) {
		return errors.New("claim ledger evidence receipt set is not canonical")
	}
	evidenceSet := make(map[string]bool, len(ledger.EvidenceReceiptIDs))
	for _, receiptID := range ledger.EvidenceReceiptIDs {
		evidenceSet[receiptID] = true
	}
	previousClaimID := ""
	verified, partial, unresolved, refuted := uint64(0), uint64(0), uint64(0), uint64(0)
	for _, claim := range ledger.Claims {
		if domainevidence.ValidateClaimRecord(claim) != nil || previousClaimID != "" && claim.ClaimID <= previousClaimID {
			return errors.New("claim ledger claim order or authority is invalid")
		}
		previousClaimID = claim.ClaimID
		for _, receiptID := range append(append([]string(nil), claim.EvidenceIDs...), claim.CounterEvidenceIDs...) {
			if !evidenceSet[receiptID] {
				return errors.New("claim ledger references evidence outside its witnessed receipt set")
			}
		}
		switch claim.SupportState {
		case domainevidence.ClaimVerified:
			verified++
		case domainevidence.ClaimPartial:
			partial++
		case domainevidence.ClaimUnresolved:
			unresolved++
		case domainevidence.ClaimRefuted:
			refuted++
		default:
			return errors.New("claim ledger support state is invalid")
		}
	}
	if verified != ledger.VerifiedClaimCount || partial != ledger.PartialClaimCount || unresolved != ledger.UnresolvedClaimCount || refuted != ledger.RefutedClaimCount ||
		ledger.LedgerDigest != claimLedgerDigestV1(ledger) {
		return errors.New("claim ledger counts or integrity are invalid")
	}
	return nil
}

func ParseClaimLedgerV1(body []byte) (ClaimLedgerV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 8 << 20, MaxDepth: 16, MaxTokens: 400_000, MaxStringBytes: 1 << 20,
	}); err != nil {
		return ClaimLedgerV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var ledger ClaimLedgerV1
	if err := decoder.Decode(&ledger); err != nil {
		return ClaimLedgerV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ClaimLedgerV1{}, errors.New("claim ledger contains trailing JSON")
	}
	canonical, err := json.Marshal(ledger)
	if err != nil || !bytes.Equal(body, canonical) {
		return ClaimLedgerV1{}, errors.New("claim ledger is not canonically encoded")
	}
	return ledger, ValidateClaimLedgerV1(ledger)
}

func ClaimLedgerV1Bytes(ledger ClaimLedgerV1) ([]byte, error) {
	if err := ValidateClaimLedgerV1(ledger); err != nil {
		return nil, err
	}
	return json.Marshal(ledger)
}

func claimLedgerDigestV1(ledger ClaimLedgerV1) string {
	ledger.LedgerDigest = ""
	body, _ := json.Marshal(ledger)
	return domainsecurity.SHA256Hex(append([]byte("analytix.claim-ledger/digest/v1\x00"), body...))
}

func canonicalEvidenceReceiptIDs(values []string) []string {
	if values == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validEvidenceReceiptID(value) || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func canonicalEvidenceReceiptIDSlice(values []string) bool {
	if values == nil {
		return false
	}
	for index, value := range values {
		if !validEvidenceReceiptID(value) || index > 0 && value <= values[index-1] {
			return false
		}
	}
	return true
}

func validEvidenceReceiptID(value string) bool {
	return strings.HasPrefix(value, "evr_") && domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, "evr_"))
}
