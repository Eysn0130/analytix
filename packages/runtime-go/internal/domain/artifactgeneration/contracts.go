package artifactgeneration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	InventorySchemaVersionV1   = 1
	ReceiptSchemaVersionV1     = 1
	ReceiptFormatV1            = "immutable_generation_v1"
	FileTypeRegularV1          = "regular_file"
	FileTypeNativeExecutableV1 = "native_executable"

	InventoryFileNameV1    = "inventory.v1.json"
	ReceiptFileNameV1      = "receipt.v1.json"
	RegularFileModeV1      = uint32(0o600)
	NativeExecutableModeV1 = uint32(0o700)
	DirectoryModeV1        = uint32(0o700)

	MaxFilesV1          = 4096
	MaxPathBytesV1      = 1024
	MaxComponentBytesV1 = 255
	MaxContractBytesV1  = 4 << 20
	MaxContractTokensV1 = 1 << 18
	MaxTotalBytesV1     = 1 << 40
)

var ErrInvalidContract = errors.New("artifact_generation_contract_invalid")

// SourceFileV1 is an in-memory input to PrepareV1. Body is copied before the
// generation crosses a filesystem boundary.
type SourceFileV1 struct {
	Path string
	Type string
	Mode uint32
	Body []byte
}

// FileInventoryEntryV1 binds one exact relative payload path to its complete
// SHA-256 and byte length. Generation metadata files are deliberately not
// payload entries; their fixed names are covered by ReceiptV1.
type FileInventoryEntryV1 struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
	Size   uint64 `json:"size"`
}

// InventoryV1 is a closed, canonical inventory. Files must be strictly sorted
// by Path and GenerationID is derived from the exact ordered payload shape.
type InventoryV1 struct {
	SchemaVersion int                    `json:"schemaVersion"`
	GenerationID  string                 `json:"generationId"`
	DirectoryMode uint32                 `json:"directoryMode"`
	Files         []FileInventoryEntryV1 `json:"files"`
	FileCount     uint64                 `json:"fileCount"`
	TotalBytes    uint64                 `json:"totalBytes"`
}

// ReceiptV1 binds the canonical inventory bytes to the generation that may be
// made visible. It contains no mutable timestamp or caller-controlled path.
type ReceiptV1 struct {
	SchemaVersion   int    `json:"schemaVersion"`
	Format          string `json:"format"`
	GenerationID    string `json:"generationId"`
	InventoryDigest string `json:"inventoryDigest"`
	DirectoryMode   uint32 `json:"directoryMode"`
	InventoryType   string `json:"inventoryType"`
	InventoryMode   uint32 `json:"inventoryMode"`
	ReceiptType     string `json:"receiptType"`
	ReceiptMode     uint32 `json:"receiptMode"`
	FileCount       uint64 `json:"fileCount"`
	TotalBytes      uint64 `json:"totalBytes"`
}

// PreparedV1 owns immutable copies of all payload and contract bytes.
type PreparedV1 struct {
	files          []SourceFileV1
	inventory      InventoryV1
	receipt        ReceiptV1
	inventoryBytes []byte
	receiptBytes   []byte
}

func PrepareV1(source []SourceFileV1) (PreparedV1, error) {
	if len(source) == 0 || len(source) > MaxFilesV1 {
		return PreparedV1{}, ErrInvalidContract
	}
	files := make([]SourceFileV1, len(source))
	for index := range source {
		files[index] = SourceFileV1{Path: source[index].Path, Type: source[index].Type, Mode: source[index].Mode, Body: append([]byte(nil), source[index].Body...)}
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	entries := make([]FileInventoryEntryV1, len(files))
	var total uint64
	for index := range files {
		if err := ValidatePayloadPathV1(files[index].Path); err != nil || !validFileTypeModeV1(files[index].Type, files[index].Mode) || index > 0 &&
			(files[index-1].Path == files[index].Path || strings.HasPrefix(files[index].Path, files[index-1].Path+"/")) {
			return PreparedV1{}, ErrInvalidContract
		}
		size := uint64(len(files[index].Body))
		if size > MaxTotalBytesV1 || total > MaxTotalBytesV1-size {
			return PreparedV1{}, ErrInvalidContract
		}
		total += size
		entries[index] = FileInventoryEntryV1{Path: files[index].Path, Type: files[index].Type, Mode: files[index].Mode, SHA256: DigestBytesV1(files[index].Body), Size: size}
	}
	generationID, err := generationIDV1(entries, uint64(len(entries)), total)
	if err != nil {
		return PreparedV1{}, errors.Join(ErrInvalidContract, err)
	}
	inventory := InventoryV1{
		SchemaVersion: InventorySchemaVersionV1,
		GenerationID:  generationID,
		DirectoryMode: DirectoryModeV1,
		Files:         entries,
		FileCount:     uint64(len(entries)),
		TotalBytes:    total,
	}
	inventoryBytes, err := CanonicalInventoryBytesV1(inventory)
	if err != nil {
		return PreparedV1{}, err
	}
	receipt := ReceiptV1{
		SchemaVersion:   ReceiptSchemaVersionV1,
		Format:          ReceiptFormatV1,
		GenerationID:    generationID,
		InventoryDigest: DigestBytesV1(inventoryBytes),
		DirectoryMode:   DirectoryModeV1,
		InventoryType:   FileTypeRegularV1,
		InventoryMode:   RegularFileModeV1,
		ReceiptType:     FileTypeRegularV1,
		ReceiptMode:     RegularFileModeV1,
		FileCount:       uint64(len(entries)),
		TotalBytes:      total,
	}
	receiptBytes, err := CanonicalReceiptBytesV1(receipt, inventory)
	if err != nil {
		return PreparedV1{}, err
	}
	prepared := PreparedV1{
		files: files, inventory: inventory, receipt: receipt,
		inventoryBytes: inventoryBytes, receiptBytes: receiptBytes,
	}
	if err := prepared.Validate(); err != nil {
		return PreparedV1{}, err
	}
	return prepared, nil
}

func (prepared PreparedV1) Validate() error {
	if len(prepared.files) == 0 || len(prepared.inventoryBytes) == 0 || len(prepared.receiptBytes) == 0 {
		return ErrInvalidContract
	}
	parsedInventory, err := ParseInventoryV1(prepared.inventoryBytes)
	if err != nil || parsedInventory.GenerationID != prepared.inventory.GenerationID {
		return ErrInvalidContract
	}
	parsedReceipt, err := ParseReceiptV1(prepared.receiptBytes, prepared.inventoryBytes)
	if err != nil || parsedReceipt != prepared.receipt || len(prepared.files) != len(parsedInventory.Files) {
		return ErrInvalidContract
	}
	for index := range prepared.files {
		entry := parsedInventory.Files[index]
		if prepared.files[index].Path != entry.Path || prepared.files[index].Type != entry.Type || prepared.files[index].Mode != entry.Mode ||
			uint64(len(prepared.files[index].Body)) != entry.Size || DigestBytesV1(prepared.files[index].Body) != entry.SHA256 {
			return ErrInvalidContract
		}
	}
	return nil
}

func (prepared PreparedV1) Files() []SourceFileV1 {
	files := make([]SourceFileV1, len(prepared.files))
	for index := range prepared.files {
		files[index] = SourceFileV1{Path: prepared.files[index].Path, Type: prepared.files[index].Type, Mode: prepared.files[index].Mode, Body: append([]byte(nil), prepared.files[index].Body...)}
	}
	return files
}

func (prepared PreparedV1) Inventory() InventoryV1 {
	return cloneInventoryV1(prepared.inventory)
}

func (prepared PreparedV1) Receipt() ReceiptV1 { return prepared.receipt }

func (prepared PreparedV1) InventoryBytes() []byte {
	return append([]byte(nil), prepared.inventoryBytes...)
}

func (prepared PreparedV1) ReceiptBytes() []byte {
	return append([]byte(nil), prepared.receiptBytes...)
}

func ParseInventoryV1(body []byte) (InventoryV1, error) {
	var inventory InventoryV1
	if err := parseCanonicalV1(body, []string{"schemaVersion", "generationId", "directoryMode", "files", "fileCount", "totalBytes"}, &inventory); err != nil {
		return InventoryV1{}, err
	}
	if err := ValidateInventoryV1(inventory); err != nil {
		return InventoryV1{}, err
	}
	return cloneInventoryV1(inventory), nil
}

func ParseReceiptV1(body, inventoryBytes []byte) (ReceiptV1, error) {
	var receipt ReceiptV1
	if err := parseCanonicalV1(body, []string{"schemaVersion", "format", "generationId", "inventoryDigest", "directoryMode", "inventoryType", "inventoryMode", "receiptType", "receiptMode", "fileCount", "totalBytes"}, &receipt); err != nil {
		return ReceiptV1{}, err
	}
	inventory, err := ParseInventoryV1(inventoryBytes)
	if err != nil {
		return ReceiptV1{}, err
	}
	if err := ValidateReceiptV1(receipt, inventory, inventoryBytes); err != nil {
		return ReceiptV1{}, err
	}
	return receipt, nil
}

func CanonicalInventoryBytesV1(inventory InventoryV1) ([]byte, error) {
	if err := ValidateInventoryV1(inventory); err != nil {
		return nil, err
	}
	return json.Marshal(inventory)
}

func CanonicalReceiptBytesV1(receipt ReceiptV1, inventory InventoryV1) ([]byte, error) {
	inventoryBytes, err := CanonicalInventoryBytesV1(inventory)
	if err != nil {
		return nil, err
	}
	if err := ValidateReceiptV1(receipt, inventory, inventoryBytes); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func ValidateInventoryV1(inventory InventoryV1) error {
	if inventory.SchemaVersion != InventorySchemaVersionV1 || !ValidDigestV1(inventory.GenerationID) || inventory.DirectoryMode != DirectoryModeV1 ||
		len(inventory.Files) == 0 || len(inventory.Files) > MaxFilesV1 || inventory.FileCount != uint64(len(inventory.Files)) || inventory.TotalBytes > MaxTotalBytesV1 {
		return ErrInvalidContract
	}
	var total uint64
	for index := range inventory.Files {
		entry := inventory.Files[index]
		if err := ValidatePayloadPathV1(entry.Path); err != nil || !validFileTypeModeV1(entry.Type, entry.Mode) || !ValidDigestV1(entry.SHA256) || index > 0 &&
			(inventory.Files[index-1].Path >= entry.Path || strings.HasPrefix(entry.Path, inventory.Files[index-1].Path+"/")) ||
			entry.Size > MaxTotalBytesV1 || total > MaxTotalBytesV1-entry.Size {
			return ErrInvalidContract
		}
		total += entry.Size
	}
	if total != inventory.TotalBytes {
		return ErrInvalidContract
	}
	expected, err := generationIDV1(inventory.Files, inventory.FileCount, inventory.TotalBytes)
	if err != nil || inventory.GenerationID != expected {
		return ErrInvalidContract
	}
	return nil
}

func ValidateReceiptV1(receipt ReceiptV1, inventory InventoryV1, inventoryBytes []byte) error {
	if receipt.SchemaVersion != ReceiptSchemaVersionV1 || receipt.Format != ReceiptFormatV1 ||
		receipt.GenerationID != inventory.GenerationID || receipt.InventoryDigest != DigestBytesV1(inventoryBytes) ||
		receipt.DirectoryMode != DirectoryModeV1 || receipt.InventoryType != FileTypeRegularV1 || receipt.InventoryMode != RegularFileModeV1 ||
		receipt.ReceiptType != FileTypeRegularV1 || receipt.ReceiptMode != RegularFileModeV1 ||
		receipt.FileCount != inventory.FileCount || receipt.TotalBytes != inventory.TotalBytes {
		return ErrInvalidContract
	}
	return nil
}

func ValidatePayloadPathV1(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > MaxPathBytesV1 || !utf8.ValidString(value) ||
		strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.ContainsRune(value, 0) || path.Clean(value) != value {
		return ErrInvalidContract
	}
	components := strings.Split(value, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." || strings.HasPrefix(component, ".") ||
			len(component) > MaxComponentBytesV1 || !safeComponentV1(component) {
			return ErrInvalidContract
		}
	}
	if len(components) == 1 && (value == InventoryFileNameV1 || value == ReceiptFileNameV1) {
		return ErrInvalidContract
	}
	return nil
}

func ValidDigestV1(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func DigestBytesV1(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func generationIDV1(files []FileInventoryEntryV1, count, total uint64) (string, error) {
	material := struct {
		SchemaVersion int                    `json:"schemaVersion"`
		DirectoryMode uint32                 `json:"directoryMode"`
		Files         []FileInventoryEntryV1 `json:"files"`
		FileCount     uint64                 `json:"fileCount"`
		TotalBytes    uint64                 `json:"totalBytes"`
	}{SchemaVersion: InventorySchemaVersionV1, DirectoryMode: DirectoryModeV1, Files: files, FileCount: count, TotalBytes: total}
	body, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return DigestBytesV1(body), nil
}

func parseCanonicalV1(body []byte, fields []string, target any) error {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxContractBytesV1, MaxDepth: 8,
		MaxTokens: MaxContractTokensV1, MaxStringBytes: MaxPathBytesV1,
	})
	if err != nil || len(object) != len(fields) {
		return errors.Join(ErrInvalidContract, err)
	}
	for _, field := range fields {
		if raw, exists := object[field]; !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return ErrInvalidContract
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return errors.Join(ErrInvalidContract, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidContract
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, body) {
		return errors.Join(ErrInvalidContract, err)
	}
	return nil
}

func safeComponentV1(component string) bool {
	for _, current := range component {
		if current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' || current >= '0' && current <= '9' || current == '-' || current == '_' || current == '.' {
			continue
		}
		return false
	}
	return true
}

func validFileTypeModeV1(fileType string, mode uint32) bool {
	return fileType == FileTypeRegularV1 && mode == RegularFileModeV1 ||
		fileType == FileTypeNativeExecutableV1 && mode == NativeExecutableModeV1
}

func cloneInventoryV1(inventory InventoryV1) InventoryV1 {
	inventory.Files = append([]FileInventoryEntryV1(nil), inventory.Files...)
	return inventory
}
