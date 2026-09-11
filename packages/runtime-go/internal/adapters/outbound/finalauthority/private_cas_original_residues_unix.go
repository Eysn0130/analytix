//go:build darwin || linux

package finalauthority

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"strings"
)

type privateCASOriginalResidueV1 struct {
	temp           privateCASPreparedRecoveryTemp
	emptyShard     bool
	shard          privateCASShardIdentity
	mode, uid, gid uint32
}

func privateCASOriginalResiduesFromPlanV1(plan privateCASPreparedRecoveryPlan, pins map[string]privateCASShardIdentity) (privateCASOriginalResiduesV1, error) {
	if len(plan.shards) != len(pins) {
		return nil, errors.New("private CAS original shard inventory differs from authority")
	}
	var residues privateCASOriginalResiduesV1
	for _, shard := range plan.shards {
		identity, ok := pins[shard.name]
		if !ok || identity != shard.identity {
			return nil, errors.New("private CAS original shard is unbound")
		}
		if len(shard.committed)+len(shard.temps) == 0 {
			if residues == nil {
				residues = make(privateCASOriginalResiduesV1)
			}
			residues[shard.name+"/"] = privateCASOriginalEmptyShardV1(shard)
		}
		records := make(map[string]privateCASPreparedRecoveryRecord, len(shard.committed))
		for _, record := range shard.committed {
			if record.identity.Nlink != 1 && record.identity.Nlink != 2 {
				return nil, errors.New("private CAS original committed record has unsafe links")
			}
			records[record.name] = record
		}
		linkedRecords := make(map[string]bool)
		for _, temp := range shard.temps {
			if temp.phase != privateCASRecoveryQuarantinePlain || temp.transactionID != "" || !privateWriteTempName(temp.originalName, shard.name) {
				return nil, errors.New("private CAS original residue requires canonical plain state")
			}
			if temp.linked {
				marker := strings.Index(temp.originalName, ".json-")
				name := strings.TrimPrefix(temp.originalName[:marker+len(".json")], ".")
				record, found := records[name]
				if !found || linkedRecords[name] || temp.identity.Nlink != 2 || record.identity.Nlink != 2 ||
					!(privateCASUnixExactObject(record.identity, temp.identity)) || record.bodySHA256 != temp.bodySHA256 {
					return nil, errors.New("private CAS original linked residue lacks its exact committed partner")
				}
				linkedRecords[name] = true
			} else if temp.identity.Nlink != 1 {
				return nil, errors.New("private CAS original unlinked residue is not single-link")
			}
			if residues == nil {
				residues = make(privateCASOriginalResiduesV1)
			}
			residues[shard.name+"/"+temp.name] = privateCASOriginalResidueV1{temp: temp}
		}
		for _, record := range shard.committed {
			if record.identity.Nlink == 2 && !linkedRecords[record.name] {
				return nil, errors.New("private CAS original committed record has an unobserved link")
			}
		}
	}
	return residues, nil
}

func privateCASOriginalResidueEqualV1(left, right privateCASOriginalResidueV1) bool {
	if left.emptyShard || right.emptyShard {
		return left.emptyShard == right.emptyShard && left.shard == right.shard && left.mode == right.mode && left.uid == right.uid && left.gid == right.gid
	}
	l, r := left.temp, right.temp
	return l.name == r.name && l.originalName == r.originalName && l.transactionID == r.transactionID &&
		l.phase == r.phase && l.linked == r.linked && l.bodySHA256 == r.bodySHA256 && privateCASUnixExactObject(l.identity, r.identity)
}

func privateCASOriginalEmptyShardV1(shard privateCASPreparedRecoveryShard) privateCASOriginalResidueV1 {
	return privateCASOriginalResidueV1{emptyShard: true, shard: shard.identity, mode: uint32(shard.stat.Mode), uid: shard.stat.Uid, gid: shard.stat.Gid}
}

func privateCASRestoreOriginalEmptyShardsV1(plan privateCASPreparedRecoveryPlan, expected, current privateCASOriginalResiduesV1) privateCASOriginalResiduesV1 {
	for _, shard := range plan.shards {
		name := shard.name + "/"
		if entry, found := expected[name]; found && entry.emptyShard {
			if current == nil {
				current = make(privateCASOriginalResiduesV1)
			}
			current[name] = privateCASOriginalEmptyShardV1(shard)
		}
	}
	return current
}

func privateCASOriginalShardPinsV1(plan privateCASPreparedRecoveryPlan) map[string]privateCASShardIdentity {
	pins := make(map[string]privateCASShardIdentity, len(plan.shards))
	for _, shard := range plan.shards {
		pins[shard.name] = shard.identity
	}
	return pins
}

// This path is selected only after the live generation proves an immutable
// original create residue for this absent shard. The original temporary name
// is never touched. A failed call may leave a new empty canonical shard;
// later observation must prove it before opening or filling it.
func privateCASUnixCreateIndependentOriginalShardV1(parent int, name string, device uint64) (int, error) {
	if !validPrivateShard(name) {
		return -1, errors.New("original CAS independent shard name is invalid")
	}
	return privateCASUnixCreateIndependentOriginalDirectoryV1(parent, name, device)
}

func privateCASUnixCreateIndependentOriginalDirectoryV1(parent int, name string, device uint64) (int, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return -1, errors.New("original CAS independent directory component is invalid")
	}
	if err := unix.Mkdirat(parent, name, 0o700); err != nil {
		return -1, errors.Join(errors.New("original CAS independent shard exclusive creation failed"), err)
	}
	created, present, err := privateCASUnixOpenExactBoundDirectory(parent, name, device)
	if err != nil || !present {
		return -1, errors.Join(errors.New("original CAS independently created shard is unavailable"), err)
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = unix.Close(created)
		}
	}()
	var identity unix.Stat_t
	if err := unix.Fstat(created, &identity); err != nil || !existingPrivateAuthorityRootSafe(identity, uint32(os.Geteuid())) || !existingPrivateAuthorityExtendedSecuritySafe(created) || uint64(identity.Dev) != device {
		return -1, errors.Join(errors.New("original CAS independently created shard is unsafe"), err)
	}
	if err := errors.Join(unix.Fsync(created), unix.Fsync(parent)); err != nil {
		return -1, err
	}
	named, present, err := privateCASUnixOpenExactBoundDirectory(parent, name, device)
	if err != nil || !present {
		return -1, errors.Join(errors.New("original CAS independent shard name changed"), err)
	}
	var current unix.Stat_t
	statErr := unix.Fstat(named, &current)
	closeErr := unix.Close(named)
	if statErr != nil || closeErr != nil || !privateCASUnixExactObject(identity, current) {
		return -1, errors.Join(errors.New("original CAS independent shard identity changed"), statErr, closeErr)
	}
	accepted = true
	return created, nil
}
