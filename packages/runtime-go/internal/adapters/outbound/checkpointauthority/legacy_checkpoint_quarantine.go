package checkpointauthority

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

const maxLegacyCheckpointSnapshotAuditBytesV1 = 16 * 1024 * 1024

type legacyCheckpointSnapshotAuditV1 struct {
	SchemaVersion          string                                          `json:"schemaVersion"`
	Purpose                string                                          `json:"purpose"`
	SourceRelativePath     string                                          `json:"sourceRelativePath"`
	SourceRootIdentity     string                                          `json:"sourceRootIdentity"`
	PayloadName            string                                          `json:"payloadName"`
	TreeSHA256             string                                          `json:"treeSha256"`
	FileCount              int                                             `json:"fileCount"`
	TotalBytes             int64                                           `json:"totalBytes"`
	Entries                []persistencefs.LegacyCheckpointSnapshotEntryV1 `json:"entries"`
	OriginalBytesPreserved bool                                            `json:"originalBytesPreserved"`
	AuthorityEligible      bool                                            `json:"authorityEligible"`
	PublicEventEligible    bool                                            `json:"publicEventEligible"`
	QuarantinedAt          string                                          `json:"quarantinedAt"`
	AuditDigest            string                                          `json:"auditDigest"`
}

type legacyCheckpointSnapshotAuditDigestV1 struct {
	SchemaVersion          string                                          `json:"schemaVersion"`
	Purpose                string                                          `json:"purpose"`
	SourceRelativePath     string                                          `json:"sourceRelativePath"`
	SourceRootIdentity     string                                          `json:"sourceRootIdentity"`
	PayloadName            string                                          `json:"payloadName"`
	TreeSHA256             string                                          `json:"treeSha256"`
	FileCount              int                                             `json:"fileCount"`
	TotalBytes             int64                                           `json:"totalBytes"`
	Entries                []persistencefs.LegacyCheckpointSnapshotEntryV1 `json:"entries"`
	OriginalBytesPreserved bool                                            `json:"originalBytesPreserved"`
	AuthorityEligible      bool                                            `json:"authorityEligible"`
	PublicEventEligible    bool                                            `json:"publicEventEligible"`
	QuarantinedAt          string                                          `json:"quarantinedAt"`
}

// preflightLegacyCheckpointSnapshotQuarantineState validates the frozen
// sidecar projection without opening or mutating its audit CAS. The prepared
// owner plan separately binds audit bytes to this state before any cleanup.
func preflightLegacyCheckpointSnapshotQuarantineState(
	state persistencefs.LegacyCheckpointSnapshotQuarantineStateV1,
) error {
	if state.SourcePresent && state.Payload != nil {
		return errors.New("legacy checkpoint source and quarantined payload both exist")
	}
	if state.SourcePresent {
		if state.Source.SHA256 == "" {
			return errors.New("legacy checkpoint source inventory is invalid")
		}
		return preflightLegacyCheckpointSnapshotAuditSize(state.Source)
	}
	if state.Payload == nil {
		return nil
	}
	if persistencefs.LegacyCheckpointSnapshotPayloadDigestV1(state.Payload.Name) != state.Payload.Tree.SHA256 {
		return errors.New("legacy checkpoint quarantine payload is not content-bound")
	}
	return preflightLegacyCheckpointSnapshotAuditSize(state.Payload.Tree)
}

func loadLegacyCheckpointSnapshotAudits(
	ctx context.Context,
	store *finalauthority.SecurePrivateCAS,
) ([]legacyCheckpointSnapshotAuditV1, error) {
	files, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(files) > 1 {
		return nil, errors.New("legacy checkpoint quarantine audit inventory is ambiguous")
	}
	audits := make([]legacyCheckpointSnapshotAuditV1, 0, len(files))
	for _, file := range files {
		var audit legacyCheckpointSnapshotAuditV1
		decoder := json.NewDecoder(bytes.NewReader(file.Body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&audit); err != nil || file.Digest != audit.AuditDigest {
			return nil, errors.New("legacy checkpoint quarantine audit record is invalid")
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, errors.New("legacy checkpoint quarantine audit record has trailing content")
		}
		canonical, err := legacyCheckpointSnapshotAuditBytes(audit)
		if err != nil || !bytes.Equal(canonical, file.Body) {
			return nil, errors.New("legacy checkpoint quarantine audit record is non-canonical")
		}
		audits = append(audits, audit)
	}
	return audits, nil
}

func newLegacyCheckpointSnapshotAudit(
	payload persistencefs.LegacyCheckpointSnapshotPayloadV1,
	quarantinedAt time.Time,
) (legacyCheckpointSnapshotAuditV1, []byte, error) {
	if quarantinedAt.IsZero() || quarantinedAt.Location() != time.UTC {
		return legacyCheckpointSnapshotAuditV1{}, nil, errors.New("legacy checkpoint quarantine time is invalid")
	}
	audit := legacyCheckpointSnapshotAuditV1{
		SchemaVersion:          persistencefs.LegacyCheckpointSnapshotQuarantineSchemaV1,
		Purpose:                "legacy_checkpoint_snapshot_forensic_quarantine",
		SourceRelativePath:     persistencefs.LegacyCheckpointSnapshotDirectoryV1,
		SourceRootIdentity:     payload.Tree.RootIdentity,
		PayloadName:            payload.Name,
		TreeSHA256:             payload.Tree.SHA256,
		FileCount:              payload.Tree.FileCount,
		TotalBytes:             payload.Tree.TotalBytes,
		Entries:                append([]persistencefs.LegacyCheckpointSnapshotEntryV1(nil), payload.Tree.Entries...),
		OriginalBytesPreserved: true,
		AuthorityEligible:      false,
		PublicEventEligible:    false,
		QuarantinedAt:          quarantinedAt.Format(time.RFC3339Nano),
	}
	digestBody, err := json.Marshal(legacyCheckpointSnapshotAuditDigestV1{
		SchemaVersion: audit.SchemaVersion, Purpose: audit.Purpose, SourceRelativePath: audit.SourceRelativePath,
		SourceRootIdentity: audit.SourceRootIdentity, PayloadName: audit.PayloadName, TreeSHA256: audit.TreeSHA256,
		FileCount: audit.FileCount, TotalBytes: audit.TotalBytes, Entries: audit.Entries,
		OriginalBytesPreserved: audit.OriginalBytesPreserved, AuthorityEligible: audit.AuthorityEligible,
		PublicEventEligible: audit.PublicEventEligible, QuarantinedAt: audit.QuarantinedAt,
	})
	if err != nil {
		return legacyCheckpointSnapshotAuditV1{}, nil, err
	}
	digest := sha256.Sum256(digestBody)
	audit.AuditDigest = hex.EncodeToString(digest[:])
	body, err := legacyCheckpointSnapshotAuditBytes(audit)
	if err == nil && len(body) > maxLegacyCheckpointSnapshotAuditBytesV1 {
		return legacyCheckpointSnapshotAuditV1{}, nil, errors.New("legacy checkpoint quarantine audit exceeds its private CAS bound")
	}
	return audit, body, err
}

func preflightLegacyCheckpointSnapshotAuditSize(tree persistencefs.LegacyCheckpointSnapshotTreeV1) error {
	payload := persistencefs.LegacyCheckpointSnapshotPayloadV1{
		Name: "legacy-v1-" + tree.SHA256 + "-00000000000000000000000000000000",
		Tree: tree,
	}
	_, _, err := newLegacyCheckpointSnapshotAudit(payload, time.Unix(1, 0).UTC())
	return err
}

func legacyCheckpointSnapshotAuditBytes(audit legacyCheckpointSnapshotAuditV1) ([]byte, error) {
	if audit.AuditDigest == "" {
		return nil, errors.New("legacy checkpoint quarantine audit digest is missing")
	}
	return json.Marshal(audit)
}

func validateLegacyCheckpointSnapshotAudit(
	audit legacyCheckpointSnapshotAuditV1,
	payload persistencefs.LegacyCheckpointSnapshotPayloadV1,
) error {
	parsedTime, err := time.Parse(time.RFC3339Nano, audit.QuarantinedAt)
	if err != nil || parsedTime.IsZero() || parsedTime.Location() != time.UTC {
		return errors.New("legacy checkpoint quarantine audit time is invalid")
	}
	if audit.SchemaVersion != persistencefs.LegacyCheckpointSnapshotQuarantineSchemaV1 ||
		audit.Purpose != "legacy_checkpoint_snapshot_forensic_quarantine" ||
		audit.SourceRelativePath != persistencefs.LegacyCheckpointSnapshotDirectoryV1 ||
		audit.SourceRootIdentity != payload.Tree.RootIdentity || audit.PayloadName != payload.Name ||
		audit.TreeSHA256 != payload.Tree.SHA256 || audit.FileCount != payload.Tree.FileCount ||
		audit.TotalBytes != payload.Tree.TotalBytes || !audit.OriginalBytesPreserved ||
		audit.AuthorityEligible || audit.PublicEventEligible || len(audit.Entries) != len(payload.Tree.Entries) {
		return errors.New("legacy checkpoint quarantine audit does not bind its payload")
	}
	for index := range audit.Entries {
		if audit.Entries[index] != payload.Tree.Entries[index] {
			return errors.New("legacy checkpoint quarantine audit entry manifest does not bind its payload")
		}
	}
	digestBody, err := json.Marshal(legacyCheckpointSnapshotAuditDigestV1{
		SchemaVersion: audit.SchemaVersion, Purpose: audit.Purpose, SourceRelativePath: audit.SourceRelativePath,
		SourceRootIdentity: audit.SourceRootIdentity, PayloadName: audit.PayloadName, TreeSHA256: audit.TreeSHA256,
		FileCount: audit.FileCount, TotalBytes: audit.TotalBytes, Entries: audit.Entries,
		OriginalBytesPreserved: audit.OriginalBytesPreserved, AuthorityEligible: audit.AuthorityEligible,
		PublicEventEligible: audit.PublicEventEligible, QuarantinedAt: audit.QuarantinedAt,
	})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(digestBody)
	if audit.AuditDigest != hex.EncodeToString(digest[:]) {
		return errors.New("legacy checkpoint quarantine audit digest is invalid")
	}
	return nil
}

func newLegacyCheckpointSnapshotPayloadName(treeDigest string) (string, error) {
	if len(treeDigest) != 64 || strings.ToLower(treeDigest) != treeDigest {
		return "", errors.New("legacy checkpoint tree digest is invalid")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	name := "legacy-v1-" + treeDigest + "-" + hex.EncodeToString(nonce)
	if !persistencefs.ValidLegacyCheckpointSnapshotPayloadNameV1(name) {
		return "", errors.New("legacy checkpoint payload name generation failed")
	}
	return name, nil
}

func legacyCheckpointSnapshotDataDir(checkpointRoot string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(checkpointRoot))
	if err != nil || checkpointRoot == "" || filepath.Clean(absolute) != absolute ||
		filepath.Base(absolute) != "checkpoint-authority" || filepath.Base(filepath.Dir(absolute)) != "private" {
		return "", errors.New("checkpoint authority root cannot bind legacy snapshot migration")
	}
	return filepath.Dir(filepath.Dir(absolute)), nil
}
