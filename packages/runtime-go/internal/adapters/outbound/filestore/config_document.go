package filestore

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"

	domainruntimeconfig "analytix.local/runtime-go/internal/domain/runtimeconfig"
)

type RuntimeConfigSnapshotV1 struct {
	raw     []byte
	digest  [sha256.Size]byte
	present bool
}

func LoadRuntimeConfigSnapshotV1(jsonText string, path string) (RuntimeConfigSnapshotV1, error) {
	var raw []byte
	if jsonText != "" {
		if len(jsonText) > domainruntimeconfig.MaxJSONDocumentBytesV1 {
			return RuntimeConfigSnapshotV1{}, errors.New("runtime configuration JSON size is invalid")
		}
		if strings.TrimSpace(jsonText) != "" {
			raw = []byte(jsonText)
		}
	}
	if len(raw) > 0 {
		// The inline document is the authoritative source when present.
	} else if strings.TrimSpace(path) != "" {
		var present bool
		var err error
		raw, present, err = readRuntimeConfigPathSnapshotV1(path, int64(domainruntimeconfig.MaxJSONDocumentBytesV1))
		if err != nil {
			return RuntimeConfigSnapshotV1{}, err
		}
		if !present {
			return RuntimeConfigSnapshotV1{}, nil
		}
	} else {
		return RuntimeConfigSnapshotV1{}, nil
	}
	normalized, err := domainruntimeconfig.NormalizeAndValidateJSONDocumentV1(raw)
	if err != nil {
		return RuntimeConfigSnapshotV1{}, err
	}
	return RuntimeConfigSnapshotV1{raw: normalized, digest: sha256.Sum256(normalized), present: true}, nil
}

func (snapshot RuntimeConfigSnapshotV1) Present() bool {
	return snapshot.present
}

func (snapshot RuntimeConfigSnapshotV1) Digest() [sha256.Size]byte {
	return snapshot.digest
}

func (snapshot RuntimeConfigSnapshotV1) Bytes() []byte {
	if !snapshot.present {
		return nil
	}
	return append([]byte(nil), snapshot.raw...)
}

func (snapshot RuntimeConfigSnapshotV1) Document() (map[string]any, bool, error) {
	if !snapshot.present {
		return nil, false, nil
	}
	var document map[string]any
	if err := json.Unmarshal(snapshot.raw, &document); err != nil {
		return nil, false, err
	}
	return document, true, nil
}

func RuntimeConfigDocument(jsonText string, path string) (map[string]any, bool, error) {
	snapshot, err := LoadRuntimeConfigSnapshotV1(jsonText, path)
	if err != nil {
		return nil, false, err
	}
	return snapshot.Document()
}
