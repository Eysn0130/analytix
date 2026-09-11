package evidenceregistry

import (
	"context"
	"errors"
	"path"
	"runtime"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// OriginalGraphV2 retains every authenticated original node. It deliberately
// has no latest/current registry projection: sibling candidates and capsules
// written before their index or witness advance remain unselected evidence.
type OriginalGraphV2 = registryport.OriginalHistoryV2

func OriginalFilesContainV2(files map[string]OriginalLegacyEntryV1) bool {
	_, indexes := files["indexes"]
	_, capsules := files["capsules"]
	return indexes || capsules
}

// ParseOriginalGraphV2 verifies the complete original or authenticated
// semantic endpoint, including each branch's own ancestry. Installation and
// enrollment identities must come from the caller's independent anchor.
func ParseOriginalGraphV2(ctx context.Context, files map[string]OriginalLegacyEntryV1, contexts []domainsecurity.TurnSecurityContext, installationID, enrollmentID string, verifier finalauthorityport.Verifier) (_ OriginalGraphV2, resultErr error) {
	empty := OriginalGraphV2{}
	graph, err := parseOriginalRegistryRecordsV2(ctx, files, contexts, installationID, enrollmentID, verifier)
	if err != nil {
		return empty, err
	}
	children := map[string][]string{}
	var roots []string
	for digest, index := range graph.Indexes {
		capsule, found := graph.Capsules[index.Entry.CapsuleRecordDigest]
		if !found || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(index, capsule) {
			return empty, errors.New("original registry V2 index lost its exact capsule")
		}
		if index.Generation == 1 {
			roots = append(roots, digest)
			continue
		}
		previous, found := graph.Indexes[index.PreviousIndexDigest]
		if !found || domainevidence.ValidateEvidenceRegistryAuthorityIndexTransitionV2(previous, index) != nil {
			return empty, errors.New("original registry V2 index lost its exact predecessor")
		}
		children[index.PreviousIndexDigest] = append(children[index.PreviousIndexDigest], digest)
	}
	// The stack keeps only the nearest matching identity on this branch's
	// ancestry. Leaving a branch restores it before visiting any sibling.
	type frame struct {
		digest   string
		leaving  bool
		previous domainevidence.EvidenceRegistryAuthorityIndexEntry
		present  bool
	}
	ancestry := map[string]domainevidence.EvidenceRegistryAuthorityIndexEntry{}
	var stack []frame
	for _, digest := range roots {
		stack = append(stack, frame{digest: digest})
	}
	visited := 0
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		index := graph.Indexes[item.digest]
		identity := index.Entry.ThreadID + "\x00" + index.Entry.TurnID
		if item.leaving {
			if item.present {
				ancestry[identity] = item.previous
			} else {
				delete(ancestry, identity)
			}
			continue
		}
		previous, found := ancestry[identity]
		if found {
			if err := domainevidence.ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(previous, index.Entry, graph.Capsules[index.Entry.CapsuleRecordDigest]); err != nil {
				return empty, err
			}
		} else if index.Entry.RegistrySequence != 1 {
			return empty, errors.New("original registry V2 identity has no exact genesis")
		}
		visited++
		ancestry[identity] = index.Entry
		stack = append(stack, frame{digest: item.digest, leaving: true, previous: previous, present: found})
		for _, child := range children[item.digest] {
			stack = append(stack, frame{digest: child})
		}
	}
	if visited != len(graph.Indexes) {
		return empty, errors.New("original registry V2 graph contains an unreachable node")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return graph, nil
}

// ValidateOriginalFilesV2 authenticates every present record independently.
// A signed semantic program may explain an intermediate graph; both complete
// endpoints must separately pass ParseOriginalGraphV2 before any effect.
func ValidateOriginalFilesV2(ctx context.Context, files map[string]OriginalLegacyEntryV1, contexts []domainsecurity.TurnSecurityContext, installationID, enrollmentID string, verifier finalauthorityport.Verifier) error {
	_, err := parseOriginalRegistryRecordsV2(ctx, files, contexts, installationID, enrollmentID, verifier)
	return err
}

func parseOriginalRegistryRecordsV2(ctx context.Context, files map[string]OriginalLegacyEntryV1, contexts []domainsecurity.TurnSecurityContext, installationID, enrollmentID string, verifier finalauthorityport.Verifier) (_ OriginalGraphV2, resultErr error) {
	empty := OriginalGraphV2{}
	if ctx == nil || verifier == nil || !domainsecurity.IsSHA256Hex(installationID) || !domainsecurity.IsSHA256Hex(enrollmentID) || !OriginalFilesContainV2(files) {
		return empty, errors.New("original registry V2 independent authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	byIdentity := map[string]domainsecurity.TurnSecurityContext{}
	for _, frozen := range contexts {
		identity := frozen.ThreadID + "\x00" + frozen.TurnID
		if domainsecurity.ValidateTurnSecurityContext(frozen) != nil || byIdentity[identity].ContextDigest != "" {
			return empty, errors.New("original registry V2 primary context denominator is invalid")
		}
		byIdentity[identity] = frozen
	}
	graph := OriginalGraphV2{Indexes: map[string]domainevidence.EvidenceRegistryAuthorityIndexV2{}, Capsules: map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}}
	var total int64
	for name, entry := range files {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		if len(files) > domainstartup.MaxManagedSnapshotEntriesV1 || name == "" || path.Clean(name) != name || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") || entry.Mode == 0 || entry.Mode & ^uint32(0o777) != 0 || runtime.GOOS != "windows" && entry.Mode&0o077 != 0 {
			return empty, errors.New("original registry V2 address or mode is invalid")
		}
		if name != "." {
			if parent, found := files[path.Dir(name)]; !found || !parent.Directory {
				return empty, errors.New("original registry V2 parent is absent")
			}
		} else {
			if !entry.Directory || len(entry.Body) != 0 {
				return empty, errors.New("original registry V2 root is invalid")
			}
			continue
		}
		if name == ".registry.lock" {
			if entry.Directory || len(entry.Body) != 0 {
				return empty, errors.New("original registry V2 legacy lock is not empty")
			}
			continue
		}
		parts := strings.Split(name, "/")
		limit := maxEvidenceRegistryAuthorityIndexV2Bytes
		if parts[0] == "capsules" {
			limit = maxEvidenceRegistryAuthorityCapsuleV2Bytes
		} else if parts[0] != "indexes" {
			return empty, errors.New("original registry V2 has an unknown owner entry")
		}
		if len(parts) > 3 || len(parts) >= 2 && !domainprivatecas.ValidShardV1(parts[1]) {
			return empty, errors.New("original registry V2 target grammar is invalid")
		}
		if len(parts) < 3 {
			if !entry.Directory || len(entry.Body) != 0 {
				return empty, errors.New("original registry V2 directory is invalid")
			}
			continue
		}
		if entry.Directory || len(entry.Body) > limit {
			return empty, errors.New("original registry V2 record kind or size is invalid")
		}
		total += int64(len(entry.Body))
		if total > domainstartup.MaxSemanticStagedTotalBytesV1 {
			return empty, errors.New("original registry V2 byte budget exceeded")
		}
		if _, residue := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); residue {
			continue
		}
		digest := strings.TrimSuffix(parts[2], ".json")
		if !domainprivatecas.ValidDigestV1(digest) || parts[2] != digest+".json" || digest[:2] != parts[1] {
			return empty, errors.New("original registry V2 record address is invalid")
		}
		if parts[0] == "indexes" {
			index, err := domainevidence.ParseEvidenceRegistryAuthorityIndexV2(entry.Body)
			if err != nil || index.IndexDigest != digest {
				return empty, errors.Join(errors.New("original registry V2 index content address changed"), err)
			}
			if err := domainevidence.ValidateEvidenceRegistryAuthorityIndexForInstallationV2(index, installationID, enrollmentID, verifier.KeyID(), verifier.PublicKey()); err != nil {
				return empty, err
			}
			if err := verifyOriginalRegistrySignatureV1(ctx, verifier, index.AuthorityKeyID, index.AuthorityPublicKey, index.AuthoritySignature, domainevidence.EvidenceRegistryAuthorityIndexSigningBytesV2(index)); err != nil {
				return empty, err
			}
			graph.Indexes[digest] = index
		} else {
			capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(entry.Body)
			if err != nil || capsule.RecordDigest != digest || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(capsule.SecurityContext) != nil || byIdentity[capsule.SecurityContext.ThreadID+"\x00"+capsule.SecurityContext.TurnID] != capsule.SecurityContext {
				return empty, errors.Join(errors.New("original registry V2 capsule lost its exact original context or address"), err)
			}
			seal := capsule.Seal
			if err := verifyOriginalRegistrySignatureV1(ctx, verifier, seal.AuthorityKeyID, seal.AuthorityPublicKey, seal.AuthoritySignature, domainevidence.EvidenceRegistryAuthoritySealSigningBytes(seal)); err != nil {
				return empty, err
			}
			graph.Capsules[digest] = capsule
		}
	}
	return graph, ctx.Err()
}
