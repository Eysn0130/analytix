package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	SourceProbeVersion = 1

	// DatasetSnapshotIDPrefixV2 is reserved for the content-rooted snapshot
	// contract. A string with this prefix is never authority by itself: the
	// host must also resolve a current registry record, contract, content
	// roots, and producer identity before factual execution can be enabled.
	DatasetSnapshotIDPrefixV2 = "dsv2_"

	SourceProbeBlockerDatasetSnapshotAuthorityUnavailable = "dataset_snapshot_authority_unavailable"
	SourceProbeBlockerLegacySnapshotAuditOnly             = SourceProbeBlockerDatasetSnapshotAuthorityUnavailable
	SourceProbeBlockerRegistryAuthorityRequired           = SourceProbeBlockerDatasetSnapshotAuthorityUnavailable
	SourceProbeBlockerSnapshotIdentityInvalid             = SourceProbeBlockerDatasetSnapshotAuthorityUnavailable
)

func IsDatasetSnapshotIDV1(value string) bool {
	return validDatasetSnapshotIDV1(value)
}

// IsDatasetSnapshotIDV2Syntax recognizes only the canonical reserved V2
// identifier shape. It deliberately says nothing about registry membership,
// currentness, content roots, the manifest contract, or producer identity.
func IsDatasetSnapshotIDV2Syntax(value string) bool {
	return strings.HasPrefix(value, DatasetSnapshotIDPrefixV2) &&
		isCanonicalSHA256Hex(strings.TrimPrefix(value, DatasetSnapshotIDPrefixV2))
}

// validSourceProbeDatasetSnapshotIDSyntax preserves historical DSV1 probes as
// audit records and permits witnessed DSV2 identities to cross the source
// probe boundary. Syntax is not factual authority: both versions remain
// subject to DatasetSnapshotFactAuthorityBlocker and host-side current-witness
// validation.
func validSourceProbeDatasetSnapshotIDSyntax(value string) bool {
	return validDatasetSnapshotIDV1(value) || IsDatasetSnapshotIDV2Syntax(value)
}

// DatasetSnapshotFactAuthorityBlocker returns the fixed host blocker for a
// snapshot identifier that cannot currently authorize case facts. DSV1 is a
// legacy audit identity. A bare DSV2 identifier is only a claim about shape;
// this runtime does not yet receive a registry-backed V2 authority record.
func DatasetSnapshotFactAuthorityBlocker(datasetSnapshotID string) string {
	switch {
	case validDatasetSnapshotIDV1(datasetSnapshotID):
		return SourceProbeBlockerLegacySnapshotAuditOnly
	case IsDatasetSnapshotIDV2Syntax(datasetSnapshotID):
		return SourceProbeBlockerRegistryAuthorityRequired
	default:
		return SourceProbeBlockerSnapshotIdentityInvalid
	}
}

// SourceProbeCanAuthorizeFacts is the common factual-execution and evidence
// issuance gate. Structurally verified DSV1 probes remain readable as audit
// records, but cannot advertise or execute factual tools, mint evidence, or
// support publication. Bare DSV2 self-reports also cannot pass this gate.
func SourceProbeCanAuthorizeFacts(probe VerifiedSourceProbe) bool {
	if ValidateVerifiedSourceProbe(probe) != nil {
		return false
	}
	return DatasetSnapshotFactAuthorityBlocker(probe.DatasetSnapshotID) == ""
}

// SourceProbeEligibleForHostAuthorityV2 is a structural precondition for a
// host-owned, callback-scoped evidence authority. It deliberately does not
// grant fact authority: the caller must additionally prove that the exact
// DSV2 record is selected by a fresh shared witness head and that this probe
// remains current under the same source lease.
func SourceProbeEligibleForHostAuthorityV2(probe VerifiedSourceProbe) bool {
	return ValidateVerifiedSourceProbe(probe) == nil &&
		IsDatasetSnapshotIDV2Syntax(probe.DatasetSnapshotID)
}

type SourceProbeResponse struct {
	Version           int    `json:"version"`
	ServerName        string `json:"serverName"`
	ServerVersion     string `json:"serverVersion"`
	CaseID            string `json:"caseId"`
	CaseBindingHash   string `json:"caseBindingHash"`
	DatasetSnapshotID string `json:"datasetSnapshotId"`
	Ready             bool   `json:"ready"`
	ReadOnly          bool   `json:"readOnly"`
	Blocker           string `json:"blocker"`
	CheckedAt         string `json:"checkedAt"`
}

type VerifiedSourceProbe struct {
	Version            int    `json:"version"`
	ServerID           string `json:"serverId"`
	ServerIdentity     string `json:"serverIdentity"`
	ConnectionEpoch    uint64 `json:"connectionEpoch"`
	CatalogFingerprint string `json:"catalogFingerprint"`
	SpecFingerprint    string `json:"specFingerprint"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	ProbeContextDigest string `json:"probeContextDigest"`
	CaseID             string `json:"caseId"`
	CaseBindingHash    string `json:"caseBindingHash"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	ReadOnly           bool   `json:"readOnly"`
	CheckedAt          string `json:"checkedAt"`
	ProbeDigest        string `json:"probeDigest"`
}

type VerifiedSourceProbeInput struct {
	ServerID           string
	ServerIdentity     string
	ConnectionEpoch    uint64
	CatalogFingerprint string
	SpecFingerprint    string
	ThreadID           string
	TurnID             string
	ContextEpoch       uint64
	ContextDigest      string
	DatasetSnapshotID  string
	CheckedAt          time.Time
	Response           SourceProbeResponse
}

func ParseSourceProbeResponse(value any) (SourceProbeResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return SourceProbeResponse{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var response SourceProbeResponse
	if err := decoder.Decode(&response); err != nil {
		return SourceProbeResponse{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SourceProbeResponse{}, errors.New("source probe contains trailing JSON")
	}
	if err := ValidateSourceProbeResponse(response); err != nil {
		return SourceProbeResponse{}, err
	}
	return response, nil
}

func ValidateSourceProbeResponse(response SourceProbeResponse) error {
	if response.Version != SourceProbeVersion || strings.TrimSpace(response.ServerName) == "" || strings.TrimSpace(response.ServerVersion) == "" ||
		strings.TrimSpace(response.CaseID) == "" || !isSHA256Hex(response.CaseBindingHash) || !response.ReadOnly ||
		strings.TrimSpace(response.CheckedAt) == "" {
		return errors.New("source probe is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, response.CheckedAt); err != nil {
		return errors.New("source probe checkedAt is invalid")
	}
	if response.Ready {
		if !validSourceProbeDatasetSnapshotIDSyntax(response.DatasetSnapshotID) || strings.TrimSpace(response.Blocker) != "" {
			return errors.New("ready source probe has invalid snapshot state")
		}
	} else if strings.TrimSpace(response.DatasetSnapshotID) != "" || strings.TrimSpace(response.Blocker) == "" {
		return errors.New("unready source probe has invalid blocker state")
	}
	return nil
}

func NewVerifiedSourceProbe(input VerifiedSourceProbeInput) (VerifiedSourceProbe, error) {
	if err := ValidateSourceProbeResponse(input.Response); err != nil {
		return VerifiedSourceProbe{}, err
	}
	expectedSnapshotID := input.DatasetSnapshotID
	if !validSourceProbeDatasetSnapshotIDSyntax(expectedSnapshotID) || input.Response.DatasetSnapshotID != expectedSnapshotID {
		return VerifiedSourceProbe{}, errors.New("source probe dataset snapshot does not match frozen host authority")
	}
	identity, err := ParseVerifiedMCPServerIdentity(strings.TrimSpace(input.ServerIdentity))
	if err != nil || !VerifiedMCPServerIdentityCanAuthorizeFacts(identity) || identity.ServerID != strings.TrimSpace(input.ServerID) || identity.ConnectionEpoch != input.ConnectionEpoch ||
		identity.ObservedName != input.Response.ServerName || identity.ObservedVersion != input.Response.ServerVersion {
		return VerifiedSourceProbe{}, errors.New("source probe verified server identity is mismatched")
	}
	checkedAt := input.CheckedAt.UTC()
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	probe := VerifiedSourceProbe{
		Version: SourceProbeVersion, ServerID: strings.TrimSpace(input.ServerID), ServerIdentity: strings.TrimSpace(input.ServerIdentity),
		ConnectionEpoch: input.ConnectionEpoch, CatalogFingerprint: strings.TrimSpace(input.CatalogFingerprint), SpecFingerprint: strings.TrimSpace(input.SpecFingerprint),
		ThreadID: strings.TrimSpace(input.ThreadID), TurnID: strings.TrimSpace(input.TurnID), ContextEpoch: input.ContextEpoch,
		ProbeContextDigest: strings.TrimSpace(input.ContextDigest), CaseID: strings.TrimSpace(input.Response.CaseID), CaseBindingHash: strings.TrimSpace(input.Response.CaseBindingHash),
		DatasetSnapshotID: expectedSnapshotID, ReadOnly: input.Response.ReadOnly, CheckedAt: checkedAt.Format(time.RFC3339Nano),
	}
	probe.ProbeDigest = verifiedSourceProbeDigest(probe)
	if err := ValidateVerifiedSourceProbe(probe); err != nil {
		return VerifiedSourceProbe{}, err
	}
	return probe, nil
}

func ValidateVerifiedSourceProbe(probe VerifiedSourceProbe) error {
	if probe.Version != SourceProbeVersion || strings.TrimSpace(probe.ServerID) == "" || strings.TrimSpace(probe.ServerIdentity) == "" ||
		probe.ConnectionEpoch == 0 || !isSHA256Hex(probe.CatalogFingerprint) || !isSHA256Hex(probe.SpecFingerprint) ||
		strings.TrimSpace(probe.ThreadID) == "" || strings.TrimSpace(probe.TurnID) == "" || probe.ContextEpoch == 0 || !isSHA256Hex(probe.ProbeContextDigest) ||
		strings.TrimSpace(probe.CaseID) == "" || !isSHA256Hex(probe.CaseBindingHash) || !validSourceProbeDatasetSnapshotIDSyntax(probe.DatasetSnapshotID) ||
		!probe.ReadOnly || strings.TrimSpace(probe.CheckedAt) == "" || !isSHA256Hex(probe.ProbeDigest) {
		return errors.New("verified source probe is incomplete")
	}
	identity, err := ParseVerifiedMCPServerIdentity(probe.ServerIdentity)
	if err != nil || !VerifiedMCPServerIdentityCanAuthorizeFacts(identity) || identity.ServerID != probe.ServerID || identity.ConnectionEpoch != probe.ConnectionEpoch {
		return errors.New("verified source probe server identity is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, probe.CheckedAt); err != nil {
		return errors.New("verified source probe checkedAt is invalid")
	}
	if verifiedSourceProbeDigest(probe) != probe.ProbeDigest {
		return errors.New("verified source probe integrity is invalid")
	}
	return nil
}

func VerifiedSourceProbeRecord(probe VerifiedSourceProbe) map[string]any {
	body, _ := json.Marshal(probe)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func verifiedSourceProbeDigest(probe VerifiedSourceProbe) string {
	probe.ProbeDigest = ""
	body, _ := json.Marshal(probe)
	return SHA256Hex(body)
}
