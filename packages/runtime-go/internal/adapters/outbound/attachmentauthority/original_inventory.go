package attachmentauthority

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type OriginalEntryV1 struct {
	Directory bool
	Mode      uint32
	Body      []byte
}

type OriginalInventoryV1 struct {
	Owners             []domainattachment.OwnerRecordV1
	UploadIntents      []domainattachment.UploadIntentV1
	UploadDispositions []domainattachment.UploadDispositionV1
	UseReceipts        []domainattachment.AttachmentUseReceiptV1
	UseDispositions    []domainattachment.AttachmentUseDispositionV1
}

var originalAttachmentLeafLimitsV1 = map[string]int{
	"owners":              domainattachment.MaxOwnerRecordBytesV1,
	"upload-intents":      domainattachment.MaxAttachmentUploadRecordBytesV1,
	"upload-dispositions": domainattachment.MaxAttachmentUploadRecordBytesV1,
	"use-receipts":        domainattachment.MaxAttachmentUseRecordBytesV1,
	"use-dispositions":    domainattachment.MaxAttachmentUseRecordBytesV1,
}

type originalAttachmentPhysicalOwnerV1 interface {
	Present() bool
	Revalidate(context.Context) error
	VisitCommittedFiles(context.Context, string, func(finalauthorityadapter.SecurePrivateCASFile) error) error
}

// SnapshotOriginalFilesV1 never opens a mutable store or consumes residue.
// Its private physical proof is separate from the caller's non-private Core
// snapshot and original or authenticated semantic Before graph.
func (prepared *PreparedRecoveryV1) SnapshotOriginalFilesV1(ctx context.Context) (files map[string]OriginalEntryV1, resultErr error) {
	if prepared == nil || prepared.owner == nil {
		return nil, errors.New("original attachment physical owner is unavailable")
	}
	return snapshotOriginalAttachmentFilesV1(ctx, prepared.root, prepared.owner)
}

func snapshotOriginalAttachmentFilesV1(ctx context.Context, root string, owner originalAttachmentPhysicalOwnerV1) (files map[string]OriginalEntryV1, resultErr error) {
	if ctx == nil {
		return nil, errors.New("original attachment context is unavailable")
	}
	if err := owner.Revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, owner.Revalidate(ctx), ctx.Err())
		if resultErr != nil {
			files = nil
		}
	}()
	files = map[string]OriginalEntryV1{}
	if !owner.Present() {
		return files, nil
	}
	committed := map[string][]byte{}
	for leaf := range originalAttachmentLeafLimitsV1 {
		if err := owner.VisitCommittedFiles(ctx, leaf, func(file finalauthorityadapter.SecurePrivateCASFile) error {
			committed[leaf+"/"+file.Digest[:2]+"/"+file.Digest+".json"] = append([]byte(nil), file.Body...)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	var total int64
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(files) >= domainstartup.MaxManagedSnapshotEntriesV1 {
			return errors.New("original attachment entry budget exceeded")
		}
		before, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		value := OriginalEntryV1{Directory: before.IsDir(), Mode: uint32(before.Mode().Perm())}
		if !value.Directory {
			if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > domainstartup.MaxSemanticManagedFileBytesV1 {
				return errors.New("original attachment file kind or size is invalid")
			}
			total += before.Size()
			if total > domainstartup.MaxSemanticStagedTotalBytesV1 {
				return errors.New("original attachment byte budget exceeded")
			}
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(before, opened) || before.Mode() != opened.Mode() || before.Size() != opened.Size() {
				return errors.Join(errors.New("original attachment file changed during open"), statErr, file.Close())
			}
			body, readErr := io.ReadAll(io.LimitReader(file, before.Size()+1))
			after, statErr := file.Stat()
			closeErr := file.Close()
			if readErr != nil || statErr != nil || closeErr != nil {
				return errors.Join(readErr, statErr, closeErr)
			}
			if !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(body)) != before.Size() {
				return errors.New("original attachment file changed during read")
			}
			if bound, ok := committed[relative]; ok && !bytes.Equal(bound, body) {
				return errors.New("original attachment bytes differ from the bound CAS observation")
			}
			value.Body = body
		}
		files[relative] = value
		return nil
	})
	if err != nil {
		return nil, err
	}
	for name, body := range committed {
		if value, ok := files[name]; !ok || value.Directory || !bytes.Equal(body, value.Body) {
			return nil, errors.New("original attachment committed inventory is incomplete")
		}
	}
	return files, nil
}

// ParseOriginalInventoryV1 validates a complete historical graph with a
// verification-only installation key. It neither signs nor validates a current
// execution grant. Residue stays opaque and remains in the caller's raw set.
func ParseOriginalInventoryV1(ctx context.Context, files map[string]OriginalEntryV1, verifier finalauthorityport.Verifier) (OriginalInventoryV1, error) {
	return parseOriginalInventoryV1(ctx, files, verifier, true)
}

// ParseOriginalEndpointV1 accepts an original or authenticated semantic
// endpoint while a real, complete five-leaf physical observation is held.
// Missing directories remain absence, not reconstructed records or permission
// to create/recover anything. The caller verifies the signed Before/After
// mapping; ordinary standalone parsing still requires complete topology.
func (prepared *PreparedOriginalInventoryV1) ParseOriginalEndpointV1(ctx context.Context, files map[string]OriginalEntryV1, verifier finalauthorityport.Verifier) (inventory OriginalInventoryV1, resultErr error) {
	if prepared == nil || len(prepared.leaves) != len(originalAttachmentLeafLimitsV1) {
		return inventory, errors.New("original attachment physical leaf denominator is incomplete")
	}
	complete := true
	for name := range originalAttachmentLeafLimitsV1 {
		if leaf := prepared.leaves[name]; leaf == nil || leaf.RootPath() != filepath.Join(prepared.root, name) {
			return inventory, errors.New("original attachment physical leaf denominator is incomplete")
		}
		if entry, exists := files[name]; len(files) != 0 && (!exists || !entry.Directory) {
			complete = false
		}
	}
	if complete {
		// Complete endpoints retain the existing pure parser. Runtime's whole
		// observation still brackets the caller; only partial admission needs
		// the additional explicit physical evidence in this method.
		return ParseOriginalInventoryV1(ctx, files, verifier)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return inventory, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.Revalidate(ctx))
		if resultErr != nil {
			inventory = OriginalInventoryV1{}
		}
	}()
	return parseOriginalInventoryV1(ctx, files, verifier, false)
}

func parseOriginalInventoryV1(ctx context.Context, files map[string]OriginalEntryV1, verifier finalauthorityport.Verifier, requireComplete bool) (inventory OriginalInventoryV1, resultErr error) {
	if ctx == nil {
		return inventory, errors.New("original attachment context is unavailable")
	}
	defer func() {
		resultErr = errors.Join(resultErr, ctx.Err())
		if resultErr != nil {
			inventory = OriginalInventoryV1{}
		}
	}()
	if requireComplete && len(files) != 0 {
		for leaf := range originalAttachmentLeafLimitsV1 {
			if entry, present := files[leaf]; !present || !entry.Directory {
				return inventory, errors.New("complete original attachment leaf is absent")
			}
		}
	}
	hasChildren := make(map[string]bool, len(files))
	for name := range files {
		if name != "." {
			hasChildren[path.Dir(name)] = true
		}
	}
	names := make([]string, 0, len(files))
	var total int64
	for name, entry := range files {
		if err := ctx.Err(); err != nil {
			return inventory, err
		}
		if len(files) > domainstartup.MaxManagedSnapshotEntriesV1 || name == "" || path.Clean(name) != name || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || entry.Mode == 0 || entry.Mode & ^uint32(0o777) != 0 || runtime.GOOS != "windows" && entry.Mode&0o077 != 0 {
			return inventory, errors.New("original attachment address or mode is invalid")
		}
		if name != "." {
			if parent, ok := files[path.Dir(name)]; !ok || !parent.Directory {
				return inventory, errors.New("original attachment parent is absent")
			}
		}
		parts := strings.Split(name, "/")
		if name == "." {
			if !entry.Directory || len(entry.Body) != 0 {
				return inventory, errors.New("original attachment root is not a directory")
			}
			continue
		}
		if originalAttachmentCreateDirectoryV1(name) {
			if !entry.Directory || len(entry.Body) != 0 {
				return inventory, errors.New("original attachment creation residue is not an empty directory")
			}
			if hasChildren[name] {
				return inventory, errors.New("original attachment creation residue contains an entry")
			}
			continue
		}
		limit, known := originalAttachmentLeafLimitsV1[parts[0]]
		if !known || len(parts) > 3 || (len(parts) >= 2 && !domainprivatecas.ValidShardV1(parts[1])) {
			return inventory, errors.New("original attachment target grammar is invalid")
		}
		if len(parts) < 3 {
			if !entry.Directory || len(entry.Body) != 0 {
				return inventory, errors.New("original attachment directory is invalid")
			}
			continue
		}
		if entry.Directory || len(entry.Body) > limit {
			return inventory, errors.New("original attachment record kind or size is invalid")
		}
		total += int64(len(entry.Body))
		if total > domainstartup.MaxSemanticStagedTotalBytesV1 {
			return inventory, errors.New("original attachment byte budget exceeded")
		}
		if _, residue := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); residue {
			continue
		}
		digest := strings.TrimSuffix(parts[2], ".json")
		if !domainprivatecas.ValidDigestV1(digest) || parts[2] != digest+".json" || digest[:2] != parts[1] {
			return inventory, errors.New("original attachment record address is invalid")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	visit := func(leaf string, visitor func(finalauthorityadapter.SecurePrivateCASFile) error) error {
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			if strings.HasPrefix(name, leaf+"/") {
				if err := visitor(finalauthorityadapter.SecurePrivateCASFile{Digest: strings.TrimSuffix(path.Base(name), ".json"), Body: files[name].Body}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := validateOriginalAttachmentRecordsV1(ctx, visit); err != nil {
		return inventory, err
	}
	for _, name := range names {
		body := files[name].Body
		switch strings.Split(name, "/")[0] {
		case "owners":
			record, err := parseCanonicalOwner(body)
			if err != nil {
				return inventory, err
			}
			inventory.Owners = append(inventory.Owners, record)
		case "upload-intents":
			record, err := parseCanonicalUploadIntent(body)
			if err != nil {
				return inventory, err
			}
			inventory.UploadIntents = append(inventory.UploadIntents, record)
		case "upload-dispositions":
			record, err := parseCanonicalUploadDisposition(body)
			if err != nil {
				return inventory, err
			}
			inventory.UploadDispositions = append(inventory.UploadDispositions, record)
		case "use-receipts":
			record, err := domainattachment.ParseAttachmentUseReceiptV1(body)
			if err != nil {
				return inventory, err
			}
			keyID, publicKey, signature, err := domainattachment.AttachmentUseReceiptV1AuthorityMaterial(record)
			if err != nil {
				return inventory, err
			}
			if verifier == nil {
				return inventory, errors.New("original attachment verifier is unavailable")
			}
			if err := verifier.VerifyTrusted(ctx, keyID, publicKey, domainattachment.AttachmentUseReceiptV1SigningBytes(record), signature); err != nil {
				return inventory, err
			}
			inventory.UseReceipts = append(inventory.UseReceipts, record)
		case "use-dispositions":
			record, err := domainattachment.ParseAttachmentUseDispositionV1(body)
			if err != nil {
				return inventory, err
			}
			keyID, publicKey, signature, err := domainattachment.AttachmentUseDispositionV1AuthorityMaterial(record)
			if err != nil {
				return inventory, err
			}
			if verifier == nil {
				return inventory, errors.New("original attachment verifier is unavailable")
			}
			if err := verifier.VerifyTrusted(ctx, keyID, publicKey, domainattachment.AttachmentUseDispositionV1SigningBytes(record), signature); err != nil {
				return inventory, err
			}
			inventory.UseDispositions = append(inventory.UseDispositions, record)
		}
	}
	return inventory, nil
}

func originalAttachmentCreateDirectoryV1(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		for leaf := range originalAttachmentLeafLimitsV1 {
			if name == domainprivatecas.CreateDirectoryResidueNameV1(leaf) {
				return true
			}
		}
	}
	if len(parts) == 2 {
		_, known := originalAttachmentLeafLimitsV1[parts[0]]
		return known && domainprivatecas.CreateDirectoryResidueMatchesShardV1(parts[1])
	}
	return false
}
