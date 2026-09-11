//go:build darwin || linux

package finalauthority

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
)

func privateCASRecoveryPlanBindingForJournalV4(
	prepared *PreparedSecurePrivateCASRecoveryV1,
	rootID string,
	planIndex uint32,
) (privateCASRecoveryPlanBindingV4, error) {
	if prepared == nil || prepared.rootPath == "" || prepared.maxBytes <= 0 ||
		planIndex >= domainprivatecas.MaxRecoveryJournalPlansV1 {
		return privateCASRecoveryPlanBindingV4{}, errors.New("private CAS Unix recovery journal plan is invalid")
	}
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix.private-cas-recovery-plan/unix/v4"))
	privateCASWriteFingerprintField(hasher, []byte(rootID))
	privateCASWriteFingerprintField(hasher, []byte(prepared.rootPath))
	privateCASWriteUint64V4(hasher, uint64(prepared.maxBytes))
	privateCASWriteRootBindingV4(hasher, prepared.binding)
	if prepared.present {
		privateCASWriteFingerprintField(hasher, []byte{1})
		privateCASWriteUint64V4(hasher, prepared.authority.dev)
		privateCASWriteUint64V4(hasher, prepared.authority.ino)
	} else {
		privateCASWriteFingerprintField(hasher, []byte{0})
	}
	type committedShardV4 struct {
		name      string
		identity  privateCASShardIdentity
		committed []privateCASPreparedRecoveryRecord
	}
	committedShards := make([]committedShardV4, 0, len(prepared.observation.plan.shards))
	for _, shard := range prepared.observation.plan.shards {
		if len(shard.committed) == 0 {
			continue
		}
		committed := append([]privateCASPreparedRecoveryRecord(nil), shard.committed...)
		sort.Slice(committed, func(left, right int) bool { return committed[left].name < committed[right].name })
		committedShards = append(committedShards, committedShardV4{name: shard.name, identity: shard.identity, committed: committed})
	}
	sort.Slice(committedShards, func(left, right int) bool { return committedShards[left].name < committedShards[right].name })
	privateCASWriteUint64V4(hasher, uint64(len(committedShards)))
	for _, shard := range committedShards {
		privateCASWriteFingerprintField(hasher, []byte(shard.name))
		privateCASWriteUint64V4(hasher, shard.identity.dev)
		privateCASWriteUint64V4(hasher, shard.identity.ino)
		privateCASWriteUint64V4(hasher, uint64(len(shard.committed)))
		for _, record := range shard.committed {
			privateCASWriteFingerprintField(hasher, []byte(record.name))
			privateCASWriteFingerprintField(hasher, []byte(record.digest))
			privateCASWriteUint64V4(hasher, uint64(record.identity.Dev))
			privateCASWriteUint64V4(hasher, record.identity.Ino)
			privateCASWriteFingerprintField(hasher, record.bodySHA256[:])
			privateCASWriteUint64V4(hasher, record.byteLength)
		}
	}
	planDigest := hex.EncodeToString(hasher.Sum(nil))
	targets := make([]privateCASRecoveryTargetObservationV4, 0)
	for _, shard := range prepared.observation.plan.shards {
		shardDigest := privateCASUnixRecoveryIdentityDigestV4("shard", shard.identity.dev, shard.identity.ino)
		for _, temp := range shard.temps {
			if temp.identity.Size < 0 {
				return privateCASRecoveryPlanBindingV4{}, errors.New("private CAS Unix recovery target has a negative length")
			}
			entry, err := domainprivatecas.NewRecoveryTargetEntryV1(domainprivatecas.RecoveryTargetEntryInputV1{
				PlanIndex: planIndex, RootID: rootID, PlanAuthorityDigest: planDigest,
				ShardName: shard.name, ShardIdentityDigest: shardDigest,
				OriginalName: temp.originalName,
				ObjectIdentityDigest: privateCASUnixRecoveryIdentityDigestV4(
					"object", uint64(temp.identity.Dev), temp.identity.Ino,
				),
				BodySHA256: hex.EncodeToString(temp.bodySHA256[:]), ByteLength: uint64(temp.identity.Size), Linked: temp.linked,
			})
			if err != nil {
				return privateCASRecoveryPlanBindingV4{}, err
			}
			targets = append(targets, privateCASRecoveryTargetObservationV4{
				entry: entry, phase: temp.phase, transactionID: temp.transactionID,
			})
		}
	}
	sort.Slice(targets, func(left, right int) bool { return targets[left].entry.TargetID < targets[right].entry.TargetID })
	return privateCASRecoveryPlanBindingV4{
		rootID: rootID, rootPath: prepared.rootPath, planAuthorityDigest: planDigest, targets: targets,
	}, nil
}

func privateCASUnixRecoveryIdentityDigestV4(kind string, device, inode uint64) string {
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix.private-cas-recovery-unix-identity/v4"))
	privateCASWriteFingerprintField(hasher, []byte(kind))
	privateCASWriteUint64V4(hasher, device)
	privateCASWriteUint64V4(hasher, inode)
	return hex.EncodeToString(hasher.Sum(nil))
}
