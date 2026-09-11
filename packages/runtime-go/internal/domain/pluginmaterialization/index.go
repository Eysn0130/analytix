package pluginmaterialization

import (
	"encoding/json"
	"errors"
	"time"
)

type IndexV1 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	Purpose                     string `json:"purpose"`
	IndexDigest                 string `json:"indexDigest"`
	PluginName                  string `json:"pluginName"`
	PluginVersion               string `json:"pluginVersion"`
	GenerationID                string `json:"generationId"`
	ActiveRelativePath          string `json:"activeRelativePath"`
	ReceiptID                   string `json:"receiptId"`
	ReceiptSHA256               string `json:"receiptSha256"`
	IntentID                    string `json:"intentId"`
	SourceTreeSHA256            string `json:"sourceTreeSha256"`
	SourceTreeFileCount         uint64 `json:"sourceTreeFileCount"`
	DiscoverableGenerationCount int    `json:"discoverableGenerationCount"`
	FactToolsEnabled            bool   `json:"factToolsEnabled"`
	CommittedAt                 string `json:"committedAt"`
}

func NewIndexV1(receipt ReceiptV1, committedAt time.Time) (IndexV1, error) {
	if err := ValidateReceiptV1(receipt); err != nil {
		return IndexV1{}, err
	}
	index := IndexV1{
		SchemaVersion: SchemaVersionV1, Purpose: IndexPurposeV1,
		PluginName: receipt.PluginName, PluginVersion: receipt.PluginVersion,
		GenerationID: receipt.GenerationID, ActiveRelativePath: receipt.ActiveRelativePath,
		ReceiptID: receipt.ReceiptID, ReceiptSHA256: ReceiptSHA256V1(receipt), IntentID: receipt.IntentID,
		SourceTreeSHA256: receipt.SourceTreeSHA256, SourceTreeFileCount: receipt.SourceTreeFileCount,
		DiscoverableGenerationCount: DiscoverableGenerationV1, FactToolsEnabled: false,
		CommittedAt: committedAt.UTC().Format(time.RFC3339Nano),
	}
	index.IndexDigest = deriveIndexDigest(index)
	if err := ValidateIndexForReceiptV1(index, receipt); err != nil {
		return IndexV1{}, err
	}
	return index, nil
}

func ValidateIndexV1(index IndexV1) error {
	if index.SchemaVersion != SchemaVersionV1 || index.Purpose != IndexPurposeV1 ||
		!canonicalDigest(index.IndexDigest) || index.IndexDigest != deriveIndexDigest(index) ||
		!validPluginIdentityV1(index.PluginName, index.PluginVersion) ||
		!canonicalDigest(index.GenerationID) || !canonicalRelativePath(index.ActiveRelativePath) ||
		!canonicalDigest(index.ReceiptID) || !canonicalDigest(index.ReceiptSHA256) || !canonicalDigest(index.IntentID) ||
		!canonicalDigest(index.SourceTreeSHA256) || index.SourceTreeFileCount == 0 || index.SourceTreeFileCount > MaxSourceTreeFilesV1 ||
		index.DiscoverableGenerationCount != DiscoverableGenerationV1 || index.FactToolsEnabled || !canonicalTime(index.CommittedAt) {
		return errors.New("bundled plugin materialization index is invalid")
	}
	return nil
}

func ValidateIndexForReceiptV1(index IndexV1, receipt ReceiptV1) error {
	if ValidateIndexV1(index) != nil || ValidateReceiptV1(receipt) != nil ||
		index.PluginName != receipt.PluginName || index.PluginVersion != receipt.PluginVersion ||
		index.GenerationID != receipt.GenerationID || index.ActiveRelativePath != receipt.ActiveRelativePath ||
		index.ReceiptID != receipt.ReceiptID || index.ReceiptSHA256 != ReceiptSHA256V1(receipt) ||
		index.IntentID != receipt.IntentID || index.SourceTreeSHA256 != receipt.SourceTreeSHA256 ||
		index.SourceTreeFileCount != receipt.SourceTreeFileCount {
		return errors.New("bundled plugin materialization index does not bind the receipt")
	}
	return nil
}

func IndexV1Bytes(index IndexV1) ([]byte, error) {
	return canonicalBytes(index, func() error { return ValidateIndexV1(index) })
}

func ParseIndexV1(body []byte) (IndexV1, error) {
	var index IndexV1
	err := strictParse(body, &index, func() error { return ValidateIndexV1(index) })
	return index, err
}

func deriveIndexDigest(index IndexV1) string {
	index.IndexDigest = ""
	return digestWithDomain("analytix.bundled-plugin-materialization-index/digest/v1", index)
}

func IndexSHA256V1(index IndexV1) string {
	body, err := IndexV1Bytes(index)
	if err != nil {
		return ""
	}
	return digestWithDomain("analytix.bundled-plugin-materialization-index/body/v1", json.RawMessage(body))
}
