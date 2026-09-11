package evidenceregistry

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path"
	"runtime"
	"sort"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// OriginalLegacyEntryV1 carries observed bytes and permission bits. It grants
// no filesystem or registry authority. The root, when present, has path ".".
type OriginalLegacyEntryV1 = finalauthorityadapter.SecurePrivateCASOriginalEntryV1

type originalLegacyGraphV1 struct {
	index    *domainevidence.EvidenceRegistryAuthorityIndex
	capsules map[string]domainevidence.EvidenceRegistryAuthorityCapsule
	heads    map[string]domainevidence.EvidenceRegistryAuthorityCapsule
	ledgers  map[string][]byte
}

// ValidateOriginalLegacyFilesV1 checks every existing canonical signed record
// independently. A signed semantic program may explain an intermediate graph;
// callers must separately validate both complete transaction endpoints with
// ParseOriginalLegacyInventoryV1 before allowing any recovery effect.
func ValidateOriginalLegacyFilesV1(ctx context.Context, files map[string]OriginalLegacyEntryV1, verifier finalauthorityport.Verifier) error {
	_, err := parseOriginalLegacyFilesV1(ctx, files, verifier)
	return err
}

func verifyOriginalRegistrySignatureV1(ctx context.Context, verifier finalauthorityport.Verifier, key, public, signature string, body []byte) error {
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(public)
	signatureBytes, signatureErr := base64.RawURLEncoding.DecodeString(signature)
	if publicErr != nil || signatureErr != nil {
		return errors.Join(errors.New("original registry signature encoding is invalid"), publicErr, signatureErr)
	}
	return errors.Join(verifier.VerifyTrusted(ctx, key, publicKey, body, signatureBytes), ctx.Err())
}

func parseOriginalLegacyFilesV1(ctx context.Context, files map[string]OriginalLegacyEntryV1, verifier finalauthorityport.Verifier) (originalLegacyGraphV1, error) {
	graph := originalLegacyGraphV1{capsules: map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}, heads: map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}, ledgers: map[string][]byte{}}
	if ctx == nil || verifier == nil {
		return graph, errors.New("original registry verification is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return graph, err
	}
	count, totalBytes := 0, int64(0)
	folded := map[string]bool{}
	for name, entry := range files {
		if name == "." {
			continue
		}
		count++
		if !entry.Directory {
			totalBytes += int64(len(entry.Body))
		}
		fold := path.Dir(name) + "\x00" + strings.ToLower(path.Base(name))
		if count > maxRegistryOwnerEntries || totalBytes > maxRegistrySiblingBytes || folded[fold] {
			return graph, errors.New("original registry candidate exceeds physical inventory bounds or aliases an entry")
		}
		folded[fold] = true
	}
	for name, entry := range files {
		if err := ctx.Err(); err != nil {
			return graph, err
		}
		if name == "" || path.Clean(name) != name || path.IsAbs(name) || strings.Contains(name, "\\") || name == ".." || strings.HasPrefix(name, "../") || entry.Mode & ^uint32(0o777) != 0 || runtime.GOOS != "windows" && entry.Mode&0o077 != 0 {
			return graph, errors.New("original registry entry address or mode is invalid")
		}
		if name != "." {
			parent, found := files[path.Dir(name)]
			if !found || !parent.Directory {
				return graph, errors.New("original registry entry parent is absent")
			}
		}
		if entry.Directory {
			if len(entry.Body) != 0 || name != "." && name != evidenceRegistryCapsuleDirectory && name != evidenceRegistryProjectionDirectory {
				return graph, errors.New("original registry directory is invalid")
			}
			continue
		}
		switch name {
		case evidenceRegistryAuthorityIndexFile:
			index, err := domainevidence.ParseEvidenceRegistryAuthorityIndex(entry.Body)
			if err != nil {
				return graph, err
			}
			if err := verifyOriginalRegistrySignatureV1(ctx, verifier, index.AuthorityKeyID, index.AuthorityPublicKey, index.AuthoritySignature, domainevidence.EvidenceRegistryAuthorityIndexSigningBytes(index)); err != nil {
				return graph, err
			}
			graph.index = &index
			continue
		case ".registry.lock":
			continue
		}
		dir, base := path.Dir(name), path.Base(name)
		if dir == "." && strings.HasPrefix(base, ".registry-authority-index-") && strings.HasSuffix(base, ".tmp") {
			continue
		}
		if dir == evidenceRegistryCapsuleDirectory && strings.HasPrefix(base, ".capsule-") && strings.HasSuffix(base, ".tmp") {
			continue
		}
		if dir == evidenceRegistryProjectionDirectory && strings.HasPrefix(base, ".") && strings.HasSuffix(base, ".tmp") && (strings.Contains(base, ".jsonl-") || strings.Contains(base, ".head.json-")) {
			continue
		}
		if dir == evidenceRegistryProjectionDirectory && strings.HasSuffix(base, ".jsonl") {
			key := strings.TrimSuffix(base, ".jsonl")
			if !domainsecurity.IsSHA256Hex(key) {
				return graph, errors.New("original registry ledger address is invalid")
			}
			graph.ledgers[key] = append([]byte(nil), entry.Body...)
			continue
		}
		capsuleKind := dir == evidenceRegistryCapsuleDirectory && strings.HasSuffix(base, ".json")
		headKind := dir == evidenceRegistryProjectionDirectory && strings.HasSuffix(base, ".head.json")
		if !capsuleKind && !headKind {
			return graph, errors.New("original registry contains an unknown entry")
		}
		capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(entry.Body)
		if err != nil {
			return graph, err
		}
		seal := capsule.Seal
		if err := verifyOriginalRegistrySignatureV1(ctx, verifier, seal.AuthorityKeyID, seal.AuthorityPublicKey, seal.AuthoritySignature, domainevidence.EvidenceRegistryAuthoritySealSigningBytes(seal)); err != nil {
			return graph, err
		}
		if capsuleKind {
			digest := strings.TrimSuffix(base, ".json")
			if !domainsecurity.IsSHA256Hex(digest) || digest != domainsecurity.SHA256Hex(entry.Body) {
				return graph, errors.New("original registry capsule address is invalid")
			}
			graph.capsules[digest] = capsule
		} else {
			key := strings.TrimSuffix(base, ".head.json")
			if key != domainevidence.EvidenceRegistryProjectionKey(capsule.SecurityContext.ThreadID, capsule.SecurityContext.TurnID) {
				return graph, errors.New("original registry head address is invalid")
			}
			graph.heads[key] = capsule
		}
	}
	if len(files) > 0 {
		if root, found := files["."]; !found || !root.Directory {
			return graph, errors.New("original registry root is invalid")
		}
	}
	return graph, ctx.Err()
}

// ParseOriginalLegacyInventoryV1 validates a complete original or candidate
// V1 graph against the entire frozen primary context denominator. It retains
// historical contexts accepted by the existing V1 parser; it never supplies
// the witnessed V2 head or current execution authority.
func ParseOriginalLegacyInventoryV1(ctx context.Context, files map[string]OriginalLegacyEntryV1, contexts []domainsecurity.TurnSecurityContext, verifier finalauthorityport.Verifier) ([]registryport.InventoryRecord, error) {
	graph, err := parseOriginalLegacyFilesV1(ctx, files, verifier)
	if err != nil {
		return nil, err
	}
	byIdentity := map[string]domainsecurity.TurnSecurityContext{}
	for _, frozen := range contexts {
		if domainsecurity.ValidateTurnSecurityContext(frozen) != nil {
			return nil, errors.New("original registry inventory context is invalid")
		}
		identity := frozen.ThreadID + "\x00" + frozen.TurnID
		if _, found := byIdentity[identity]; found {
			return nil, errors.New("original registry inventory contains a duplicate context")
		}
		byIdentity[identity] = frozen
	}
	currentByIdentity := map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}
	currentByProjection := map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}
	records := []registryport.InventoryRecord{}
	if graph.index != nil {
		for _, entry := range graph.index.Entries {
			frozen, found := byIdentity[entry.ThreadID+"\x00"+entry.TurnID]
			capsule, capsuleFound := graph.capsules[entry.CapsuleSHA256]
			if !found || !capsuleFound || capsule.SecurityContext != frozen || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsule(*graph.index, entry, capsule) {
				return nil, errors.New("original registry index lost its exact capsule or durable context")
			}
			currentByIdentity[entry.ThreadID+"\x00"+entry.TurnID] = capsule
			currentByProjection[entry.ProjectionKey] = capsule
			records = append(records, registryport.InventoryRecord{Context: frozen, Registry: capsule.Registry})
		}
	}
	for _, capsule := range graph.capsules {
		current, found := currentByIdentity[capsule.Registry.ThreadID+"\x00"+capsule.Registry.TurnID]
		if !found || !authorityCapsuleIsProjectionOf(current, capsule) {
			return nil, errors.New("original registry capsule is orphaned or proves a rollback or divergent chain")
		}
	}
	for key, head := range graph.heads {
		current, found := currentByProjection[key]
		if !found || !authorityCapsuleIsProjectionOf(current, head) {
			return nil, errors.New("original registry head diverges from current signed index")
		}
	}
	for key, projection := range graph.ledgers {
		current, found := currentByProjection[key]
		if !found {
			return nil, errors.New("original registry ledger has no current signed index")
		}
		ledger, err := domainevidence.CanonicalEvidenceRegistryLedger(current.Registry)
		if err != nil {
			return nil, err
		}
		if len(projection) > len(ledger) || !bytes.Equal(projection, ledger[:len(projection)]) || len(projection) > 0 && projection[len(projection)-1] != '\n' {
			return nil, errors.New("original registry ledger is not a complete-line canonical prefix")
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Context.ThreadID != records[j].Context.ThreadID {
			return records[i].Context.ThreadID < records[j].Context.ThreadID
		}
		return records[i].Context.TurnID < records[j].Context.TurnID
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
