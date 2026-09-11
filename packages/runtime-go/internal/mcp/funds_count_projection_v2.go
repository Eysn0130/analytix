package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

const (
	fundsSourceProbeSchemaVersionV2 = 2
	fundsSourceProbePurposeV2       = "analytix.funds-source-probe/v2"
	fundsCountToolOutcomePurposeV2  = "analytix.funds-count-tool-outcome/v2"
)

type fundsSourceProbeResponseV2 struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Purpose          string `json:"purpose"`
	ServerName       string `json:"serverName"`
	ServerVersion    string `json:"serverVersion"`
	ProjectionDigest string `json:"projectionDigest"`
	Ready            bool   `json:"ready"`
	ReadOnly         bool   `json:"readOnly"`
}

type fundsCountCanaryObservationV2 struct {
	projectionDigest string
	rawResultSHA256  string
	grantGeneration  uint64
	connectionEpoch  uint64
}

func fundsCountProjectionForSelectionV2(
	securityContext domainsecurity.TurnSecurityContext,
	selection datasetsnapshotport.CurrentSelectionV2,
) (domainsecurity.FundsCountProjectionV2, error) {
	if !datasetSelectionMatchesSourceContextV2(selection, securityContext) {
		return domainsecurity.FundsCountProjectionV2{}, errors.New("funds count projection dataset selection is invalid")
	}
	producer, err := domainsecurity.ResolveFundsProducerContentBindingV2(
		selection.Snapshot.Manifest,
		selection.Snapshot.FundsProducerContent,
		selection.Snapshot.FundsProducerContentV2,
	)
	if err != nil {
		return domainsecurity.FundsCountProjectionV2{}, err
	}
	projection, err := domainsecurity.NewFundsCountProjectionV2(domainsecurity.FundsCountProjectionInputV2{
		TurnSecurityContextDigest:          securityContext.ContextDigest,
		DatasetSnapshotID:                  selection.Snapshot.Record.DatasetSnapshotID,
		DatasetSelectionDigest:             selection.SelectionDigest,
		DatasetRecordDigest:                selection.Snapshot.Record.RecordDigest,
		DatasetManifestDigest:              selection.Snapshot.Manifest.ManifestDigest,
		FundsProducerContentID:             producer.ID,
		FundsProducerContentManifestSHA256: producer.ManifestSHA256,
		DetailContentSHA256:                producer.DetailContentSHA256,
		RowCount:                           producer.DetailRowCount,
	})
	if err != nil {
		return domainsecurity.FundsCountProjectionV2{}, err
	}
	return projection, nil
}

func fundsCountProjectionMetaV2(projection domainsecurity.FundsCountProjectionV2) (map[string]any, error) {
	record, err := domainsecurity.FundsCountProjectionV2Record(projection)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		domainsecurity.FundsCountProjectionMetaKeyV2: record,
	}, nil
}

func sourceProbeNativeParamsV2(projection domainsecurity.FundsCountProjectionV2) (map[string]any, error) {
	meta, err := fundsCountProjectionMetaV2(projection)
	if err != nil {
		return nil, err
	}
	return map[string]any{"_meta": meta}, nil
}

func fundsCountToolNativeParamsV2(
	arguments map[string]any,
	projection domainsecurity.FundsCountProjectionV2,
) (map[string]any, error) {
	meta, err := fundsCountProjectionMetaV2(projection)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"name": "count_case_rows", "arguments": arguments, "_meta": meta,
	}, nil
}

func fundsEvidenceReadNativeParamsV2(
	projection domainsecurity.FundsCountProjectionV2,
) (map[string]any, error) {
	meta, err := fundsCountProjectionMetaV2(projection)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"tool": "count_case_rows", "tableName": domainsecurity.FundsCountProjectionTableV2,
		"noFilter": true, "_meta": meta,
	}, nil
}

func parseFundsSourceProbeResponseV2(
	value any,
	expected domainsecurity.FundsCountProjectionV2,
	serverName string,
	serverVersion string,
) (fundsSourceProbeResponseV2, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return fundsSourceProbeResponseV2{}, err
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 16 * 1024, MaxDepth: 4, MaxTokens: 64, MaxStringBytes: 4096,
	}); err != nil {
		return fundsSourceProbeResponseV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var response fundsSourceProbeResponseV2
	if err := decoder.Decode(&response); err != nil {
		return fundsSourceProbeResponseV2{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fundsSourceProbeResponseV2{}, errors.New("funds source probe v2 contains trailing JSON")
	}
	if domainsecurity.ValidateFundsCountProjectionV2(expected) != nil ||
		response.SchemaVersion != fundsSourceProbeSchemaVersionV2 ||
		response.Purpose != fundsSourceProbePurposeV2 ||
		response.ServerName != serverName ||
		response.ServerVersion != serverVersion ||
		response.ProjectionDigest != expected.ProjectionDigest ||
		!response.Ready || !response.ReadOnly {
		return fundsSourceProbeResponseV2{}, errors.New("funds source probe v2 contradicts host projection")
	}
	return response, nil
}

func callFundsCountToolWithProjectionV2(
	ctx context.Context,
	client mcpTransportClient,
	arguments map[string]any,
	projection domainsecurity.FundsCountProjectionV2,
) (domainmcp.LosslessToolResult, error) {
	native, ok := client.(mcpNativeEvidenceClient)
	if !ok {
		return domainmcp.LosslessToolResult{}, errors.New("MCP transport cannot carry the funds count projection")
	}
	params, err := fundsCountToolNativeParamsV2(arguments, projection)
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	return native.CallNativeLosslessContext(ctx, "tools/call", params)
}

func validateFundsCountToolResultV2(
	result domainmcp.LosslessToolResult,
	expected domainsecurity.FundsCountProjectionV2,
) (fundsCountCanaryObservationV2, error) {
	if !domainmcp.ValidLosslessToolResult(result) ||
		result.RawSHA256 != domainsecurity.SHA256Hex(result.RawResult) ||
		result.HostPrivate != nil || domainsecurity.ValidateFundsCountProjectionV2(expected) != nil {
		return fundsCountCanaryObservationV2{}, errors.New("funds count canary lossless result is invalid")
	}
	top, err := domainjsonstrict.DecodeRawObject(result.RawResult, domainjsonstrict.Options{
		MaxBytes: 64 * 1024, MaxDepth: 6, MaxTokens: 192, MaxStringBytes: 4096,
	})
	if err != nil || !rawObjectHasExactKeysV2(top, "content", "structuredContent") {
		return fundsCountCanaryObservationV2{}, errors.New("funds count canary result envelope is invalid")
	}
	var content []json.RawMessage
	if len(bytes.TrimSpace(top["content"])) == 0 || bytes.TrimSpace(top["content"])[0] != '[' ||
		json.Unmarshal(top["content"], &content) != nil || content == nil || len(content) != 0 {
		return fundsCountCanaryObservationV2{}, errors.New("funds count canary content is invalid")
	}
	structured, err := domainjsonstrict.DecodeRawObject(top["structuredContent"], domainjsonstrict.Options{
		MaxBytes: 64 * 1024, MaxDepth: 5, MaxTokens: 176, MaxStringBytes: 4096,
	})
	if err != nil || !rawObjectHasExactKeysV2(
		structured, "schemaVersion", "purpose", "semanticStatus", "data",
	) {
		return fundsCountCanaryObservationV2{}, errors.New("funds count canary structured result is invalid")
	}
	var header struct {
		SchemaVersion  int    `json:"schemaVersion"`
		Purpose        string `json:"purpose"`
		SemanticStatus string `json:"semanticStatus"`
	}
	headerBody, _ := json.Marshal(map[string]json.RawMessage{
		"schemaVersion":  structured["schemaVersion"],
		"purpose":        structured["purpose"],
		"semanticStatus": structured["semanticStatus"],
	})
	decoder := json.NewDecoder(bytes.NewReader(headerBody))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&header) != nil || header.SchemaVersion != 2 ||
		header.Purpose != fundsCountToolOutcomePurposeV2 || header.SemanticStatus != "success" {
		return fundsCountCanaryObservationV2{}, errors.New("funds count canary outcome header is invalid")
	}
	decoder = json.NewDecoder(bytes.NewReader(structured["data"]))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var observed domainsecurity.FundsCountProjectionV2
	if decoder.Decode(&observed) != nil {
		return fundsCountCanaryObservationV2{}, errors.New("funds count canary projection is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) ||
		domainsecurity.ValidateFundsCountProjectionV2(observed) != nil || !reflect.DeepEqual(observed, expected) {
		return fundsCountCanaryObservationV2{}, errors.New("funds count canary projection contradicts host authority")
	}
	return fundsCountCanaryObservationV2{
		projectionDigest: observed.ProjectionDigest,
		rawResultSHA256:  result.RawSHA256,
	}, nil
}

func rawObjectHasExactKeysV2(object map[string]json.RawMessage, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}
