package privatecastopology

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	RecoveryTargetChunkSchemaVersionV1          = 1
	RecoveryTargetChunkPurposeV1                = "analytix.private-cas-recovery-target-chunk/v1"
	RecoveryJournalPreparationSchemaVersionV1   = 1
	RecoveryJournalPreparationPurposeV1         = "analytix.private-cas-recovery-preparation/v1"
	RecoveryJournalManifestSchemaVersionV1      = 1
	RecoveryJournalManifestPurposeV1            = "analytix.private-cas-recovery-manifest/v1"
	RecoveryJournalCommitWitnessSchemaVersionV1 = 1
	RecoveryJournalCommitWitnessPurposeV1       = "analytix.private-cas-recovery-commit-witness/v1"
	RecoveryJournalCompletionSchemaVersionV1    = 1
	RecoveryJournalCompletionPurposeV1          = "analytix.private-cas-recovery-completion/v1"
	RecoveryJournalAuthorityAlgorithmV1         = "Ed25519"

	MaxRecoveryJournalParticipantsV1 = 256
	MaxRecoveryJournalPlansV1        = 256
	MaxRecoveryJournalTopologiesV1   = 256
	MaxRecoveryTargetsV1             = 65_536
	MaxRecoveryTargetChunksV1        = 256
	MaxRecoveryTargetsPerChunkV1     = 256
	MaxRecoveryTargetBodyBytesV1     = 1 << 40

	MaxRecoveryTargetChunkBytesV1      = 1 << 20
	MaxRecoveryJournalRecordBytesV1    = 256 << 10
	maxRecoveryJournalTextBytesV1      = 1024
	maxRecoveryJournalTimestampBytesV1 = 64
)

var (
	recoveryTargetIDDomainV1            = []byte("analytix.private-cas-recovery-target/id/v1\x00")
	recoveryTargetChunkDomainV1         = []byte("analytix.private-cas-recovery-target-chunk/digest/v1\x00")
	recoveryTargetSetRootDomainV1       = []byte("analytix.private-cas-recovery-target-set/root/v1\x00")
	recoveryPreparationSignDomainV1     = []byte("analytix.private-cas-recovery-preparation/signature/v1\x00")
	recoveryPreparationDigestDomainV1   = []byte("analytix.private-cas-recovery-preparation/digest/v1\x00")
	recoveryManifestTransactionDomainV1 = []byte("analytix.private-cas-recovery-manifest/transaction/v1\x00")
	recoveryManifestSignDomainV1        = []byte("analytix.private-cas-recovery-manifest/signature/v1\x00")
	recoveryManifestDigestDomainV1      = []byte("analytix.private-cas-recovery-manifest/digest/v1\x00")
	recoveryCommitSignDomainV1          = []byte("analytix.private-cas-recovery-commit-witness/signature/v1\x00")
	recoveryCommitDigestDomainV1        = []byte("analytix.private-cas-recovery-commit-witness/digest/v1\x00")
	recoveryCompletionSignDomainV1      = []byte("analytix.private-cas-recovery-completion/signature/v1\x00")
	recoveryCompletionDigestDomainV1    = []byte("analytix.private-cas-recovery-completion/digest/v1\x00")
)

// RecoveryTargetEntryV1 identifies one exact ordinary private-CAS write
// residue. RootID is a logical catalog id, never an absolute host path.
type RecoveryTargetEntryV1 struct {
	TargetID             string `json:"targetId"`
	PlanIndex            uint32 `json:"planIndex"`
	RootID               string `json:"rootId"`
	PlanAuthorityDigest  string `json:"planAuthorityDigest"`
	ShardName            string `json:"shardName"`
	ShardIdentityDigest  string `json:"shardIdentityDigest"`
	OriginalName         string `json:"originalName"`
	ObjectIdentityDigest string `json:"objectIdentityDigest"`
	BodySHA256           string `json:"bodySha256"`
	ByteLength           uint64 `json:"byteLength"`
	Linked               bool   `json:"linked"`
}

type RecoveryTargetEntryInputV1 struct {
	PlanIndex            uint32
	RootID               string
	PlanAuthorityDigest  string
	ShardName            string
	ShardIdentityDigest  string
	OriginalName         string
	ObjectIdentityDigest string
	BodySHA256           string
	ByteLength           uint64
	Linked               bool
}

// RecoveryTargetChunkV1 bounds target inventory records independently of the
// manifest. The signed manifest commits to the ordered chunk digests.
type RecoveryTargetChunkV1 struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Purpose       string                  `json:"purpose"`
	ChunkIndex    uint32                  `json:"chunkIndex"`
	EntryCount    uint32                  `json:"entryCount"`
	Entries       []RecoveryTargetEntryV1 `json:"entries"`
	ChunkDigest   string                  `json:"chunkDigest"`
}

type RecoveryJournalPreparationV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	RootBindingDigest        string `json:"rootBindingDigest"`
	AuthoritySetDigest       string `json:"authoritySetDigest"`
	ParticipantCount         uint32 `json:"participantCount"`
	PlanCount                uint32 `json:"planCount"`
	TopologyCount            uint32 `json:"topologyCount"`
	JournalDirectoryIdentity string `json:"journalDirectoryIdentity"`
	TargetsDirectoryIdentity string `json:"targetsDirectoryIdentity"`
	PreparedAt               string `json:"preparedAt"`
	AuthorityAlgorithm       string `json:"authorityAlgorithm"`
	AuthorityKeyID           string `json:"authorityKeyId"`
	AuthoritySignature       string `json:"authoritySignature"`
	PreparationDigest        string `json:"preparationDigest"`
}

type RecoveryJournalPreparationInputV1 struct {
	RootBindingDigest        string
	AuthoritySetDigest       string
	ParticipantCount         uint32
	PlanCount                uint32
	TopologyCount            uint32
	JournalDirectoryIdentity string
	TargetsDirectoryIdentity string
	PreparedAt               time.Time
	AuthorityKeyID           string
}

type RecoveryJournalManifestV1 struct {
	SchemaVersion            int      `json:"schemaVersion"`
	Purpose                  string   `json:"purpose"`
	TransactionID            string   `json:"transactionId"`
	PreparationDigest        string   `json:"preparationDigest"`
	RootBindingDigest        string   `json:"rootBindingDigest"`
	AuthoritySetDigest       string   `json:"authoritySetDigest"`
	TargetSetRoot            string   `json:"targetSetRoot"`
	ChunkDigests             []string `json:"chunkDigests"`
	TargetCount              uint32   `json:"targetCount"`
	ChunkCount               uint32   `json:"chunkCount"`
	ParticipantCount         uint32   `json:"participantCount"`
	PlanCount                uint32   `json:"planCount"`
	TopologyCount            uint32   `json:"topologyCount"`
	JournalDirectoryIdentity string   `json:"journalDirectoryIdentity"`
	TargetsDirectoryIdentity string   `json:"targetsDirectoryIdentity"`
	ManifestedAt             string   `json:"manifestedAt"`
	AuthorityAlgorithm       string   `json:"authorityAlgorithm"`
	AuthorityKeyID           string   `json:"authorityKeyId"`
	AuthoritySignature       string   `json:"authoritySignature"`
	ManifestDigest           string   `json:"manifestDigest"`
}

type RecoveryJournalManifestInputV1 struct {
	Preparation  RecoveryJournalPreparationV1
	Chunks       []RecoveryTargetChunkV1
	ManifestedAt time.Time
}

type RecoveryJournalCommitWitnessV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	TransactionID            string `json:"transactionId"`
	ManifestDigest           string `json:"manifestDigest"`
	RootBindingDigest        string `json:"rootBindingDigest"`
	AuthoritySetDigest       string `json:"authoritySetDigest"`
	TargetSetRoot            string `json:"targetSetRoot"`
	CommitTargetID           string `json:"commitTargetId"`
	TargetCount              uint32 `json:"targetCount"`
	ChunkCount               uint32 `json:"chunkCount"`
	ParticipantCount         uint32 `json:"participantCount"`
	PlanCount                uint32 `json:"planCount"`
	TopologyCount            uint32 `json:"topologyCount"`
	JournalDirectoryIdentity string `json:"journalDirectoryIdentity"`
	TargetsDirectoryIdentity string `json:"targetsDirectoryIdentity"`
	WitnessedAt              string `json:"witnessedAt"`
	AuthorityAlgorithm       string `json:"authorityAlgorithm"`
	AuthorityKeyID           string `json:"authorityKeyId"`
	AuthoritySignature       string `json:"authoritySignature"`
	WitnessDigest            string `json:"witnessDigest"`
}

type RecoveryJournalCommitWitnessInputV1 struct {
	Manifest       RecoveryJournalManifestV1
	Chunks         []RecoveryTargetChunkV1
	CommitTargetID string
	WitnessedAt    time.Time
}

type RecoveryJournalCompletionReceiptV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	TransactionID            string `json:"transactionId"`
	ManifestDigest           string `json:"manifestDigest"`
	CommitWitnessDigest      string `json:"commitWitnessDigest"`
	RootBindingDigest        string `json:"rootBindingDigest"`
	AuthoritySetDigest       string `json:"authoritySetDigest"`
	TargetSetRoot            string `json:"targetSetRoot"`
	FinalInventoryDigest     string `json:"finalInventoryDigest"`
	TargetCount              uint32 `json:"targetCount"`
	ChunkCount               uint32 `json:"chunkCount"`
	ParticipantCount         uint32 `json:"participantCount"`
	PlanCount                uint32 `json:"planCount"`
	TopologyCount            uint32 `json:"topologyCount"`
	JournalDirectoryIdentity string `json:"journalDirectoryIdentity"`
	TargetsDirectoryIdentity string `json:"targetsDirectoryIdentity"`
	CompletedAt              string `json:"completedAt"`
	AuthorityAlgorithm       string `json:"authorityAlgorithm"`
	AuthorityKeyID           string `json:"authorityKeyId"`
	AuthoritySignature       string `json:"authoritySignature"`
	ReceiptDigest            string `json:"receiptDigest"`
}

type RecoveryJournalCompletionInputV1 struct {
	Manifest             RecoveryJournalManifestV1
	CommitWitness        RecoveryJournalCommitWitnessV1
	FinalInventoryDigest string
	CompletedAt          time.Time
}

type RecoveryJournalSessionStateV1 string

const (
	RecoveryJournalSessionPreparedV1        RecoveryJournalSessionStateV1 = "prepared"
	RecoveryJournalSessionManifestedV1      RecoveryJournalSessionStateV1 = "manifested"
	RecoveryJournalSessionCommitWitnessedV1 RecoveryJournalSessionStateV1 = "commit_witnessed"
	RecoveryJournalSessionCompletedV1       RecoveryJournalSessionStateV1 = "completed"
)

// RecoveryJournalSessionV1 is an in-memory reconstruction. Persistence
// adapters load immutable records independently and must authenticate every
// signature through their trusted key authority before returning a session.
type RecoveryJournalSessionV1 struct {
	State             RecoveryJournalSessionStateV1
	Preparation       RecoveryJournalPreparationV1
	Chunks            []RecoveryTargetChunkV1
	Manifest          *RecoveryJournalManifestV1
	CommitWitness     *RecoveryJournalCommitWitnessV1
	CompletionReceipt *RecoveryJournalCompletionReceiptV1
}

func NewRecoveryTargetEntryV1(input RecoveryTargetEntryInputV1) (RecoveryTargetEntryV1, error) {
	entry := RecoveryTargetEntryV1{
		PlanIndex: input.PlanIndex, RootID: input.RootID,
		PlanAuthorityDigest: input.PlanAuthorityDigest, ShardName: input.ShardName,
		ShardIdentityDigest: input.ShardIdentityDigest, OriginalName: input.OriginalName,
		ObjectIdentityDigest: input.ObjectIdentityDigest, BodySHA256: input.BodySHA256,
		ByteLength: input.ByteLength, Linked: input.Linked,
	}
	entry.TargetID = recoveryTargetIDV1(entry)
	return entry, ValidateRecoveryTargetEntryV1(entry)
}

func ValidateRecoveryTargetEntryV1(entry RecoveryTargetEntryV1) error {
	if !validRecoveryRootIDV1(entry.RootID) || !ValidDigestV1(entry.PlanAuthorityDigest) ||
		!ValidShardV1(entry.ShardName) || !ValidDigestV1(entry.ShardIdentityDigest) ||
		!validRecoveryOriginalNameV1(entry.OriginalName, entry.ShardName) ||
		!ValidDigestV1(entry.ObjectIdentityDigest) || !ValidDigestV1(entry.BodySHA256) ||
		entry.ByteLength > MaxRecoveryTargetBodyBytesV1 || !ValidDigestV1(entry.TargetID) ||
		entry.TargetID != recoveryTargetIDV1(entry) {
		return errors.New("private CAS recovery target entry is invalid")
	}
	return nil
}

func RecoveryTargetEntryV1Bytes(entry RecoveryTargetEntryV1) ([]byte, error) {
	if err := ValidateRecoveryTargetEntryV1(entry); err != nil {
		return nil, err
	}
	return json.Marshal(entry)
}

func ParseRecoveryTargetEntryV1(body []byte) (RecoveryTargetEntryV1, error) {
	return parseRecoveryCanonicalV1(body, MaxRecoveryJournalRecordBytesV1, 16, 256, ValidateRecoveryTargetEntryV1)
}

func BuildRecoveryTargetChunksV1(entries []RecoveryTargetEntryV1) ([]RecoveryTargetChunkV1, error) {
	if len(entries) == 0 || len(entries) > MaxRecoveryTargetsV1 {
		return nil, errors.New("private CAS recovery target set is outside its bound")
	}
	canonical := append([]RecoveryTargetEntryV1(nil), entries...)
	for _, entry := range canonical {
		if err := ValidateRecoveryTargetEntryV1(entry); err != nil {
			return nil, err
		}
	}
	sort.Slice(canonical, func(left, right int) bool {
		return recoveryTargetEntryLessV1(canonical[left], canonical[right])
	})
	if err := validateCanonicalRecoveryTargetEntriesV1(canonical); err != nil {
		return nil, err
	}
	chunks := make([]RecoveryTargetChunkV1, 0, (len(canonical)+MaxRecoveryTargetsPerChunkV1-1)/MaxRecoveryTargetsPerChunkV1)
	for offset := 0; offset < len(canonical); offset += MaxRecoveryTargetsPerChunkV1 {
		end := offset + MaxRecoveryTargetsPerChunkV1
		if end > len(canonical) {
			end = len(canonical)
		}
		chunk := RecoveryTargetChunkV1{
			SchemaVersion: RecoveryTargetChunkSchemaVersionV1, Purpose: RecoveryTargetChunkPurposeV1,
			ChunkIndex: uint32(len(chunks)), EntryCount: uint32(end - offset),
			Entries: append([]RecoveryTargetEntryV1(nil), canonical[offset:end]...),
		}
		chunk.ChunkDigest = recoveryTargetChunkDigestV1(chunk)
		if err := ValidateRecoveryTargetChunkV1(chunk); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func ValidateRecoveryTargetChunkV1(chunk RecoveryTargetChunkV1) error {
	if chunk.SchemaVersion != RecoveryTargetChunkSchemaVersionV1 || chunk.Purpose != RecoveryTargetChunkPurposeV1 ||
		len(chunk.Entries) == 0 || len(chunk.Entries) > MaxRecoveryTargetsPerChunkV1 ||
		chunk.EntryCount != uint32(len(chunk.Entries)) || chunk.ChunkIndex >= MaxRecoveryTargetChunksV1 ||
		!ValidDigestV1(chunk.ChunkDigest) {
		return errors.New("private CAS recovery target chunk is invalid")
	}
	for _, entry := range chunk.Entries {
		if err := ValidateRecoveryTargetEntryV1(entry); err != nil {
			return err
		}
	}
	if err := validateCanonicalRecoveryTargetEntriesV1(chunk.Entries); err != nil {
		return err
	}
	if chunk.ChunkDigest != recoveryTargetChunkDigestV1(chunk) {
		return errors.New("private CAS recovery target chunk digest is invalid")
	}
	body, err := json.Marshal(chunk)
	if err != nil || len(body) > MaxRecoveryTargetChunkBytesV1 {
		return errors.New("private CAS recovery target chunk exceeds its canonical bound")
	}
	return nil
}

func RecoveryTargetChunkV1Bytes(chunk RecoveryTargetChunkV1) ([]byte, error) {
	if err := ValidateRecoveryTargetChunkV1(chunk); err != nil {
		return nil, err
	}
	return json.Marshal(chunk)
}

func ParseRecoveryTargetChunkV1(body []byte) (RecoveryTargetChunkV1, error) {
	return parseRecoveryCanonicalV1(body, MaxRecoveryTargetChunkBytesV1, 16, 20_000, ValidateRecoveryTargetChunkV1)
}

func RecoveryTargetSetRootV1(chunks []RecoveryTargetChunkV1, planCount uint32) (string, uint32, error) {
	targetCount, err := validateRecoveryTargetSetV1(chunks, planCount)
	if err != nil {
		return "", 0, err
	}
	type targetSetDescriptorV1 struct {
		SchemaVersion int      `json:"schemaVersion"`
		Purpose       string   `json:"purpose"`
		TargetCount   uint32   `json:"targetCount"`
		ChunkCount    uint32   `json:"chunkCount"`
		ChunkDigests  []string `json:"chunkDigests"`
	}
	digests := make([]string, len(chunks))
	for index := range chunks {
		digests[index] = chunks[index].ChunkDigest
	}
	body, _ := json.Marshal(targetSetDescriptorV1{
		SchemaVersion: 1, Purpose: "analytix.private-cas-recovery-target-set/v1",
		TargetCount: targetCount, ChunkCount: uint32(len(chunks)), ChunkDigests: digests,
	})
	return recoveryDomainDigestV1(recoveryTargetSetRootDomainV1, body), targetCount, nil
}

func NewRecoveryJournalPreparationDraftV1(input RecoveryJournalPreparationInputV1) (RecoveryJournalPreparationV1, error) {
	preparation := RecoveryJournalPreparationV1{
		SchemaVersion: RecoveryJournalPreparationSchemaVersionV1, Purpose: RecoveryJournalPreparationPurposeV1,
		RootBindingDigest: input.RootBindingDigest, AuthoritySetDigest: input.AuthoritySetDigest,
		ParticipantCount: input.ParticipantCount, PlanCount: input.PlanCount, TopologyCount: input.TopologyCount,
		JournalDirectoryIdentity: input.JournalDirectoryIdentity, TargetsDirectoryIdentity: input.TargetsDirectoryIdentity,
		PreparedAt: canonicalRecoveryTimeV1(input.PreparedAt), AuthorityAlgorithm: RecoveryJournalAuthorityAlgorithmV1,
		AuthorityKeyID: input.AuthorityKeyID,
	}
	return preparation, validateRecoveryJournalPreparationPayloadV1(preparation)
}

func SealRecoveryJournalPreparationV1(preparation RecoveryJournalPreparationV1, signature string) (RecoveryJournalPreparationV1, error) {
	if preparation.AuthoritySignature != "" || preparation.PreparationDigest != "" ||
		validateRecoveryJournalPreparationPayloadV1(preparation) != nil || !validRecoverySignatureV1(signature) {
		return RecoveryJournalPreparationV1{}, errors.New("private CAS recovery preparation cannot be sealed")
	}
	preparation.AuthoritySignature = signature
	preparation.PreparationDigest = recoveryPreparationDigestV1(preparation)
	return preparation, ValidateRecoveryJournalPreparationV1(preparation)
}

func RecoveryJournalPreparationSigningBytesV1(preparation RecoveryJournalPreparationV1) ([]byte, error) {
	if err := validateRecoveryJournalPreparationPayloadV1(preparation); err != nil {
		return nil, err
	}
	preparation.AuthoritySignature = ""
	preparation.PreparationDigest = ""
	return recoverySigningBytesV1(recoveryPreparationSignDomainV1, preparation), nil
}

func ValidateRecoveryJournalPreparationV1(preparation RecoveryJournalPreparationV1) error {
	if err := validateRecoveryJournalPreparationPayloadV1(preparation); err != nil {
		return err
	}
	if !validRecoverySignatureV1(preparation.AuthoritySignature) || !ValidDigestV1(preparation.PreparationDigest) ||
		preparation.PreparationDigest != recoveryPreparationDigestV1(preparation) {
		return errors.New("private CAS recovery preparation envelope is invalid")
	}
	return validateRecoveryRecordBoundV1(preparation)
}

func RecoveryJournalPreparationV1Bytes(preparation RecoveryJournalPreparationV1) ([]byte, error) {
	if err := ValidateRecoveryJournalPreparationV1(preparation); err != nil {
		return nil, err
	}
	return json.Marshal(preparation)
}

func ParseRecoveryJournalPreparationV1(body []byte) (RecoveryJournalPreparationV1, error) {
	return parseRecoveryCanonicalV1(body, MaxRecoveryJournalRecordBytesV1, 8, 256, ValidateRecoveryJournalPreparationV1)
}

func NewRecoveryJournalManifestDraftV1(input RecoveryJournalManifestInputV1) (RecoveryJournalManifestV1, error) {
	if err := ValidateRecoveryJournalPreparationV1(input.Preparation); err != nil {
		return RecoveryJournalManifestV1{}, err
	}
	targetSetRoot, targetCount, err := RecoveryTargetSetRootV1(input.Chunks, input.Preparation.PlanCount)
	if err != nil {
		return RecoveryJournalManifestV1{}, err
	}
	digests := make([]string, len(input.Chunks))
	for index := range input.Chunks {
		digests[index] = input.Chunks[index].ChunkDigest
	}
	manifest := RecoveryJournalManifestV1{
		SchemaVersion: RecoveryJournalManifestSchemaVersionV1, Purpose: RecoveryJournalManifestPurposeV1,
		PreparationDigest: input.Preparation.PreparationDigest,
		RootBindingDigest: input.Preparation.RootBindingDigest, AuthoritySetDigest: input.Preparation.AuthoritySetDigest,
		TargetSetRoot: targetSetRoot, ChunkDigests: digests, TargetCount: targetCount, ChunkCount: uint32(len(digests)),
		ParticipantCount: input.Preparation.ParticipantCount, PlanCount: input.Preparation.PlanCount,
		TopologyCount:            input.Preparation.TopologyCount,
		JournalDirectoryIdentity: input.Preparation.JournalDirectoryIdentity,
		TargetsDirectoryIdentity: input.Preparation.TargetsDirectoryIdentity,
		ManifestedAt:             canonicalRecoveryTimeV1(input.ManifestedAt), AuthorityAlgorithm: input.Preparation.AuthorityAlgorithm,
		AuthorityKeyID: input.Preparation.AuthorityKeyID,
	}
	manifest.TransactionID = recoveryManifestTransactionIDV1(manifest)
	return manifest, validateRecoveryJournalManifestPayloadV1(manifest)
}

func SealRecoveryJournalManifestV1(manifest RecoveryJournalManifestV1, signature string) (RecoveryJournalManifestV1, error) {
	if manifest.AuthoritySignature != "" || manifest.ManifestDigest != "" ||
		validateRecoveryJournalManifestPayloadV1(manifest) != nil || !validRecoverySignatureV1(signature) {
		return RecoveryJournalManifestV1{}, errors.New("private CAS recovery manifest cannot be sealed")
	}
	manifest.AuthoritySignature = signature
	manifest.ManifestDigest = recoveryManifestDigestV1(manifest)
	return manifest, ValidateRecoveryJournalManifestV1(manifest)
}

func RecoveryJournalManifestSigningBytesV1(manifest RecoveryJournalManifestV1) ([]byte, error) {
	if err := validateRecoveryJournalManifestPayloadV1(manifest); err != nil {
		return nil, err
	}
	manifest.AuthoritySignature = ""
	manifest.ManifestDigest = ""
	return recoverySigningBytesV1(recoveryManifestSignDomainV1, manifest), nil
}

func ValidateRecoveryJournalManifestV1(manifest RecoveryJournalManifestV1) error {
	if err := validateRecoveryJournalManifestPayloadV1(manifest); err != nil {
		return err
	}
	if !validRecoverySignatureV1(manifest.AuthoritySignature) || !ValidDigestV1(manifest.ManifestDigest) ||
		manifest.ManifestDigest != recoveryManifestDigestV1(manifest) {
		return errors.New("private CAS recovery manifest envelope is invalid")
	}
	return validateRecoveryRecordBoundV1(manifest)
}

func RecoveryJournalManifestV1Bytes(manifest RecoveryJournalManifestV1) ([]byte, error) {
	if err := ValidateRecoveryJournalManifestV1(manifest); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func ParseRecoveryJournalManifestV1(body []byte) (RecoveryJournalManifestV1, error) {
	return parseRecoveryCanonicalV1(body, MaxRecoveryJournalRecordBytesV1, 8, 1024, ValidateRecoveryJournalManifestV1)
}

func NewRecoveryJournalCommitWitnessDraftV1(input RecoveryJournalCommitWitnessInputV1) (RecoveryJournalCommitWitnessV1, error) {
	if err := ValidateRecoveryJournalManifestForChunksV1(input.Manifest, input.Chunks); err != nil {
		return RecoveryJournalCommitWitnessV1{}, err
	}
	found := false
	for _, chunk := range input.Chunks {
		for _, entry := range chunk.Entries {
			if entry.TargetID == input.CommitTargetID {
				found = true
			}
		}
	}
	if !found {
		return RecoveryJournalCommitWitnessV1{}, errors.New("private CAS recovery commit target is not in the manifest")
	}
	manifest := input.Manifest
	witness := RecoveryJournalCommitWitnessV1{
		SchemaVersion: RecoveryJournalCommitWitnessSchemaVersionV1, Purpose: RecoveryJournalCommitWitnessPurposeV1,
		TransactionID: manifest.TransactionID, ManifestDigest: manifest.ManifestDigest,
		RootBindingDigest: manifest.RootBindingDigest, AuthoritySetDigest: manifest.AuthoritySetDigest,
		TargetSetRoot: manifest.TargetSetRoot, CommitTargetID: input.CommitTargetID,
		TargetCount: manifest.TargetCount, ChunkCount: manifest.ChunkCount,
		ParticipantCount: manifest.ParticipantCount, PlanCount: manifest.PlanCount, TopologyCount: manifest.TopologyCount,
		JournalDirectoryIdentity: manifest.JournalDirectoryIdentity,
		TargetsDirectoryIdentity: manifest.TargetsDirectoryIdentity,
		WitnessedAt:              canonicalRecoveryTimeV1(input.WitnessedAt), AuthorityAlgorithm: manifest.AuthorityAlgorithm,
		AuthorityKeyID: manifest.AuthorityKeyID,
	}
	return witness, validateRecoveryJournalCommitWitnessPayloadV1(witness)
}

func SealRecoveryJournalCommitWitnessV1(witness RecoveryJournalCommitWitnessV1, signature string) (RecoveryJournalCommitWitnessV1, error) {
	if witness.AuthoritySignature != "" || witness.WitnessDigest != "" ||
		validateRecoveryJournalCommitWitnessPayloadV1(witness) != nil || !validRecoverySignatureV1(signature) {
		return RecoveryJournalCommitWitnessV1{}, errors.New("private CAS recovery commit witness cannot be sealed")
	}
	witness.AuthoritySignature = signature
	witness.WitnessDigest = recoveryCommitWitnessDigestV1(witness)
	return witness, ValidateRecoveryJournalCommitWitnessV1(witness)
}

func RecoveryJournalCommitWitnessSigningBytesV1(witness RecoveryJournalCommitWitnessV1) ([]byte, error) {
	if err := validateRecoveryJournalCommitWitnessPayloadV1(witness); err != nil {
		return nil, err
	}
	witness.AuthoritySignature = ""
	witness.WitnessDigest = ""
	return recoverySigningBytesV1(recoveryCommitSignDomainV1, witness), nil
}

func ValidateRecoveryJournalCommitWitnessV1(witness RecoveryJournalCommitWitnessV1) error {
	if err := validateRecoveryJournalCommitWitnessPayloadV1(witness); err != nil {
		return err
	}
	if !validRecoverySignatureV1(witness.AuthoritySignature) || !ValidDigestV1(witness.WitnessDigest) ||
		witness.WitnessDigest != recoveryCommitWitnessDigestV1(witness) {
		return errors.New("private CAS recovery commit witness envelope is invalid")
	}
	return validateRecoveryRecordBoundV1(witness)
}

func RecoveryJournalCommitWitnessV1Bytes(witness RecoveryJournalCommitWitnessV1) ([]byte, error) {
	if err := ValidateRecoveryJournalCommitWitnessV1(witness); err != nil {
		return nil, err
	}
	return json.Marshal(witness)
}

func ParseRecoveryJournalCommitWitnessV1(body []byte) (RecoveryJournalCommitWitnessV1, error) {
	return parseRecoveryCanonicalV1(body, MaxRecoveryJournalRecordBytesV1, 8, 512, ValidateRecoveryJournalCommitWitnessV1)
}

func NewRecoveryJournalCompletionDraftV1(input RecoveryJournalCompletionInputV1) (RecoveryJournalCompletionReceiptV1, error) {
	if err := ValidateRecoveryJournalCommitWitnessForManifestV1(input.CommitWitness, input.Manifest); err != nil {
		return RecoveryJournalCompletionReceiptV1{}, err
	}
	manifest := input.Manifest
	receipt := RecoveryJournalCompletionReceiptV1{
		SchemaVersion: RecoveryJournalCompletionSchemaVersionV1, Purpose: RecoveryJournalCompletionPurposeV1,
		TransactionID: manifest.TransactionID, ManifestDigest: manifest.ManifestDigest,
		CommitWitnessDigest: input.CommitWitness.WitnessDigest,
		RootBindingDigest:   manifest.RootBindingDigest, AuthoritySetDigest: manifest.AuthoritySetDigest,
		TargetSetRoot: manifest.TargetSetRoot, FinalInventoryDigest: input.FinalInventoryDigest,
		TargetCount: manifest.TargetCount, ChunkCount: manifest.ChunkCount,
		ParticipantCount: manifest.ParticipantCount, PlanCount: manifest.PlanCount, TopologyCount: manifest.TopologyCount,
		JournalDirectoryIdentity: manifest.JournalDirectoryIdentity,
		TargetsDirectoryIdentity: manifest.TargetsDirectoryIdentity,
		CompletedAt:              canonicalRecoveryTimeV1(input.CompletedAt), AuthorityAlgorithm: manifest.AuthorityAlgorithm,
		AuthorityKeyID: manifest.AuthorityKeyID,
	}
	return receipt, validateRecoveryJournalCompletionPayloadV1(receipt)
}

func SealRecoveryJournalCompletionV1(receipt RecoveryJournalCompletionReceiptV1, signature string) (RecoveryJournalCompletionReceiptV1, error) {
	if receipt.AuthoritySignature != "" || receipt.ReceiptDigest != "" ||
		validateRecoveryJournalCompletionPayloadV1(receipt) != nil || !validRecoverySignatureV1(signature) {
		return RecoveryJournalCompletionReceiptV1{}, errors.New("private CAS recovery completion cannot be sealed")
	}
	receipt.AuthoritySignature = signature
	receipt.ReceiptDigest = recoveryCompletionDigestV1(receipt)
	return receipt, ValidateRecoveryJournalCompletionReceiptV1(receipt)
}

func RecoveryJournalCompletionSigningBytesV1(receipt RecoveryJournalCompletionReceiptV1) ([]byte, error) {
	if err := validateRecoveryJournalCompletionPayloadV1(receipt); err != nil {
		return nil, err
	}
	receipt.AuthoritySignature = ""
	receipt.ReceiptDigest = ""
	return recoverySigningBytesV1(recoveryCompletionSignDomainV1, receipt), nil
}

func ValidateRecoveryJournalCompletionReceiptV1(receipt RecoveryJournalCompletionReceiptV1) error {
	if err := validateRecoveryJournalCompletionPayloadV1(receipt); err != nil {
		return err
	}
	if !validRecoverySignatureV1(receipt.AuthoritySignature) || !ValidDigestV1(receipt.ReceiptDigest) ||
		receipt.ReceiptDigest != recoveryCompletionDigestV1(receipt) {
		return errors.New("private CAS recovery completion envelope is invalid")
	}
	return validateRecoveryRecordBoundV1(receipt)
}

func RecoveryJournalCompletionReceiptV1Bytes(receipt RecoveryJournalCompletionReceiptV1) ([]byte, error) {
	if err := ValidateRecoveryJournalCompletionReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func ParseRecoveryJournalCompletionReceiptV1(body []byte) (RecoveryJournalCompletionReceiptV1, error) {
	return parseRecoveryCanonicalV1(body, MaxRecoveryJournalRecordBytesV1, 8, 512, ValidateRecoveryJournalCompletionReceiptV1)
}

func ValidateRecoveryJournalManifestForPreparationV1(manifest RecoveryJournalManifestV1, preparation RecoveryJournalPreparationV1) error {
	if ValidateRecoveryJournalManifestV1(manifest) != nil || ValidateRecoveryJournalPreparationV1(preparation) != nil ||
		manifest.PreparationDigest != preparation.PreparationDigest ||
		manifest.RootBindingDigest != preparation.RootBindingDigest || manifest.AuthoritySetDigest != preparation.AuthoritySetDigest ||
		manifest.ParticipantCount != preparation.ParticipantCount || manifest.PlanCount != preparation.PlanCount ||
		manifest.TopologyCount != preparation.TopologyCount ||
		manifest.JournalDirectoryIdentity != preparation.JournalDirectoryIdentity ||
		manifest.TargetsDirectoryIdentity != preparation.TargetsDirectoryIdentity ||
		manifest.AuthorityAlgorithm != preparation.AuthorityAlgorithm || manifest.AuthorityKeyID != preparation.AuthorityKeyID ||
		!recoveryTimeNotBeforeV1(manifest.ManifestedAt, preparation.PreparedAt) {
		return errors.New("private CAS recovery manifest does not match its preparation")
	}
	return nil
}

func ValidateRecoveryJournalManifestForChunksV1(manifest RecoveryJournalManifestV1, chunks []RecoveryTargetChunkV1) error {
	if err := ValidateRecoveryJournalManifestV1(manifest); err != nil {
		return err
	}
	root, count, err := RecoveryTargetSetRootV1(chunks, manifest.PlanCount)
	if err != nil || root != manifest.TargetSetRoot || count != manifest.TargetCount || len(chunks) != int(manifest.ChunkCount) {
		return errors.New("private CAS recovery manifest target set is not exact")
	}
	for index := range chunks {
		if manifest.ChunkDigests[index] != chunks[index].ChunkDigest {
			return errors.New("private CAS recovery manifest chunk digest is not exact")
		}
	}
	return nil
}

func ValidateRecoveryJournalCommitWitnessForManifestV1(witness RecoveryJournalCommitWitnessV1, manifest RecoveryJournalManifestV1) error {
	if ValidateRecoveryJournalCommitWitnessV1(witness) != nil || ValidateRecoveryJournalManifestV1(manifest) != nil ||
		witness.TransactionID != manifest.TransactionID || witness.ManifestDigest != manifest.ManifestDigest ||
		witness.RootBindingDigest != manifest.RootBindingDigest || witness.AuthoritySetDigest != manifest.AuthoritySetDigest ||
		witness.TargetSetRoot != manifest.TargetSetRoot || witness.TargetCount != manifest.TargetCount ||
		witness.ChunkCount != manifest.ChunkCount || witness.ParticipantCount != manifest.ParticipantCount ||
		witness.PlanCount != manifest.PlanCount || witness.TopologyCount != manifest.TopologyCount ||
		witness.JournalDirectoryIdentity != manifest.JournalDirectoryIdentity ||
		witness.TargetsDirectoryIdentity != manifest.TargetsDirectoryIdentity ||
		witness.AuthorityAlgorithm != manifest.AuthorityAlgorithm || witness.AuthorityKeyID != manifest.AuthorityKeyID ||
		!recoveryTimeNotBeforeV1(witness.WitnessedAt, manifest.ManifestedAt) {
		return errors.New("private CAS recovery commit witness does not match its manifest")
	}
	return nil
}

func ValidateRecoveryJournalCompletionForWitnessV1(receipt RecoveryJournalCompletionReceiptV1, witness RecoveryJournalCommitWitnessV1) error {
	if ValidateRecoveryJournalCompletionReceiptV1(receipt) != nil || ValidateRecoveryJournalCommitWitnessV1(witness) != nil ||
		receipt.TransactionID != witness.TransactionID || receipt.ManifestDigest != witness.ManifestDigest ||
		receipt.CommitWitnessDigest != witness.WitnessDigest || receipt.RootBindingDigest != witness.RootBindingDigest ||
		receipt.AuthoritySetDigest != witness.AuthoritySetDigest || receipt.TargetSetRoot != witness.TargetSetRoot ||
		receipt.TargetCount != witness.TargetCount || receipt.ChunkCount != witness.ChunkCount ||
		receipt.ParticipantCount != witness.ParticipantCount || receipt.PlanCount != witness.PlanCount ||
		receipt.TopologyCount != witness.TopologyCount ||
		receipt.JournalDirectoryIdentity != witness.JournalDirectoryIdentity ||
		receipt.TargetsDirectoryIdentity != witness.TargetsDirectoryIdentity ||
		receipt.AuthorityAlgorithm != witness.AuthorityAlgorithm || receipt.AuthorityKeyID != witness.AuthorityKeyID ||
		!recoveryTimeNotBeforeV1(receipt.CompletedAt, witness.WitnessedAt) {
		return errors.New("private CAS recovery completion does not match its commit witness")
	}
	return nil
}

func ValidateRecoveryJournalSessionV1(session RecoveryJournalSessionV1) error {
	if err := ValidateRecoveryJournalPreparationV1(session.Preparation); err != nil {
		return err
	}
	if err := validateRecoveryChunkPrefixV1(session.Chunks); err != nil {
		return err
	}
	switch session.State {
	case RecoveryJournalSessionPreparedV1:
		if session.Manifest != nil || session.CommitWitness != nil || session.CompletionReceipt != nil {
			return errors.New("private CAS recovery prepared session has terminal records")
		}
	case RecoveryJournalSessionManifestedV1:
		if session.Manifest == nil || session.CommitWitness != nil || session.CompletionReceipt != nil {
			return errors.New("private CAS recovery manifested session shape is invalid")
		}
	case RecoveryJournalSessionCommitWitnessedV1:
		if session.Manifest == nil || session.CommitWitness == nil || session.CompletionReceipt != nil {
			return errors.New("private CAS recovery witnessed session shape is invalid")
		}
	case RecoveryJournalSessionCompletedV1:
		if session.Manifest == nil || session.CommitWitness == nil || session.CompletionReceipt == nil {
			return errors.New("private CAS recovery completed session shape is invalid")
		}
	default:
		return errors.New("private CAS recovery session state is unknown")
	}
	if session.Manifest == nil {
		return nil
	}
	if err := ValidateRecoveryJournalManifestForPreparationV1(*session.Manifest, session.Preparation); err != nil {
		return err
	}
	if err := ValidateRecoveryJournalManifestForChunksV1(*session.Manifest, session.Chunks); err != nil {
		return err
	}
	if session.CommitWitness == nil {
		return nil
	}
	if err := ValidateRecoveryJournalCommitWitnessForManifestV1(*session.CommitWitness, *session.Manifest); err != nil {
		return err
	}
	commitFound := false
	for _, chunk := range session.Chunks {
		for _, entry := range chunk.Entries {
			commitFound = commitFound || entry.TargetID == session.CommitWitness.CommitTargetID
		}
	}
	if !commitFound {
		return errors.New("private CAS recovery session commit target is absent")
	}
	if session.CompletionReceipt == nil {
		return nil
	}
	return ValidateRecoveryJournalCompletionForWitnessV1(*session.CompletionReceipt, *session.CommitWitness)
}

func validateRecoveryJournalPreparationPayloadV1(preparation RecoveryJournalPreparationV1) error {
	if preparation.SchemaVersion != RecoveryJournalPreparationSchemaVersionV1 || preparation.Purpose != RecoveryJournalPreparationPurposeV1 ||
		!ValidDigestV1(preparation.RootBindingDigest) || !ValidDigestV1(preparation.AuthoritySetDigest) ||
		!validRecoveryCountsV1(preparation.ParticipantCount, preparation.PlanCount, preparation.TopologyCount) ||
		!ValidDigestV1(preparation.JournalDirectoryIdentity) || !ValidDigestV1(preparation.TargetsDirectoryIdentity) ||
		preparation.JournalDirectoryIdentity == preparation.TargetsDirectoryIdentity || !validRecoveryTimeV1(preparation.PreparedAt) ||
		preparation.AuthorityAlgorithm != RecoveryJournalAuthorityAlgorithmV1 || !ValidDigestV1(preparation.AuthorityKeyID) {
		return errors.New("private CAS recovery preparation payload is invalid")
	}
	return nil
}

func validateRecoveryJournalManifestPayloadV1(manifest RecoveryJournalManifestV1) error {
	if manifest.SchemaVersion != RecoveryJournalManifestSchemaVersionV1 || manifest.Purpose != RecoveryJournalManifestPurposeV1 ||
		!ValidDigestV1(manifest.TransactionID) || manifest.TransactionID != recoveryManifestTransactionIDV1(manifest) ||
		!ValidDigestV1(manifest.PreparationDigest) || !ValidDigestV1(manifest.RootBindingDigest) ||
		!ValidDigestV1(manifest.AuthoritySetDigest) || !ValidDigestV1(manifest.TargetSetRoot) ||
		manifest.TargetCount == 0 || manifest.TargetCount > MaxRecoveryTargetsV1 ||
		manifest.ChunkCount == 0 || manifest.ChunkCount > MaxRecoveryTargetChunksV1 ||
		manifest.ChunkCount != uint32(len(manifest.ChunkDigests)) ||
		!validRecoveryCountsV1(manifest.ParticipantCount, manifest.PlanCount, manifest.TopologyCount) ||
		!ValidDigestV1(manifest.JournalDirectoryIdentity) || !ValidDigestV1(manifest.TargetsDirectoryIdentity) ||
		manifest.JournalDirectoryIdentity == manifest.TargetsDirectoryIdentity || !validRecoveryTimeV1(manifest.ManifestedAt) ||
		manifest.AuthorityAlgorithm != RecoveryJournalAuthorityAlgorithmV1 || !ValidDigestV1(manifest.AuthorityKeyID) {
		return errors.New("private CAS recovery manifest payload is invalid")
	}
	for _, digest := range manifest.ChunkDigests {
		if !ValidDigestV1(digest) {
			return errors.New("private CAS recovery manifest chunk digest is invalid")
		}
	}
	return nil
}

func validateRecoveryJournalCommitWitnessPayloadV1(witness RecoveryJournalCommitWitnessV1) error {
	if witness.SchemaVersion != RecoveryJournalCommitWitnessSchemaVersionV1 || witness.Purpose != RecoveryJournalCommitWitnessPurposeV1 ||
		!ValidDigestV1(witness.TransactionID) || !ValidDigestV1(witness.ManifestDigest) ||
		!ValidDigestV1(witness.RootBindingDigest) || !ValidDigestV1(witness.AuthoritySetDigest) ||
		!ValidDigestV1(witness.TargetSetRoot) || !ValidDigestV1(witness.CommitTargetID) ||
		witness.TargetCount == 0 || witness.TargetCount > MaxRecoveryTargetsV1 ||
		witness.ChunkCount == 0 || witness.ChunkCount > MaxRecoveryTargetChunksV1 ||
		!validRecoveryCountsV1(witness.ParticipantCount, witness.PlanCount, witness.TopologyCount) ||
		!ValidDigestV1(witness.JournalDirectoryIdentity) || !ValidDigestV1(witness.TargetsDirectoryIdentity) ||
		witness.JournalDirectoryIdentity == witness.TargetsDirectoryIdentity || !validRecoveryTimeV1(witness.WitnessedAt) ||
		witness.AuthorityAlgorithm != RecoveryJournalAuthorityAlgorithmV1 || !ValidDigestV1(witness.AuthorityKeyID) {
		return errors.New("private CAS recovery commit witness payload is invalid")
	}
	return nil
}

func validateRecoveryJournalCompletionPayloadV1(receipt RecoveryJournalCompletionReceiptV1) error {
	if receipt.SchemaVersion != RecoveryJournalCompletionSchemaVersionV1 || receipt.Purpose != RecoveryJournalCompletionPurposeV1 ||
		!ValidDigestV1(receipt.TransactionID) || !ValidDigestV1(receipt.ManifestDigest) ||
		!ValidDigestV1(receipt.CommitWitnessDigest) || !ValidDigestV1(receipt.RootBindingDigest) ||
		!ValidDigestV1(receipt.AuthoritySetDigest) || !ValidDigestV1(receipt.TargetSetRoot) ||
		!ValidDigestV1(receipt.FinalInventoryDigest) || receipt.TargetCount == 0 || receipt.TargetCount > MaxRecoveryTargetsV1 ||
		receipt.ChunkCount == 0 || receipt.ChunkCount > MaxRecoveryTargetChunksV1 ||
		!validRecoveryCountsV1(receipt.ParticipantCount, receipt.PlanCount, receipt.TopologyCount) ||
		!ValidDigestV1(receipt.JournalDirectoryIdentity) || !ValidDigestV1(receipt.TargetsDirectoryIdentity) ||
		receipt.JournalDirectoryIdentity == receipt.TargetsDirectoryIdentity || !validRecoveryTimeV1(receipt.CompletedAt) ||
		receipt.AuthorityAlgorithm != RecoveryJournalAuthorityAlgorithmV1 || !ValidDigestV1(receipt.AuthorityKeyID) {
		return errors.New("private CAS recovery completion payload is invalid")
	}
	return nil
}

func validateRecoveryTargetSetV1(chunks []RecoveryTargetChunkV1, planCount uint32) (uint32, error) {
	if planCount == 0 || planCount > MaxRecoveryJournalPlansV1 || len(chunks) == 0 || len(chunks) > MaxRecoveryTargetChunksV1 {
		return 0, errors.New("private CAS recovery target set is outside its bound")
	}
	targetCount := 0
	var previous *RecoveryTargetEntryV1
	for index := range chunks {
		chunk := chunks[index]
		if err := ValidateRecoveryTargetChunkV1(chunk); err != nil || chunk.ChunkIndex != uint32(index) {
			return 0, errors.New("private CAS recovery target chunk sequence is invalid")
		}
		for entryIndex := range chunk.Entries {
			entry := chunk.Entries[entryIndex]
			if entry.PlanIndex >= planCount {
				return 0, errors.New("private CAS recovery target plan index is outside the manifest")
			}
			if previous != nil && !recoveryTargetEntryLessV1(*previous, entry) {
				return 0, errors.New("private CAS recovery targets are not globally canonical")
			}
			copyEntry := entry
			previous = &copyEntry
			targetCount++
			if targetCount > MaxRecoveryTargetsV1 {
				return 0, errors.New("private CAS recovery target count exceeds its bound")
			}
		}
	}
	return uint32(targetCount), nil
}

func validateRecoveryChunkPrefixV1(chunks []RecoveryTargetChunkV1) error {
	if len(chunks) > MaxRecoveryTargetChunksV1 {
		return errors.New("private CAS recovery target chunk prefix exceeds its bound")
	}
	for index := range chunks {
		if ValidateRecoveryTargetChunkV1(chunks[index]) != nil || chunks[index].ChunkIndex != uint32(index) {
			return errors.New("private CAS recovery target chunk prefix is invalid")
		}
		if index > 0 {
			left := chunks[index-1].Entries[len(chunks[index-1].Entries)-1]
			right := chunks[index].Entries[0]
			if !recoveryTargetEntryLessV1(left, right) {
				return errors.New("private CAS recovery target chunk prefix is not canonical")
			}
		}
	}
	return nil
}

func validateCanonicalRecoveryTargetEntriesV1(entries []RecoveryTargetEntryV1) error {
	seenPhysical := make(map[string]struct{}, len(entries))
	for index := range entries {
		if index > 0 && !recoveryTargetEntryLessV1(entries[index-1], entries[index]) {
			return errors.New("private CAS recovery target entries are not sorted and unique")
		}
		physical := entries[index].RootID + "\x00" + entries[index].ShardName + "\x00" + entries[index].OriginalName
		if _, duplicate := seenPhysical[physical]; duplicate {
			return errors.New("private CAS recovery target entries repeat a physical object")
		}
		seenPhysical[physical] = struct{}{}
	}
	return nil
}

func recoveryTargetEntryLessV1(left, right RecoveryTargetEntryV1) bool {
	if left.PlanIndex != right.PlanIndex {
		return left.PlanIndex < right.PlanIndex
	}
	if left.RootID != right.RootID {
		return left.RootID < right.RootID
	}
	if left.ShardName != right.ShardName {
		return left.ShardName < right.ShardName
	}
	if left.OriginalName != right.OriginalName {
		return left.OriginalName < right.OriginalName
	}
	return left.TargetID < right.TargetID
}

func recoveryTargetIDV1(entry RecoveryTargetEntryV1) string {
	entry.TargetID = ""
	body, _ := json.Marshal(entry)
	return recoveryDomainDigestV1(recoveryTargetIDDomainV1, body)
}

func recoveryTargetChunkDigestV1(chunk RecoveryTargetChunkV1) string {
	chunk.ChunkDigest = ""
	body, _ := json.Marshal(chunk)
	return recoveryDomainDigestV1(recoveryTargetChunkDomainV1, body)
}

func recoveryPreparationDigestV1(preparation RecoveryJournalPreparationV1) string {
	preparation.PreparationDigest = ""
	body, _ := json.Marshal(preparation)
	return recoveryDomainDigestV1(recoveryPreparationDigestDomainV1, body)
}

// recoveryManifestTransactionIDV1 hashes the canonical manifest authority
// payload before its derived transaction id, signature, and record digest are
// populated. This avoids a self-referential transaction id while binding every
// caller-controlled manifest field.
func recoveryManifestTransactionIDV1(manifest RecoveryJournalManifestV1) string {
	manifest.TransactionID = ""
	manifest.AuthoritySignature = ""
	manifest.ManifestDigest = ""
	body, _ := json.Marshal(manifest)
	return recoveryDomainDigestV1(recoveryManifestTransactionDomainV1, body)
}

func recoveryManifestDigestV1(manifest RecoveryJournalManifestV1) string {
	manifest.ManifestDigest = ""
	body, _ := json.Marshal(manifest)
	return recoveryDomainDigestV1(recoveryManifestDigestDomainV1, body)
}

func recoveryCommitWitnessDigestV1(witness RecoveryJournalCommitWitnessV1) string {
	witness.WitnessDigest = ""
	body, _ := json.Marshal(witness)
	return recoveryDomainDigestV1(recoveryCommitDigestDomainV1, body)
}

func recoveryCompletionDigestV1(receipt RecoveryJournalCompletionReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return recoveryDomainDigestV1(recoveryCompletionDigestDomainV1, body)
}

func recoverySigningBytesV1(domain []byte, value any) []byte {
	body, _ := json.Marshal(value)
	digest := sha256.Sum256(body)
	result := append([]byte(nil), domain...)
	return append(result, digest[:]...)
}

func recoveryDomainDigestV1(domain []byte, body []byte) string {
	digest := sha256.New()
	_, _ = digest.Write(domain)
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func validRecoveryCountsV1(participants, plans, topologies uint32) bool {
	return participants > 0 && participants <= MaxRecoveryJournalParticipantsV1 &&
		plans >= participants && plans <= MaxRecoveryJournalPlansV1 &&
		topologies <= MaxRecoveryJournalTopologiesV1
}

func validRecoveryRootIDV1(value string) bool {
	if !validRecoveryTextV1(value, maxRecoveryJournalTextBytesV1) || path.IsAbs(value) || path.Clean(value) != value ||
		strings.Contains(value, `\`) || strings.Contains(value, "//") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	_, known := RootByRelativePathV1(value)
	return known
}

func validRecoveryOriginalNameV1(value, shard string) bool {
	return validRecoveryTextV1(value, 255) && !strings.ContainsAny(value, `/\`) && PrivateWriteTempNameV1(value, shard)
}

func validRecoverySignatureV1(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == ed25519.SignatureSize && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func validRecoveryTextV1(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func canonicalRecoveryTimeV1(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Round(0).Format(time.RFC3339Nano)
}

func validRecoveryTimeV1(value string) bool {
	if !validRecoveryTextV1(value, maxRecoveryJournalTimestampBytesV1) || !strings.HasSuffix(value, "Z") {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && !parsed.IsZero() && parsed.UTC().Round(0).Format(time.RFC3339Nano) == value
}

func recoveryTimeNotBeforeV1(later, earlier string) bool {
	if !validRecoveryTimeV1(later) || !validRecoveryTimeV1(earlier) {
		return false
	}
	laterTime, _ := time.Parse(time.RFC3339Nano, later)
	earlierTime, _ := time.Parse(time.RFC3339Nano, earlier)
	return !laterTime.Before(earlierTime)
}

func validateRecoveryRecordBoundV1(value any) error {
	body, err := json.Marshal(value)
	if err != nil || len(body) > MaxRecoveryJournalRecordBytesV1 {
		return errors.New("private CAS recovery journal record exceeds its canonical bound")
	}
	return nil
}

func parseRecoveryCanonicalV1[T any](
	body []byte,
	maxBytes int,
	maxDepth int,
	maxTokens int,
	validate func(T) error,
) (T, error) {
	var zero T
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxBytes, MaxDepth: maxDepth,
		MaxTokens: maxTokens, MaxStringBytes: maxRecoveryJournalTextBytesV1,
	}); err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var value T
	if err := decoder.Decode(&value); err != nil {
		return zero, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return zero, errors.New("private CAS recovery journal record contains trailing JSON")
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(body, canonical) {
		return zero, errors.New("private CAS recovery journal record is not canonically encoded")
	}
	if err := validate(value); err != nil {
		return zero, fmt.Errorf("private CAS recovery journal record validation failed: %w", err)
	}
	return value, nil
}
