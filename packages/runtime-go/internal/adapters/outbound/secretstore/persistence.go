package secretstore

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	persistentFormatVersion = 1
	lifecycleActive         = "active"
	lifecycleTombstoned     = "tombstoned"
	maxPersistentStoreBytes = 64 << 20
)

type persistentDocument struct {
	Version int                         `json:"version"`
	Records map[string]persistentRecord `json:"records"`
}

type persistentRecord struct {
	Purpose   string            `json:"purpose"`
	Lifecycle string            `json:"lifecycle"`
	Envelope  encryptedEnvelope `json:"envelope"`
}

type persistenceBackend interface {
	Load(string) (persistentDocument, error)
	Commit(string, persistentDocument) error
}

type atomicCommitHooks struct {
	beforeReplace                         func() error
	afterReplaceBeforeDirectorySync       func() error
	afterCommitMarkerLink                 func() error
	afterRollbackRequiredRemoval          func() error
	beforeRollbackRequiredReestablishment func() error
}

type filePersistence struct {
	atomicHooks *atomicCommitHooks
}

func (filePersistence) Load(path string) (persistentDocument, error) {
	if err := recoverPrivateFileCommit(path); err != nil {
		return persistentDocument{}, portsecretstore.ErrPersistence
	}
	content, err := readPrivateCommittedFile(path, maxPersistentStoreBytes)
	if errors.Is(err, os.ErrNotExist) {
		return newPersistentDocument(), nil
	}
	if err != nil {
		return persistentDocument{}, portsecretstore.ErrPersistence
	}
	defer clearBytes(content)

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var document persistentDocument
	if err := decoder.Decode(&document); err != nil {
		return persistentDocument{}, portsecretstore.ErrPersistence
	}
	if err := requireJSONEOF(decoder); err != nil {
		return persistentDocument{}, portsecretstore.ErrPersistence
	}
	if err := validatePersistentDocument(document); err != nil {
		return persistentDocument{}, portsecretstore.ErrPersistence
	}
	return document, nil
}

func (persistence filePersistence) Commit(path string, document persistentDocument) error {
	if err := validatePersistentDocument(document); err != nil {
		return portsecretstore.ErrPersistence
	}
	content, err := json.Marshal(document)
	if err != nil {
		return portsecretstore.ErrPersistence
	}
	content = append(content, '\n')
	defer clearBytes(content)
	if err := writePrivateFileAtomically(path, content, persistence.atomicHooks); err != nil {
		return portsecretstore.ErrPersistence
	}
	return nil
}

func newPersistentDocument() persistentDocument {
	return persistentDocument{Version: persistentFormatVersion, Records: make(map[string]persistentRecord)}
}

func validatePersistentDocument(document persistentDocument) error {
	if document.Version != persistentFormatVersion || document.Records == nil {
		return portsecretstore.ErrPersistence
	}
	for rawRef, record := range document.Records {
		ref := portsecretstore.CredentialRef(rawRef)
		if err := portsecretstore.ValidateCredentialRef(ref); err != nil {
			return portsecretstore.ErrPersistence
		}
		purpose, err := portsecretstore.NormalizePurpose(record.Purpose)
		if err != nil || string(purpose) != record.Purpose {
			return portsecretstore.ErrPersistence
		}
		if record.Lifecycle != lifecycleActive && record.Lifecycle != lifecycleTombstoned {
			return portsecretstore.ErrPersistence
		}
		if record.Envelope.Version != envelopeVersion {
			return portsecretstore.ErrPersistence
		}
		nonce, err := base64.RawStdEncoding.Strict().DecodeString(record.Envelope.Nonce)
		if err != nil || len(nonce) != 12 {
			clearBytes(nonce)
			return portsecretstore.ErrPersistence
		}
		clearBytes(nonce)
		ciphertext, err := base64.RawStdEncoding.Strict().DecodeString(record.Envelope.Ciphertext)
		if err != nil || len(ciphertext) < 16 {
			clearBytes(ciphertext)
			return portsecretstore.ErrPersistence
		}
		clearBytes(ciphertext)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return portsecretstore.ErrPersistence
}
