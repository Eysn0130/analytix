package authorityadvance

import (
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	MonotonicAdvanceIntentSchemaVersionV2     = 2
	MonotonicAdvanceIntentPurposeV2           = "analytix.monotonic-advance-intent/v2"
	MonotonicAdvanceSettlementSchemaVersionV2 = 2
	MonotonicAdvanceSettlementPurposeV2       = "analytix.monotonic-advance-settlement/v2"
	MonotonicAdvanceAlgorithmV2               = "Ed25519"

	// MaxMonotonicAdvanceJournalRecordBytesV2 is the one shared domain and
	// immutable-store bound. A domain-valid V2 record is therefore persistable
	// by every conforming V2 journal adapter.
	MaxMonotonicAdvanceJournalRecordBytesV2  = 16 << 20
	MaxMonotonicAdvanceRangeStepsV2          = 16_384
	MaxMonotonicAdvanceCommittedRangeBytesV2 = 64 << 20
)

// AdvanceRootV2 is a new closed wire enum. V1 remains frozen and cannot encode
// enrollment-to-generation-one genesis without changing its signed grammar.
type AdvanceRootV2 string

const (
	AdvanceRootThreadRiskV2       AdvanceRootV2 = "thread_risk"
	AdvanceRootEvidenceGenesisV2  AdvanceRootV2 = "evidence_authority_genesis"
	AdvanceRootDatasetSnapshotV2  AdvanceRootV2 = "dataset_snapshot"
	AdvanceRootEvidenceRegistryV2 AdvanceRootV2 = "evidence_registry"
	AdvanceRootPublicationV2      AdvanceRootV2 = "publication"
)

func ValidateAdvanceRootV2(root AdvanceRootV2) error {
	switch root {
	case AdvanceRootThreadRiskV2, AdvanceRootEvidenceGenesisV2, AdvanceRootDatasetSnapshotV2,
		AdvanceRootEvidenceRegistryV2, AdvanceRootPublicationV2:
		return nil
	default:
		return errors.New("monotonic advance V2 root is unknown")
	}
}

func NamespaceForAdvanceRootV2(root AdvanceRootV2) (string, error) {
	switch root {
	case AdvanceRootThreadRiskV2:
		return domainsecurity.ThreadRiskAuthorityNamespaceV1, nil
	case AdvanceRootEvidenceGenesisV2, AdvanceRootDatasetSnapshotV2,
		AdvanceRootEvidenceRegistryV2, AdvanceRootPublicationV2:
		return domainsecurity.EvidenceRegistryAuthorityNamespaceV1, nil
	default:
		return "", errors.New("monotonic advance V2 root is unknown")
	}
}

type ThreadRiskGenesisBindingV2 struct {
	EnrollmentCheckpoint domainsecurity.MonotonicHeadCheckpointV1  `json:"enrollmentCheckpoint"`
	FirstIndex           domainsecurity.ThreadRiskAuthorityIndexV1 `json:"firstIndex"`
}

type EvidenceAuthorityGenesisBindingV2 struct {
	EnrollmentCheckpoint domainsecurity.MonotonicHeadCheckpointV1 `json:"enrollmentCheckpoint"`
	FirstBundle          domainevidence.EvidenceAuthorityBundleV1 `json:"firstBundle"`
}

// AdvanceTransitionBindingV2 is a four-branch closed union. The successor
// branches reuse frozen signed index/bundle values, not the V1 intent grammar.
type AdvanceTransitionBindingV2 struct {
	PreviousStateDigest string `json:"previousStateDigest"`
	NextStateDigest     string `json:"nextStateDigest"`
	PreviousGeneration  uint64 `json:"previousGeneration"`
	NextGeneration      uint64 `json:"nextGeneration"`
	PreviousRootDigest  string `json:"previousRootDigest"`
	NextRootDigest      string `json:"nextRootDigest"`
	PreviousRootCount   uint64 `json:"previousRootCount"`
	NextRootCount       uint64 `json:"nextRootCount"`

	ThreadRiskGenesis *ThreadRiskGenesisBindingV2        `json:"threadRiskGenesis,omitempty"`
	EvidenceGenesis   *EvidenceAuthorityGenesisBindingV2 `json:"evidenceGenesis,omitempty"`
	ThreadRisk        *ThreadRiskTransitionBindingV1     `json:"threadRisk,omitempty"`
	EvidenceBundle    *EvidenceBundleTransitionBindingV1 `json:"evidenceBundle,omitempty"`
}

func NewThreadRiskGenesisTransitionBindingV2(
	enrollment domainsecurity.MonotonicHeadCheckpointV1,
	first domainsecurity.ThreadRiskAuthorityIndexV1,
) (AdvanceTransitionBindingV2, error) {
	binding := AdvanceTransitionBindingV2{
		PreviousStateDigest: enrollment.CurrentStateDigest,
		NextStateDigest:     first.IndexDigest,
		PreviousGeneration:  enrollment.Generation,
		NextGeneration:      first.Generation,
		PreviousRootDigest:  "",
		NextRootDigest:      first.IndexDigest,
		PreviousRootCount:   0,
		NextRootCount:       uint64(len(first.Entries)),
		ThreadRiskGenesis: &ThreadRiskGenesisBindingV2{
			EnrollmentCheckpoint: enrollment,
			FirstIndex:           first,
		},
	}
	return binding, ValidateAdvanceTransitionBindingV2(AdvanceRootThreadRiskV2, binding)
}

func NewEvidenceAuthorityGenesisTransitionBindingV2(
	enrollment domainsecurity.MonotonicHeadCheckpointV1,
	first domainevidence.EvidenceAuthorityBundleV1,
) (AdvanceTransitionBindingV2, error) {
	binding := AdvanceTransitionBindingV2{
		PreviousStateDigest: enrollment.CurrentStateDigest,
		NextStateDigest:     first.RecordDigest,
		PreviousGeneration:  enrollment.Generation,
		NextGeneration:      first.Generation,
		PreviousRootDigest:  "",
		NextRootDigest:      first.RecordDigest,
		PreviousRootCount:   0,
		NextRootCount:       0,
		EvidenceGenesis: &EvidenceAuthorityGenesisBindingV2{
			EnrollmentCheckpoint: enrollment,
			FirstBundle:          first,
		},
	}
	return binding, ValidateAdvanceTransitionBindingV2(AdvanceRootEvidenceGenesisV2, binding)
}

func NewThreadRiskTransitionBindingV2(
	previous, next domainsecurity.ThreadRiskAuthorityIndexV1,
) (AdvanceTransitionBindingV2, error) {
	binding := AdvanceTransitionBindingV2{
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
	return binding, ValidateAdvanceTransitionBindingV2(AdvanceRootThreadRiskV2, binding)
}

func NewEvidenceTransitionBindingV2(
	previous, next domainevidence.EvidenceAuthorityBundleV1,
) (AdvanceRootV2, AdvanceTransitionBindingV2, error) {
	if err := domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, next); err != nil {
		return "", AdvanceTransitionBindingV2{}, err
	}
	rootV1, previousDigest, nextDigest, previousCount, nextCount, err := changedEvidenceRootV1(previous, next)
	if err != nil {
		return "", AdvanceTransitionBindingV2{}, err
	}
	root := AdvanceRootV2(rootV1)
	binding := AdvanceTransitionBindingV2{
		PreviousStateDigest: previous.RecordDigest,
		NextStateDigest:     next.RecordDigest,
		PreviousGeneration:  previous.Generation,
		NextGeneration:      next.Generation,
		PreviousRootDigest:  previousDigest,
		NextRootDigest:      nextDigest,
		PreviousRootCount:   previousCount,
		NextRootCount:       nextCount,
		EvidenceBundle: &EvidenceBundleTransitionBindingV1{
			PreviousBundle: previous,
			NextBundle:     next,
		},
	}
	return root, binding, ValidateAdvanceTransitionBindingV2(root, binding)
}

func ValidateAdvanceTransitionBindingV2(root AdvanceRootV2, binding AdvanceTransitionBindingV2) error {
	if err := ValidateAdvanceRootV2(root); err != nil {
		return err
	}
	branches := 0
	for _, present := range []bool{
		binding.ThreadRiskGenesis != nil,
		binding.EvidenceGenesis != nil,
		binding.ThreadRisk != nil,
		binding.EvidenceBundle != nil,
	} {
		if present {
			branches++
		}
	}
	if branches != 1 {
		return errors.New("monotonic advance V2 transition must select exactly one binding")
	}
	switch root {
	case AdvanceRootThreadRiskV2:
		if binding.ThreadRiskGenesis != nil {
			genesis := binding.ThreadRiskGenesis
			enrollment := genesis.EnrollmentCheckpoint
			first := genesis.FirstIndex
			if domainsecurity.ValidateMonotonicHeadCheckpointV1(enrollment) != nil ||
				domainsecurity.ValidateThreadRiskAuthorityIndexV1(first) != nil || enrollment.Generation != 0 ||
				enrollment.Namespace != domainsecurity.ThreadRiskAuthorityNamespaceV1 || first.Generation != 1 ||
				first.PreviousIndexDigest != "" || first.InstallationID != enrollment.InstallationID ||
				first.EnrollmentID != enrollment.EnrollmentID || first.Namespace != enrollment.Namespace ||
				binding.PreviousStateDigest != enrollment.CurrentStateDigest || binding.NextStateDigest != first.IndexDigest ||
				binding.PreviousGeneration != 0 || binding.NextGeneration != 1 || binding.PreviousRootDigest != "" ||
				binding.NextRootDigest != first.IndexDigest || binding.PreviousRootCount != 0 ||
				binding.NextRootCount != uint64(len(first.Entries)) {
				return errors.New("thread risk genesis V2 transition is invalid")
			}
			return nil
		}
		if binding.ThreadRisk == nil {
			return errors.New("thread risk advance V2 has the wrong transition binding")
		}
		previous := binding.ThreadRisk.PreviousIndex
		next := binding.ThreadRisk.NextIndex
		if domainsecurity.ValidateThreadRiskAuthorityIndexTransitionV1(previous, next) != nil ||
			binding.PreviousStateDigest != previous.IndexDigest || binding.NextStateDigest != next.IndexDigest ||
			binding.PreviousGeneration != previous.Generation || binding.NextGeneration != next.Generation ||
			binding.PreviousRootDigest != previous.IndexDigest || binding.NextRootDigest != next.IndexDigest ||
			binding.PreviousRootCount != uint64(len(previous.Entries)) || binding.NextRootCount != uint64(len(next.Entries)) {
			return errors.New("thread risk advance V2 transition summary mismatch")
		}
		return nil
	case AdvanceRootEvidenceGenesisV2:
		if binding.EvidenceGenesis == nil {
			return errors.New("evidence authority genesis V2 has the wrong transition binding")
		}
		genesis := binding.EvidenceGenesis
		enrollment := genesis.EnrollmentCheckpoint
		first := genesis.FirstBundle
		if domainsecurity.ValidateMonotonicHeadCheckpointV1(enrollment) != nil ||
			domainevidence.ValidateEvidenceAuthorityBundleV1(first) != nil || enrollment.Generation != 0 ||
			enrollment.Namespace != domainsecurity.EvidenceRegistryAuthorityNamespaceV1 || first.Generation != 1 ||
			first.PreviousBundleDigest != "" || first.InstallationID != enrollment.InstallationID ||
			first.EnrollmentID != enrollment.EnrollmentID || first.Namespace != enrollment.Namespace ||
			first.DatasetSnapshotIndexDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() ||
			first.EvidenceRegistryIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() ||
			first.PublicationIndexDigest != domainpublication.PublicationIndexGenesisDigestV1() ||
			first.DatasetSnapshotCount != 0 || first.EvidenceRegistryCount != 0 || first.PublicationCount != 0 ||
			binding.PreviousStateDigest != enrollment.CurrentStateDigest || binding.NextStateDigest != first.RecordDigest ||
			binding.PreviousGeneration != 0 || binding.NextGeneration != 1 || binding.PreviousRootDigest != "" ||
			binding.NextRootDigest != first.RecordDigest || binding.PreviousRootCount != 0 || binding.NextRootCount != 0 {
			return errors.New("evidence authority genesis V2 transition is invalid")
		}
		return nil
	case AdvanceRootDatasetSnapshotV2, AdvanceRootEvidenceRegistryV2, AdvanceRootPublicationV2:
		if binding.EvidenceBundle == nil {
			return errors.New("evidence advance V2 has the wrong transition binding")
		}
		previous := binding.EvidenceBundle.PreviousBundle
		next := binding.EvidenceBundle.NextBundle
		if err := domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, next); err != nil {
			return errors.New("evidence advance V2 bundle transition is invalid")
		}
		rootV1, previousDigest, nextDigest, previousCount, nextCount, err := changedEvidenceRootV1(previous, next)
		if err != nil || root != AdvanceRootV2(rootV1) || binding.PreviousStateDigest != previous.RecordDigest ||
			binding.NextStateDigest != next.RecordDigest || binding.PreviousGeneration != previous.Generation ||
			binding.NextGeneration != next.Generation || binding.PreviousRootDigest != previousDigest ||
			binding.NextRootDigest != nextDigest || binding.PreviousRootCount != previousCount ||
			binding.NextRootCount != nextCount {
			return errors.New("evidence advance V2 transition summary or root mismatch")
		}
		return nil
	default:
		return errors.New("monotonic advance V2 root is unknown")
	}
}
