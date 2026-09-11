package finalauthority

import (
	"context"
	"errors"
	"os"
	"runtime"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// This proof is adapter-owned and can only originate from a complete prepared
// recovery observation. Residues and original empty shard identities stay
// physical observations and never become committed records.
type privateCASOriginalResiduesV1 map[string]privateCASOriginalResidueV1

type privateCASOriginalOpeningV1 struct {
	residues privateCASOriginalResiduesV1
	creates  *PreparedSecurePrivateCASOriginalCreateResiduesV1
}

// OpenPreservingOriginalResiduesV1 opens an existing, unchanged prepared leaf
// without recovering its original plain residues or empty shards. A linked
// residue must name its exact committed partner, with exactly two links and no
// unobserved aliases. The caller owns complete owner semantics and permission
// to write independent records. Signed transaction phases and incomplete owner
// topology remain excluded.
func (prepared *PreparedSecurePrivateCASRecoveryV1) OpenPreservingOriginalResiduesV1(ctx context.Context) (*SecurePrivateCAS, error) {
	if prepared == nil || !prepared.present || prepared.access == nil || prepared.gate == nil {
		return nil, errors.New("private CAS original opening requires a present prepared leaf")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prepared.originalCreates != nil {
		if err := prepared.originalCreates.Revalidate(ctx); err != nil {
			return nil, err
		}
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return nil, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	var store *SecurePrivateCAS
	err := withExistingPrivateCASAccess(ctx, prepared.access, prepared.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS original opening binding changed")
		}
		if err := acquirePrivateCASGate(ctx, prepared.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(prepared.gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || !present || authority != prepared.authority {
			return errors.Join(errors.New("private CAS original opening identity changed"), err)
		}
		observation, err := observePrivateCASWithOriginalCreatesV1(ctx, authority, prepared.rootPath, prepared.maxBytes, prepared.originalCreates)
		if err != nil || observation.fingerprint != prepared.observation.fingerprint {
			return errors.Join(errors.New("private CAS original opening inventory changed"), err)
		}
		pins := privateCASOriginalShardPinsV1(observation.plan)
		residues, err := privateCASOriginalResiduesFromPlanV1(observation.plan, pins)
		if err != nil {
			return err
		}
		generation, err := attachPrivateCASRootGeneration(ctx, privateCASRootGenerationKey{binding: binding, rootPath: prepared.rootPath}, authority, pins, prepared.maxBytes, privateCASOriginalOpeningV1{residues: residues, creates: prepared.originalCreates})
		if err != nil {
			return err
		}
		store = &SecurePrivateCAS{gate: prepared.gate, root: authority, binding: binding, rootPath: prepared.rootPath, maxBytes: prepared.maxBytes, generation: generation, access: prepared.access}
		current, err := observePrivateCASWithOriginalCreatesV1(ctx, authority, prepared.rootPath, prepared.maxBytes, prepared.originalCreates)
		if err != nil || current.fingerprint != observation.fingerprint {
			return errors.Join(errors.New("private CAS original opening changed before activation"), err)
		}
		return ctx.Err()
	})
	if prepared.originalCreates != nil {
		err = errors.Join(err, prepared.originalCreates.Revalidate(ctx))
	}
	if err != nil {
		if store != nil {
			err = errors.Join(err, store.Close())
		}
		return nil, err
	}
	runtime.SetFinalizer(store, func(current *SecurePrivateCAS) { _ = current.Close() })
	return store, nil
}

func privateCASOriginalResiduesEqualV1(left, right privateCASOriginalResiduesV1) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for name, entry := range left {
		other, ok := right[name]
		if !ok || !privateCASOriginalResidueEqualV1(entry, other) {
			return false
		}
	}
	return true
}

// A fresh explicit original opening may observe a previously empty shard
// after this generation's exact +1 commit filled it. The existing generation
// still validates that directory's original identity and complete records;
// this never upgrades an ordinary generation or adopts a new empty shard.
func privateCASOriginalResiduesCompatibleOpeningV1(existing, candidate privateCASOriginalResiduesV1) bool {
	if existing == nil {
		return candidate == nil
	}
	for name, entry := range candidate {
		previous, found := existing[name]
		if !found || !privateCASOriginalResidueEqualV1(previous, entry) {
			return false
		}
	}
	for name, entry := range existing {
		if entry.emptyShard {
			continue
		}
		current, found := candidate[name]
		if !found || !privateCASOriginalResidueEqualV1(entry, current) {
			return false
		}
	}
	return true
}

// Call only within the existing access lease and process gate; these low-level
// readers deliberately do not acquire either lock or the recovery barrier again.
func observePrivateCASOriginalResiduesV1(ctx context.Context, authority privateCASRootAuthority, rootPath string, pins map[string]privateCASShardIdentity, maxBytes int, expected privateCASOriginalResiduesV1, originalCreates *PreparedSecurePrivateCASOriginalCreateResiduesV1) (privateCASRecoveryObservation, error) {
	if err := privateCASContextError(ctx); err != nil {
		return privateCASRecoveryObservation{}, err
	}
	observation, err := observePrivateCASWithOriginalCreatesV1(ctx, authority, rootPath, maxBytes, originalCreates)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	residues, err := privateCASOriginalResiduesFromPlanV1(observation.plan, pins)
	if err == nil {
		residues = privateCASRestoreOriginalEmptyShardsV1(observation.plan, expected, residues)
	}
	if err != nil || !privateCASOriginalResiduesEqualV1(expected, residues) {
		return privateCASRecoveryObservation{}, errors.Join(ErrSecurePrivateCASIntegrity, errors.New("private CAS original residue inventory changed"), err)
	}
	if err := privateCASContextError(ctx); err != nil {
		return privateCASRecoveryObservation{}, err
	}
	return observation, nil
}

func privateCASOriginalCommittedRecordsV1(observation privateCASRecoveryObservation) map[string]privateCASAuthorizedRecord {
	records := make(map[string]privateCASAuthorizedRecord, len(observation.committedMaterials))
	for _, material := range observation.committedMaterials {
		records[material.Digest] = privateCASAuthorizedRecord{bodySHA256: material.BodySHA256}
	}
	return records
}

func (generation *privateCASRootGeneration) observeOriginalLockedV1(ctx context.Context) (privateCASRecoveryObservation, error) {
	observation, err := observePrivateCASOriginalResiduesV1(ctx, generation.root, generation.key.rootPath, generation.shardPins, generation.maxBytes, generation.originalResidues, generation.originalCreates)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	if !privateCASAuthorizedRecordsEqual(privateCASOriginalCommittedRecordsV1(observation), generation.records) {
		return privateCASRecoveryObservation{}, errors.Join(ErrSecurePrivateCASIntegrity, errors.New("private CAS original opening discovered unauthorized committed records"))
	}
	return observation, nil
}

func (generation *privateCASRootGeneration) withOriginalObservationLockedV1(ctx context.Context, read func(privateCASRecoveryObservation) error) (resultErr error) {
	before, err := generation.observeOriginalLockedV1(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if generation.beforeOriginalReadRevalidation != nil {
			generation.beforeOriginalReadRevalidation()
		}
		after, err := generation.observeOriginalLockedV1(ctx)
		if err != nil || before.fingerprint != after.fingerprint {
			resultErr = errors.Join(resultErr, ErrSecurePrivateCASIntegrity, errors.New("private CAS original inventory changed during read"), err)
		}
		resultErr = errors.Join(resultErr, privateCASContextError(ctx))
	}()
	return read(before)
}

func (generation *privateCASRootGeneration) readOriginalLockedV1(ctx context.Context, digest string) ([]byte, error) {
	var body []byte
	found := false
	err := generation.withOriginalObservationLockedV1(ctx, func(observation privateCASRecoveryObservation) error {
		if _, found = generation.records[digest]; !found {
			return nil
		}
		var err error
		body, err = securePrivateCASReadPreparedCommitted(ctx, generation.root, observation, generation.maxBytes, digest)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, os.ErrNotExist
	}
	return body, nil
}

func (generation *privateCASRootGeneration) visitOriginalLockedV1(ctx context.Context, visit func(SecurePrivateCASFile) error) error {
	return generation.withOriginalObservationLockedV1(ctx, func(observation privateCASRecoveryObservation) error {
		for _, material := range observation.committedMaterials {
			body, err := securePrivateCASReadPreparedCommitted(ctx, generation.root, observation, generation.maxBytes, material.Digest)
			if err != nil {
				return err
			}
			if err := visit(SecurePrivateCASFile{Digest: material.Digest, Body: body}); err != nil {
				return err
			}
		}
		return privateCASContextError(ctx)
	})
}
