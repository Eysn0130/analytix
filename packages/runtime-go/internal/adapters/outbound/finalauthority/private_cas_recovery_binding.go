package finalauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"path/filepath"
	"sort"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const SecurePrivateCASRecoveryAlgorithmV4 = "analytix.private-cas-recovery/v4"

// SecurePrivateCASRecoveryRootBindingV4 maps one in-memory prepared CAS plan
// to its public logical catalog id. RootPath is checked against the prepared
// handle-bound plan but is never persisted; only its domain-separated digest
// contributes to the signed authority set.
type SecurePrivateCASRecoveryRootBindingV4 struct {
	RootID   string
	RootPath string
}

// PreparedSecurePrivateCASRecoveryParticipantV4 preserves the semantic owner
// boundary which validated a set of CAS leaves. SemanticAuthorityDigest must
// bind the owner validator version and any non-CAS state whose interpretation
// is required before residue cleanup.
type PreparedSecurePrivateCASRecoveryParticipantV4 struct {
	ParticipantID           string
	SemanticAuthorityDigest string
	Authority               PreparedSecurePrivateCASRecoveryAuthorityV3
	Roots                   []SecurePrivateCASRecoveryRootBindingV4
}

// SecurePrivateCASRecoveryTargetFreezeAuthorityV4 marks an already-bound
// optional capability whose physical CAS inventory must remain immutable while
// its semantics are unavailable. Its plans still participate in the complete
// root/topology manifest and revalidation barrier, but no plain residue may
// become a recovery target. Signed/staged transaction conflicts are validated
// before this marker is honored and remain fatal.
type SecurePrivateCASRecoveryTargetFreezeAuthorityV4 interface {
	FreezeSecurePrivateCASRecoveryTargetsV4() bool
}

type privateCASRecoveryTargetObservationV4 struct {
	entry         domainprivatecas.RecoveryTargetEntryV1
	phase         privateCASRecoveryQuarantinePhase
	transactionID string
}

type privateCASRecoveryPlanBindingV4 struct {
	rootID                string
	rootPath              string
	planAuthorityDigest   string
	targets               []privateCASRecoveryTargetObservationV4
	recoveryTargetsFrozen bool
}

type privateCASRecoveryParticipantBindingV4 struct {
	participantID           string
	semanticAuthorityDigest string
	authority               PreparedSecurePrivateCASRecoveryAuthorityV3
	plans                   []privateCASRecoveryPlanBindingV4
	topologyRoots           []string
	topologyDigests         []string
	recoveryTargetsFrozen   bool
}

type preparedSecurePrivateCASRecoveryAuthoritySetV4 struct {
	v3                 *preparedSecurePrivateCASRecoveryAuthoritySetV3
	participants       []privateCASRecoveryParticipantBindingV4
	plans              []privateCASRecoveryPlanBindingV4
	authoritySetDigest string
}

func prepareSecurePrivateCASRecoveryAuthoritySetV4(
	ctx context.Context,
	participants []PreparedSecurePrivateCASRecoveryParticipantV4,
) (*preparedSecurePrivateCASRecoveryAuthoritySetV4, error) {
	if len(participants) > domainprivatecas.MaxRecoveryJournalParticipantsV1 {
		return nil, errors.New("private CAS recovery V4 participant manifest is outside its bound")
	}
	if len(participants) == 0 {
		prepared := &preparedSecurePrivateCASRecoveryAuthoritySetV4{
			participants:       []privateCASRecoveryParticipantBindingV4{},
			plans:              []privateCASRecoveryPlanBindingV4{},
			authoritySetDigest: privateCASRecoveryAuthoritySetDigestV4(nil),
		}
		if err := prepared.revalidate(ctx); err != nil {
			return nil, err
		}
		return prepared, nil
	}
	authorities := make([]PreparedSecurePrivateCASRecoveryAuthorityV3, 0, len(participants))
	for _, participant := range participants {
		authorities = append(authorities, participant.Authority)
	}
	v3, err := prepareSecurePrivateCASRecoveryAuthoritySetV3(ctx, authorities)
	if err != nil {
		return nil, err
	}
	prepared := &preparedSecurePrivateCASRecoveryAuthoritySetV4{
		v3:           v3,
		participants: make([]privateCASRecoveryParticipantBindingV4, 0, len(participants)),
		plans:        make([]privateCASRecoveryPlanBindingV4, 0, len(v3.plans)),
	}
	seenParticipants := make(map[string]struct{}, len(participants))
	seenRootIDs := make(map[string]struct{}, len(v3.plans))
	globalPlanIndex := 0
	for participantIndex, input := range participants {
		participantID := strings.TrimSpace(input.ParticipantID)
		if !validPrivateCASRecoveryParticipantIDV4(participantID) || participantID != input.ParticipantID ||
			!domainprivatecas.ValidDigestV1(input.SemanticAuthorityDigest) {
			return nil, errors.New("private CAS recovery V4 participant authority is invalid")
		}
		foldedParticipant := strings.ToLower(participantID)
		if _, duplicate := seenParticipants[foldedParticipant]; duplicate {
			return nil, errors.New("private CAS recovery V4 repeats a participant")
		}
		seenParticipants[foldedParticipant] = struct{}{}
		v3Participant := v3.participants[participantIndex]
		if input.Authority != v3Participant.authority || len(input.Roots) != len(v3Participant.plans) {
			return nil, errors.New("private CAS recovery V4 participant root manifest is incomplete")
		}
		binding := privateCASRecoveryParticipantBindingV4{
			participantID: participantID, semanticAuthorityDigest: input.SemanticAuthorityDigest,
			authority: input.Authority, plans: make([]privateCASRecoveryPlanBindingV4, 0, len(input.Roots)),
			topologyRoots:         append([]string(nil), v3Participant.topologyRoots...),
			topologyDigests:       make([]string, 0, len(v3Participant.topologies)),
			recoveryTargetsFrozen: securePrivateCASRecoveryTargetsFrozenV4(input.Authority),
		}
		for topologyIndex, topologyV3 := range v3Participant.topologies {
			topologyV4, ok := topologyV3.(SecurePrivateCASRecoveryTopologyAuthorityV4)
			if !ok {
				return nil, errors.New("private CAS recovery V4 topology has no durable authority digest")
			}
			digest := topologyV4.PrivateCASRecoveryTopologyDigestV4()
			if !domainprivatecas.ValidDigestV1(digest) ||
				topologyV4.PrivateCASRecoveryTopologyRootV3() != binding.topologyRoots[topologyIndex] {
				return nil, errors.New("private CAS recovery V4 topology authority is invalid")
			}
			binding.topologyDigests = append(binding.topologyDigests, digest)
		}
		for localIndex, root := range input.Roots {
			if globalPlanIndex >= domainprivatecas.MaxRecoveryJournalPlansV1 ||
				root.RootID != strings.TrimSpace(root.RootID) || root.RootPath != strings.TrimSpace(root.RootPath) ||
				root.RootPath == "" || filepath.Clean(root.RootPath) != root.RootPath {
				return nil, errors.New("private CAS recovery V4 root binding is invalid")
			}
			spec, known := domainprivatecas.RootByRelativePathV1(root.RootID)
			if !known || spec.RootID != root.RootID || v3Participant.plans[localIndex].rootPath != root.RootPath {
				return nil, errors.New("private CAS recovery V4 root is outside the catalog or prepared plan")
			}
			if _, duplicate := seenRootIDs[root.RootID]; duplicate {
				return nil, errors.New("private CAS recovery V4 repeats a logical root")
			}
			seenRootIDs[root.RootID] = struct{}{}
			planBinding, err := privateCASRecoveryPlanBindingForJournalV4(
				v3Participant.plans[localIndex], root.RootID, uint32(globalPlanIndex),
			)
			if err != nil {
				return nil, err
			}
			planBinding.recoveryTargetsFrozen = binding.recoveryTargetsFrozen
			if binding.recoveryTargetsFrozen {
				if err := freezePlainPrivateCASRecoveryTargetsV4(&planBinding); err != nil {
					return nil, err
				}
			}
			binding.plans = append(binding.plans, planBinding)
			prepared.plans = append(prepared.plans, planBinding)
			globalPlanIndex++
		}
		prepared.participants = append(prepared.participants, binding)
	}
	if len(prepared.plans) != len(v3.plans) || len(prepared.plans) > domainprivatecas.MaxRecoveryJournalPlansV1 {
		return nil, errors.New("private CAS recovery V4 plan manifest is incomplete")
	}
	prepared.authoritySetDigest = privateCASRecoveryAuthoritySetDigestV4(prepared.participants)
	if !domainprivatecas.ValidDigestV1(prepared.authoritySetDigest) {
		return nil, errors.New("private CAS recovery V4 authority set digest is invalid")
	}
	if err := prepared.revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4) revalidate(ctx context.Context) error {
	if prepared == nil || !domainprivatecas.ValidDigestV1(prepared.authoritySetDigest) {
		return errors.New("private CAS recovery V4 authority set is invalid")
	}
	if prepared.v3 == nil {
		if len(prepared.participants) != 0 || len(prepared.plans) != 0 ||
			prepared.authoritySetDigest != privateCASRecoveryAuthoritySetDigestV4(nil) {
			return errors.New("private CAS recovery V4 empty authority set is invalid")
		}
		return nil
	}
	if len(prepared.participants) == 0 || len(prepared.plans) == 0 {
		return errors.New("private CAS recovery V4 authority set is invalid")
	}
	if err := prepared.v3.revalidate(ctx); err != nil {
		return err
	}
	refreshedParticipants := make([]privateCASRecoveryParticipantBindingV4, 0, len(prepared.participants))
	globalPlanIndex := 0
	for participantIndex, frozen := range prepared.participants {
		v3Participant := prepared.v3.participants[participantIndex]
		current := privateCASRecoveryParticipantBindingV4{
			participantID: frozen.participantID, semanticAuthorityDigest: frozen.semanticAuthorityDigest,
			authority: frozen.authority, topologyRoots: append([]string(nil), frozen.topologyRoots...),
			topologyDigests:       make([]string, 0, len(v3Participant.topologies)),
			plans:                 make([]privateCASRecoveryPlanBindingV4, 0, len(v3Participant.plans)),
			recoveryTargetsFrozen: frozen.recoveryTargetsFrozen,
		}
		if securePrivateCASRecoveryTargetsFrozenV4(frozen.authority) != frozen.recoveryTargetsFrozen {
			return errors.New("private CAS recovery target freeze authority changed")
		}
		for topologyIndex, topologyV3 := range v3Participant.topologies {
			topologyV4, ok := topologyV3.(SecurePrivateCASRecoveryTopologyAuthorityV4)
			if !ok || topologyV4.PrivateCASRecoveryTopologyRootV3() != frozen.topologyRoots[topologyIndex] {
				return errors.New("private CAS recovery V4 topology authority changed")
			}
			digest := topologyV4.PrivateCASRecoveryTopologyDigestV4()
			if digest != frozen.topologyDigests[topologyIndex] {
				return errors.New("private CAS recovery V4 topology digest changed")
			}
			current.topologyDigests = append(current.topologyDigests, digest)
		}
		for localIndex, plan := range v3Participant.plans {
			if globalPlanIndex >= len(prepared.plans) {
				return errors.New("private CAS recovery V4 plan set changed")
			}
			frozenPlan := prepared.plans[globalPlanIndex]
			currentPlan, err := privateCASRecoveryPlanBindingForJournalV4(
				plan, frozenPlan.rootID, uint32(globalPlanIndex),
			)
			if err != nil {
				return err
			}
			currentPlan.recoveryTargetsFrozen = frozen.recoveryTargetsFrozen
			if frozen.recoveryTargetsFrozen {
				if err := freezePlainPrivateCASRecoveryTargetsV4(&currentPlan); err != nil {
					return err
				}
			}
			if currentPlan.rootPath != frozenPlan.rootPath ||
				currentPlan.planAuthorityDigest != frozenPlan.planAuthorityDigest ||
				currentPlan.recoveryTargetsFrozen != frozenPlan.recoveryTargetsFrozen ||
				plan != v3Participant.plans[localIndex] {
				return errors.New("private CAS recovery V4 plan authority changed")
			}
			// Residue names and membership are transaction state, not plan
			// authority. Refresh only that body-free observation after proving
			// the stable plan digest is unchanged.
			prepared.plans[globalPlanIndex].targets = currentPlan.targets
			current.plans = append(current.plans, currentPlan)
			globalPlanIndex++
		}
		refreshedParticipants = append(refreshedParticipants, current)
	}
	if globalPlanIndex != len(prepared.plans) ||
		privateCASRecoveryAuthoritySetDigestV4(refreshedParticipants) != prepared.authoritySetDigest {
		return errors.New("private CAS recovery V4 authority set changed")
	}
	return nil
}

func securePrivateCASRecoveryTargetsFrozenV4(authority PreparedSecurePrivateCASRecoveryAuthorityV3) bool {
	marker, ok := authority.(SecurePrivateCASRecoveryTargetFreezeAuthorityV4)
	return ok && marker.FreezeSecurePrivateCASRecoveryTargetsV4()
}

func freezePlainPrivateCASRecoveryTargetsV4(plan *privateCASRecoveryPlanBindingV4) error {
	if plan == nil {
		return errors.New("private CAS recovery frozen target plan is invalid")
	}
	for _, target := range plan.targets {
		if target.phase != privateCASRecoveryQuarantinePlain || target.transactionID != "" {
			return errors.New("private CAS recovery frozen participant contains a staged or committed target")
		}
	}
	plan.targets = nil
	return nil
}

func (prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4) currentTargets() []privateCASRecoveryTargetObservationV4 {
	if prepared == nil {
		return nil
	}
	targets := make([]privateCASRecoveryTargetObservationV4, 0)
	for _, plan := range prepared.plans {
		targets = append(targets, plan.targets...)
	}
	sort.Slice(targets, func(left, right int) bool {
		return targets[left].entry.TargetID < targets[right].entry.TargetID
	})
	return targets
}

func privateCASRecoveryAuthoritySetDigestV4(participants []privateCASRecoveryParticipantBindingV4) string {
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix.private-cas-recovery-authority-set/v4"))
	privateCASWriteFingerprintField(hasher, []byte(SecurePrivateCASRecoveryAlgorithmV4))
	privateCASWriteUint64V4(hasher, uint64(len(participants)))
	for participantIndex, participant := range participants {
		privateCASWriteUint64V4(hasher, uint64(participantIndex))
		privateCASWriteFingerprintField(hasher, []byte(participant.participantID))
		privateCASWriteFingerprintField(hasher, []byte(participant.semanticAuthorityDigest))
		if participant.recoveryTargetsFrozen {
			privateCASWriteFingerprintField(hasher, []byte("recovery-targets-frozen"))
		}
		privateCASWriteUint64V4(hasher, uint64(len(participant.plans)))
		for _, plan := range participant.plans {
			privateCASWriteFingerprintField(hasher, []byte(plan.rootID))
			privateCASWriteFingerprintField(hasher, []byte(plan.planAuthorityDigest))
		}
		privateCASWriteUint64V4(hasher, uint64(len(participant.topologyDigests)))
		for index := range participant.topologyDigests {
			privateCASWriteFingerprintField(hasher, []byte(participant.topologyRoots[index]))
			privateCASWriteFingerprintField(hasher, []byte(participant.topologyDigests[index]))
		}
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func privateCASRecoveryFinalInventoryDigestV4(authoritySetDigest string) string {
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix.private-cas-recovery-final-inventory/v4"))
	privateCASWriteFingerprintField(hasher, []byte(authoritySetDigest))
	privateCASWriteUint64V4(hasher, 0)
	return hex.EncodeToString(hasher.Sum(nil))
}

func privateCASWriteRootBindingV4(writer hash.Hash, binding privatecasport.RootBinding) {
	privateCASWriteFingerprintField(writer, []byte(binding.RootPath))
	privateCASWriteFingerprintField(writer, []byte(binding.RelativePath))
	privateCASWriteFingerprintField(writer, []byte(binding.RootIdentity.Kind))
	privateCASWriteUint64V4(writer, binding.RootIdentity.Device)
	privateCASWriteUint64V4(writer, binding.RootIdentity.Inode)
	privateCASWriteUint64V4(writer, binding.RootIdentity.VolumeSerial)
	privateCASWriteFingerprintField(writer, binding.RootIdentity.FileID[:])
}

func privateCASWriteUint64V4(writer hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	privateCASWriteFingerprintField(writer, encoded[:])
}

func validPrivateCASRecoveryParticipantIDV4(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			(index > 0 && (character == '-' || character == '_')) {
			continue
		}
		return false
	}
	return true
}
