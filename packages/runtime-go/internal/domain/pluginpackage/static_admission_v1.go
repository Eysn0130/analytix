package pluginpackage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	FirstPartyFundsPackageIDV1        = "analytix-fund-analysis"
	StaticFirstPartyEntryPolicyV1     = "host-static-first-party"
	StaticAdmissionSigningAlgorithmV1 = "Ed25519"
	SupportedLifecycleProtocolV1      = 1
)

var ErrStaticAdmissionDeniedV1 = errors.New("plugin package static admission denied")

type StaticAdmissionEvidenceV1 struct {
	ArtifactIntegrityVerified bool
	PackageAuthoritySHA256    string
	ProvenanceAuthorityDigest string
	ProvenanceClassification  string
	ProvenanceDispositionKind string
	PlatformAnchor            string
	Publishable               bool
	FactToolsEnabled          bool
	SigningAlgorithm          string
}

type StaticAdmissionInputV1 struct {
	CanonicalDeclaration       []byte
	DeclarationRawSHA256       string
	DeclarationCanonicalSHA256 string
	Evidence                   StaticAdmissionEvidenceV1
}

// StaticAdmissionDecisionV1 records an admitted publisher request. It is not a
// runtime capability grant and carries no Host lifecycle state.
type StaticAdmissionDecisionV1 struct {
	Identity                   PackageIdentityV1
	DeclarationRawSHA256       string
	DeclarationCanonicalSHA256 string
	RequestedCapabilities      []CapabilityRequestV1
	Lifecycle                  LifecycleV1
}

type capabilityCeilingV1 struct {
	protocolVersion int
	scopes          map[string]struct{}
}

var firstPartyFundsCapabilityCeilingV1 = map[string]capabilityCeilingV1{
	"funds.case.read": {
		protocolVersion: 1,
		scopes:          map[string]struct{}{"case:bound": {}, "source:verified": {}},
	},
	"funds.source.read": {
		protocolVersion: 1,
		scopes:          map[string]struct{}{"case:bound": {}, "source:verified": {}},
	},
}

func AdmitStaticFirstPartyV1(input StaticAdmissionInputV1) (StaticAdmissionDecisionV1, error) {
	declaration, err := ParseDeclarationV1(input.CanonicalDeclaration)
	if err != nil {
		return StaticAdmissionDecisionV1{}, ErrStaticAdmissionDeniedV1
	}
	canonical, err := CanonicalDeclarationV1Bytes(declaration)
	canonicalDigest := sha256.Sum256(canonical)
	if err != nil || !bytes.Equal(input.CanonicalDeclaration, canonical) ||
		!canonicalSHA256V1(input.DeclarationRawSHA256) ||
		!canonicalSHA256V1(input.DeclarationCanonicalSHA256) ||
		hex.EncodeToString(canonicalDigest[:]) != input.DeclarationCanonicalSHA256 ||
		declaration.SchemaVersion != SchemaVersionV1 ||
		declaration.PackageID != FirstPartyFundsPackageIDV1 ||
		declaration.Lifecycle.ProtocolVersion != SupportedLifecycleProtocolV1 ||
		declaration.Lifecycle.EntryPolicy != StaticFirstPartyEntryPolicyV1 ||
		!validStaticAdmissionEvidenceV1(input.Evidence) ||
		!withinFirstPartyFundsCapabilityCeilingV1(declaration.RequestedCapabilities) {
		return StaticAdmissionDecisionV1{}, ErrStaticAdmissionDeniedV1
	}
	return StaticAdmissionDecisionV1{
		Identity:                   declaration.IdentityV1(),
		DeclarationRawSHA256:       input.DeclarationRawSHA256,
		DeclarationCanonicalSHA256: input.DeclarationCanonicalSHA256,
		RequestedCapabilities:      cloneCapabilityRequestsV1(declaration.RequestedCapabilities),
		Lifecycle:                  declaration.Lifecycle,
	}, nil
}

func validStaticAdmissionEvidenceV1(evidence StaticAdmissionEvidenceV1) bool {
	if !evidence.ArtifactIntegrityVerified || evidence.Publishable || evidence.FactToolsEnabled ||
		!canonicalSHA256V1(evidence.PackageAuthoritySHA256) ||
		!canonicalSHA256V1(evidence.ProvenanceAuthorityDigest) ||
		evidence.SigningAlgorithm != StaticAdmissionSigningAlgorithmV1 {
		return false
	}
	switch evidence.ProvenanceClassification {
	case "development_clean_non_publishable", "development_dirty_non_publishable":
		return evidence.ProvenanceDispositionKind == "development_non_publishable" &&
			evidence.PlatformAnchor == "macos_nonpublishable_resource_seal"
	case "controlled_release_clean_candidate_non_publishable", "controlled_release_dirty_non_publishable":
		return evidence.ProvenanceDispositionKind == "controlled_release_receipt" &&
			evidence.PlatformAnchor == "macos_developer_id_resource_seal"
	default:
		return false
	}
}

func withinFirstPartyFundsCapabilityCeilingV1(requests []CapabilityRequestV1) bool {
	for _, request := range requests {
		ceiling, ok := firstPartyFundsCapabilityCeilingV1[request.ID]
		if !ok || request.ProtocolVersion != ceiling.protocolVersion {
			return false
		}
		for _, scope := range request.ScopeConstraints {
			if _, ok := ceiling.scopes[scope]; !ok {
				return false
			}
		}
	}
	return true
}

func cloneCapabilityRequestsV1(requests []CapabilityRequestV1) []CapabilityRequestV1 {
	cloned := append([]CapabilityRequestV1(nil), requests...)
	for index := range cloned {
		cloned[index].ScopeConstraints = append([]string(nil), requests[index].ScopeConstraints...)
	}
	return cloned
}

func canonicalSHA256V1(value string) bool {
	if value != strings.TrimSpace(value) || len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
