package evidenceregistry

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	evidenceRegistryAuthorityIndexFile     = ".registry-authority-index.json"
	evidenceRegistryCapsuleDirectory       = ".registry-capsules"
	evidenceRegistryProjectionDirectory    = ".registry-projections"
	maxEvidenceRegistryAuthorityIndexBytes = 64 * 1024 * 1024
)

func (s *Store) authorityIndexPath() string {
	return filepath.Join(s.root, evidenceRegistryAuthorityIndexFile)
}

func (s *Store) authorityCapsuleDirectory() string {
	return filepath.Join(s.root, evidenceRegistryCapsuleDirectory)
}

func (s *Store) authorityCapsuleBlobPath(digest string) string {
	return filepath.Join(s.authorityCapsuleDirectory(), strings.TrimSpace(digest)+".json")
}

func (s *Store) readAuthorityIndexLocked() (domainevidence.EvidenceRegistryAuthorityIndex, error) {
	path := s.authorityIndexPath()
	before, err := validateRegistryRegularFile(path)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, err
	}
	if before.Size() <= 0 || before.Size() > maxEvidenceRegistryAuthorityIndexBytes {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, errors.New("evidence registry authority index size is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, err
	}
	defer file.Close()
	if err := validateOpenedRegistryFile(file, before); err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, maxEvidenceRegistryAuthorityIndexBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxEvidenceRegistryAuthorityIndexBytes {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, errors.Join(errors.New("evidence registry authority index body is invalid"), err)
	}
	index, err := domainevidence.ParseEvidenceRegistryAuthorityIndex(body)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if publicErr != nil || signatureErr != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, errors.Join(errors.New("evidence registry authority index encoding is invalid"), publicErr, signatureErr)
	}
	if err := s.authority.VerifyTrusted(nil, index.AuthorityKeyID, publicKey, domainevidence.EvidenceRegistryAuthorityIndexSigningBytes(index), signature); err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, errors.Join(errors.New("evidence registry authority index is not trusted by this installation"), err)
	}
	return index, nil
}

func (s *Store) readAuthorityCapsuleBlobLocked(index domainevidence.EvidenceRegistryAuthorityIndex, entry domainevidence.EvidenceRegistryAuthorityIndexEntry) (domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	if !domainsecurity.IsSHA256Hex(entry.CapsuleSHA256) || entry.CapsuleByteLength == 0 || entry.CapsuleByteLength > maxEvidenceRegistryAuthorityCapsuleBytes {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule reference is invalid")
	}
	path := s.authorityCapsuleBlobPath(entry.CapsuleSHA256)
	body, _, err := s.readExactAuthorityCapsuleBlobLocked(path, int(entry.CapsuleByteLength))
	if err != nil || uint64(len(body)) != entry.CapsuleByteLength || domainsecurity.SHA256Hex(body) != entry.CapsuleSHA256 {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.Join(errors.New("evidence registry authority capsule blob integrity is invalid"), err)
	}
	capsule, err := s.parseTrustedAuthorityCapsule(body)
	if err != nil || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsule(index, entry, capsule) {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.Join(errors.New("evidence registry authority capsule blob does not match the signed index"), err)
	}
	return capsule, nil
}

func (s *Store) parseTrustedAuthorityCapsule(body []byte) (domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(body)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	seal := capsule.Seal
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(seal.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(seal.AuthoritySignature)
	if publicErr != nil || signatureErr != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.Join(errors.New("evidence registry authority capsule encoding is invalid"), publicErr, signatureErr)
	}
	if err := s.authority.VerifyTrusted(nil, seal.AuthorityKeyID, publicKey, domainevidence.EvidenceRegistryAuthoritySealSigningBytes(seal), signature); err != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.Join(errors.New("evidence registry authority capsule is not trusted by this installation"), err)
	}
	return capsule, nil
}

func (s *Store) currentAuthorityCapsuleLocked(securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceRegistryAuthorityIndex, domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	index, err := s.readAuthorityIndexLocked()
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	entry, ok := domainevidence.EvidenceRegistryAuthorityIndexEntryForContext(index, securityContext)
	if !ok {
		return index, domainevidence.EvidenceRegistryAuthorityCapsule{}, os.ErrNotExist
	}
	capsule, err := s.readAuthorityCapsuleBlobLocked(index, entry)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndex{}, domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	return index, capsule, nil
}

func (s *Store) writeAuthorityCapsuleBlobLocked(capsule domainevidence.EvidenceRegistryAuthorityCapsule) (domainevidence.EvidenceRegistryAuthorityIndexEntry, error) {
	entry, err := domainevidence.NewEvidenceRegistryAuthorityIndexEntry(capsule)
	if err != nil || entry.CapsuleByteLength > maxEvidenceRegistryAuthorityCapsuleBytes {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, errors.New("evidence registry authority capsule blob is invalid")
	}
	body, err := domainevidence.CanonicalEvidenceRegistryAuthorityCapsuleBytes(capsule)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
	}
	dir := s.authorityCapsuleDirectory()
	if err := ensureRegistryDirectory(dir); err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
	}
	path := s.authorityCapsuleBlobPath(entry.CapsuleSHA256)
	if existing, _, err := s.readExactAuthorityCapsuleBlobLocked(path, len(body)); err == nil {
		if !bytes.Equal(existing, body) {
			return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, errors.New("evidence registry capsule digest path contains different bytes")
		}
		return entry, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
	}
	temp, err := os.CreateTemp(dir, ".capsule-*.tmp")
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
	}
	written, err := temp.Write(body)
	if err != nil || written != len(body) {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, errors.New("evidence registry authority capsule blob write was incomplete")
	}
	if err := temp.Sync(); err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
	}
	if err := temp.Close(); err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
	}
	closed = true
	if err := os.Link(tempPath, path); err != nil {
		if existing, _, readErr := s.readExactAuthorityCapsuleBlobLocked(path, len(body)); readErr != nil || !bytes.Equal(existing, body) {
			return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, err
		}
	}
	cleanupInstalledFailure := func(cause error) (domainevidence.EvidenceRegistryAuthorityIndexEntry, error) {
		if cleanupErr := s.cleanupUncommittedAuthorityCapsuleLocked(path, tempPath, body); cleanupErr != nil {
			s.poisoned = errors.Join(cause, errors.New("uncommitted evidence registry capsule cleanup failed"), cleanupErr)
			return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, s.poisoned
		}
		return domainevidence.EvidenceRegistryAuthorityIndexEntry{}, cause
	}
	if s.faults != nil && s.faults.AfterAuthorityCapsuleInstall != nil {
		if err := s.faults.AfterAuthorityCapsuleInstall(); err != nil {
			return cleanupInstalledFailure(err)
		}
	}
	if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cleanupInstalledFailure(err)
	}
	if err := syncRegistryDirectory(dir); err != nil {
		return cleanupInstalledFailure(err)
	}
	readback, _, err := s.readExactAuthorityCapsuleBlobLocked(path, len(body))
	if err != nil || !bytes.Equal(readback, body) {
		return cleanupInstalledFailure(errors.New("evidence registry authority capsule blob readback failed"))
	}
	return entry, nil
}

func (s *Store) cleanupUncommittedAuthorityCapsuleLocked(path, tempPath string, expected []byte) error {
	digest := strings.TrimSuffix(filepath.Base(path), ".json")
	if !domainsecurity.IsSHA256Hex(digest) || domainsecurity.SHA256Hex(expected) != digest {
		return errors.New("uncommitted evidence registry capsule cleanup identity is invalid")
	}
	if index, err := s.readAuthorityIndexLocked(); err == nil {
		for _, entry := range index.Entries {
			if entry.CapsuleSHA256 == digest {
				return errors.New("uncommitted capsule cleanup target is already indexed")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if body, linkedTemp, err := s.readExactAuthorityCapsuleBlobLocked(path, len(expected)); err == nil {
		if !bytes.Equal(body, expected) {
			return errors.New("uncommitted capsule cleanup bytes are mismatched")
		}
		if linkedTemp != "" && filepath.Clean(linkedTemp) != filepath.Clean(tempPath) {
			return errors.New("uncommitted capsule cleanup temp identity is mismatched")
		}
		if linkedTemp != "" {
			if err := os.Remove(linkedTemp); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info, err := os.Lstat(tempPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("uncommitted capsule temp cleanup identity is invalid")
		}
		if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncRegistryDirectory(filepath.Dir(path))
}

func (s *Store) writeAuthorityIndexLocked(index domainevidence.EvidenceRegistryAuthorityIndex) error {
	if domainevidence.ValidateEvidenceRegistryAuthorityIndex(index) != nil {
		return errors.New("evidence registry authority index is invalid")
	}
	if err := s.validateAuthorityIndexPredecessorLocked(index); err != nil {
		return err
	}
	body, err := json.Marshal(index)
	if err != nil || len(body) == 0 || len(body) > maxEvidenceRegistryAuthorityIndexBytes {
		return errors.New("evidence registry authority index encoding is invalid")
	}
	path := s.authorityIndexPath()
	if before, err := validateRegistryRegularFile(path); err == nil {
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		validateErr := validateOpenedRegistryFile(file, before)
		closeErr := file.Close()
		if err := errors.Join(validateErr, closeErr); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(s.root, ".registry-authority-index-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	written, err := temp.Write(body)
	if err != nil || written != len(body) {
		return errors.New("evidence registry authority index write was incomplete")
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	closed = true
	if s.faults != nil && s.faults.BeforeAuthorityIndexReplace != nil {
		if err := s.faults.BeforeAuthorityIndexReplace(); err != nil {
			return err
		}
	}
	if err := s.validateAuthorityIndexPredecessorLocked(index); err != nil {
		return err
	}
	if err := atomicReplaceRegistryFile(tempPath, path); err != nil {
		return s.classifyAuthorityIndexCommitFailure(index, err)
	}
	if s.faults != nil && s.faults.AfterAuthorityIndexReplace != nil {
		if err := s.faults.AfterAuthorityIndexReplace(); err != nil {
			return s.classifyAuthorityIndexCommitFailure(index, err)
		}
	}
	if err := syncRegistryDirectory(s.root); err != nil {
		return s.classifyAuthorityIndexCommitFailure(index, err)
	}
	readback, err := s.readAuthorityIndexLocked()
	if err != nil || readback.RecordDigest != index.RecordDigest {
		return s.classifyAuthorityIndexCommitFailure(index, errors.New("evidence registry authority index readback failed"))
	}
	return nil
}

func (s *Store) validateAuthorityIndexPredecessorLocked(target domainevidence.EvidenceRegistryAuthorityIndex) error {
	current, err := s.readAuthorityIndexLocked()
	if target.Generation == 1 {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New("evidence registry genesis index cannot overwrite existing authority")
	}
	if err != nil {
		return err
	}
	if current.RecordDigest != target.PreviousIndexDigest || current.Generation+1 != target.Generation ||
		current.AuthorityKeyID != target.AuthorityKeyID || current.InstallationID != target.InstallationID {
		return errors.New("evidence registry authority index predecessor changed before commit")
	}
	return nil
}

func (s *Store) removeAuthorityCapsuleBlobLocked(entry domainevidence.EvidenceRegistryAuthorityIndexEntry) error {
	if !domainsecurity.IsSHA256Hex(entry.CapsuleSHA256) || entry.CapsuleByteLength == 0 || entry.CapsuleByteLength > maxEvidenceRegistryAuthorityCapsuleBytes {
		return errors.New("evidence registry capsule cleanup reference is invalid")
	}
	path := s.authorityCapsuleBlobPath(entry.CapsuleSHA256)
	body, linkedTemp, err := s.readExactAuthorityCapsuleBlobLocked(path, int(entry.CapsuleByteLength))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || domainsecurity.SHA256Hex(body) != entry.CapsuleSHA256 {
		return errors.New("evidence registry capsule cleanup target is invalid")
	}
	if linkedTemp != "" {
		if err := os.Remove(linkedTemp); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncRegistryDirectory(s.authorityCapsuleDirectory())
}

func (s *Store) classifyAuthorityIndexCommitFailure(target domainevidence.EvidenceRegistryAuthorityIndex, cause error) error {
	readback, err := s.readAuthorityIndexLocked()
	if err == nil && readback.RecordDigest == target.RecordDigest {
		s.poisoned = fmt.Errorf("%w: target generation is visible but durability was not proven: %v", ErrEvidenceRegistryAuthorityCommitIndeterminate, cause)
		return s.poisoned
	}
	if err == nil && target.Generation > 1 && readback.RecordDigest == target.PreviousIndexDigest {
		return cause
	}
	if target.Generation == 1 && errors.Is(err, os.ErrNotExist) {
		return cause
	}
	s.poisoned = fmt.Errorf("%w: current generation cannot be classified after commit failure: %v", ErrEvidenceRegistryAuthorityCommitIndeterminate, cause)
	return s.poisoned
}

func readExactRegistryFile(path string, length int) ([]byte, error) {
	return readExactRegistryFileWithLinkCount(path, length, 1)
}

func readExactRegistryFileWithLinkCount(path string, length int, allowedLinks uint64) ([]byte, error) {
	before, err := validateRegistryRegularFile(path)
	if err != nil {
		return nil, err
	}
	if length <= 0 || before.Size() != int64(length) {
		return nil, errors.New("evidence registry file length is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := validateOpenedRegistryFileWithLinkCount(file, before, allowedLinks); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(length)+1))
	if err != nil || len(body) != length {
		return nil, errors.New("evidence registry file read was incomplete")
	}
	return body, nil
}

func (s *Store) readExactAuthorityCapsuleBlobLocked(path string, length int) ([]byte, string, error) {
	allowedLinks, linkedTemp, err := s.authorityCapsuleLinkStateLocked(path)
	if err != nil {
		return nil, "", err
	}
	body, err := readExactRegistryFileWithLinkCount(path, length, allowedLinks)
	return body, linkedTemp, err
}

func (s *Store) authorityCapsuleLinkStateLocked(path string) (uint64, string, error) {
	before, err := validateRegistryRegularFile(path)
	if err != nil {
		return 0, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	after, statErr := file.Stat()
	if statErr != nil || after == nil || !os.SameFile(before, after) {
		_ = file.Close()
		return 0, "", errors.New("evidence registry capsule blob identity is invalid")
	}
	links, linksErr := registryRegularFileLinkCount(file, after)
	closeErr := file.Close()
	if linksErr != nil || closeErr != nil {
		return 0, "", errors.New("evidence registry capsule blob identity is invalid")
	}
	if links == 1 {
		return 1, "", nil
	}
	if links != 2 {
		return 0, "", errors.New("evidence registry capsule blob has unsafe link authority")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return 0, "", err
	}
	linkedTemp := ""
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".capsule-") || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		tempPath := filepath.Join(filepath.Dir(path), entry.Name())
		tempInfo, err := validateRegistryRegularFile(tempPath)
		if err != nil {
			return 0, "", err
		}
		if !os.SameFile(before, tempInfo) {
			continue
		}
		if linkedTemp != "" {
			return 0, "", errors.New("evidence registry capsule blob has multiple temp aliases")
		}
		linkedTemp = tempPath
	}
	if linkedTemp == "" {
		return 0, "", errors.New("evidence registry capsule blob hardlink has no matching crash temp")
	}
	return 2, linkedTemp, nil
}

func (s *Store) validateAuthorityCapsuleTempLocked(path string) error {
	before, err := validateRegistryRegularFile(path)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	after, statErr := file.Stat()
	if statErr != nil || after == nil || !os.SameFile(before, after) {
		_ = file.Close()
		return errors.New("evidence registry capsule temp identity is invalid")
	}
	links, linksErr := registryRegularFileLinkCount(file, after)
	closeErr := file.Close()
	if linksErr != nil || closeErr != nil {
		return errors.New("evidence registry capsule temp identity is invalid")
	}
	if links == 1 {
		return nil
	}
	if links != 2 {
		return errors.New("evidence registry capsule temp has unsafe link authority")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	matchingBlob := ""
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || !domainsecurity.IsSHA256Hex(strings.TrimSuffix(entry.Name(), ".json")) {
			continue
		}
		blobPath := filepath.Join(filepath.Dir(path), entry.Name())
		blobInfo, err := validateRegistryRegularFile(blobPath)
		if err != nil {
			return err
		}
		if !os.SameFile(before, blobInfo) {
			continue
		}
		if matchingBlob != "" {
			return errors.New("evidence registry capsule temp has multiple blob aliases")
		}
		matchingBlob = blobPath
	}
	if matchingBlob == "" {
		return errors.New("evidence registry capsule temp hardlink has no content-addressed blob")
	}
	allowedLinks, linkedTemp, err := s.authorityCapsuleLinkStateLocked(matchingBlob)
	if err != nil || allowedLinks != 2 || linkedTemp != path {
		return errors.New("evidence registry capsule temp hardlink is not a repairable crash residue")
	}
	return nil
}

func authorityCapsuleIsProjectionOf(current, projected domainevidence.EvidenceRegistryAuthorityCapsule) bool {
	if domainevidence.ValidateEvidenceRegistryAuthorityCapsule(current) != nil || domainevidence.ValidateEvidenceRegistryAuthorityCapsule(projected) != nil ||
		projected.SecurityContext != current.SecurityContext || projected.Seal.AuthorityKeyID != current.Seal.AuthorityKeyID ||
		projected.Seal.AuthorityPublicKey != current.Seal.AuthorityPublicKey || projected.Registry.Sequence > current.Registry.Sequence {
		return false
	}
	currentLedger, currentErr := domainevidence.CanonicalEvidenceRegistryLedger(current.Registry)
	projectedLedger, projectedErr := domainevidence.CanonicalEvidenceRegistryLedger(projected.Registry)
	return currentErr == nil && projectedErr == nil && len(projectedLedger) <= len(currentLedger) && bytes.Equal(projectedLedger, currentLedger[:len(projectedLedger)])
}

func (s *Store) newAuthorityIndexLocked(ctx context.Context, previous *domainevidence.EvidenceRegistryAuthorityIndex, capsule domainevidence.EvidenceRegistryAuthorityCapsule) (domainevidence.EvidenceRegistryAuthorityIndex, error) {
	return domainevidence.NewEvidenceRegistryAuthorityIndex(previous, capsule, s.authority.KeyID(), s.authority.PublicKey(), func(message []byte) ([]byte, error) {
		return s.authority.Sign(ctx, message)
	})
}
