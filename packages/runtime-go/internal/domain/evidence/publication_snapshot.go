package evidence

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const PublicationSnapshotProofVersion = 1

type PublicationSourceSnapshot struct {
	ReceiptID          string `json:"receiptId"`
	ServerID           string `json:"serverId"`
	ServerIdentity     string `json:"serverIdentity"`
	ServerVersion      string `json:"serverVersion"`
	ConnectionEpoch    uint64 `json:"connectionEpoch"`
	ToolName           string `json:"toolName"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	CatalogFingerprint string `json:"catalogFingerprint"`
	SpecFingerprint    string `json:"specFingerprint"`
	ProbeDigest        string `json:"probeDigest"`
	CheckedAt          string `json:"checkedAt"`
}

type PublicationSnapshotProof struct {
	SchemaVersion       int                         `json:"schemaVersion"`
	ContextDigest       string                      `json:"contextDigest"`
	DatasetSnapshotID   string                      `json:"datasetSnapshotId"`
	SourceManifestHash  string                      `json:"sourceManifestHash"`
	RegistrySequence    uint64                      `json:"registrySequence"`
	RegistryStateDigest string                      `json:"registryStateDigest"`
	EvidenceReceiptIDs  []string                    `json:"evidenceReceiptIds"`
	Sources             []PublicationSourceSnapshot `json:"sources"`
	CheckedAt           string                      `json:"checkedAt"`
	ProofDigest         string                      `json:"proofDigest"`
}

type PublicationSnapshotProofInput struct {
	Context            domainsecurity.TurnSecurityContext
	RegistryHead       EvidenceRegistryHead
	EvidenceReceiptIDs []string
	Sources            []PublicationSourceSnapshot
	CheckedAt          time.Time
}

func NewPublicationSnapshotProof(input PublicationSnapshotProofInput) (PublicationSnapshotProof, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil {
		return PublicationSnapshotProof{}, errors.New("publication snapshot proof requires current V2 case fact authority")
	}
	checkedAt := input.CheckedAt.UTC()
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	proof := PublicationSnapshotProof{
		SchemaVersion: PublicationSnapshotProofVersion, ContextDigest: input.Context.ContextDigest,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, SourceManifestHash: input.Context.SourceManifestHash,
		RegistrySequence: input.RegistryHead.Sequence, RegistryStateDigest: input.RegistryHead.StateDigest,
		EvidenceReceiptIDs: canonicalEvidenceStrings(input.EvidenceReceiptIDs), Sources: clonePublicationSources(input.Sources),
		CheckedAt: checkedAt.Format(time.RFC3339Nano),
	}
	if proof.EvidenceReceiptIDs == nil {
		proof.EvidenceReceiptIDs = []string{}
	}
	if proof.Sources == nil {
		proof.Sources = []PublicationSourceSnapshot{}
	}
	sort.Slice(proof.Sources, func(i, j int) bool { return proof.Sources[i].ReceiptID < proof.Sources[j].ReceiptID })
	proof.ProofDigest = publicationSnapshotProofDigest(proof)
	if domainsecurity.ValidateTurnSecurityContext(input.Context) != nil || ValidateEvidenceRegistryHead(input.RegistryHead) != nil ||
		input.RegistryHead.ContextDigest != input.Context.ContextDigest || input.RegistryHead.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
		ValidatePublicationSnapshotProof(proof) != nil {
		return PublicationSnapshotProof{}, errors.New("publication snapshot proof input is invalid")
	}
	return proof, nil
}

func ValidatePublicationSnapshotProof(proof PublicationSnapshotProof) error {
	if proof.SchemaVersion != PublicationSnapshotProofVersion || !validSHA256(proof.ContextDigest) ||
		strings.TrimSpace(proof.DatasetSnapshotID) == "" || !validSHA256(proof.SourceManifestHash) || proof.RegistrySequence == 0 ||
		!validSHA256(proof.RegistryStateDigest) || proof.EvidenceReceiptIDs == nil || len(proof.EvidenceReceiptIDs) == 0 ||
		proof.Sources == nil || len(proof.Sources) != len(proof.EvidenceReceiptIDs) || strings.TrimSpace(proof.CheckedAt) == "" ||
		!validSHA256(proof.ProofDigest) {
		return errors.New("publication snapshot proof is incomplete")
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, proof.CheckedAt)
	if err != nil {
		return errors.New("publication snapshot proof checkedAt is invalid")
	}
	if canonical := canonicalEvidenceStrings(proof.EvidenceReceiptIDs); len(canonical) != len(proof.EvidenceReceiptIDs) {
		return errors.New("publication snapshot proof receipt set is invalid")
	} else {
		for index := range canonical {
			if canonical[index] != proof.EvidenceReceiptIDs[index] {
				return errors.New("publication snapshot proof receipt order is invalid")
			}
		}
	}
	for index, source := range proof.Sources {
		if strings.TrimSpace(source.ReceiptID) == "" || source.ReceiptID != proof.EvidenceReceiptIDs[index] ||
			strings.TrimSpace(source.ServerID) == "" || strings.TrimSpace(source.ServerIdentity) == "" || strings.TrimSpace(source.ServerVersion) == "" ||
			source.ConnectionEpoch == 0 || strings.TrimSpace(source.ToolName) == "" || source.DatasetSnapshotID != proof.DatasetSnapshotID || !validSHA256(source.CatalogFingerprint) ||
			!validSHA256(source.SpecFingerprint) || !validSHA256(source.ProbeDigest) || strings.TrimSpace(source.CheckedAt) == "" {
			return errors.New("publication source snapshot is invalid")
		}
		sourceCheckedAt, err := time.Parse(time.RFC3339Nano, source.CheckedAt)
		if err != nil || sourceCheckedAt.After(checkedAt) {
			return errors.New("publication source snapshot checkedAt is invalid")
		}
	}
	if publicationSnapshotProofDigest(proof) != proof.ProofDigest {
		return errors.New("publication snapshot proof integrity is invalid")
	}
	return nil
}

// ValidatePublicationSnapshotProofAgainstRegistry replays the signed proof
// against the sealed registry head so source identity cannot be accepted from
// proof shape or a non-empty receipt ID alone during restart.
func ValidatePublicationSnapshotProofAgainstRegistry(proof *PublicationSnapshotProof, context domainsecurity.TurnSecurityContext, envelope FinalAnswerEnvelope, registry EvidenceReceiptRegistry) error {
	head, err := NewEvidenceRegistryHead(registry)
	if err != nil || !EvidenceReceiptRegistryMatchesContext(registry, context) || ValidatePublicationSnapshotProofValue(proof, context, envelope, &head) != nil {
		return errors.New("publication snapshot proof registry authority is invalid")
	}
	if !FinalAnswerRequiresPublicationSnapshotProof(envelope) {
		return nil
	}
	for _, source := range proof.Sources {
		registered, resolveErr := VerifyEvidenceReceiptMembership(registry, context, source.ReceiptID)
		if resolveErr != nil || registered.Revoked {
			return errors.New("publication snapshot proof receipt membership is invalid")
		}
		receipt := registered.Receipt
		if source.ServerID != publicationReceiptServerID(receipt.ToolName) || source.ServerIdentity != receipt.ServerIdentity ||
			source.ServerVersion != receipt.ServerVersion || source.ConnectionEpoch != receipt.ConnectionEpoch ||
			source.ToolName != receipt.ToolName || source.DatasetSnapshotID != receipt.DatasetSnapshotID {
			return errors.New("publication snapshot proof source binding is invalid")
		}
	}
	return nil
}

func publicationReceiptServerID(toolName string) string {
	serverID, _, ok := domainmcpname.Parse(toolName)
	if !ok {
		return ""
	}
	return serverID
}

func PublicationSnapshotProofRecord(proof PublicationSnapshotProof) map[string]any {
	body, _ := json.Marshal(proof)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func FinalAnswerRequiresPublicationSnapshotProof(envelope FinalAnswerEnvelope) bool {
	return finalAnswerVariantRequiresPublicationSnapshotProof(envelope.Variant) && len(envelope.EvidenceReceiptIDs) > 0
}

func finalAnswerVariantRequiresPublicationSnapshotProof(variant FinalAnswerVariant) bool {
	return variant == EvidenceBackedAnswer || variant == PartialEvidenceAnswer || variant == VerifiedNoHitAnswer
}

func FinalAnswerVariantRequiresPublicationSnapshotProof(variant FinalAnswerVariant) bool {
	return finalAnswerVariantRequiresPublicationSnapshotProof(variant)
}

func ValidatePublicationSnapshotProofValue(proof *PublicationSnapshotProof, context domainsecurity.TurnSecurityContext, envelope FinalAnswerEnvelope, head *EvidenceRegistryHead) error {
	if !FinalAnswerRequiresPublicationSnapshotProof(envelope) {
		if proof != nil {
			return errors.New("boundary final contains publication snapshot proof")
		}
		return nil
	}
	if proof == nil || ValidatePublicationSnapshotProof(*proof) != nil || domainsecurity.ValidateTurnSecurityContext(context) != nil ||
		proof.ContextDigest != context.ContextDigest || proof.DatasetSnapshotID != context.DatasetSnapshotID ||
		proof.SourceManifestHash != context.SourceManifestHash || len(proof.EvidenceReceiptIDs) != len(envelope.EvidenceReceiptIDs) {
		return errors.New("fact final lacks a matching publication snapshot proof")
	}
	for index := range envelope.EvidenceReceiptIDs {
		if proof.EvidenceReceiptIDs[index] != envelope.EvidenceReceiptIDs[index] {
			return errors.New("publication snapshot proof receipt set is mismatched")
		}
	}
	if head != nil && (ValidateEvidenceRegistryHead(*head) != nil || proof.ContextDigest != head.ContextDigest ||
		proof.DatasetSnapshotID != head.DatasetSnapshotID || proof.RegistrySequence != head.Sequence || proof.RegistryStateDigest != head.StateDigest) {
		return errors.New("publication snapshot proof registry head is mismatched")
	}
	return nil
}

func clonePublicationSources(sources []PublicationSourceSnapshot) []PublicationSourceSnapshot {
	if sources == nil {
		return nil
	}
	return append([]PublicationSourceSnapshot(nil), sources...)
}

func clonePublicationSnapshotProof(proof *PublicationSnapshotProof) *PublicationSnapshotProof {
	if proof == nil {
		return nil
	}
	cloned := *proof
	cloned.EvidenceReceiptIDs = append([]string(nil), proof.EvidenceReceiptIDs...)
	cloned.Sources = clonePublicationSources(proof.Sources)
	return &cloned
}

func publicationSnapshotProofDigestValue(proof *PublicationSnapshotProof) string {
	if proof == nil {
		return ""
	}
	return proof.ProofDigest
}

func publicationSnapshotProofDigest(proof PublicationSnapshotProof) string {
	proof.ProofDigest = ""
	body, _ := json.Marshal(proof)
	return domainsecurity.SHA256Hex(body)
}
