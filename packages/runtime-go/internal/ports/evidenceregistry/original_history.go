package evidenceregistry

import domainevidence "analytix.local/runtime-go/internal/domain/evidence"

// OriginalHistoryV2 is a complete original graph, authenticated by its
// producer against physical inventory, independent enrollment, current
// installation key and the full frozen primary context denominator. It has
// no selected head or membership authority. Every unselected local candidate,
// sibling and standalone capsule remains part of the observation.
type OriginalHistoryV2 struct {
	Indexes  map[string]domainevidence.EvidenceRegistryAuthorityIndexV2
	Capsules map[string]domainevidence.EvidenceRegistryAuthorityCapsule
}

// UnavailableOriginalInventoryV1 identifies a complete frozen raw owner whose
// domain graph is unusable. Its producer separately proves physical topology,
// Core, journal and independent installation health. It contains no records
// authorized for use, and does not mean that the owner is empty.
type UnavailableOriginalInventoryV1 struct {
	InventoryDigest string
}

// OriginalObservationV1 explicitly distinguishes unavailable raw inventory
// from valid empty history. Unavailable excludes both other alternatives.
type OriginalObservationV1 struct {
	Legacy      []InventoryRecord
	HistoryV2   *OriginalHistoryV2
	Unavailable *UnavailableOriginalInventoryV1
}
