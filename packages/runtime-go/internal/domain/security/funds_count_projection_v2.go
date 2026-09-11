package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	FundsCountProjectionSchemaVersionV2 = 2
	FundsCountProjectionPurposeV2       = "analytix.host.funds-count-projection/v2"
	FundsCountProjectionTableV2         = "analysis_txn_detail_idx"
	FundsCountProjectionMetaKeyV2       = "analytixFundsCountProjectionV2"
)

var fundsCountProjectionDigestDomainV2 = []byte("analytix.host.funds-count-projection/digest/v2\x00")

// FundsCountProjectionV2 is path-free, PII-free evidence material derived
// only from one exact callback-scoped DSV2/FPC selection. Copying this value
// does not preserve source, dataset, execution, receipt, or publication
// authority.
type FundsCountProjectionV2 struct {
	SchemaVersion                      int    `json:"schemaVersion"`
	Purpose                            string `json:"purpose"`
	TurnSecurityContextDigest          string `json:"turnSecurityContextDigest"`
	DatasetSnapshotID                  string `json:"datasetSnapshotId"`
	DatasetSelectionDigest             string `json:"datasetSelectionDigest"`
	DatasetRecordDigest                string `json:"datasetRecordDigest"`
	DatasetManifestDigest              string `json:"datasetManifestDigest"`
	FundsProducerContentID             string `json:"fundsProducerContentId"`
	FundsProducerContentManifestSHA256 string `json:"fundsProducerContentManifestSha256"`
	DetailContentSHA256                string `json:"detailContentSha256"`
	TableName                          string `json:"tableName"`
	RowCount                           string `json:"rowCount"`
	ProjectionDigest                   string `json:"projectionDigest"`
}

type FundsCountProjectionInputV2 struct {
	TurnSecurityContextDigest          string
	DatasetSnapshotID                  string
	DatasetSelectionDigest             string
	DatasetRecordDigest                string
	DatasetManifestDigest              string
	FundsProducerContentID             string
	FundsProducerContentManifestSHA256 string
	DetailContentSHA256                string
	RowCount                           uint64
}

func NewFundsCountProjectionV2(input FundsCountProjectionInputV2) (FundsCountProjectionV2, error) {
	projection := FundsCountProjectionV2{
		SchemaVersion:                      FundsCountProjectionSchemaVersionV2,
		Purpose:                            FundsCountProjectionPurposeV2,
		TurnSecurityContextDigest:          input.TurnSecurityContextDigest,
		DatasetSnapshotID:                  input.DatasetSnapshotID,
		DatasetSelectionDigest:             input.DatasetSelectionDigest,
		DatasetRecordDigest:                input.DatasetRecordDigest,
		DatasetManifestDigest:              input.DatasetManifestDigest,
		FundsProducerContentID:             input.FundsProducerContentID,
		FundsProducerContentManifestSHA256: input.FundsProducerContentManifestSHA256,
		DetailContentSHA256:                input.DetailContentSHA256,
		TableName:                          FundsCountProjectionTableV2,
		RowCount:                           strconv.FormatUint(input.RowCount, 10),
	}
	projection.ProjectionDigest = fundsCountProjectionDigestV2(projection)
	if err := ValidateFundsCountProjectionV2(projection); err != nil {
		return FundsCountProjectionV2{}, err
	}
	return projection, nil
}

func ValidateFundsCountProjectionV2(projection FundsCountProjectionV2) error {
	count, err := strconv.ParseUint(projection.RowCount, 10, 64)
	if projection.SchemaVersion != FundsCountProjectionSchemaVersionV2 ||
		projection.Purpose != FundsCountProjectionPurposeV2 ||
		!IsSHA256Hex(projection.TurnSecurityContextDigest) ||
		!IsDatasetSnapshotIDV2Syntax(projection.DatasetSnapshotID) ||
		!IsSHA256Hex(projection.DatasetSelectionDigest) ||
		!IsSHA256Hex(projection.DatasetRecordDigest) ||
		!IsSHA256Hex(projection.DatasetManifestDigest) ||
		!isSupportedFundsProducerContentIDForCountProjectionV2(projection.FundsProducerContentID) ||
		!IsSHA256Hex(projection.FundsProducerContentManifestSHA256) ||
		!IsSHA256Hex(projection.DetailContentSHA256) ||
		projection.DatasetSnapshotID != DatasetSnapshotIDPrefixV2+projection.DatasetManifestDigest ||
		projection.TableName != FundsCountProjectionTableV2 ||
		err != nil || strconv.FormatUint(count, 10) != projection.RowCount ||
		!IsSHA256Hex(projection.ProjectionDigest) ||
		projection.ProjectionDigest != fundsCountProjectionDigestV2(projection) {
		return errors.New("funds count projection v2 is invalid")
	}
	return nil
}

func isSupportedFundsProducerContentIDForCountProjectionV2(value string) bool {
	return IsFundsProducerContentIDV1Syntax(value) || IsFundsProducerContentIDV2Syntax(value)
}

func ParseFundsCountProjectionV2(value any) (FundsCountProjectionV2, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return FundsCountProjectionV2{}, err
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 * 1024, MaxDepth: 4, MaxTokens: 128, MaxStringBytes: 4096,
	}); err != nil {
		return FundsCountProjectionV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var projection FundsCountProjectionV2
	if err := decoder.Decode(&projection); err != nil {
		return FundsCountProjectionV2{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return FundsCountProjectionV2{}, errors.New("funds count projection v2 contains trailing JSON")
	}
	return projection, ValidateFundsCountProjectionV2(projection)
}

func FundsCountProjectionV2Record(projection FundsCountProjectionV2) (map[string]any, error) {
	if err := ValidateFundsCountProjectionV2(projection); err != nil {
		return nil, err
	}
	body, err := json.Marshal(projection)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	record := map[string]any{}
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	return record, nil
}

func fundsCountProjectionDigestV2(projection FundsCountProjectionV2) string {
	record := map[string]any{
		"datasetManifestDigest":              projection.DatasetManifestDigest,
		"datasetRecordDigest":                projection.DatasetRecordDigest,
		"datasetSelectionDigest":             projection.DatasetSelectionDigest,
		"datasetSnapshotId":                  projection.DatasetSnapshotID,
		"detailContentSha256":                projection.DetailContentSHA256,
		"fundsProducerContentId":             projection.FundsProducerContentID,
		"fundsProducerContentManifestSha256": projection.FundsProducerContentManifestSHA256,
		"projectionDigest":                   "",
		"purpose":                            projection.Purpose,
		"rowCount":                           projection.RowCount,
		"schemaVersion":                      projection.SchemaVersion,
		"tableName":                          projection.TableName,
		"turnSecurityContextDigest":          projection.TurnSecurityContextDigest,
	}
	body, _ := json.Marshal(record)
	return SHA256Hex(append(append([]byte(nil), fundsCountProjectionDigestDomainV2...), body...))
}
