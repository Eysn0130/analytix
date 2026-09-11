package filestore

import (
	"encoding/json"
	"errors"
	"strings"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// ValidOriginalAttachmentFileV1 is the same address grammar used by live
// reconciliation, without opening a store or interpreting ownerless bytes.
func ValidOriginalAttachmentFileV1(leaf, name string) bool {
	suffix := ".json"
	if leaf == "content" {
		suffix = ".bin"
	} else if leaf != "metadata" {
		return false
	}
	id := strings.TrimSuffix(name, suffix)
	return validAttachmentID(id) && id+suffix == name
}

// ParseOriginalAttachmentMetadataV1 validates bytes already bound to an
// original physical snapshot. It performs no current binding lookup or write.
func ParseOriginalAttachmentMetadataV1(id string, body []byte) (domainattachment.OwnerRecordV1, error) {
	if !validAttachmentID(id) {
		return domainattachment.OwnerRecordV1{}, ErrAttachmentContentIntegrity
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: int(domainstartup.MaxSemanticManagedFileBytesV1), MaxDepth: 32, MaxTokens: 100000, MaxStringBytes: int(domainstartup.MaxSemanticManagedFileBytesV1)}); err != nil {
		return domainattachment.OwnerRecordV1{}, err
	}
	var metadata map[string]any
	if err := json.Unmarshal(body, &metadata); err != nil {
		return domainattachment.OwnerRecordV1{}, err
	}
	owner, err := validateAttachmentMetadataIntegrity(id, metadata)
	if err != nil {
		return domainattachment.OwnerRecordV1{}, errors.Join(ErrAttachmentContentIntegrity, err)
	}
	return owner, nil
}
