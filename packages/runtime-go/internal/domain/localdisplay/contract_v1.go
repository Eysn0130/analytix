package localdisplay

import (
	"encoding/json"
	"errors"
	"regexp"
	"unicode/utf8"
)

const (
	SchemaV1                = "typed-local-data-surface/v1"
	ResponseSchemaVersionV1 = 1

	KindImportMappingPreviewV1 = "import_mapping_preview"
	KindCleaningDiffPreviewV1  = "cleaning_diff_preview"
	KindDirectSourcePreviewV1  = "direct_source_preview"
	KindAcceptedSlotDisplayV1  = "accepted_slot_display"

	DisplayModeFullV1   = "full"
	DisplayModeMaskedV1 = "masked"

	MaximumRowOffsetV1     = uint32(100_000)
	MaximumRowLimitV1      = uint16(100)
	MaximumCellBytesV1     = 4 * 1024
	MaximumResponseBytesV1 = 1 * 1024 * 1024
)

const CanonicalContractJSONV1 = `{"schema":"typed-local-data-surface/v1","kinds":["import_mapping_preview","cleaning_diff_preview","direct_source_preview","accepted_slot_display"],"displayModes":["full","masked"],"fields":{"import_mapping_preview":["sourceColumn","sampleValue","inferredType","targetField","parseStatus","mappingStatus"],"cleaning_diff_preview":["transactionTime","account","card","accountName","identityNumber","amountText","direction","counterpartyAccount","counterpartyName","counterpartyIdentityNumber","counterpartyBank","summary","currency","merchantName","remark"],"direct_source_preview":["transactionTime","account","card","accountName","identityNumber","amountText","direction","counterpartyAccount","counterpartyName","counterpartyIdentityNumber","counterpartyBank","summary","currency","merchantName","remark"],"accepted_slot_display":["account","card"]},"statuses":{"importParse":["parsed","invalid","unsupported"],"importMapping":["mapped","unmapped","conflict"],"cleaning":["unchanged","changed","added","removed","invalid"]},"selectors":{"request":"^tlsel1_[a-f0-9]{64}$","generation":"^tlgen1_[a-f0-9]{64}$","snapshot":"^tlsnap1_[a-f0-9]{64}$","lineage":"^tllin1_[a-f0-9]{64}$"},"requestProperties":{"import_mapping_preview":["kind","selector","fields","rowOffset","rowLimit","displayMode"],"cleaning_diff_preview":["kind","selector","fields","rowOffset","rowLimit","displayMode"],"direct_source_preview":["kind","view","fields","rowOffset","rowLimit","displayMode"],"accepted_slot_display":["kind","threadId","turnId","acceptedFinalDigest","displayMode"]},"responseProperties":{"import_mapping_preview":["schemaVersion","kind","selector","lineage","displayMode","fields","rowOffset","rowLimit","hasMore","rows"],"cleaning_diff_preview":["schemaVersion","kind","selector","lineage","displayMode","fields","rowOffset","rowLimit","hasMore","rows"],"direct_source_preview":["schemaVersion","kind","caseId","datasetSnapshotId","displayMode","view","fields","rowOffset","rowLimit","hasMore","rows"],"accepted_slot_display":["schemaVersion","kind","threadId","turnId","acceptedFinalDigest","caseId","datasetSnapshotId","contextEpoch","displayMode","slots"]},"lineageProperties":{"import_mapping_preview":["importGeneration","sourceItemGeneration","parserGeneration","mappingGeneration"],"cleaning_diff_preview":["inputSnapshot","ruleGeneration","ruleDigest","outputSnapshot","transformLineage"],"direct_source_preview":[],"accepted_slot_display":[]},"rowProperties":{"import_mapping_preview":["rowIndex","parseStatus","mappingStatus","cells"],"cleaning_diff_preview":["rowIndex","status","cells"],"direct_source_preview":["rowIndex","cells"],"accepted_slot_display":["slotId","field","displayValue","claimIds","receiptIds"]},"cellProperties":{"import_mapping_preview":["field","displayValue"],"cleaning_diff_preview":["field","beforeDisplayValue","afterDisplayValue"],"direct_source_preview":["field","displayValue"],"accepted_slot_display":[]},"limits":{"rowOffset":100000,"rowLimit":100,"cellBytes":4096,"responseBytes":1048576}}`

var (
	errInvalidContractV1 = errors.New("typed-local-data-surface/v1 value is invalid")
	selectorPatternV1    = regexp.MustCompile(`^tlsel1_[a-f0-9]{64}$`)
	generationPatternV1  = regexp.MustCompile(`^tlgen1_[a-f0-9]{64}$`)
	snapshotPatternV1    = regexp.MustCompile(`^tlsnap1_[a-f0-9]{64}$`)
	lineagePatternV1     = regexp.MustCompile(`^tllin1_[a-f0-9]{64}$`)
	digestPatternV1      = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

var importMappingPreviewFieldsV1 = []string{
	"sourceColumn", "sampleValue", "inferredType", "targetField", "parseStatus", "mappingStatus",
}

var directSourcePreviewFieldsV1 = []string{
	"transactionTime", "account", "card", "accountName", "identityNumber", "amountText", "direction",
	"counterpartyAccount", "counterpartyName", "counterpartyIdentityNumber", "counterpartyBank",
	"summary", "currency", "merchantName", "remark",
}

type canonicalFieldsV1 struct {
	ImportMappingPreview []string `json:"import_mapping_preview"`
	CleaningDiffPreview  []string `json:"cleaning_diff_preview"`
	DirectSourcePreview  []string `json:"direct_source_preview"`
	AcceptedSlotDisplay  []string `json:"accepted_slot_display"`
}

type canonicalStatusesV1 struct {
	ImportParse   []string `json:"importParse"`
	ImportMapping []string `json:"importMapping"`
	Cleaning      []string `json:"cleaning"`
}

type canonicalSelectorsV1 struct {
	Request    string `json:"request"`
	Generation string `json:"generation"`
	Snapshot   string `json:"snapshot"`
	Lineage    string `json:"lineage"`
}

type canonicalLimitsV1 struct {
	RowOffset     uint32 `json:"rowOffset"`
	RowLimit      uint16 `json:"rowLimit"`
	CellBytes     int    `json:"cellBytes"`
	ResponseBytes int    `json:"responseBytes"`
}

type canonicalContractV1 struct {
	Schema             string               `json:"schema"`
	Kinds              []string             `json:"kinds"`
	DisplayModes       []string             `json:"displayModes"`
	Fields             canonicalFieldsV1    `json:"fields"`
	Statuses           canonicalStatusesV1  `json:"statuses"`
	Selectors          canonicalSelectorsV1 `json:"selectors"`
	RequestProperties  canonicalFieldsV1    `json:"requestProperties"`
	ResponseProperties canonicalFieldsV1    `json:"responseProperties"`
	LineageProperties  canonicalFieldsV1    `json:"lineageProperties"`
	RowProperties      canonicalFieldsV1    `json:"rowProperties"`
	CellProperties     canonicalFieldsV1    `json:"cellProperties"`
	Limits             canonicalLimitsV1    `json:"limits"`
}

func CanonicalContractV1() canonicalContractV1 {
	return canonicalContractV1{
		Schema: SchemaV1,
		Kinds: []string{
			KindImportMappingPreviewV1, KindCleaningDiffPreviewV1,
			KindDirectSourcePreviewV1, KindAcceptedSlotDisplayV1,
		},
		DisplayModes: []string{DisplayModeFullV1, DisplayModeMaskedV1},
		Fields: canonicalFieldsV1{
			ImportMappingPreview: ImportMappingPreviewFieldsV1(),
			CleaningDiffPreview:  CleaningDiffPreviewFieldsV1(),
			DirectSourcePreview:  DirectSourcePreviewFieldsV1(),
			AcceptedSlotDisplay:  []string{"account", "card"},
		},
		Statuses: canonicalStatusesV1{
			ImportParse:   []string{"parsed", "invalid", "unsupported"},
			ImportMapping: []string{"mapped", "unmapped", "conflict"},
			Cleaning:      []string{"unchanged", "changed", "added", "removed", "invalid"},
		},
		Selectors: canonicalSelectorsV1{
			Request: selectorPatternV1.String(), Generation: generationPatternV1.String(),
			Snapshot: snapshotPatternV1.String(), Lineage: lineagePatternV1.String(),
		},
		RequestProperties: canonicalFieldsV1{
			ImportMappingPreview: ContractRequestPropertiesV1(KindImportMappingPreviewV1),
			CleaningDiffPreview:  ContractRequestPropertiesV1(KindCleaningDiffPreviewV1),
			DirectSourcePreview:  ContractRequestPropertiesV1(KindDirectSourcePreviewV1),
			AcceptedSlotDisplay:  ContractRequestPropertiesV1(KindAcceptedSlotDisplayV1),
		},
		ResponseProperties: canonicalFieldsV1{
			ImportMappingPreview: ContractResponsePropertiesV1(KindImportMappingPreviewV1),
			CleaningDiffPreview:  ContractResponsePropertiesV1(KindCleaningDiffPreviewV1),
			DirectSourcePreview:  ContractResponsePropertiesV1(KindDirectSourcePreviewV1),
			AcceptedSlotDisplay:  ContractResponsePropertiesV1(KindAcceptedSlotDisplayV1),
		},
		LineageProperties: canonicalFieldsV1{
			ImportMappingPreview: ContractLineagePropertiesV1(KindImportMappingPreviewV1),
			CleaningDiffPreview:  ContractLineagePropertiesV1(KindCleaningDiffPreviewV1),
			DirectSourcePreview:  ContractLineagePropertiesV1(KindDirectSourcePreviewV1),
			AcceptedSlotDisplay:  ContractLineagePropertiesV1(KindAcceptedSlotDisplayV1),
		},
		RowProperties: canonicalFieldsV1{
			ImportMappingPreview: ContractRowPropertiesV1(KindImportMappingPreviewV1),
			CleaningDiffPreview:  ContractRowPropertiesV1(KindCleaningDiffPreviewV1),
			DirectSourcePreview:  ContractRowPropertiesV1(KindDirectSourcePreviewV1),
			AcceptedSlotDisplay:  ContractRowPropertiesV1(KindAcceptedSlotDisplayV1),
		},
		CellProperties: canonicalFieldsV1{
			ImportMappingPreview: ContractCellPropertiesV1(KindImportMappingPreviewV1),
			CleaningDiffPreview:  ContractCellPropertiesV1(KindCleaningDiffPreviewV1),
			DirectSourcePreview:  ContractCellPropertiesV1(KindDirectSourcePreviewV1),
			AcceptedSlotDisplay:  ContractCellPropertiesV1(KindAcceptedSlotDisplayV1),
		},
		Limits: canonicalLimitsV1{
			RowOffset: MaximumRowOffsetV1, RowLimit: MaximumRowLimitV1,
			CellBytes: MaximumCellBytesV1, ResponseBytes: MaximumResponseBytesV1,
		},
	}
}

func ContractRequestPropertiesV1(kind string) []string {
	switch kind {
	case KindImportMappingPreviewV1, KindCleaningDiffPreviewV1:
		return []string{"kind", "selector", "fields", "rowOffset", "rowLimit", "displayMode"}
	case KindDirectSourcePreviewV1:
		return []string{"kind", "view", "fields", "rowOffset", "rowLimit", "displayMode"}
	case KindAcceptedSlotDisplayV1:
		return []string{"kind", "threadId", "turnId", "acceptedFinalDigest", "displayMode"}
	default:
		return nil
	}
}

func ContractResponsePropertiesV1(kind string) []string {
	switch kind {
	case KindImportMappingPreviewV1, KindCleaningDiffPreviewV1:
		return []string{"schemaVersion", "kind", "selector", "lineage", "displayMode", "fields", "rowOffset", "rowLimit", "hasMore", "rows"}
	case KindDirectSourcePreviewV1:
		return []string{"schemaVersion", "kind", "caseId", "datasetSnapshotId", "displayMode", "view", "fields", "rowOffset", "rowLimit", "hasMore", "rows"}
	case KindAcceptedSlotDisplayV1:
		return []string{"schemaVersion", "kind", "threadId", "turnId", "acceptedFinalDigest", "caseId", "datasetSnapshotId", "contextEpoch", "displayMode", "slots"}
	default:
		return nil
	}
}

func ContractLineagePropertiesV1(kind string) []string {
	switch kind {
	case KindImportMappingPreviewV1:
		return []string{"importGeneration", "sourceItemGeneration", "parserGeneration", "mappingGeneration"}
	case KindCleaningDiffPreviewV1:
		return []string{"inputSnapshot", "ruleGeneration", "ruleDigest", "outputSnapshot", "transformLineage"}
	case KindDirectSourcePreviewV1, KindAcceptedSlotDisplayV1:
		return []string{}
	default:
		return nil
	}
}

func ContractRowPropertiesV1(kind string) []string {
	switch kind {
	case KindImportMappingPreviewV1:
		return []string{"rowIndex", "parseStatus", "mappingStatus", "cells"}
	case KindCleaningDiffPreviewV1:
		return []string{"rowIndex", "status", "cells"}
	case KindDirectSourcePreviewV1:
		return []string{"rowIndex", "cells"}
	case KindAcceptedSlotDisplayV1:
		return []string{"slotId", "field", "displayValue", "claimIds", "receiptIds"}
	default:
		return nil
	}
}

func ContractCellPropertiesV1(kind string) []string {
	switch kind {
	case KindImportMappingPreviewV1, KindDirectSourcePreviewV1:
		return []string{"field", "displayValue"}
	case KindCleaningDiffPreviewV1:
		return []string{"field", "beforeDisplayValue", "afterDisplayValue"}
	case KindAcceptedSlotDisplayV1:
		return []string{}
	default:
		return nil
	}
}

func ImportMappingPreviewFieldsV1() []string {
	return append([]string(nil), importMappingPreviewFieldsV1...)
}

func CleaningDiffPreviewFieldsV1() []string {
	return DirectSourcePreviewFieldsV1()
}

func DirectSourcePreviewFieldsV1() []string {
	return append([]string(nil), directSourcePreviewFieldsV1...)
}

func ValidSelectorV1(value string) bool   { return selectorPatternV1.MatchString(value) }
func ValidGenerationV1(value string) bool { return generationPatternV1.MatchString(value) }
func ValidSnapshotV1(value string) bool   { return snapshotPatternV1.MatchString(value) }
func ValidLineageV1(value string) bool    { return lineagePatternV1.MatchString(value) }
func ValidDigestV1(value string) bool     { return digestPatternV1.MatchString(value) }

func ValidDisplayModeV1(value string) bool {
	return value == DisplayModeFullV1 || value == DisplayModeMaskedV1
}

func ValidateFieldsV1(fields []string, allowed []string) error {
	if len(fields) == 0 || len(fields) > len(allowed) {
		return errInvalidContractV1
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, ok := allowedSet[field]; !ok {
			return errInvalidContractV1
		}
		if _, duplicate := seen[field]; duplicate {
			return errInvalidContractV1
		}
		seen[field] = struct{}{}
	}
	return nil
}

// ExactValueV1 is a private callback-only carrier. It deliberately refuses
// JSON serialization and redacts String formatting so exact bytes can only be
// projected at an allowlisted typed-local sink.
type ExactValueV1 struct {
	exact   string
	present bool
}

func NewExactValueV1(exact string) (ExactValueV1, error) {
	if !utf8.ValidString(exact) || len([]byte(exact)) > MaximumCellBytesV1 {
		return ExactValueV1{}, errInvalidContractV1
	}
	return ExactValueV1{exact: exact, present: true}, nil
}

func (value ExactValueV1) UseExactV1(use func(string) error) error {
	if !value.present || use == nil || !utf8.ValidString(value.exact) || len([]byte(value.exact)) > MaximumCellBytesV1 {
		return errInvalidContractV1
	}
	return use(value.exact)
}

func (ExactValueV1) String() string         { return "[typed-local exact value]" }
func (value ExactValueV1) GoString() string { return value.String() }

func (ExactValueV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("typed-local exact value cannot be serialized")
}

func (*ExactValueV1) UnmarshalJSON([]byte) error {
	return errors.New("typed-local exact value cannot be deserialized")
}

type ImportMappingAuthorityV1 struct {
	Selector             string
	ImportGeneration     string
	SourceItemGeneration string
	ParserGeneration     string
	MappingGeneration    string
}

func ValidateImportMappingAuthorityV1(value ImportMappingAuthorityV1) error {
	if !ValidSelectorV1(value.Selector) || !ValidGenerationV1(value.ImportGeneration) ||
		!ValidGenerationV1(value.SourceItemGeneration) || !ValidGenerationV1(value.ParserGeneration) ||
		!ValidGenerationV1(value.MappingGeneration) {
		return errInvalidContractV1
	}
	return nil
}

type CleaningDiffAuthorityV1 struct {
	Selector         string
	InputSnapshot    string
	RuleGeneration   string
	RuleDigest       string
	OutputSnapshot   string
	TransformLineage string
}

func ValidateCleaningDiffAuthorityV1(value CleaningDiffAuthorityV1) error {
	if !ValidSelectorV1(value.Selector) || !ValidSnapshotV1(value.InputSnapshot) ||
		!ValidGenerationV1(value.RuleGeneration) || !ValidDigestV1(value.RuleDigest) ||
		!ValidSnapshotV1(value.OutputSnapshot) || !ValidLineageV1(value.TransformLineage) {
		return errInvalidContractV1
	}
	return nil
}

type ImportMappingCellV1 struct {
	Field string
	Value ExactValueV1
}

type ImportMappingRowV1 struct {
	RowIndex      uint32
	ParseStatus   string
	MappingStatus string
	Cells         []ImportMappingCellV1
}

type CleaningDiffCellV1 struct {
	Field  string
	Before ExactValueV1
	After  ExactValueV1
}

type CleaningDiffRowV1 struct {
	RowIndex uint32
	Status   string
	Cells    []CleaningDiffCellV1
}

func ValidImportParseStatusV1(value string) bool {
	return value == "parsed" || value == "invalid" || value == "unsupported"
}

func ValidImportMappingStatusV1(value string) bool {
	return value == "mapped" || value == "unmapped" || value == "conflict"
}

func ValidCleaningStatusV1(value string) bool {
	return value == "unchanged" || value == "changed" || value == "added" ||
		value == "removed" || value == "invalid"
}

func MarshalCanonicalContractV1() ([]byte, error) {
	return json.Marshal(CanonicalContractV1())
}
