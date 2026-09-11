package privatecastopology

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

type ResidueKindV1 string

const (
	ResidueOrdinaryWriteV1   ResidueKindV1 = "ordinary_write"
	ResidueRecoveryStageV1   ResidueKindV1 = "recovery_stage"
	ResidueRecoveryCommitV1  ResidueKindV1 = "recovery_commit"
	ResidueCreateDirectoryV1 ResidueKindV1 = "create_directory"
)

const (
	RecoveryStagePrefixV1   = ".analytix-cas-recovery-v2-stage-"
	RecoveryCommitPrefixV1  = ".analytix-cas-recovery-v2-commit-"
	CreateDirectoryPrefixV1 = ".analytix-cas-create-"
)

type ResidueNameV1 struct {
	Kind          ResidueKindV1
	OriginalName  string
	TransactionID string
}

func ClassifyRecordResidueNameV1(name string, shard string) (ResidueNameV1, bool) {
	if PrivateWriteTempNameV1(name, shard) {
		return ResidueNameV1{Kind: ResidueOrdinaryWriteV1, OriginalName: name}, true
	}
	kind := ResidueKindV1("")
	prefix := ""
	switch {
	case strings.HasPrefix(name, RecoveryStagePrefixV1):
		kind, prefix = ResidueRecoveryStageV1, RecoveryStagePrefixV1
	case strings.HasPrefix(name, RecoveryCommitPrefixV1):
		kind, prefix = ResidueRecoveryCommitV1, RecoveryCommitPrefixV1
	default:
		return ResidueNameV1{}, false
	}
	remainder := strings.TrimPrefix(name, prefix)
	separator := strings.IndexByte(remainder, '-')
	if separator != 64 {
		return ResidueNameV1{}, false
	}
	transactionID := remainder[:separator]
	if !ValidDigestV1(transactionID) {
		return ResidueNameV1{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(remainder[separator+1:])
	if err != nil || len(decoded) == 0 || len(decoded) > 160 {
		return ResidueNameV1{}, false
	}
	original := string(decoded)
	if !PrivateWriteTempNameV1(original, shard) || strings.ContainsAny(original, `/\`) {
		return ResidueNameV1{}, false
	}
	return ResidueNameV1{
		Kind: kind, OriginalName: original, TransactionID: transactionID,
	}, true
}

func RecoveryQuarantineNameV1(
	kind ResidueKindV1,
	transactionID string,
	original string,
	shard string,
) (string, bool) {
	if !ValidDigestV1(transactionID) || !PrivateWriteTempNameV1(original, shard) {
		return "", false
	}
	prefix := ""
	switch kind {
	case ResidueRecoveryStageV1:
		prefix = RecoveryStagePrefixV1
	case ResidueRecoveryCommitV1:
		prefix = RecoveryCommitPrefixV1
	default:
		return "", false
	}
	name := prefix + transactionID + "-" + base64.RawURLEncoding.EncodeToString([]byte(original))
	if len(name) > 255 {
		return "", false
	}
	return name, true
}

func PrivateWriteTempNameV1(name string, shard string) bool {
	if !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".tmp") {
		return false
	}
	marker := strings.Index(name, ".json-")
	if marker != 65 || len(name) <= marker+len(".json-.tmp") {
		return false
	}
	digest := name[1:marker]
	return ValidDigestV1(digest) && digest[:2] == shard
}

func LooksLikeRecordResidueNameV1(name string) bool {
	return strings.HasPrefix(name, RecoveryStagePrefixV1) ||
		strings.HasPrefix(name, RecoveryCommitPrefixV1) ||
		strings.HasPrefix(name, ".") && strings.Contains(name, ".json-") && strings.HasSuffix(name, ".tmp")
}

func CreateDirectoryResidueNameV1(component string) string {
	digest := sha256.Sum256([]byte(component))
	return CreateDirectoryPrefixV1 + hex.EncodeToString(digest[:8]) + ".tmp"
}

func IsCreateDirectoryResidueNameV1(name string) bool {
	const suffix = ".tmp"
	if !strings.HasPrefix(name, CreateDirectoryPrefixV1) || !strings.HasSuffix(name, suffix) {
		return false
	}
	digest := strings.TrimSuffix(strings.TrimPrefix(name, CreateDirectoryPrefixV1), suffix)
	return len(digest) == 16 && IsLowerHexV1(digest)
}

// LooksLikeCreateDirectoryResidueNameV1 reserves the complete create-residue
// namespace, including malformed and case-aliased spellings. Callers must
// fail closed on a lookalike that is not an exact, path-bound residue.
func LooksLikeCreateDirectoryResidueNameV1(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), ".analytix-cas-create")
}

func CreateDirectoryResidueMatchesComponentV1(name string, component string) bool {
	return IsCreateDirectoryResidueNameV1(name) && name == CreateDirectoryResidueNameV1(component)
}

func CreateDirectoryResidueMatchesShardV1(name string) bool {
	if !IsCreateDirectoryResidueNameV1(name) {
		return false
	}
	for high := 0; high < 16; high++ {
		for low := 0; low < 16; low++ {
			component := string([]byte{lowerHexDigit(high), lowerHexDigit(low)})
			if name == CreateDirectoryResidueNameV1(component) {
				return true
			}
		}
	}
	return false
}

func ValidShardV1(value string) bool {
	return len(value) == 2 && IsLowerHexV1(value)
}

func ValidDigestV1(value string) bool {
	return len(value) == 64 && IsLowerHexV1(value)
}

func IsLowerHexV1(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func lowerHexDigit(value int) byte {
	if value < 10 {
		return byte('0' + value)
	}
	return byte('a' + value - 10)
}
