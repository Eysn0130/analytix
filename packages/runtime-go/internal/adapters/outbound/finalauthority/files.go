package finalauthority

import domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"

type privateCASRecoveryQuarantinePhase uint8

const (
	privateCASRecoveryQuarantinePlain privateCASRecoveryQuarantinePhase = iota
	privateCASRecoveryQuarantineStaged
	privateCASRecoveryQuarantineCommitted
)

const (
	privateCASRecoveryStagePrefix  = domainprivatecas.RecoveryStagePrefixV1
	privateCASRecoveryCommitPrefix = domainprivatecas.RecoveryCommitPrefixV1
)

func privateWriteTempName(name, shard string) bool {
	return domainprivatecas.PrivateWriteTempNameV1(name, shard)
}

func privateCASRecoveryTempName(
	name string,
	shard string,
) (string, string, privateCASRecoveryQuarantinePhase, bool) {
	classified, ok := domainprivatecas.ClassifyRecordResidueNameV1(name, shard)
	if !ok {
		return "", "", privateCASRecoveryQuarantinePlain, false
	}
	phase := privateCASRecoveryQuarantinePlain
	switch classified.Kind {
	case domainprivatecas.ResidueOrdinaryWriteV1:
	case domainprivatecas.ResidueRecoveryStageV1:
		phase = privateCASRecoveryQuarantineStaged
	case domainprivatecas.ResidueRecoveryCommitV1:
		phase = privateCASRecoveryQuarantineCommitted
	default:
		return "", "", privateCASRecoveryQuarantinePlain, false
	}
	return classified.OriginalName, classified.TransactionID, phase, true
}

func privateCASRecoveryQuarantineName(
	phase privateCASRecoveryQuarantinePhase,
	transactionID string,
	original string,
	shard string,
) (string, bool) {
	kind := domainprivatecas.ResidueKindV1("")
	if phase == privateCASRecoveryQuarantineCommitted {
		kind = domainprivatecas.ResidueRecoveryCommitV1
	} else if phase == privateCASRecoveryQuarantineStaged {
		kind = domainprivatecas.ResidueRecoveryStageV1
	} else {
		return "", false
	}
	return domainprivatecas.RecoveryQuarantineNameV1(kind, transactionID, original, shard)
}

func validPrivateShard(value string) bool {
	return domainprivatecas.ValidShardV1(value)
}

func validPrivateDigest(value string) bool {
	return domainprivatecas.ValidDigestV1(value)
}

func privateCASCreateDirectoryResidueName(component string) string {
	return domainprivatecas.CreateDirectoryResidueNameV1(component)
}
