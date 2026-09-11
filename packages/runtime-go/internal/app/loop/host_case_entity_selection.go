package loop

import (
	"encoding/json"
	"errors"
	"sort"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const hostCaseEntitySelectionPurposeV1 = "analytix.host-case-entity-selection/v1"

// HostCaseEntitySelectionV1 is process-private provenance for the exact set of
// case entities admitted from one persisted user ingress. Its fields are
// deliberately opaque: provider output, model tool arguments, and public
// callers cannot construct a wider selection by copying identifiers into a
// stage_case_report request.
type HostCaseEntitySelectionV1 struct {
	securityContext     domainsecurity.TurnSecurityContext
	ingressRecordID     string
	ingressRecordDigest string
	state               *hostCaseEntitySelectionStateV1
	selectionDigest     string
}

type hostCaseEntitySelectionStateV1 struct {
	references []domaincaseentity.ReferenceV1
	aliases    []domaincaseentity.ModelEntityAliasV1
}

// HostCaseEntitySelectionViewV1 is exposed only inside UseExactV1's synchronous
// host callback. It contains provider-safe stable references and hash-only
// ingress provenance, never source-exact account values.
type HostCaseEntitySelectionViewV1 struct {
	IngressRecordID     string
	IngressRecordDigest string
	EntityReferences    []domaincaseentity.ReferenceV1
	EntityAliases       []domaincaseentity.ModelEntityAliasV1
	SelectionDigest     string
}

// NewHostCaseEntitySelectionFromPersistedIngressV1 must be called only with
// the private record reference and entity references returned by the same
// successful caseentity ingress compilation. The constructor is internal to
// the Go runtime and is not a public/model/tool-input surface.
func NewHostCaseEntitySelectionFromPersistedIngressV1(
	securityContext domainsecurity.TurnSecurityContext,
	record caseentityapp.PrivateRecordReferenceV1,
	references []domaincaseentity.ReferenceV1,
) (HostCaseEntitySelectionV1, error) {
	return newHostCaseEntitySelectionV1(securityContext, record, references, nil)
}

func NewHostCaseEntitySelectionFromPersistedIngressAliasesV1(
	securityContext domainsecurity.TurnSecurityContext,
	record caseentityapp.PrivateRecordReferenceV1,
	references []domaincaseentity.ReferenceV1,
	aliases []domaincaseentity.ModelEntityAliasV1,
) (HostCaseEntitySelectionV1, error) {
	return newHostCaseEntitySelectionV1(securityContext, record, references, aliases)
}

func newHostCaseEntitySelectionV1(
	securityContext domainsecurity.TurnSecurityContext,
	record caseentityapp.PrivateRecordReferenceV1,
	references []domaincaseentity.ReferenceV1,
	aliases []domaincaseentity.ModelEntityAliasV1,
) (HostCaseEntitySelectionV1, error) {
	canonical, err := canonicalHostCaseEntityReferencesV1(references)
	canonicalAliases, aliasErr := canonicalHostCaseEntityAliasesV1(aliases)
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!domainsecurity.IsSHA256Hex(record.RecordID) ||
		!domainsecurity.IsSHA256Hex(record.RecordDigest) || err != nil || aliasErr != nil ||
		(len(canonicalAliases) != 0 && len(canonicalAliases) != len(canonical)) {
		return HostCaseEntitySelectionV1{}, errors.New("host case entity selection provenance is invalid")
	}
	selection := HostCaseEntitySelectionV1{
		securityContext: securityContext,
		ingressRecordID: record.RecordID, ingressRecordDigest: record.RecordDigest,
		state: &hostCaseEntitySelectionStateV1{
			references: canonical,
			aliases:    canonicalAliases,
		},
	}
	selection.selectionDigest = hostCaseEntitySelectionDigestV1(selection)
	if !selection.validV1() {
		return HostCaseEntitySelectionV1{}, errors.New("host case entity selection is invalid")
	}
	return selection, nil
}

func (selection HostCaseEntitySelectionV1) AvailableV1() bool {
	return selection.validV1()
}

// UseExactV1 rejects a copied selection under another case, turn, epoch, or
// snapshot before exposing its bounded stable-reference allowlist.
func (selection HostCaseEntitySelectionV1) UseExactV1(
	securityContext domainsecurity.TurnSecurityContext,
	use func(HostCaseEntitySelectionViewV1) error,
) error {
	if use == nil || securityContext != selection.securityContext || !selection.validV1() {
		return errors.New("host case entity selection is unavailable")
	}
	return use(HostCaseEntitySelectionViewV1{
		IngressRecordID:     selection.ingressRecordID,
		IngressRecordDigest: selection.ingressRecordDigest,
		EntityReferences: append(
			[]domaincaseentity.ReferenceV1(nil),
			selection.referencesV1()...,
		),
		EntityAliases:   append([]domaincaseentity.ModelEntityAliasV1(nil), selection.aliasesV1()...),
		SelectionDigest: selection.selectionDigest,
	})
}

func (selection HostCaseEntitySelectionV1) AllowsModelAliasV1(
	securityContext domainsecurity.TurnSecurityContext,
	alias domaincaseentity.ModelEntityAliasV1,
) bool {
	if securityContext != selection.securityContext || !selection.validV1() ||
		domaincaseentity.ValidateModelEntityAliasV1(string(alias)) != nil {
		return false
	}
	aliases := selection.aliasesV1()
	index := sort.Search(len(aliases), func(index int) bool { return aliases[index] >= alias })
	return index < len(aliases) && aliases[index] == alias
}

func (selection HostCaseEntitySelectionV1) validV1() bool {
	references := selection.referencesV1()
	canonical, err := canonicalHostCaseEntityReferencesV1(references)
	aliases := selection.aliasesV1()
	canonicalAliases, aliasErr := canonicalHostCaseEntityAliasesV1(aliases)
	return domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(selection.securityContext) == nil &&
		domainsecurity.IsSHA256Hex(selection.ingressRecordID) &&
		domainsecurity.IsSHA256Hex(selection.ingressRecordDigest) && err == nil &&
		sameHostCaseEntityReferencesV1(canonical, references) && aliasErr == nil &&
		(len(aliases) == 0 || len(aliases) == len(references)) &&
		sameHostCaseEntityAliasesV1(canonicalAliases, aliases) &&
		domainsecurity.IsSHA256Hex(selection.selectionDigest) &&
		selection.selectionDigest == hostCaseEntitySelectionDigestV1(selection)
}

func (selection HostCaseEntitySelectionV1) referencesV1() []domaincaseentity.ReferenceV1 {
	if selection.state == nil {
		return nil
	}
	return selection.state.references
}

func (selection HostCaseEntitySelectionV1) aliasesV1() []domaincaseentity.ModelEntityAliasV1 {
	if selection.state == nil {
		return nil
	}
	return selection.state.aliases
}

func canonicalHostCaseEntityAliasesV1(
	aliases []domaincaseentity.ModelEntityAliasV1,
) ([]domaincaseentity.ModelEntityAliasV1, error) {
	canonical := append([]domaincaseentity.ModelEntityAliasV1(nil), aliases...)
	sort.Slice(canonical, func(left, right int) bool { return canonical[left] < canonical[right] })
	for index, alias := range canonical {
		if domaincaseentity.ValidateModelEntityAliasV1(string(alias)) != nil ||
			index > 0 && alias == canonical[index-1] {
			return nil, errors.New("host case entity selection contains an invalid or duplicate alias")
		}
	}
	return canonical, nil
}

func canonicalHostCaseEntityReferencesV1(
	references []domaincaseentity.ReferenceV1,
) ([]domaincaseentity.ReferenceV1, error) {
	if len(references) == 0 || len(references) > 64 {
		return nil, errors.New("host case entity selection is empty or unbounded")
	}
	canonical := append([]domaincaseentity.ReferenceV1(nil), references...)
	sort.Slice(canonical, func(left, right int) bool { return canonical[left] < canonical[right] })
	for index, reference := range canonical {
		if domaincaseentity.ValidateReferenceV1(string(reference)) != nil ||
			index > 0 && reference == canonical[index-1] {
			return nil, errors.New("host case entity selection contains an invalid or duplicate reference")
		}
	}
	return canonical, nil
}

func sameHostCaseEntityReferencesV1(
	left, right []domaincaseentity.ReferenceV1,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameHostCaseEntityAliasesV1(
	left, right []domaincaseentity.ModelEntityAliasV1,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameHostCaseEntitySelectionV1(
	left, right HostCaseEntitySelectionV1,
) bool {
	leftAvailable, rightAvailable := left.validV1(), right.validV1()
	if !leftAvailable || !rightAvailable {
		return !leftAvailable && !rightAvailable
	}
	return left.securityContext == right.securityContext &&
		left.ingressRecordID == right.ingressRecordID &&
		left.ingressRecordDigest == right.ingressRecordDigest &&
		left.selectionDigest == right.selectionDigest &&
		sameHostCaseEntityReferencesV1(left.referencesV1(), right.referencesV1()) &&
		sameHostCaseEntityAliasesV1(left.aliasesV1(), right.aliasesV1())
}

func hostCaseEntitySelectionDigestV1(selection HostCaseEntitySelectionV1) string {
	body, err := json.Marshal(struct {
		Purpose             string                                `json:"purpose"`
		TenantID            string                                `json:"tenantId"`
		UserID              string                                `json:"userId"`
		CaseID              string                                `json:"caseId"`
		CaseBindingHash     string                                `json:"caseBindingHash"`
		ThreadID            string                                `json:"threadId"`
		TurnID              string                                `json:"turnId"`
		ContextDigest       string                                `json:"contextDigest"`
		ContextEpoch        uint64                                `json:"contextEpoch"`
		DatasetSnapshotID   string                                `json:"datasetSnapshotId"`
		SourceManifestHash  string                                `json:"sourceManifestHash"`
		IngressRecordID     string                                `json:"ingressRecordId"`
		IngressRecordDigest string                                `json:"ingressRecordDigest"`
		EntityReferences    []domaincaseentity.ReferenceV1        `json:"entityReferences"`
		EntityAliases       []domaincaseentity.ModelEntityAliasV1 `json:"entityAliases"`
	}{
		Purpose:             hostCaseEntitySelectionPurposeV1,
		TenantID:            selection.securityContext.TenantID,
		UserID:              selection.securityContext.UserID,
		CaseID:              selection.securityContext.CaseID,
		CaseBindingHash:     selection.securityContext.CaseBindingHash,
		ThreadID:            selection.securityContext.ThreadID,
		TurnID:              selection.securityContext.TurnID,
		ContextDigest:       selection.securityContext.ContextDigest,
		ContextEpoch:        selection.securityContext.ContextEpoch,
		DatasetSnapshotID:   selection.securityContext.DatasetSnapshotID,
		SourceManifestHash:  selection.securityContext.SourceManifestHash,
		IngressRecordID:     selection.ingressRecordID,
		IngressRecordDigest: selection.ingressRecordDigest,
		EntityReferences: append(
			[]domaincaseentity.ReferenceV1(nil),
			selection.referencesV1()...,
		),
		EntityAliases: append([]domaincaseentity.ModelEntityAliasV1(nil), selection.aliasesV1()...),
	})
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(append(
		[]byte("analytix.host-case-entity-selection/digest/v1\x00"), body...,
	))
}

func (HostCaseEntitySelectionV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("host case entity selection does not support ordinary JSON serialization")
}

func (*HostCaseEntitySelectionV1) UnmarshalJSON([]byte) error {
	return errors.New("host case entity selection does not support ordinary JSON deserialization")
}

func (HostCaseEntitySelectionV1) String() string {
	return "HostCaseEntitySelectionV1{private:[REDACTED]}"
}

func (selection HostCaseEntitySelectionV1) GoString() string {
	return selection.String()
}
