package attachment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	OwnerRecordSchemaVersion = 1
	OwnerRecordPurpose       = "analytix.attachment-owner/v1"
	MaxOwnerRecordBytesV1    = 128 * 1024
)

type OwnerRecordV1 struct {
	SchemaVersion          int                                      `json:"schemaVersion"`
	Purpose                string                                   `json:"purpose"`
	AttachmentID           string                                   `json:"attachmentId"`
	OwnerNonce             string                                   `json:"ownerNonce"`
	BlobSHA256             string                                   `json:"blobSHA256"`
	ByteSize               int64                                    `json:"byteSize"`
	MIMEType               string                                   `json:"mimeType"`
	ThreadID               string                                   `json:"threadId"`
	WorkspaceRealPath      string                                   `json:"workspaceRealPath"`
	CaseBindingObservation *domainsecurity.CaseBindingObservationV1 `json:"caseBindingObservation,omitempty"`
	ProjectionSHA256       string                                   `json:"projectionSHA256"`
	CreatedAt              string                                   `json:"createdAt"`
	OwnerDigest            string                                   `json:"ownerDigest"`
}

type OwnerRecordInputV1 struct {
	OwnerNonce             string
	BlobSHA256             string
	ByteSize               int64
	MIMEType               string
	ThreadID               string
	WorkspaceRealPath      string
	CaseBindingObservation *domainsecurity.CaseBindingObservationV1
	ProjectionSHA256       string
	CreatedAt              time.Time
}

func NewOwnerRecordV1(input OwnerRecordInputV1) (OwnerRecordV1, error) {
	record := OwnerRecordV1{
		SchemaVersion:     OwnerRecordSchemaVersion,
		Purpose:           OwnerRecordPurpose,
		OwnerNonce:        strings.TrimSpace(input.OwnerNonce),
		BlobSHA256:        strings.TrimSpace(input.BlobSHA256),
		ByteSize:          input.ByteSize,
		MIMEType:          strings.TrimSpace(input.MIMEType),
		ThreadID:          strings.TrimSpace(input.ThreadID),
		WorkspaceRealPath: strings.TrimSpace(input.WorkspaceRealPath),
		ProjectionSHA256:  strings.TrimSpace(input.ProjectionSHA256),
		CreatedAt:         input.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if input.CaseBindingObservation != nil {
		observation := *input.CaseBindingObservation
		record.CaseBindingObservation = &observation
	}
	record.OwnerDigest = ownerDigest(record)
	record.AttachmentID = "att_" + record.OwnerDigest[:24]
	if err := ValidateOwnerRecordV1(record); err != nil {
		return OwnerRecordV1{}, err
	}
	return record, nil
}

func ValidateOwnerRecordV1(record OwnerRecordV1) error {
	createdAt, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if record.SchemaVersion != OwnerRecordSchemaVersion || record.Purpose != OwnerRecordPurpose ||
		len(record.OwnerNonce) != 32 || !domainsecurity.IsSHA256Hex(record.BlobSHA256) || record.ByteSize < 0 ||
		record.MIMEType == "" || strings.TrimSpace(record.MIMEType) != record.MIMEType ||
		record.ThreadID == "" || strings.TrimSpace(record.ThreadID) != record.ThreadID ||
		record.WorkspaceRealPath == "" || strings.TrimSpace(record.WorkspaceRealPath) != record.WorkspaceRealPath ||
		!domainsecurity.IsSHA256Hex(record.ProjectionSHA256) || err != nil ||
		createdAt.Location() != time.UTC || createdAt.Format(time.RFC3339Nano) != record.CreatedAt ||
		!domainsecurity.IsSHA256Hex(record.OwnerDigest) || record.OwnerDigest != ownerDigest(record) ||
		record.AttachmentID != "att_"+record.OwnerDigest[:24] {
		return errors.New("attachment owner record integrity is invalid")
	}
	for _, value := range record.OwnerNonce {
		if !strings.ContainsRune("0123456789abcdef", value) {
			return errors.New("attachment owner nonce is invalid")
		}
	}
	if record.CaseBindingObservation != nil {
		observation := *record.CaseBindingObservation
		if domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
			observation.State != domainsecurity.CaseBindingStateValid ||
			observation.WorkspaceRealPath != record.WorkspaceRealPath {
			return errors.New("attachment owner case binding is invalid")
		}
	}
	return nil
}

func ParseOwnerRecordV1(body []byte) (OwnerRecordV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxOwnerRecordBytesV1, MaxDepth: 12, MaxTokens: 512, MaxStringBytes: 64 * 1024,
	}); err != nil {
		return OwnerRecordV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var record OwnerRecordV1
	if err := decoder.Decode(&record); err != nil {
		return OwnerRecordV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return OwnerRecordV1{}, errors.New("attachment owner record contains trailing JSON")
	}
	return record, ValidateOwnerRecordV1(record)
}

func OwnerRecordV1Bytes(record OwnerRecordV1) ([]byte, error) {
	if err := ValidateOwnerRecordV1(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func ownerDigest(record OwnerRecordV1) string {
	record.AttachmentID = ""
	record.OwnerDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}
