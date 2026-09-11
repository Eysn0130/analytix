package authorityadvance

import (
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	MonotonicAdvanceIntentSchemaVersionV1     = 1
	MonotonicAdvanceIntentPurposeV1           = "analytix.monotonic-advance-intent/v1"
	MonotonicAdvanceSettlementSchemaVersionV1 = 1
	MonotonicAdvanceSettlementPurposeV1       = "analytix.monotonic-advance-settlement/v1"
	MonotonicAdvanceAlgorithmV1               = "Ed25519"

	maxMonotonicAdvanceIntentBytesV1     = 16 << 20
	maxMonotonicAdvanceSettlementBytesV1 = 32 << 20
	maxMonotonicAdvanceRangeStepsV1      = 32_768
)

// AdvanceRootV1 is the closed set of mutable logical roots covered by the two
// enrolled monotonic namespaces. The three evidence roots intentionally share
// one evidence-bundle witness head; there is no alias or catch-all root.
type AdvanceRootV1 string

const (
	AdvanceRootThreadRisk       AdvanceRootV1 = "thread_risk"
	AdvanceRootDatasetSnapshot  AdvanceRootV1 = "dataset_snapshot"
	AdvanceRootEvidenceRegistry AdvanceRootV1 = "evidence_registry"
	AdvanceRootPublication      AdvanceRootV1 = "publication"
)

func ValidateAdvanceRootV1(root AdvanceRootV1) error {
	switch root {
	case AdvanceRootThreadRisk, AdvanceRootDatasetSnapshot, AdvanceRootEvidenceRegistry, AdvanceRootPublication:
		return nil
	default:
		return errors.New("monotonic advance root is unknown")
	}
}

func NamespaceForAdvanceRootV1(root AdvanceRootV1) (string, error) {
	switch root {
	case AdvanceRootThreadRisk:
		return domainsecurity.ThreadRiskAuthorityNamespaceV1, nil
	case AdvanceRootDatasetSnapshot, AdvanceRootEvidenceRegistry, AdvanceRootPublication:
		return domainsecurity.EvidenceRegistryAuthorityNamespaceV1, nil
	default:
		return "", errors.New("monotonic advance root is unknown")
	}
}

// ThreadRiskTransitionBindingV1 retains both exact signed indexes. The intent
// therefore proves which concrete transition the request was meant to commit,
// rather than merely repeating an untrusted next-state digest.
type ThreadRiskTransitionBindingV1 struct {
	PreviousIndex domainsecurity.ThreadRiskAuthorityIndexV1 `json:"previousIndex"`
	NextIndex     domainsecurity.ThreadRiskAuthorityIndexV1 `json:"nextIndex"`
}

// EvidenceBundleTransitionBindingV1 retains the exact signed shared bundles.
// This is required to derive the one changed child root mechanically and to
// reject a dataset/evidence/publication root label that does not match it.
type EvidenceBundleTransitionBindingV1 struct {
	PreviousBundle domainevidence.EvidenceAuthorityBundleV1 `json:"previousBundle"`
	NextBundle     domainevidence.EvidenceAuthorityBundleV1 `json:"nextBundle"`
}

// AdvanceTransitionBindingV1 binds the witness state transition and the
// logical child root transition in one closed union. Exactly one branch is
// present and must agree with Root on the enclosing intent.
type AdvanceTransitionBindingV1 struct {
	PreviousStateDigest string `json:"previousStateDigest"`
	NextStateDigest     string `json:"nextStateDigest"`
	PreviousGeneration  uint64 `json:"previousGeneration"`
	NextGeneration      uint64 `json:"nextGeneration"`
	PreviousRootDigest  string `json:"previousRootDigest"`
	NextRootDigest      string `json:"nextRootDigest"`
	PreviousRootCount   uint64 `json:"previousRootCount"`
	NextRootCount       uint64 `json:"nextRootCount"`

	ThreadRisk     *ThreadRiskTransitionBindingV1     `json:"threadRisk,omitempty"`
	EvidenceBundle *EvidenceBundleTransitionBindingV1 `json:"evidenceBundle,omitempty"`
}

func NewThreadRiskTransitionBindingV1(
	previous, next domainsecurity.ThreadRiskAuthorityIndexV1,
) (AdvanceTransitionBindingV1, error) {
	binding := AdvanceTransitionBindingV1{
		PreviousStateDigest: previous.IndexDigest,
		NextStateDigest:     next.IndexDigest,
		PreviousGeneration:  previous.Generation,
		NextGeneration:      next.Generation,
		PreviousRootDigest:  previous.IndexDigest,
		NextRootDigest:      next.IndexDigest,
		PreviousRootCount:   uint64(len(previous.Entries)),
		NextRootCount:       uint64(len(next.Entries)),
		ThreadRisk: &ThreadRiskTransitionBindingV1{
			PreviousIndex: previous,
			NextIndex:     next,
		},
	}
	return binding, ValidateAdvanceTransitionBindingV1(AdvanceRootThreadRisk, binding)
}

// NewEvidenceTransitionBindingV1 derives the logical root from the only child
// pair that advances. Callers cannot choose the root independently.
func NewEvidenceTransitionBindingV1(
	previous, next domainevidence.EvidenceAuthorityBundleV1,
) (AdvanceRootV1, AdvanceTransitionBindingV1, error) {
	if err := domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, next); err != nil {
		return "", AdvanceTransitionBindingV1{}, err
	}
	root, previousRootDigest, nextRootDigest, previousRootCount, nextRootCount, err :=
		changedEvidenceRootV1(previous, next)
	if err != nil {
		return "", AdvanceTransitionBindingV1{}, err
	}
	binding := AdvanceTransitionBindingV1{
		PreviousStateDigest: previous.RecordDigest,
		NextStateDigest:     next.RecordDigest,
		PreviousGeneration:  previous.Generation,
		NextGeneration:      next.Generation,
		PreviousRootDigest:  previousRootDigest,
		NextRootDigest:      nextRootDigest,
		PreviousRootCount:   previousRootCount,
		NextRootCount:       nextRootCount,
		EvidenceBundle: &EvidenceBundleTransitionBindingV1{
			PreviousBundle: previous,
			NextBundle:     next,
		},
	}
	if err := ValidateAdvanceTransitionBindingV1(root, binding); err != nil {
		return "", AdvanceTransitionBindingV1{}, err
	}
	return root, binding, nil
}

func ValidateAdvanceTransitionBindingV1(root AdvanceRootV1, binding AdvanceTransitionBindingV1) error {
	if err := ValidateAdvanceRootV1(root); err != nil {
		return err
	}
	if binding.ThreadRisk != nil == (binding.EvidenceBundle != nil) {
		return errors.New("monotonic advance transition must select exactly one binding")
	}
	switch root {
	case AdvanceRootThreadRisk:
		if binding.ThreadRisk == nil || binding.EvidenceBundle != nil {
			return errors.New("thread risk advance has the wrong transition binding")
		}
		previous := binding.ThreadRisk.PreviousIndex
		next := binding.ThreadRisk.NextIndex
		if err := domainsecurity.ValidateThreadRiskAuthorityIndexTransitionV1(previous, next); err != nil {
			return err
		}
		if binding.PreviousStateDigest != previous.IndexDigest || binding.NextStateDigest != next.IndexDigest ||
			binding.PreviousGeneration != previous.Generation || binding.NextGeneration != next.Generation ||
			binding.PreviousRootDigest != previous.IndexDigest || binding.NextRootDigest != next.IndexDigest ||
			binding.PreviousRootCount != uint64(len(previous.Entries)) || binding.NextRootCount != uint64(len(next.Entries)) {
			return errors.New("thread risk advance transition summary mismatch")
		}
		return nil
	case AdvanceRootDatasetSnapshot, AdvanceRootEvidenceRegistry, AdvanceRootPublication:
		if binding.EvidenceBundle == nil || binding.ThreadRisk != nil {
			return errors.New("evidence advance has the wrong transition binding")
		}
		previous := binding.EvidenceBundle.PreviousBundle
		next := binding.EvidenceBundle.NextBundle
		if err := domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, next); err != nil {
			return err
		}
		derivedRoot, previousRootDigest, nextRootDigest, previousRootCount, nextRootCount, err :=
			changedEvidenceRootV1(previous, next)
		if err != nil {
			return err
		}
		if root != derivedRoot || binding.PreviousStateDigest != previous.RecordDigest ||
			binding.NextStateDigest != next.RecordDigest || binding.PreviousGeneration != previous.Generation ||
			binding.NextGeneration != next.Generation || binding.PreviousRootDigest != previousRootDigest ||
			binding.NextRootDigest != nextRootDigest || binding.PreviousRootCount != previousRootCount ||
			binding.NextRootCount != nextRootCount {
			return errors.New("evidence advance transition summary or root mismatch")
		}
		return nil
	default:
		return errors.New("monotonic advance root is unknown")
	}
}

func changedEvidenceRootV1(
	previous, next domainevidence.EvidenceAuthorityBundleV1,
) (AdvanceRootV1, string, string, uint64, uint64, error) {
	type candidate struct {
		root           AdvanceRootV1
		previousDigest string
		nextDigest     string
		previousCount  uint64
		nextCount      uint64
	}
	candidates := [...]candidate{
		{AdvanceRootDatasetSnapshot, previous.DatasetSnapshotIndexDigest, next.DatasetSnapshotIndexDigest, previous.DatasetSnapshotCount, next.DatasetSnapshotCount},
		{AdvanceRootEvidenceRegistry, previous.EvidenceRegistryIndexDigest, next.EvidenceRegistryIndexDigest, previous.EvidenceRegistryCount, next.EvidenceRegistryCount},
		{AdvanceRootPublication, previous.PublicationIndexDigest, next.PublicationIndexDigest, previous.PublicationCount, next.PublicationCount},
	}
	var changed *candidate
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.previousDigest == candidate.nextDigest && candidate.previousCount == candidate.nextCount {
			continue
		}
		if changed != nil {
			return "", "", "", 0, 0, errors.New("evidence advance changes more than one logical root")
		}
		changed = candidate
	}
	if changed == nil {
		return "", "", "", 0, 0, errors.New("evidence advance changes no logical root")
	}
	return changed.root, changed.previousDigest, changed.nextDigest, changed.previousCount, changed.nextCount, nil
}
