//go:build windows

package finalauthority

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
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
		return privateCASRecoveryPlanBindingV4{}, errors.New("private CAS Windows recovery journal plan is invalid")
	}
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix.private-cas-recovery-plan/windows/v4"))
	privateCASWriteFingerprintField(hasher, []byte(rootID))
	privateCASWriteFingerprintField(hasher, []byte(prepared.rootPath))
	privateCASWriteUint64V4(hasher, uint64(prepared.maxBytes))
	privateCASWriteRootBindingV4(hasher, prepared.binding)
	if prepared.present {
		privateCASWriteFingerprintField(hasher, []byte{1})
		privateCASWriteWindowsFileIDV4(hasher, prepared.authority.id)
	} else {
		privateCASWriteFingerprintField(hasher, []byte{0})
	}
	type committedShardV4 struct {
		name      string
		identity  privateWindowsObjectIdentity
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
		privateCASWriteWindowsFileIDV4(hasher, shard.identity.id)
		privateCASWriteUint64V4(hasher, uint64(len(shard.committed)))
		for _, record := range shard.committed {
			privateCASWriteFingerprintField(hasher, []byte(record.name))
			privateCASWriteFingerprintField(hasher, []byte(record.digest))
			privateCASWriteWindowsFileIDV4(hasher, record.identity.id)
			privateCASWriteFingerprintField(hasher, record.bodySHA256[:])
			privateCASWriteUint64V4(hasher, record.byteLength)
		}
	}
	planDigest := hex.EncodeToString(hasher.Sum(nil))
	targets := make([]privateCASRecoveryTargetObservationV4, 0)
	for _, shard := range prepared.observation.plan.shards {
		shardDigest := privateCASWindowsRecoveryIdentityDigestV4("shard", shard.identity.id)
		for _, temp := range shard.temps {
			byteLength := uint64(temp.identity.sizeHigh)<<32 | uint64(temp.identity.sizeLow)
			entry, err := domainprivatecas.NewRecoveryTargetEntryV1(domainprivatecas.RecoveryTargetEntryInputV1{
				PlanIndex: planIndex, RootID: rootID, PlanAuthorityDigest: planDigest,
				ShardName: shard.name, ShardIdentityDigest: shardDigest,
				OriginalName:         temp.originalName,
				ObjectIdentityDigest: privateCASWindowsRecoveryIdentityDigestV4("object", temp.identity.id),
				BodySHA256:           hex.EncodeToString(temp.bodySHA256[:]), ByteLength: byteLength, Linked: temp.linked,
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

func privateCASWriteWindowsFileIDV4(writer hash.Hash, identity privateWindowsFileIDInfo) {
	privateCASWriteUint64V4(writer, identity.VolumeSerialNumber)
	privateCASWriteFingerprintField(writer, identity.FileID[:])
}

func privateCASWindowsRecoveryIdentityDigestV4(kind string, identity privateWindowsFileIDInfo) string {
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix.private-cas-recovery-windows-identity/v4"))
	privateCASWriteFingerprintField(hasher, []byte(kind))
	privateCASWriteWindowsFileIDV4(hasher, identity)
	return hex.EncodeToString(hasher.Sum(nil))
}
