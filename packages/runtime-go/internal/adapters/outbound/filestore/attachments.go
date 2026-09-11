package filestore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	attachmentuploadport "analytix.local/runtime-go/internal/ports/attachmentupload"
)

const (
	DefaultAttachmentMaxImageBytes                 = domainmodel.AttachmentMaxImageBytes
	DefaultAttachmentMaxImageDimension             = domainmodel.AttachmentMaxImageDimension
	DefaultAttachmentMaxDocumentBytes              = domainmodel.AttachmentMaxDocumentBytes
	DefaultAttachmentMaxDocumentTextChars          = domainmodel.AttachmentMaxDocumentTextChars
	DefaultAttachmentTextFallbackMaxBase64Bytes    = domainmodel.AttachmentTextFallbackMaxBase64Bytes
	DefaultAttachmentTextFallbackMaxImageDimension = domainmodel.AttachmentTextFallbackMaxImageDimension
	DefaultAttachmentTextFallbackPreferredMimeType = domainmodel.AttachmentTextFallbackPreferredMimeType
)

var DefaultAttachmentAllowedMimeTypes = domainmodel.AttachmentAllowedMimeTypes

var DefaultAttachmentImageMimeTypes = domainmodel.AttachmentImageMimeTypes

var DefaultAttachmentDocumentMimeTypes = domainmodel.AttachmentDocumentMimeTypes

var ErrAttachmentContentIntegrity = errors.New("attachment content integrity is invalid")

type PersistentAttachmentStore struct {
	root        string
	metadataDir string
	contentDir  string
	mu          sync.Mutex
}

type preparedAttachmentUpload struct {
	store          *PersistentAttachmentStore
	owner          domainattachment.OwnerRecordV1
	metadata       map[string]any
	metadataSHA256 string
	content        []byte
	mu             sync.Mutex
	state          string
}

var _ attachmentuploadport.Store = (*PersistentAttachmentStore)(nil)
var _ attachmentuploadport.PreparedUpload = (*preparedAttachmentUpload)(nil)

type detectedAttachmentImage struct {
	MIMEType string
	Width    int
	Height   int
}

func NewPersistentAttachmentStore(dataDir string) (*PersistentAttachmentStore, error) {
	root := filepath.Join(dataDir, "attachments")
	return &PersistentAttachmentStore{
		root:        root,
		metadataDir: filepath.Join(root, "metadata"),
		contentDir:  filepath.Join(root, "content"),
	}, nil
}

func (s *PersistentAttachmentStore) Create(body map[string]any) (map[string]any, error) {
	prepared, err := s.Prepare(context.Background(), body)
	if err != nil {
		return nil, err
	}
	return prepared.Commit(context.Background())
}

func (s *PersistentAttachmentStore) Prepare(ctx context.Context, body map[string]any) (attachmentuploadport.PreparedUpload, error) {
	if s == nil {
		return nil, errors.New("attachment store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	threadID := strings.TrimSpace(stringField(body, "threadId"))
	if threadID == "" {
		return nil, attachmentUploadInputError("attachment threadId is required")
	}
	workspace, caseBindingObservation, err := observeAttachmentUploadBinding(stringField(body, "workspace"))
	if err != nil {
		return nil, err
	}
	if workspace == "" {
		return nil, attachmentUploadInputError("attachment workspace is required")
	}
	name := strings.TrimSpace(stringField(body, "name"))
	if name == "" {
		name = "attachment"
	}
	mimeType := strings.TrimSpace(stringField(body, "mimeType"))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	dataBase64 := strings.TrimSpace(stringField(body, "dataBase64"))
	if dataBase64 == "" {
		return nil, attachmentUploadInputError("attachment dataBase64 is required")
	}
	decoded, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, attachmentUploadInputError("invalid attachment dataBase64")
	}
	detectedImage := detectAttachmentImage(decoded)
	if detectedImage != nil {
		if mimeType != "" && mimeType != "application/octet-stream" && mimeType != detectedImage.MIMEType {
			return nil, attachmentUploadInputError("declared MIME type does not match image content")
		}
		mimeType = detectedImage.MIMEType
		if !attachmentMimeAllowed(mimeType, DefaultAttachmentImageMimeTypes) {
			return nil, attachmentUploadInputError(fmt.Sprintf("image MIME type is not allowed: %s", mimeType))
		}
		if len(decoded) > DefaultAttachmentMaxImageBytes {
			return nil, attachmentUploadInputError(fmt.Sprintf("image exceeds %d byte limit", DefaultAttachmentMaxImageBytes))
		}
		if maxAttachmentImageDimension(detectedImage) > DefaultAttachmentMaxImageDimension {
			return nil, attachmentUploadInputError(fmt.Sprintf("image exceeds %dpx dimension limit", DefaultAttachmentMaxImageDimension))
		}
	} else {
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			return nil, attachmentUploadInputError(fmt.Sprintf("unsupported image MIME type: %s", mimeType))
		}
		if !attachmentMimeAllowed(mimeType, DefaultAttachmentAllowedMimeTypes) {
			return nil, attachmentUploadInputError(fmt.Sprintf("attachment MIME type is not allowed: %s", mimeType))
		}
		if len(decoded) > DefaultAttachmentMaxDocumentBytes {
			return nil, attachmentUploadInputError(fmt.Sprintf("document exceeds %d byte limit", DefaultAttachmentMaxDocumentBytes))
		}
	}
	if textFallback, ok := body["textFallback"].(map[string]any); ok {
		if err := validateAttachmentTextFallback(textFallback); err != nil {
			return nil, errors.Join(domainattachment.ErrUploadInputInvalid, err)
		}
	}
	sum := sha256.Sum256(decoded)
	hashHex := fmt.Sprintf("%x", sum[:])
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	localFilePath := strings.TrimSpace(stringField(body, "localFilePath"))
	textFallback, hasTextFallback := body["textFallback"].(map[string]any)
	documentText := strings.TrimSpace(stringField(body, "documentText"))
	pageCount, hasPageCount := numericAny(body["pageCount"])
	truncated := false

	metadata := map[string]any{
		"name":       name,
		"kind":       "document",
		"mimeType":   mimeType,
		"byteSize":   float64(len(decoded)),
		"hash":       hashHex,
		"scope":      "thread",
		"threadIds":  []any{threadID},
		"workspaces": []any{workspace},
		"createdAt":  now,
		"updatedAt":  now,
	}
	if detectedImage != nil {
		metadata["kind"] = "image"
		if detectedImage.Width > 0 {
			metadata["width"] = float64(detectedImage.Width)
		}
		if detectedImage.Height > 0 {
			metadata["height"] = float64(detectedImage.Height)
		}
	} else {
		if documentText != "" {
			if len([]rune(documentText)) > DefaultAttachmentMaxDocumentTextChars {
				runes := []rune(documentText)
				documentText = string(runes[:DefaultAttachmentMaxDocumentTextChars])
				truncated = true
			}
			metadata["documentText"] = documentText
		}
		if hasPageCount && pageCount > 0 {
			metadata["pageCount"] = float64(pageCount)
		}
		if truncated {
			metadata["truncated"] = true
		}
	}
	if localFilePath != "" {
		metadata["localFilePath"] = localFilePath
	}
	for _, key := range []string{"width", "height"} {
		if _, alreadyDetected := metadata[key]; alreadyDetected {
			continue
		}
		if value, ok := numericAny(body[key]); ok && value > 0 {
			metadata[key] = float64(value)
		}
	}
	if hasTextFallback {
		metadata["textFallback"] = contracts.CloneMap(textFallback)
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("create attachment owner nonce: %w", err)
	}
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{
		OwnerNonce: hex.EncodeToString(nonceBytes), BlobSHA256: hashHex, ByteSize: int64(len(decoded)),
		MIMEType: mimeType, ThreadID: threadID, WorkspaceRealPath: workspace,
		CaseBindingObservation: caseBindingObservation, ProjectionSHA256: attachmentProjectionSHA256(metadata), CreatedAt: nowTime,
	})
	if err != nil {
		return nil, err
	}
	ownerBody, err := domainattachment.OwnerRecordV1Bytes(owner)
	if err != nil {
		return nil, err
	}
	var ownerMap map[string]any
	if err := json.Unmarshal(ownerBody, &ownerMap); err != nil {
		return nil, err
	}
	id := owner.AttachmentID
	metadata["id"] = id
	metadata["ownerRecord"] = ownerMap
	metadataBody, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	metadataDigest := sha256.Sum256(metadataBody)
	return &preparedAttachmentUpload{
		store: s, owner: owner, metadata: contracts.CloneMap(metadata),
		metadataSHA256: fmt.Sprintf("%x", metadataDigest[:]), content: append([]byte(nil), decoded...), state: "prepared",
	}, nil
}

func (prepared *preparedAttachmentUpload) Owner() domainattachment.OwnerRecordV1 {
	if prepared == nil {
		return domainattachment.OwnerRecordV1{}
	}
	return prepared.owner
}

func (prepared *preparedAttachmentUpload) Metadata() map[string]any {
	if prepared == nil {
		return nil
	}
	return contracts.CloneMap(prepared.metadata)
}

func (prepared *preparedAttachmentUpload) MetadataSHA256() string {
	if prepared == nil {
		return ""
	}
	return prepared.metadataSHA256
}

func (prepared *preparedAttachmentUpload) Commit(ctx context.Context) (map[string]any, error) {
	if prepared == nil || prepared.store == nil {
		return nil, errors.New("prepared attachment upload is unavailable")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.state == "committed" {
		if err := prepared.store.verifyPreparedUpload(ctx, prepared); err != nil {
			return nil, err
		}
		return contracts.CloneMap(prepared.metadata), nil
	}
	if prepared.state != "prepared" {
		return nil, errors.New("prepared attachment upload is already aborted")
	}
	if err := prepared.store.commitPreparedUpload(ctx, prepared); err != nil {
		return nil, err
	}
	prepared.state = "committed"
	return contracts.CloneMap(prepared.metadata), nil
}

func (prepared *preparedAttachmentUpload) Abort(ctx context.Context) error {
	if prepared == nil || prepared.store == nil {
		return errors.New("prepared attachment upload is unavailable")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.state == "aborted" {
		return nil
	}
	if err := prepared.store.abortPreparedUpload(ctx, prepared); err != nil {
		return err
	}
	prepared.state = "aborted"
	return nil
}

func (s *PersistentAttachmentStore) commitPreparedUpload(ctx context.Context, prepared *preparedAttachmentUpload) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if prepared == nil || prepared.store != s || domainattachment.ValidateOwnerRecordV1(prepared.owner) != nil ||
		!domainsecurity.IsSHA256Hex(prepared.metadataSHA256) {
		return errors.New("prepared attachment upload integrity is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWriteDirsNoLock(); err != nil {
		return err
	}
	id := prepared.owner.AttachmentID
	contentFile, err := os.OpenFile(s.contentPath(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := contentFile.Write(prepared.content); err != nil {
		_ = contentFile.Close()
		return errors.Join(err, s.removeUploadFilesNoLock(id, false, true))
	}
	if err := contentFile.Sync(); err != nil {
		_ = contentFile.Close()
		return errors.Join(err, s.removeUploadFilesNoLock(id, false, true))
	}
	if err := contentFile.Close(); err != nil {
		return errors.Join(err, s.removeUploadFilesNoLock(id, false, true))
	}
	if err := syncAttachmentDirectoryIfPresent(s.contentDir); err != nil {
		return errors.Join(err, s.removeUploadFilesNoLock(id, false, true))
	}
	metadataCreated, err := s.writeMetadataExclusiveNoLock(id, prepared.metadata)
	if err != nil {
		return errors.Join(err, s.removeUploadFilesNoLock(id, metadataCreated, true))
	}
	return s.verifyPreparedUploadNoLock(ctx, prepared)
}

func (s *PersistentAttachmentStore) verifyPreparedUpload(ctx context.Context, prepared *preparedAttachmentUpload) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verifyPreparedUploadNoLock(ctx, prepared)
}

func (s *PersistentAttachmentStore) verifyPreparedUploadNoLock(ctx context.Context, prepared *preparedAttachmentUpload) error {
	if prepared == nil || prepared.store != s {
		return errors.New("prepared attachment upload does not belong to this store")
	}
	metadataBody, err := os.ReadFile(s.metadataPath(prepared.owner.AttachmentID))
	if err != nil {
		return err
	}
	metadataDigest := sha256.Sum256(metadataBody)
	if fmt.Sprintf("%x", metadataDigest[:]) != prepared.metadataSHA256 {
		return ErrAttachmentContentIntegrity
	}
	metadata, err := ReadJSONMapFile(s.metadataPath(prepared.owner.AttachmentID))
	if err != nil {
		return err
	}
	storedOwner, err := validateAttachmentMetadataIntegrity(prepared.owner.AttachmentID, metadata)
	if err != nil || !sameAttachmentRecoveryOwner(storedOwner, prepared.owner) {
		return ErrAttachmentContentIntegrity
	}
	return validateAttachmentRecoveryContent(ctx, s.contentPath(prepared.owner.AttachmentID), prepared.owner)
}

func (s *PersistentAttachmentStore) abortPreparedUpload(ctx context.Context, prepared *preparedAttachmentUpload) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if prepared == nil || prepared.store != s {
		return errors.New("prepared attachment upload does not belong to this store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := prepared.owner.AttachmentID
	removeMetadata := false
	if metadataBody, err := os.ReadFile(s.metadataPath(id)); err == nil {
		digest := sha256.Sum256(metadataBody)
		if fmt.Sprintf("%x", digest[:]) != prepared.metadataSHA256 {
			return errors.New("attachment upload metadata changed before abort")
		}
		removeMetadata = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	removeContent := false
	if _, err := os.Stat(s.contentPath(id)); err == nil {
		if err := validateAttachmentRecoveryContent(ctx, s.contentPath(id), prepared.owner); err != nil {
			return errors.New("attachment upload content changed before abort")
		}
		removeContent = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.removeUploadFilesNoLock(id, removeMetadata, removeContent)
}

func (s *PersistentAttachmentStore) writeMetadataExclusiveNoLock(id string, metadata map[string]any) (bool, error) {
	body, err := json.Marshal(metadata)
	if err != nil {
		return false, err
	}
	file, err := os.OpenFile(s.metadataPath(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return true, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return true, err
	}
	if err := file.Close(); err != nil {
		return true, err
	}
	return true, syncAttachmentDirectoryIfPresent(s.metadataDir)
}

func (s *PersistentAttachmentStore) removeUploadFilesNoLock(id string, removeMetadata bool, removeContent bool) error {
	var errs []error
	paths := []string{}
	if removeMetadata {
		paths = append(paths, s.metadataPath(id))
	}
	if removeContent {
		paths = append(paths, s.contentPath(id))
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if removeMetadata {
		errs = append(errs, syncAttachmentDirectoryIfPresent(s.metadataDir))
	}
	if removeContent {
		errs = append(errs, syncAttachmentDirectoryIfPresent(s.contentDir))
	}
	return errors.Join(errs...)
}

func (s *PersistentAttachmentStore) ensureWriteDirsNoLock() error {
	if err := os.MkdirAll(s.metadataDir, 0o700); err != nil {
		return err
	}
	return os.MkdirAll(s.contentDir, 0o700)
}

func (s *PersistentAttachmentStore) Metadata(id string) (map[string]any, bool, error) {
	if !validAttachmentID(id) {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, err := ReadJSONMapFile(s.metadataPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if _, err := validateAttachmentMetadataIntegrity(id, metadata); err != nil {
		return nil, false, err
	}
	return metadata, true, nil
}

func (s *PersistentAttachmentStore) Content(id string) (map[string]any, string, bool, error) {
	if !validAttachmentID(id) {
		return nil, "", false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, err := ReadJSONMapFile(s.metadataPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	data, err := os.ReadFile(s.contentPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	if err := validateAttachmentContentIntegrity(id, metadata, data); err != nil {
		return nil, "", false, err
	}
	return metadata, base64.StdEncoding.EncodeToString(data), true, nil
}

func (s *PersistentAttachmentStore) Diagnostics() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	total := 0
	entries, err := os.ReadDir(s.metadataDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			metadata, readErr := ReadJSONMapFile(filepath.Join(s.metadataDir, entry.Name()))
			if readErr != nil {
				continue
			}
			count += 1
			if bytes, ok := numericAny(metadata["byteSize"]); ok {
				total += bytes
			}
		}
	}
	return map[string]any{
		"enabled":                       true,
		"rootDir":                       filepath.ToSlash(s.root),
		"count":                         float64(count),
		"totalBytes":                    float64(total),
		"maxImageBytes":                 float64(DefaultAttachmentMaxImageBytes),
		"maxImageDimension":             float64(DefaultAttachmentMaxImageDimension),
		"allowedMimeTypes":              stringListAny(DefaultAttachmentImageMimeTypes),
		"allowedDocumentMimeTypes":      stringListAny(DefaultAttachmentDocumentMimeTypes),
		"maxDocumentBytes":              float64(DefaultAttachmentMaxDocumentBytes),
		"maxDocumentTextChars":          float64(DefaultAttachmentMaxDocumentTextChars),
		"acceptedMimeTypes":             stringListAny(DefaultAttachmentAllowedMimeTypes),
		"textFallbackMaxBase64Bytes":    float64(DefaultAttachmentTextFallbackMaxBase64Bytes),
		"textFallbackMaxImageDimension": float64(DefaultAttachmentTextFallbackMaxImageDimension),
		"textFallbackPreferredMimeType": DefaultAttachmentTextFallbackPreferredMimeType,
	}
}

func (s *PersistentAttachmentStore) metadataPath(id string) string {
	return filepath.Join(s.metadataDir, contracts.SafeRecordID(id)+".json")
}

func (s *PersistentAttachmentStore) contentPath(id string) string {
	return filepath.Join(s.contentDir, contracts.SafeRecordID(id)+".bin")
}

func observeAttachmentUploadBinding(workspace string) (string, *domainsecurity.CaseBindingObservationV1, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", nil, nil
	}
	observation, err := (CaseBindingReader{}).Observe(workspace)
	if err != nil {
		return "", nil, fmt.Errorf("observe attachment case binding: %w", err)
	}
	workspace = observation.WorkspaceRealPath
	switch observation.State {
	case domainsecurity.CaseBindingStateValid:
		return workspace, &observation, nil
	case domainsecurity.CaseBindingStateMissing, domainsecurity.CaseBindingStateNotApplicable:
		return workspace, nil, nil
	default:
		return "", nil, errors.Join(
			domainattachment.ErrUploadBindingUnsafe,
			errors.New("attachment case binding is not safe for upload"),
		)
	}
}

func attachmentUploadInputError(message string) error {
	return fmt.Errorf("%w: %s", domainattachment.ErrUploadInputInvalid, message)
}

func validateAttachmentContentIntegrity(id string, metadata map[string]any, data []byte) error {
	owner, err := validateAttachmentMetadataIntegrity(id, metadata)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	actualHash := fmt.Sprintf("%x", sum[:])
	if owner.BlobSHA256 != actualHash || owner.ByteSize != int64(len(data)) {
		return ErrAttachmentContentIntegrity
	}
	return nil
}

func validateAttachmentMetadataIntegrity(id string, metadata map[string]any) (domainattachment.OwnerRecordV1, error) {
	ownerMap, ok := metadata["ownerRecord"].(map[string]any)
	if !ok {
		return domainattachment.OwnerRecordV1{}, ErrAttachmentContentIntegrity
	}
	body, err := json.Marshal(ownerMap)
	if err != nil {
		return domainattachment.OwnerRecordV1{}, ErrAttachmentContentIntegrity
	}
	owner, err := domainattachment.ParseOwnerRecordV1(body)
	byteSize, hasByteSize := numericAny(metadata["byteSize"])
	if err != nil || owner.AttachmentID != id || strings.TrimSpace(stringField(metadata, "id")) != id ||
		owner.BlobSHA256 != strings.TrimSpace(stringField(metadata, "hash")) ||
		owner.MIMEType != strings.TrimSpace(stringField(metadata, "mimeType")) ||
		!hasByteSize || owner.ByteSize != int64(byteSize) ||
		owner.ProjectionSHA256 != attachmentProjectionSHA256(metadata) {
		return domainattachment.OwnerRecordV1{}, ErrAttachmentContentIntegrity
	}
	return owner, nil
}

func attachmentProjectionSHA256(metadata map[string]any) string {
	projection := map[string]any{}
	for _, key := range []string{
		"name", "kind", "mimeType", "byteSize", "hash", "scope", "threadIds", "workspaces",
		"width", "height", "pageCount", "truncated", "localFilePath", "documentText", "textFallback",
	} {
		if value, ok := metadata[key]; ok {
			projection[key] = contracts.CloneValue(value)
		}
	}
	body, _ := json.Marshal(projection)
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%x", sum[:])
}

func validAttachmentID(id string) bool {
	if len(id) != len("att_")+24 || !strings.HasPrefix(id, "att_") {
		return false
	}
	for _, value := range id[len("att_"):] {
		if !strings.ContainsRune("0123456789abcdef", value) {
			return false
		}
	}
	return true
}

func detectAttachmentImage(data []byte) *detectedAttachmentImage {
	if len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4e && data[3] == 0x47 &&
		data[4] == 0x0d && data[5] == 0x0a && data[6] == 0x1a && data[7] == 0x0a {
		image := &detectedAttachmentImage{MIMEType: "image/png"}
		if len(data) >= 24 {
			image.Width = int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
			image.Height = int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
		}
		return image
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return &detectedAttachmentImage{MIMEType: "image/jpeg"}
	}
	if len(data) >= 12 &&
		string(data[0:4]) == "RIFF" &&
		string(data[8:12]) == "WEBP" {
		return &detectedAttachmentImage{MIMEType: "image/webp"}
	}
	return nil
}

func maxAttachmentImageDimension(image *detectedAttachmentImage) int {
	if image == nil {
		return 0
	}
	if image.Width > image.Height {
		return image.Width
	}
	return image.Height
}

func attachmentMimeAllowed(mimeType string, allowed []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	for _, candidate := range allowed {
		if normalized == candidate {
			return true
		}
	}
	return false
}

func validateAttachmentTextFallback(textFallback map[string]any) error {
	mimeType := strings.TrimSpace(stringField(textFallback, "mimeType"))
	if mimeType == "" {
		return errors.New("fallback image MIME type is required")
	}
	if !attachmentMimeAllowed(mimeType, DefaultAttachmentImageMimeTypes) {
		return fmt.Errorf("fallback image MIME type is not allowed: %s", mimeType)
	}
	dataBase64 := strings.TrimSpace(stringField(textFallback, "dataBase64"))
	if dataBase64 == "" {
		return errors.New("fallback image dataBase64 is required")
	}
	if len([]byte(dataBase64)) > DefaultAttachmentTextFallbackMaxBase64Bytes {
		return fmt.Errorf("fallback image exceeds %d base64 byte limit", DefaultAttachmentTextFallbackMaxBase64Bytes)
	}
	if _, err := base64.StdEncoding.DecodeString(dataBase64); err != nil {
		return errors.New("invalid fallback image dataBase64")
	}
	width, _ := numericAny(textFallback["width"])
	height, _ := numericAny(textFallback["height"])
	if width > DefaultAttachmentTextFallbackMaxImageDimension || height > DefaultAttachmentTextFallbackMaxImageDimension {
		return fmt.Errorf("fallback image exceeds %dpx dimension limit", DefaultAttachmentTextFallbackMaxImageDimension)
	}
	return nil
}

func stringList(value any) []string {
	raw := []any{}
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		raw = make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
