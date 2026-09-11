//go:build windows

package finalauthority

import (
	"errors"
	"golang.org/x/sys/windows"
	"strings"
)

type privateCASOriginalResidueV1 struct {
	temp         privateCASPreparedRecoveryTemp
	emptyShard   bool
	shard        privateCASShardIdentity
	attributes   uint32
	creationTime [2]uint32
}

func privateCASOriginalResiduesFromPlanV1(plan privateCASPreparedRecoveryPlan, pins map[string]privateCASShardIdentity) (privateCASOriginalResiduesV1, error) {
	if len(plan.shards) != len(pins) {
		return nil, errors.New("private CAS original shard inventory differs from authority")
	}
	var residues privateCASOriginalResiduesV1
	for _, shard := range plan.shards {
		identity, ok := pins[shard.name]
		if !ok || identity.id != shard.identity.id {
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
			if record.identity.links != 1 && record.identity.links != 2 {
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
				if !found || linkedRecords[name] || temp.identity.links != 2 || record.identity.links != 2 ||
					!(record.identity == temp.identity) || record.bodySHA256 != temp.bodySHA256 {
					return nil, errors.New("private CAS original linked residue lacks its exact committed partner")
				}
				linkedRecords[name] = true
			} else if temp.identity.links != 1 {
				return nil, errors.New("private CAS original unlinked residue is not single-link")
			}
			if residues == nil {
				residues = make(privateCASOriginalResiduesV1)
			}
			residues[shard.name+"/"+temp.name] = privateCASOriginalResidueV1{temp: temp}
		}
		for _, record := range shard.committed {
			if record.identity.links == 2 && !linkedRecords[record.name] {
				return nil, errors.New("private CAS original committed record has an unobserved link")
			}
		}
	}
	return residues, nil
}

func privateCASOriginalResidueEqualV1(left, right privateCASOriginalResidueV1) bool {
	return left == right
}

func privateCASOriginalEmptyShardV1(shard privateCASPreparedRecoveryShard) privateCASOriginalResidueV1 {
	return privateCASOriginalResidueV1{emptyShard: true, shard: privateCASShardIdentity{id: shard.identity.id}, attributes: shard.identity.attributes, creationTime: [2]uint32{shard.identity.creationTime.LowDateTime, shard.identity.creationTime.HighDateTime}}
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
		pins[shard.name] = privateCASShardIdentity{id: shard.identity.id}
	}
	return pins
}

func privateCASWindowsCreateIndependentOriginalShardV1(parent windows.Handle, name string, volume uint64) (windows.Handle, error) {
	if !validPrivateShard(name) {
		return 0, errors.New("original CAS independent shard name is invalid")
	}
	return privateCASWindowsCreateIndependentOriginalDirectoryV1(parent, name, volume)
}

func privateCASWindowsCreateIndependentOriginalDirectoryV1(parent windows.Handle, name string, volume uint64) (windows.Handle, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return 0, errors.New("original CAS independent directory component is invalid")
	}
	created, err := privateWindowsOpenRelative(parent, name, privateWindowsMutateDirectoryAccess, windows.FILE_CREATE, true)
	if err != nil {
		return 0, errors.Join(errors.New("original CAS independent shard exclusive creation failed"), err)
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = windows.CloseHandle(created)
		}
	}()
	identity, err := privateCASWindowsCreateRecoveryExactIdentity(created, volume)
	if err != nil {
		return 0, err
	}
	if err := errors.Join(privateWindowsSyncDirectory(created), privateWindowsSyncDirectory(parent)); err != nil {
		return 0, err
	}
	named, present, err := privateCASWindowsOpenExactBoundDirectory(parent, name, volume)
	if err != nil || !present {
		return 0, errors.Join(errors.New("original CAS independent shard name changed"), err)
	}
	current, identityErr := privateCASWindowsCreateRecoveryExactIdentity(named, volume)
	closeErr := windows.CloseHandle(named)
	nameErr := privateWindowsVerifyRelativeDirectoryIdentity(parent, name, created)
	if identityErr != nil || closeErr != nil || nameErr != nil || identity.stable != current.stable {
		return 0, errors.Join(errors.New("original CAS independent shard identity changed"), identityErr, closeErr, nameErr)
	}
	accepted = true
	return created, nil
}
