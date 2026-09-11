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
	UploadIntentSchemaVersionV1      = 1
	UploadIntentPurposeV1            = "analytix.attachment-upload-intent/v1"
	UploadDispositionPurposeV1       = "analytix.attachment-upload-disposition/v1"
	UploadDispositionCommittedV1     = "committed"
	UploadDispositionRejectedV1      = "rejected"
	UploadDispositionQuarantinedV1   = "quarantined"
	MaxAttachmentUploadRecordBytesV1 = 192 * 1024
)

var (
	ErrUploadInputInvalid  = errors.New("attachment upload input is invalid")
	ErrUploadBindingUnsafe = errors.New("attachment upload binding is unsafe")
)

var uploadDispositionReasonsV1 = map[string]struct{}{
	"upload_committed":               {},
	"upload_store_failed":            {},
	"upload_authority_rejected":      {},
	"restart_completed_exact_upload": {},
	"restart_missing_upload_files":   {},
	"restart_partial_upload_files":   {},
	"restart_stale_upload_binding":   {},
	"restart_corrupt_upload_files":   {},
}

type UploadIntentV1 struct {
	SchemaVersion  int           `json:"schemaVersion"`
	Purpose        string        `json:"purpose"`
	UploadID       string        `json:"uploadId"`
	Owner          OwnerRecordV1 `json:"owner"`
	MetadataSHA256 string        `json:"metadataSHA256"`
	CreatedAt      string        `json:"createdAt"`
	IntentDigest   string        `json:"intentDigest"`
}

type UploadDispositionV1 struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Purpose        string `json:"purpose"`
	UploadID       string `json:"uploadId"`
	IntentDigest   string `json:"intentDigest"`
	AttachmentID   string `json:"attachmentId"`
	OwnerDigest    string `json:"ownerDigest"`
	Status         string `json:"status"`
	ReasonCode     string `json:"reasonCode"`
	MetadataSHA256 string `json:"metadataSHA256,omitempty"`
	ContentSHA256  string `json:"contentSHA256,omitempty"`
	DisposedAt     string `json:"disposedAt"`
	RecordDigest   string `json:"recordDigest"`
}

func NewUploadIntentV1(owner OwnerRecordV1, metadataSHA256 string) (UploadIntentV1, error) {
	metadataSHA256 = strings.TrimSpace(metadataSHA256)
	if err := ValidateOwnerRecordV1(owner); err != nil {
		return UploadIntentV1{}, err
	}
	if !domainsecurity.IsSHA256Hex(metadataSHA256) {
		return UploadIntentV1{}, errors.New("attachment upload metadata digest is invalid")
	}
	intent := UploadIntentV1{
		SchemaVersion: UploadIntentSchemaVersionV1, Purpose: UploadIntentPurposeV1,
		Owner: owner, MetadataSHA256: metadataSHA256, CreatedAt: owner.CreatedAt,
	}
	intent.UploadID = uploadIDV1(owner)
	intent.IntentDigest = uploadIntentDigestV1(intent)
	return intent, ValidateUploadIntentV1(intent)
}

func ValidateUploadIntentV1(intent UploadIntentV1) error {
	createdAt, timeErr := time.Parse(time.RFC3339Nano, intent.CreatedAt)
	ownerCreatedAt, ownerTimeErr := time.Parse(time.RFC3339Nano, intent.Owner.CreatedAt)
	if intent.SchemaVersion != UploadIntentSchemaVersionV1 || intent.Purpose != UploadIntentPurposeV1 ||
		ValidateOwnerRecordV1(intent.Owner) != nil || !domainsecurity.IsSHA256Hex(intent.UploadID) ||
		intent.UploadID != uploadIDV1(intent.Owner) || !domainsecurity.IsSHA256Hex(intent.MetadataSHA256) ||
		timeErr != nil || ownerTimeErr != nil ||
		!createdAt.Equal(ownerCreatedAt) || createdAt.UTC().Format(time.RFC3339Nano) != intent.CreatedAt ||
		!domainsecurity.IsSHA256Hex(intent.IntentDigest) || intent.IntentDigest != uploadIntentDigestV1(intent) {
		return errors.New("attachment upload intent integrity is invalid")
	}
	body, err := json.Marshal(intent)
	if err != nil || len(body) > MaxAttachmentUploadRecordBytesV1 {
		return errors.New("attachment upload intent exceeds its canonical bound")
	}
	return nil
}

func ParseUploadIntentV1(body []byte) (UploadIntentV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxAttachmentUploadRecordBytesV1, MaxDepth: 16,
		MaxTokens: 1024, MaxStringBytes: 64 * 1024,
	}); err != nil {
		return UploadIntentV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var intent UploadIntentV1
	if err := decoder.Decode(&intent); err != nil {
		return UploadIntentV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return UploadIntentV1{}, errors.New("attachment upload intent contains trailing JSON")
	}
	return intent, ValidateUploadIntentV1(intent)
}

func UploadIntentV1Bytes(intent UploadIntentV1) ([]byte, error) {
	if err := ValidateUploadIntentV1(intent); err != nil {
		return nil, err
	}
	return json.Marshal(intent)
}

func NewUploadDispositionV1(
	intent UploadIntentV1,
	status string,
	reasonCode string,
	metadataSHA256 string,
	contentSHA256 string,
	disposedAt time.Time,
) (UploadDispositionV1, error) {
	if err := ValidateUploadIntentV1(intent); err != nil {
		return UploadDispositionV1{}, err
	}
	disposedAt = disposedAt.UTC()
	if disposedAt.IsZero() {
		disposedAt = time.Now().UTC()
	}
	disposition := UploadDispositionV1{
		SchemaVersion:  UploadIntentSchemaVersionV1,
		Purpose:        UploadDispositionPurposeV1,
		UploadID:       intent.UploadID,
		IntentDigest:   intent.IntentDigest,
		AttachmentID:   intent.Owner.AttachmentID,
		OwnerDigest:    intent.Owner.OwnerDigest,
		Status:         strings.TrimSpace(status),
		ReasonCode:     strings.TrimSpace(reasonCode),
		MetadataSHA256: strings.TrimSpace(metadataSHA256),
		ContentSHA256:  strings.TrimSpace(contentSHA256),
		DisposedAt:     disposedAt.Format(time.RFC3339Nano),
	}
	disposition.RecordDigest = uploadDispositionDigestV1(disposition)
	return disposition, ValidateUploadDispositionForIntentV1(disposition, intent)
}

func ValidateUploadDispositionV1(disposition UploadDispositionV1) error {
	disposedAt, timeErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if disposition.SchemaVersion != UploadIntentSchemaVersionV1 || disposition.Purpose != UploadDispositionPurposeV1 ||
		!domainsecurity.IsSHA256Hex(disposition.UploadID) || !domainsecurity.IsSHA256Hex(disposition.IntentDigest) ||
		!validUploadAttachmentIDV1(disposition.AttachmentID) || !domainsecurity.IsSHA256Hex(disposition.OwnerDigest) ||
		!validUploadDispositionStatusV1(disposition.Status) || !validUploadDispositionReasonV1(disposition.Status, disposition.ReasonCode) ||
		!validOptionalUploadDigestV1(disposition.MetadataSHA256) || !validOptionalUploadDigestV1(disposition.ContentSHA256) ||
		timeErr != nil || disposedAt.IsZero() || disposedAt.UTC().Format(time.RFC3339Nano) != disposition.DisposedAt ||
		!domainsecurity.IsSHA256Hex(disposition.RecordDigest) || disposition.RecordDigest != uploadDispositionDigestV1(disposition) {
		return errors.New("attachment upload disposition integrity is invalid")
	}
	body, err := json.Marshal(disposition)
	if err != nil || len(body) > MaxAttachmentUploadRecordBytesV1 {
		return errors.New("attachment upload disposition exceeds its canonical bound")
	}
	return nil
}

func ValidateUploadDispositionForIntentV1(disposition UploadDispositionV1, intent UploadIntentV1) error {
	if ValidateUploadDispositionV1(disposition) != nil || ValidateUploadIntentV1(intent) != nil ||
		disposition.UploadID != intent.UploadID || disposition.IntentDigest != intent.IntentDigest ||
		disposition.AttachmentID != intent.Owner.AttachmentID || disposition.OwnerDigest != intent.Owner.OwnerDigest {
		return errors.New("attachment upload disposition does not match its intent")
	}
	disposedAt, _ := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	createdAt, _ := time.Parse(time.RFC3339Nano, intent.CreatedAt)
	if disposedAt.Before(createdAt) {
		return errors.New("attachment upload disposition predates its intent")
	}
	if disposition.Status == UploadDispositionCommittedV1 &&
		(disposition.MetadataSHA256 != intent.MetadataSHA256 || disposition.ContentSHA256 != intent.Owner.BlobSHA256) {
		return errors.New("committed attachment upload disposition lacks exact staged digests")
	}
	return nil
}

func ParseUploadDispositionV1(body []byte) (UploadDispositionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxAttachmentUploadRecordBytesV1, MaxDepth: 8,
		MaxTokens: 256, MaxStringBytes: 64 * 1024,
	}); err != nil {
		return UploadDispositionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var disposition UploadDispositionV1
	if err := decoder.Decode(&disposition); err != nil {
		return UploadDispositionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return UploadDispositionV1{}, errors.New("attachment upload disposition contains trailing JSON")
	}
	return disposition, ValidateUploadDispositionV1(disposition)
}

func UploadDispositionV1Bytes(disposition UploadDispositionV1) ([]byte, error) {
	if err := ValidateUploadDispositionV1(disposition); err != nil {
		return nil, err
	}
	return json.Marshal(disposition)
}

func uploadIDV1(owner OwnerRecordV1) string {
	return domainsecurity.SHA256Hex([]byte("analytix.attachment-upload-id/v1\x00" + owner.OwnerDigest))
}

func uploadIntentDigestV1(intent UploadIntentV1) string {
	intent.IntentDigest = ""
	body, _ := json.Marshal(intent)
	return domainsecurity.SHA256Hex(append([]byte("analytix.attachment-upload-intent-digest/v1\x00"), body...))
}

func uploadDispositionDigestV1(disposition UploadDispositionV1) string {
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	return domainsecurity.SHA256Hex(append([]byte("analytix.attachment-upload-disposition-digest/v1\x00"), body...))
}

func validUploadAttachmentIDV1(value string) bool {
	if len(value) != len("att_")+24 || !strings.HasPrefix(value, "att_") {
		return false
	}
	for _, character := range value[len("att_"):] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func validUploadDispositionStatusV1(status string) bool {
	return status == UploadDispositionCommittedV1 || status == UploadDispositionRejectedV1 ||
		status == UploadDispositionQuarantinedV1
}

func validUploadDispositionReasonV1(status string, reason string) bool {
	if _, ok := uploadDispositionReasonsV1[reason]; !ok {
		return false
	}
	switch status {
	case UploadDispositionCommittedV1:
		return reason == "upload_committed" || reason == "restart_completed_exact_upload"
	case UploadDispositionRejectedV1:
		return reason == "upload_store_failed" || reason == "upload_authority_rejected"
	case UploadDispositionQuarantinedV1:
		return strings.HasPrefix(reason, "restart_") && reason != "restart_completed_exact_upload"
	default:
		return false
	}
}

func validOptionalUploadDigestV1(value string) bool {
	return value == "" || domainsecurity.IsSHA256Hex(value)
}

func UploadIDForOwnerV1(owner OwnerRecordV1) (string, error) {
	if err := ValidateOwnerRecordV1(owner); err != nil {
		return "", err
	}
	return uploadIDV1(owner), nil
}
