package evidence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SourceRowProducerPolicySchemaVersionV1 = 1
	SourceRowProducerPolicyPurposeV1       = "analytix.source-row-producer-policy/v1"

	// FundsTransactionSourceRowPolicyIDV1 is the immutable legacy Rust/DuckDB
	// policy embedded in already-issued DSV2 graphs. Its operation and digest
	// must never be rewritten when a newer producer transport is composed.
	FundsTransactionSourceRowPolicyIDV1          = "funds.transaction-source-row-ledger/v1"
	FundsCanonicalTransactionSourceRowPolicyIDV1 = "funds.canonical-transaction-source-row-ledger/v1"
	FundsGoCountSourceRowPolicyIDV1              = "funds.go-strict-csv-count-source-row-ledger/v1"

	SourceRowProducerComponentDataEngineV1      = "data-engine"
	SourceRowProducerComponentRuntimeGoV1       = domainsecurity.FundsProducerComponentIDV2
	SourceRowProducerOperationFundsLegacyPageV1 = "transaction_source_row_page_v1"
	SourceRowProducerOperationFundsPageV1       = "funds.transaction_source_row_page_v1"
	SourceRowProducerOperationGoCountPageV1     = "build_count_case_rows_source_row_page_v1"
	SourceRowProducerRelationFundsV1            = "fc_transaction_raw"
	SourceRowProducerRelationGoCountV1          = "analytix_count_case_rows_source"
	SourceRowProducerArtifactManifestFundsV1    = "import_file_log"
	SourceRowProducerArtifactManifestGoCountV1  = "analytix_raw_artifact_manifest_v1"
	SourceRowProducerParserIDFundsV1            = "analytix.funds.transaction-row-parser"
	SourceRowProducerParserFundsV1              = "analytix.funds.transaction-row-parser/v1"
	SourceRowProducerLocatorOrderingV1          = "source_file_id_digest_asc+source_row_number_asc/v1"
	SourceRowProducerActivationTargetInactiveV1 = "target_inactive"
	SourceRowProducerActivationActiveV1         = "active"

	SourceRowRecordPathTemplateV1          = "/rows/{ordinal}"
	SourceRowRecordIDPathTemplateV1        = "/rows/{ordinal}/sourceRecordId"
	SourceRowSourceFileIDPathTemplateV1    = "/rows/{ordinal}/sourceFileId"
	SourceRowSourceRowNumberPathTemplateV1 = "/rows/{ordinal}/sourceRowNumber"

	maxSourceRowPolicyTextBytesV1 = 4 * 1024
)

var sourceRowProducerPolicyDigestDomainV1 = []byte("analytix.source-row-producer-policy/digest/v1\x00")

var fundsCanonicalCSVSourceFileIDDomainV1 = []byte("analytix.funds-source-file-id/v1\x00")

// DeriveFundsCanonicalCSVSourceFileIDV1 is the shared fixed identity contract
// used before native staging and revalidated on every native row page.
func DeriveFundsCanonicalCSVSourceFileIDV1(caseID, privateImportFileID string) (string, error) {
	if caseID == "" || caseID != strings.TrimSpace(caseID) ||
		!validRawArtifactHostSourceFileIDV1(privateImportFileID) {
		return "", errors.New("funds canonical CSV source file identity input is invalid")
	}
	hash := sha256.New()
	_, _ = hash.Write(fundsCanonicalCSVSourceFileIDDomainV1)
	for _, value := range []string{caseID, privateImportFileID} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))[:20], nil
}

// SourceRowProducerPolicyV1 is a closed host policy. It describes a typed,
// read-only producer shape; it does not advertise or enable a native
// operation. Runtime execution remains unavailable until the matching native
// operation, private row CAS, witnessed DSV2 authority, and settlement path
// are composed together.
type SourceRowProducerPolicyV1 struct {
	SchemaVersion               int         `json:"schemaVersion"`
	Purpose                     string      `json:"purpose"`
	PolicyID                    string      `json:"policyId"`
	SourceType                  string      `json:"sourceType"`
	ProducerComponentID         string      `json:"producerComponentId"`
	ProducerComponentVersion    string      `json:"producerComponentVersion"`
	Operation                   string      `json:"operation"`
	OperationSchemaHash         string      `json:"operationSchemaHash"`
	Relation                    string      `json:"relation"`
	ArtifactManifestRelation    string      `json:"artifactManifestRelation"`
	ParserID                    string      `json:"parserId"`
	ParserVersion               string      `json:"parserVersion"`
	LocatorOrdering             string      `json:"locatorOrdering"`
	RequiredSourceLocatorFields []string    `json:"requiredSourceLocatorFields"`
	LegacyLineageHintFields     []string    `json:"legacyLineageHintFields"`
	AllowedClaimTypes           []ClaimType `json:"allowedClaimTypes"`
	ActivationState             string      `json:"activationState"`
	CanMintClaims               bool        `json:"canMintClaims"`
	RecordPathTemplate          string      `json:"recordPathTemplate"`
	RecordIDPathTemplate        string      `json:"recordIdPathTemplate"`
	SourceFileIDPathTemplate    string      `json:"sourceFileIdPathTemplate"`
	SourceRowNumberPathTemplate string      `json:"sourceRowNumberPathTemplate"`
	ReadOnly                    bool        `json:"readOnly"`
	MaxRowsPerPage              uint32      `json:"maxRowsPerPage"`
	MaxPageBytes                uint64      `json:"maxPageBytes"`
	PolicyDigest                string      `json:"policyDigest"`
}

// ResolveSourceRowProducerPolicyV1 resolves only package-owned policies. MCP
// metadata, model output, settings, schemas, and database contents cannot add
// to this registry.
func ResolveSourceRowProducerPolicyV1(policyID string) (SourceRowProducerPolicyV1, bool) {
	if strings.TrimSpace(policyID) != policyID {
		return SourceRowProducerPolicyV1{}, false
	}
	var policy SourceRowProducerPolicyV1
	switch policyID {
	case FundsTransactionSourceRowPolicyIDV1:
		policy = fundsTransactionSourceRowPolicyV1()
	case FundsCanonicalTransactionSourceRowPolicyIDV1:
		policy = fundsCanonicalTransactionSourceRowPolicyV1()
	case FundsGoCountSourceRowPolicyIDV1:
		policy = fundsGoCountSourceRowPolicyV1()
	default:
		return SourceRowProducerPolicyV1{}, false
	}
	return cloneSourceRowProducerPolicyV1(policy), true
}

func ValidateSourceRowProducerPolicyV1(policy SourceRowProducerPolicyV1) error {
	registered, ok := ResolveSourceRowProducerPolicyV1(policy.PolicyID)
	if !ok || !equalSourceRowProducerPolicyV1(policy, registered) {
		return errors.New("source row producer policy is not registered")
	}
	return validateSourceRowProducerPolicyShapeV1(policy)
}

func SourceRowProducerPolicyAllowsClaimV1(policy SourceRowProducerPolicyV1, claimType ClaimType) bool {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || policy.ActivationState != "active" || !policy.CanMintClaims {
		return false
	}
	for _, allowed := range policy.AllowedClaimTypes {
		if claimType == allowed {
			return true
		}
	}
	return false
}

func ParseSourceRowProducerPolicyV1(raw []byte) (SourceRowProducerPolicyV1, error) {
	var policy SourceRowProducerPolicyV1
	if err := parseCanonicalSourceRowContractV1(raw, &policy, 128*1024, 512, maxSourceRowPolicyTextBytesV1); err != nil {
		return SourceRowProducerPolicyV1{}, err
	}
	return policy, ValidateSourceRowProducerPolicyV1(policy)
}

func SourceRowProducerPolicyV1Bytes(policy SourceRowProducerPolicyV1) ([]byte, error) {
	if err := ValidateSourceRowProducerPolicyV1(policy); err != nil {
		return nil, err
	}
	return json.Marshal(policy)
}

func fundsTransactionSourceRowPolicyV1() SourceRowProducerPolicyV1 {
	policy := SourceRowProducerPolicyV1{
		SchemaVersion:               SourceRowProducerPolicySchemaVersionV1,
		Purpose:                     SourceRowProducerPolicyPurposeV1,
		PolicyID:                    FundsTransactionSourceRowPolicyIDV1,
		SourceType:                  "transaction_source_occurrence_projection",
		ProducerComponentID:         SourceRowProducerComponentDataEngineV1,
		ProducerComponentVersion:    "analytix.data-engine-source-row/v1",
		Operation:                   SourceRowProducerOperationFundsLegacyPageV1,
		OperationSchemaHash:         "0e7e537d10dcc73e904224272dd6dec0c90aabda0bd553e3fb576c904ed9f261",
		Relation:                    SourceRowProducerRelationFundsV1,
		ArtifactManifestRelation:    SourceRowProducerArtifactManifestFundsV1,
		ParserID:                    SourceRowProducerParserIDFundsV1,
		ParserVersion:               SourceRowProducerParserFundsV1,
		LocatorOrdering:             SourceRowProducerLocatorOrderingV1,
		RequiredSourceLocatorFields: []string{"case_id", "file_id", "row_no"},
		LegacyLineageHintFields:     []string{"row_hash"},
		// The source row ledger establishes immutable row existence only. It
		// grants no semantic claim capability until a separate host-owned
		// entity/account resolver and mapping lineage are versioned.
		AllowedClaimTypes:           []ClaimType{},
		ActivationState:             SourceRowProducerActivationTargetInactiveV1,
		CanMintClaims:               false,
		RecordPathTemplate:          SourceRowRecordPathTemplateV1,
		RecordIDPathTemplate:        SourceRowRecordIDPathTemplateV1,
		SourceFileIDPathTemplate:    SourceRowSourceFileIDPathTemplateV1,
		SourceRowNumberPathTemplate: SourceRowSourceRowNumberPathTemplateV1,
		ReadOnly:                    true,
		MaxRowsPerPage:              100,
		MaxPageBytes:                1024 * 1024,
	}
	policy.PolicyDigest = sourceRowProducerPolicyDigestV1(policy)
	return policy
}

func fundsCanonicalTransactionSourceRowPolicyV1() SourceRowProducerPolicyV1 {
	policy := fundsTransactionSourceRowPolicyV1()
	policy.PolicyID = FundsCanonicalTransactionSourceRowPolicyIDV1
	policy.Operation = SourceRowProducerOperationFundsPageV1
	policy.OperationSchemaHash = fundsTransactionSourceRowOperationSchemaHashV1()
	policy.PolicyDigest = sourceRowProducerPolicyDigestV1(policy)
	return policy
}

func fundsGoCountSourceRowPolicyV1() SourceRowProducerPolicyV1 {
	policy := SourceRowProducerPolicyV1{
		SchemaVersion:               SourceRowProducerPolicySchemaVersionV1,
		Purpose:                     SourceRowProducerPolicyPurposeV1,
		PolicyID:                    FundsGoCountSourceRowPolicyIDV1,
		SourceType:                  "transaction_dataset_inventory_source_rows",
		ProducerComponentID:         SourceRowProducerComponentRuntimeGoV1,
		ProducerComponentVersion:    domainsecurity.FundsProducerComponentVersionV2,
		Operation:                   SourceRowProducerOperationGoCountPageV1,
		OperationSchemaHash:         fundsGoCountSourceRowOperationSchemaHashV1(),
		Relation:                    SourceRowProducerRelationGoCountV1,
		ArtifactManifestRelation:    SourceRowProducerArtifactManifestGoCountV1,
		ParserID:                    domainsecurity.FundsProducerParserIDV2,
		ParserVersion:               domainsecurity.FundsProducerParserVersionV2,
		LocatorOrdering:             SourceRowProducerLocatorOrderingV1,
		RequiredSourceLocatorFields: []string{"case_id", "file_id", "row_no"},
		LegacyLineageHintFields:     []string{},
		AllowedClaimTypes:           []ClaimType{ClaimCount},
		ActivationState:             SourceRowProducerActivationActiveV1,
		CanMintClaims:               true,
		RecordPathTemplate:          SourceRowRecordPathTemplateV1,
		RecordIDPathTemplate:        SourceRowRecordIDPathTemplateV1,
		SourceFileIDPathTemplate:    SourceRowSourceFileIDPathTemplateV1,
		SourceRowNumberPathTemplate: SourceRowSourceRowNumberPathTemplateV1,
		ReadOnly:                    true,
		MaxRowsPerPage:              100,
		MaxPageBytes:                1024 * 1024,
	}
	policy.PolicyDigest = sourceRowProducerPolicyDigestV1(policy)
	return policy
}

func validateSourceRowProducerPolicyShapeV1(policy SourceRowProducerPolicyV1) error {
	if policy.SchemaVersion != SourceRowProducerPolicySchemaVersionV1 || policy.Purpose != SourceRowProducerPolicyPurposeV1 ||
		!validSourceRowPolicyTextV1(policy.PolicyID) || !validSourceRowPolicyTextV1(policy.SourceType) ||
		!validSourceRowPolicyTextV1(policy.ProducerComponentID) || !validSourceRowPolicyTextV1(policy.ProducerComponentVersion) ||
		!validSourceRowPolicyTextV1(policy.Operation) || !validSourceRowSHA256V1(policy.OperationSchemaHash) ||
		!validSourceRowPolicyTextV1(policy.Relation) || !validSourceRowPolicyTextV1(policy.ArtifactManifestRelation) ||
		!validSourceRowPolicyTextV1(policy.ParserID) || !validSourceRowPolicyTextV1(policy.ParserVersion) ||
		policy.LocatorOrdering != SourceRowProducerLocatorOrderingV1 ||
		!validSourceRowProducerActivationV1(policy) ||
		!policy.ReadOnly || policy.MaxRowsPerPage == 0 || policy.MaxRowsPerPage > 500 ||
		policy.MaxPageBytes == 0 || policy.MaxPageBytes > 1024*1024 ||
		!validSourceRowPathTemplateV1(policy.RecordPathTemplate) ||
		!validSourceRowPathTemplateV1(policy.RecordIDPathTemplate) ||
		!validSourceRowPathTemplateV1(policy.SourceFileIDPathTemplate) ||
		!validSourceRowPathTemplateV1(policy.SourceRowNumberPathTemplate) ||
		!sourceRowPathTemplateWithinRecordV1(policy.RecordPathTemplate, policy.RecordIDPathTemplate) ||
		!sourceRowPathTemplateWithinRecordV1(policy.RecordPathTemplate, policy.SourceFileIDPathTemplate) ||
		!sourceRowPathTemplateWithinRecordV1(policy.RecordPathTemplate, policy.SourceRowNumberPathTemplate) ||
		!validSourceRowSHA256V1(policy.PolicyDigest) || policy.PolicyDigest != sourceRowProducerPolicyDigestV1(policy) {
		return errors.New("source row producer policy is invalid")
	}
	if len(policy.RequiredSourceLocatorFields) == 0 || policy.LegacyLineageHintFields == nil || policy.AllowedClaimTypes == nil {
		return errors.New("source row producer policy lineage contract is empty")
	}
	for index, field := range policy.RequiredSourceLocatorFields {
		if !validSourceRowPolicyTextV1(field) || index > 0 && policy.RequiredSourceLocatorFields[index-1] >= field {
			return errors.New("source row producer policy source locator fields are not canonical")
		}
	}
	for index, field := range policy.LegacyLineageHintFields {
		if !validSourceRowPolicyTextV1(field) || index > 0 && policy.LegacyLineageHintFields[index-1] >= field {
			return errors.New("source row producer policy legacy lineage hints are not canonical")
		}
	}
	for index, claimType := range policy.AllowedClaimTypes {
		if !validClaimType(claimType) || index > 0 && policy.AllowedClaimTypes[index-1] >= claimType {
			return errors.New("source row producer policy claim types are not canonical")
		}
	}
	return nil
}

func sourceRowProducerPolicyDigestV1(policy SourceRowProducerPolicyV1) string {
	policy.PolicyDigest = ""
	body, _ := json.Marshal(policy)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowProducerPolicyDigestDomainV1...), body...))
}

func fundsTransactionSourceRowOperationSchemaHashV1() string {
	return domainsecurity.SHA256Hex(fundsTransactionSourceRowOperationSchemaBytesV1())
}

func fundsTransactionSourceRowOperationSchemaBytesV1() []byte {
	sha256Schema := map[string]any{
		"type": "string", "pattern": "^[a-f0-9]{64}$", "minLength": 64, "maxLength": 64,
	}
	textSchema := map[string]any{
		"type": "string", "minLength": 1, "maxLength": maxSourceRowPolicyTextBytesV1,
	}
	integerSchema := map[string]any{
		"type": "integer", "minimum": 1, "maximum": maxSourceRowJSONIntegerV1,
	}
	cursorSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"sourceFileIdDigest", "sourceRowNumber"},
		"properties": map[string]any{
			"sourceFileIdDigest": sha256Schema,
			"sourceRowNumber":    integerSchema,
		},
	}
	typedScalarSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"kind", "value"},
		"properties": map[string]any{
			"kind": map[string]any{
				"type": "string", "enum": []string{"null", "integer", "decimal", "text"},
			},
			"value": map[string]any{"type": "string", "maxLength": 16 * 1024},
		},
	}
	rowSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{
			"sourceFileId", "sourceFileIdDigest", "sourceRowNumber", "sourceArtifactSha256",
			"canonicalRowSha256", "canonicalTypedRow", "disposition",
		},
		"properties": map[string]any{
			"sourceFileId":         map[string]any{"type": "string", "pattern": "^[a-f0-9]{20}$", "minLength": 20, "maxLength": 20},
			"sourceFileIdDigest":   sha256Schema,
			"sourceRowNumber":      integerSchema,
			"sourceArtifactSha256": sha256Schema,
			"canonicalRowSha256":   sha256Schema,
			"canonicalTypedRow": map[string]any{
				"type": "array", "minItems": 43, "maxItems": 43,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required":   []string{"name", "scalar"},
					"properties": map[string]any{"name": textSchema, "scalar": typedScalarSchema},
				},
			},
			"disposition": map[string]any{"type": "string", "const": "accepted"},
		},
	}
	inventorySchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{
			"sourceFileId", "sourceFileIdDigest", "privateImportFileId", "sourceArtifactSha256",
			"fileType", "rowCount", "acceptedRowCount", "rejectedRowCount",
		},
		"properties": map[string]any{
			"sourceFileId":         map[string]any{"type": "string", "pattern": "^[a-f0-9]{20}$", "minLength": 20, "maxLength": 20},
			"sourceFileIdDigest":   sha256Schema,
			"privateImportFileId":  textSchema,
			"sourceArtifactSha256": sha256Schema,
			"fileType":             map[string]any{"type": "string", "const": "CSV"},
			"rowCount":             integerSchema,
			"acceptedRowCount":     integerSchema,
			"rejectedRowCount":     map[string]any{"type": "integer", "const": 0},
		},
	}
	snapshotSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{
			"sourceRevision", "sourceRowCount", "sourceMaxTxnTs", "sourceMaxId", "acceptedRowCount",
			"rejectedRowCount", "duplicateRowCount", "inventoryDigest", "sourceSnapshotDigest",
		},
		"properties": map[string]any{
			"sourceRevision":       integerSchema,
			"sourceRowCount":       integerSchema,
			"sourceMaxTxnTs":       map[string]any{"type": "string", "maxLength": 64},
			"sourceMaxId":          integerSchema,
			"acceptedRowCount":     integerSchema,
			"rejectedRowCount":     map[string]any{"type": "integer", "const": 0},
			"duplicateRowCount":    map[string]any{"type": "integer", "const": 0},
			"inventoryDigest":      sha256Schema,
			"sourceSnapshotDigest": sha256Schema,
		},
	}
	shape := map[string]any{
		"schemaVersion": 1,
		"inputSchema": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"bindingKeyDigest", "caseId", "cursor", "maxRows", "parsedGenerationIdentitySha256", "relation"},
			"properties": map[string]any{
				"bindingKeyDigest":               sha256Schema,
				"caseId":                         textSchema,
				"cursor":                         map[string]any{"anyOf": []any{map[string]any{"type": "null"}, cursorSchema}},
				"maxRows":                        map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
				"parsedGenerationIdentitySha256": sha256Schema,
				"relation":                       map[string]any{"type": "string", "const": SourceRowProducerRelationFundsV1},
			},
		},
		"outputSchema": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{
				"schemaVersion", "operation", "caseId", "bindingKeyDigest", "parsedGenerationIdentitySha256",
				"relation", "materializationIdentity", "producerContentContract", "producerContentId",
				"producerContentManifestSha256", "rawArtifactManifestSha256", "duckdbContentSnapshotDigest",
				"duckdbSnapshotManifestSha256", "analyticalSchemaDigest", "parserId", "parserVersion",
				"locatorOrdering", "sourceProofStatus", "requiredHostRawReplayProfile", "hostRawReplayRequired",
				"sourceSnapshot", "inventory", "rows", "nextCursor", "complete", "pageDigest",
			},
			"properties": map[string]any{
				"schemaVersion":                  map[string]any{"type": "integer", "const": 1},
				"operation":                      map[string]any{"type": "string", "const": SourceRowProducerOperationFundsPageV1},
				"caseId":                         textSchema,
				"bindingKeyDigest":               sha256Schema,
				"parsedGenerationIdentitySha256": sha256Schema,
				"relation":                       map[string]any{"type": "string", "const": SourceRowProducerRelationFundsV1},
				"materializationIdentity":        textSchema,
				"producerContentContract":        textSchema,
				"producerContentId":              textSchema,
				"producerContentManifestSha256":  sha256Schema,
				"rawArtifactManifestSha256":      sha256Schema,
				"duckdbContentSnapshotDigest":    sha256Schema,
				"duckdbSnapshotManifestSha256":   sha256Schema,
				"analyticalSchemaDigest":         sha256Schema,
				"parserId":                       map[string]any{"type": "string", "const": SourceRowProducerParserIDFundsV1},
				"parserVersion":                  map[string]any{"type": "string", "const": SourceRowProducerParserFundsV1},
				"locatorOrdering":                map[string]any{"type": "string", "const": SourceRowProducerLocatorOrderingV1},
				"sourceProofStatus":              map[string]any{"type": "string", "const": FundsTransactionNativeSourceProofStatusV1},
				"requiredHostRawReplayProfile":   map[string]any{"type": "string", "const": FundsTransactionCSVReplayProfileV1},
				"hostRawReplayRequired":          map[string]any{"type": "boolean", "const": true},
				"sourceSnapshot":                 snapshotSchema,
				"inventory":                      map[string]any{"type": "array", "minItems": 1, "maxItems": 4096, "items": inventorySchema},
				"rows":                           map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": rowSchema},
				"nextCursor":                     map[string]any{"anyOf": []any{map[string]any{"type": "null"}, cursorSchema}},
				"complete":                       map[string]any{"type": "boolean"},
				"pageDigest":                     sha256Schema,
			},
		},
	}
	body, _ := json.Marshal(shape)
	return body
}

func fundsGoCountSourceRowOperationSchemaHashV1() string {
	return domainsecurity.SHA256Hex(goCountSourceRowOperationSchemaBytesV1(
		SourceRowProducerRelationGoCountV1,
		domainsecurity.FundsProducerParserIDV2,
		domainsecurity.FundsProducerParserVersionV2,
	))
}

func goCountSourceRowOperationSchemaBytesV1(relation, parserID, parserVersion string) []byte {
	sha256Schema := map[string]any{
		"type": "string", "pattern": "^[a-f0-9]{64}$", "minLength": 64, "maxLength": 64,
	}
	textSchema := map[string]any{
		"type": "string", "minLength": 1, "maxLength": maxSourceRowPolicyTextBytesV1,
	}
	integerSchema := map[string]any{
		"type": "integer", "minimum": 1, "maximum": maxSourceRowJSONIntegerV1,
	}
	fundsSourceFileIDSchema := map[string]any{
		"type": "string", "pattern": "^[a-f0-9]{20}$", "minLength": 20, "maxLength": 20,
	}
	typedScalarSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"kind", "value"},
		"properties": map[string]any{
			"kind": map[string]any{
				"type": "string",
				"enum": []string{"null", "bool", "integer", "decimal", "float_hex", "timestamp_utc", "text", "bytes_hex"},
			},
			"value": map[string]any{"type": "string", "maxLength": 1024 * 1024},
		},
	}
	lineageSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"generationReceiptDigest", "mappingDigest", "projectionSchemaDigest", "readerModeDigest"},
		"properties": map[string]any{
			"generationReceiptDigest": sha256Schema,
			"mappingDigest":           sha256Schema,
			"projectionSchemaDigest":  sha256Schema,
			"readerModeDigest":        sha256Schema,
		},
	}
	rowSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"canonicalRowSha256", "canonicalTypedRow", "lineage", "sourceArtifactSha256", "sourceFileId", "sourceFileIdDigest", "sourceRowNumber"},
		"properties": map[string]any{
			"canonicalRowSha256": sha256Schema,
			"canonicalTypedRow": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 256,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required":   []string{"name", "scalar"},
					"properties": map[string]any{"name": textSchema, "scalar": typedScalarSchema},
				},
			},
			"lineage":              lineageSchema,
			"sourceArtifactSha256": sha256Schema,
			"sourceFileId":         fundsSourceFileIDSchema,
			"sourceFileIdDigest":   sha256Schema,
			"sourceRowNumber":      integerSchema,
		},
	}
	shape := map[string]any{
		"schemaVersion": 1,
		"inputSchema": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"bindingKeyDigest", "caseId", "maxRows", "pageNumber", "parsedGenerationSha256", "relation"},
			"properties": map[string]any{
				"bindingKeyDigest":       sha256Schema,
				"caseId":                 textSchema,
				"maxRows":                map[string]any{"type": "integer", "const": 100},
				"pageNumber":             integerSchema,
				"parsedGenerationSha256": sha256Schema,
				"relation":               map[string]any{"type": "string", "const": relation},
			},
		},
		"outputSchema": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"locatorOrdering", "parsedGenerationSha256", "parserId", "parserVersion", "relation", "rows", "schemaVersion"},
			"properties": map[string]any{
				"schemaVersion":          map[string]any{"type": "integer", "const": 1},
				"relation":               map[string]any{"type": "string", "const": relation},
				"parserId":               map[string]any{"type": "string", "const": parserID},
				"parserVersion":          map[string]any{"type": "string", "const": parserVersion},
				"locatorOrdering":        map[string]any{"type": "string", "const": SourceRowProducerLocatorOrderingV1},
				"parsedGenerationSha256": sha256Schema,
				"rows":                   map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": rowSchema},
			},
		},
	}
	body, _ := json.Marshal(shape)
	return body
}

func cloneSourceRowProducerPolicyV1(policy SourceRowProducerPolicyV1) SourceRowProducerPolicyV1 {
	policy.RequiredSourceLocatorFields = append([]string(nil), policy.RequiredSourceLocatorFields...)
	if policy.LegacyLineageHintFields != nil {
		legacyHints := make([]string, len(policy.LegacyLineageHintFields))
		copy(legacyHints, policy.LegacyLineageHintFields)
		policy.LegacyLineageHintFields = legacyHints
	}
	if policy.AllowedClaimTypes != nil {
		allowed := make([]ClaimType, len(policy.AllowedClaimTypes))
		copy(allowed, policy.AllowedClaimTypes)
		policy.AllowedClaimTypes = allowed
	}
	return policy
}

func equalSourceRowProducerPolicyV1(left, right SourceRowProducerPolicyV1) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftBody) == string(rightBody)
}

func validSourceRowPathTemplateV1(value string) bool {
	if strings.Count(value, "{ordinal}") != 1 {
		return false
	}
	expanded := strings.Replace(value, "{ordinal}", "0", 1)
	return validSourceFieldPathV2(expanded)
}

func validSourceFileIDForPolicyV1(policy SourceRowProducerPolicyV1, value string) bool {
	if ValidateSourceRowProducerPolicyV1(policy) != nil ||
		(policy.PolicyID != FundsTransactionSourceRowPolicyIDV1 &&
			policy.PolicyID != FundsCanonicalTransactionSourceRowPolicyIDV1 &&
			policy.PolicyID != FundsGoCountSourceRowPolicyIDV1) ||
		len(value) != 20 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func validSourceRowProducerActivationV1(policy SourceRowProducerPolicyV1) bool {
	switch policy.ActivationState {
	case SourceRowProducerActivationTargetInactiveV1:
		return !policy.CanMintClaims && len(policy.AllowedClaimTypes) == 0
	case SourceRowProducerActivationActiveV1:
		return policy.CanMintClaims && len(policy.AllowedClaimTypes) > 0
	default:
		return false
	}
}

func sourceRowPathTemplateWithinRecordV1(recordTemplate, fieldTemplate string) bool {
	record := strings.Replace(recordTemplate, "{ordinal}", "0", 1)
	field := strings.Replace(fieldTemplate, "{ordinal}", "0", 1)
	return sourceJSONPointerWithinRecordV2(record, field)
}

func expandSourceRowPathTemplateV1(template string, ordinal uint32) (string, error) {
	if !validSourceRowPathTemplateV1(template) {
		return "", errors.New("source row path template is invalid")
	}
	path := strings.Replace(template, "{ordinal}", sourceRowOrdinalTextV1(ordinal), 1)
	if !validSourceFieldPathV2(path) {
		return "", errors.New("source row path is invalid")
	}
	return path, nil
}

func sourceRowOrdinalTextV1(ordinal uint32) string {
	const digits = "0123456789"
	if ordinal == 0 {
		return "0"
	}
	buffer := make([]byte, 0, 10)
	for ordinal > 0 {
		buffer = append(buffer, digits[ordinal%10])
		ordinal /= 10
	}
	for left, right := 0, len(buffer)-1; left < right; left, right = left+1, right-1 {
		buffer[left], buffer[right] = buffer[right], buffer[left]
	}
	return string(buffer)
}

func validSourceRowPolicyTextV1(value string) bool {
	if value == "" || len(value) > maxSourceRowPolicyTextBytesV1 || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
	}
	return true
}
