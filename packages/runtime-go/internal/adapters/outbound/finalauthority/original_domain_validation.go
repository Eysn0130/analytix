package finalauthority

import (
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	"context"
	"encoding/base64"
	"errors"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ValidateOriginalDomainEntriesV1 reuses an owner's pure domain validator on
// complete native original or authenticated journal endpoint bytes. It grants
// no store or recovery authority. Physical grammar, current installation and
// cancellation failures never become domain-unavailable results.
func ValidateOriginalDomainEntriesV1(ctx context.Context, files map[string]SecurePrivateCASOriginalEntryV1, leaves []SecurePrivateCASOwnerLeafV1, authority *AnchoredFileAuthority, localCheck func(string, string) error, creates *PreparedSecurePrivateCASOriginalCreateResiduesV1, validate func(PrivateCASDomainVisitor, PrivateCASDomainMaterialVisitor, func(string, string) error) error) (resultErr error) {
	if ctx == nil || validate == nil {
		return errors.New("original domain validation is unavailable")
	}
	// Authentication and domain visitors are callbacks. Freeze caller-owned
	// bytes before invoking any of them so a successful validation and a
	// subsequent detached reader capture refer to the same complete input.
	frozen := make(map[string]SecurePrivateCASOriginalEntryV1, len(files))
	for name, entry := range files {
		entry.Body = append([]byte(nil), entry.Body...)
		frozen[name] = entry
	}
	files = frozen
	revalidate := func() error {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		if creates != nil {
			if err := creates.Revalidate(ctx); err != nil {
				return err
			}
		}
		if authority != nil {
			return authority.ValidateCurrentInstallation(ctx)
		}
		return nil
	}
	if err := revalidate(); err != nil {
		return err
	}
	defer func() {
		if err := revalidate(); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if creates != nil {
		filtered := make(map[string]SecurePrivateCASOriginalEntryV1, len(files))
		for name, entry := range files {
			filtered[name] = entry
		}
		for _, state := range creates.DirectoryStatesV1() {
			absolute := filepath.Join(creates.DataRootV1(), filepath.FromSlash(state.RelativePath))
			if !strings.HasPrefix(absolute, creates.OwnerRootV1()+string(filepath.Separator)) {
				continue
			}
			name := filepath.ToSlash(strings.TrimPrefix(absolute, creates.OwnerRootV1()+string(filepath.Separator)))
			if entry, found := filtered[name]; found {
				if !entry.Directory || len(entry.Body) != 0 || entry.Mode != state.Mode&0o777 {
					return errors.New("original domain creation directory changed")
				}
				delete(filtered, name)
			}
		}
		files = filtered
	}
	emptyPartial, err := originalDomainEmptyPartialV1(ctx, files, leaves)
	if err != nil {
		return err
	}
	var committed map[string]map[string][]byte
	if emptyPartial {
		committed = map[string]map[string][]byte{}
		for _, leaf := range leaves {
			committed[leaf.Name] = map[string][]byte{}
		}
	} else {
		committed, err = ValidateOriginalFixedOwnerEntriesV1(ctx, files, leaves)
	}
	if err != nil {
		return err
	}
	check := localCheck
	if authority != nil {
		keyID, publicKey := authority.KeyID(), base64.RawURLEncoding.EncodeToString(authority.PublicKey())
		check = func(recordKeyID, recordPublicKey string) error {
			if recordKeyID != keyID || recordPublicKey != publicKey {
				return errors.New("private domain record belongs to another installation")
			}
			return nil
		}
	}
	visit := func(leaf string, read func(SecurePrivateCASFile) error) error {
		records, found := committed[leaf]
		if !found || read == nil {
			return errors.New("original domain leaf is unavailable")
		}
		digests := make([]string, 0, len(records))
		for digest := range records {
			digests = append(digests, digest)
		}
		sort.Strings(digests)
		for _, digest := range digests {
			if err := context.Cause(ctx); err != nil {
				return err
			}
			if err := read(SecurePrivateCASFile{Digest: digest, Body: records[digest]}); err != nil {
				return err
			}
		}
		return nil
	}
	materials := func(leaf string, read func(SecurePrivateCASPreparedMaterialV1) error) error {
		return visit(leaf, func(file SecurePrivateCASFile) error {
			return read(SecurePrivateCASPreparedMaterialV1{Digest: file.Digest, BodySHA256: domainsecurity.SHA256Hex(file.Body), ByteLength: uint64(len(file.Body))})
		})
	}
	domainErr := validate(visit, materials, check)
	if err := revalidate(); err != nil {
		return err
	}
	if errors.Is(domainErr, context.Canceled) || errors.Is(domainErr, context.DeadlineExceeded) {
		return domainErr
	}
	if domainErr != nil {
		return &DomainRecordUnavailableError{cause: domainErr}
	}
	return nil
}

// A physically observed or signed endpoint may be proper recursive-empty
// partial topology. Preserve the missing leaves as such; this branch grants
// neither a complete owner inventory nor writable store authority.
func originalDomainEmptyPartialV1(ctx context.Context, files map[string]SecurePrivateCASOriginalEntryV1, leaves []SecurePrivateCASOwnerLeafV1) (bool, error) {
	if len(files) == 0 {
		return false, nil
	}
	known := map[string]bool{}
	partial := false
	for _, leaf := range leaves {
		known[leaf.Name] = true
		if _, found := files[leaf.Name]; !found {
			partial = true
		}
	}
	if !partial {
		return false, nil
	}
	for name, entry := range files {
		if err := context.Cause(ctx); err != nil {
			return false, err
		}
		if !entry.Directory || len(entry.Body) != 0 {
			return false, nil
		}
		if name == "" || path.Clean(name) != name || path.IsAbs(name) || strings.Contains(name, "\\") || entry.Mode == 0 || entry.Mode & ^uint32(0o777) != 0 || runtime.GOOS != "windows" && entry.Mode&0o077 != 0 {
			return false, errors.New("original empty partial directory is invalid")
		}
		if name == "." {
			continue
		}
		parts := strings.Split(name, "/")
		if !known[parts[0]] || len(parts) > 2 || len(parts) == 2 && !domainprivatecas.ValidShardV1(parts[1]) {
			return false, errors.New("original empty partial directory is outside fixed grammar")
		}
		if parent, found := files[path.Dir(name)]; !found || !parent.Directory {
			return false, errors.New("original empty partial parent is absent")
		}
	}
	if root, found := files["."]; !found || !root.Directory {
		return false, errors.New("original empty partial root is absent")
	}
	return true, nil
}
