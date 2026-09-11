package privatecastopology

import (
	"errors"
	"sort"
	"strings"
)

type LayoutKindV1 string

const (
	LayoutOwnerLeafV1  LayoutKindV1 = "owner_leaf"
	LayoutDirectRootV1 LayoutKindV1 = "direct_root"
	LayoutDetachedV1   LayoutKindV1 = "detached_root"
)

type SnapshotBodyPolicyV1 string

const (
	SnapshotStrictCanonicalJSONV1 SnapshotBodyPolicyV1 = "strict_canonical_json"
	SnapshotOpaqueBytesV1         SnapshotBodyPolicyV1 = "opaque_bytes"
)

type RootSpecV1 struct {
	RootID             string
	RecoveryGroupID    string
	RelativeCASRoot    string
	LayoutKind         LayoutKindV1
	SnapshotBodyPolicy SnapshotBodyPolicyV1
}

var runtimeRootSpecsV1 = []RootSpecV1{
	rootSpec("runtime-sidecar-authority-v1/allocations", "backend-generation", LayoutDirectRootV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("accepted-finals/records", "accepted-finals", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("accepted-finals/dispositions", "accepted-finals", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("case-entity/bindings-v1", "case-entity", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("case-entity/ingress-v1", "case-entity", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("case-entity/thread-context-v1", "case-entity", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("case-thread-authority", "case-thread-authority", LayoutDirectRootV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("gate-continuations/receipts-v2", "gate-continuations", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("gate-continuations/dispositions-v2", "gate-continuations", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("pending-work/receipts", "pending-work", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("pending-work/dispositions", "pending-work", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("provider-cache-telemetry/attempts", "provider-cache-telemetry", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("provider-cache-telemetry/settlements", "provider-cache-telemetry", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("provider-cache-telemetry/turn-closures", "provider-cache-telemetry", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("turn-terminal-authority/intents", "turn-terminal-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("turn-terminal-authority/dispositions", "turn-terminal-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("attachment-authority/owners", "attachment-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("attachment-authority/use-receipts", "attachment-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("attachment-authority/use-dispositions", "attachment-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("attachment-authority/upload-intents", "attachment-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("attachment-authority/upload-dispositions", "attachment-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("authority-advance/v2/intents", "authority-advance", LayoutDirectRootV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("authority-advance/v2/settlements", "authority-advance", LayoutDirectRootV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("evidence-authority/bundles", "evidence-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("evidence-authority/observations", "evidence-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec(EvidenceRegistryRecoveryGroupID+"/"+EvidenceRegistryCapsulesLeafV2, EvidenceRegistryRecoveryGroupID, LayoutOwnerLeafV1, SnapshotOpaqueBytesV1),
	rootSpec(EvidenceRegistryRecoveryGroupID+"/"+EvidenceRegistryIndexesLeafV2, EvidenceRegistryRecoveryGroupID, LayoutOwnerLeafV1, SnapshotOpaqueBytesV1),
	rootSpec("dataset-snapshot-authority/legacy-records", "dataset-snapshot-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("dataset-snapshot-authority/authority-bundles-v2", "dataset-snapshot-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("dataset-snapshot-authority/indexes", "dataset-snapshot-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("dataset-snapshot-authority/materials", "dataset-snapshot-authority", LayoutOwnerLeafV1, SnapshotOpaqueBytesV1),
	rootSpec("thread-risk-policy", "thread-risk-policy", LayoutDirectRootV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("pii-authorization/grants", "pii-authorization", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/attempts", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/receipts", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/commit-receipts", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/commit-selections", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/delivery-decisions", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/grant-settlements", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/stage-completions", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/delivery-projections", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/indexes", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/claim-ledgers", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/pii-projections", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/render-inspections", "report-publication", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("report-publication/artifacts", "report-publication", LayoutOwnerLeafV1, SnapshotOpaqueBytesV1),
	rootSpec("controlled-artifact-access/access-receipts", "controlled-artifact-access", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("controlled-artifact-access/access-dispositions", "controlled-artifact-access", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("controlled-artifact-access-v2/access-receipts", "controlled-artifact-access-v2", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("controlled-artifact-access-v2/access-dispositions", "controlled-artifact-access-v2", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("checkpoint-authority/snapshot-intents", "checkpoint-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("checkpoint-authority/snapshot-completions", "checkpoint-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("checkpoint-authority/snapshot-dispositions", "checkpoint-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("checkpoint-authority/operation-group-intents-v2", "checkpoint-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("checkpoint-authority/operation-group-terminals-v2", "checkpoint-authority", LayoutOwnerLeafV1, SnapshotStrictCanonicalJSONV1),
	rootSpec("checkpoint-snapshot-quarantine/audit-records", "checkpoint-authority", LayoutDetachedV1, SnapshotStrictCanonicalJSONV1),
}

func rootSpec(
	relative string,
	group string,
	layout LayoutKindV1,
	bodyPolicy SnapshotBodyPolicyV1,
) RootSpecV1 {
	return RootSpecV1{
		RootID: relative, RecoveryGroupID: group, RelativeCASRoot: relative,
		LayoutKind: layout, SnapshotBodyPolicy: bodyPolicy,
	}
}

func RuntimeRootSpecsV1() []RootSpecV1 {
	return append([]RootSpecV1(nil), runtimeRootSpecsV1...)
}

func RootsForRecoveryGroupV1(group string) []RootSpecV1 {
	group = strings.TrimSpace(group)
	result := make([]RootSpecV1, 0)
	for _, spec := range runtimeRootSpecsV1 {
		if spec.RecoveryGroupID == group {
			result = append(result, spec)
		}
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left].RelativeCASRoot < result[right].RelativeCASRoot
	})
	return result
}

func RootByRelativePathV1(relative string) (RootSpecV1, bool) {
	relative = strings.Trim(strings.TrimSpace(relative), "/")
	for _, spec := range runtimeRootSpecsV1 {
		if spec.RelativeCASRoot == relative {
			return spec, true
		}
	}
	return RootSpecV1{}, false
}

func RootContainingRelativePathV1(relative string) (RootSpecV1, []string, bool) {
	relative = strings.Trim(strings.TrimSpace(relative), "/")
	if relative == "" {
		return RootSpecV1{}, nil, false
	}
	for _, spec := range runtimeRootSpecsV1 {
		if relative == spec.RelativeCASRoot {
			return spec, nil, true
		}
		prefix := spec.RelativeCASRoot + "/"
		if strings.HasPrefix(relative, prefix) {
			remainder := strings.Split(strings.TrimPrefix(relative, prefix), "/")
			return spec, remainder, true
		}
	}
	return RootSpecV1{}, nil, false
}

func KnownTopLevelOwnerV1(value string) bool {
	value = strings.TrimSpace(value)
	for _, spec := range runtimeRootSpecsV1 {
		top := strings.Split(spec.RelativeCASRoot, "/")[0]
		if top == value {
			return true
		}
	}
	return false
}

func KnownTopLevelOwnerAliasV1(value string) bool {
	value = strings.TrimSpace(value)
	for _, spec := range runtimeRootSpecsV1 {
		top := strings.Split(spec.RelativeCASRoot, "/")[0]
		if strings.EqualFold(top, value) {
			return top != value
		}
	}
	return false
}

func ValidateRuntimeRootSpecsV1() error {
	if len(runtimeRootSpecsV1) != 56 {
		return errors.New("runtime private CAS topology root count changed")
	}
	rootIDs := make(map[string]struct{}, len(runtimeRootSpecsV1))
	paths := make(map[string]struct{}, len(runtimeRootSpecsV1))
	folded := make(map[string]string, len(runtimeRootSpecsV1))
	groups := make(map[string]struct{})
	opaque := 0
	for _, spec := range runtimeRootSpecsV1 {
		if spec.RootID == "" || spec.RootID != strings.TrimSpace(spec.RootID) ||
			spec.RecoveryGroupID == "" || spec.RecoveryGroupID != strings.TrimSpace(spec.RecoveryGroupID) ||
			spec.RelativeCASRoot == "" || spec.RelativeCASRoot != strings.Trim(spec.RelativeCASRoot, "/") ||
			strings.Contains(spec.RelativeCASRoot, `\`) || strings.Contains(spec.RelativeCASRoot, "//") ||
			spec.RootID != spec.RelativeCASRoot {
			return errors.New("runtime private CAS topology contains an invalid root")
		}
		if spec.LayoutKind != LayoutOwnerLeafV1 && spec.LayoutKind != LayoutDirectRootV1 &&
			spec.LayoutKind != LayoutDetachedV1 {
			return errors.New("runtime private CAS topology contains an invalid layout")
		}
		if spec.SnapshotBodyPolicy != SnapshotStrictCanonicalJSONV1 &&
			spec.SnapshotBodyPolicy != SnapshotOpaqueBytesV1 {
			return errors.New("runtime private CAS topology contains an invalid body policy")
		}
		if _, duplicate := rootIDs[spec.RootID]; duplicate {
			return errors.New("runtime private CAS topology repeats a root id")
		}
		rootIDs[spec.RootID] = struct{}{}
		if _, duplicate := paths[spec.RelativeCASRoot]; duplicate {
			return errors.New("runtime private CAS topology repeats a root path")
		}
		paths[spec.RelativeCASRoot] = struct{}{}
		fold := strings.ToLower(spec.RelativeCASRoot)
		if previous, alias := folded[fold]; alias && previous != spec.RelativeCASRoot {
			return errors.New("runtime private CAS topology aliases a root by case")
		}
		folded[fold] = spec.RelativeCASRoot
		groups[spec.RecoveryGroupID] = struct{}{}
		if spec.SnapshotBodyPolicy == SnapshotOpaqueBytesV1 {
			opaque++
			if spec.RelativeCASRoot != "report-publication/artifacts" &&
				spec.RelativeCASRoot != "dataset-snapshot-authority/materials" &&
				spec.RelativeCASRoot != EvidenceRegistryRecoveryGroupID+"/"+EvidenceRegistryCapsulesLeafV2 &&
				spec.RelativeCASRoot != EvidenceRegistryRecoveryGroupID+"/"+EvidenceRegistryIndexesLeafV2 {
				return errors.New("runtime private CAS topology exposes an unexpected opaque root")
			}
		}
	}
	if len(groups) != 19 || opaque != 4 {
		return errors.New("runtime private CAS topology group or opaque policy count changed")
	}
	return nil
}
