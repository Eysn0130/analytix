package datasetsnapshot

import (
	"encoding/json"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var currentSelectionDigestDomainV2 = []byte("analytix.current-dataset-snapshot-selection/v2\x00")
var currentSelectionContentDigestDomainV2 = []byte("analytix.current-dataset-snapshot-selection-content/v2\x00")

// CanonicalCurrentSelectionContentDigestV2 binds the entire durable authority
// bundle and selection graph, including registry/publication generations. Only
// the fresh witness exchange and self digest are normalized away. This is
// comparison material; callers must still validate the full current digest and
// use its active capability for every effect.
func CanonicalCurrentSelectionContentDigestV2(selection CurrentSelectionV2) (string, error) {
	selection.Head.Request = domainsecurity.MonotonicHeadObserveRequestV1{}
	selection.Head.Observation = domainsecurity.MonotonicHeadObservationV1{}
	selection.SelectionDigest = ""
	// Bypass the snapshot's historical wire-version projection here: stable
	// content binds both producer fields, including an unexpected unused value.
	type snapshotContent ResolvedSnapshotV2
	body, err := json.Marshal(struct {
		CurrentSelectionV2
		Snapshot snapshotContent
	}{selection, snapshotContent(selection.Snapshot)})
	if err != nil {
		return "", errors.New("current dataset snapshot content cannot be frozen")
	}
	return domainsecurity.SHA256Hex(append(
		append([]byte(nil), currentSelectionContentDigestDomainV2...), body...,
	)), nil
}

// CanonicalCurrentSelectionDigestV2 computes the one canonical digest for the
// complete witnessed DSV2 selection graph. SelectionDigest is cleared before
// encoding so callers cannot make a self-referential or syntax-only claim.
func CanonicalCurrentSelectionDigestV2(selection CurrentSelectionV2) (string, error) {
	selection.SelectionDigest = ""
	body, err := json.Marshal(selection)
	if err != nil {
		return "", errors.New("current dataset snapshot selection cannot be frozen")
	}
	return domainsecurity.SHA256Hex(append(
		append([]byte(nil), currentSelectionDigestDomainV2...),
		body...,
	)), nil
}

func ValidateCurrentSelectionDigestV2(selection CurrentSelectionV2) error {
	expected, err := CanonicalCurrentSelectionDigestV2(selection)
	if err != nil || !domainsecurity.IsSHA256Hex(selection.SelectionDigest) ||
		selection.SelectionDigest != expected {
		return errors.New("current dataset snapshot selection digest is invalid")
	}
	return nil
}
