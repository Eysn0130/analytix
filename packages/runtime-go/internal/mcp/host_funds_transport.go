package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	errHostFundsTransportClosedV1 = errors.New("host funds in-process transport is closed")
	errHostFundsRequestInvalidV1  = errors.New("host funds in-process request is invalid")
	errHostFundsExecutionFailedV1 = errors.New("host funds account-flow execution failed")
)

var hostFundsEntityAliasPatternV1 = regexp.MustCompile(`^(acct|card):[1-9][0-9]{0,9}$`)
var hostFundsEntityReferencePatternV1 = regexp.MustCompile(`^cer1_[a-p]{64}$`)
var hostFundsSourceRecordIDPatternV1 = regexp.MustCompile(`^srow1_[a-f0-9]{64}$`)
var hostFundsSourceFileIDPatternV1 = regexp.MustCompile(`^[a-f0-9]{20}$`)

const (
	hostFundsBoundaryResourceURIV1 = "analytix://funds/source-unavailable"
	hostFundsBoundaryPromptNameV1  = "prepare-investigation-report"
)

// AccountFlowExecutionInput is the narrow, provider-safe request passed from
// the MCP boundary into the production funds executor. Exact account values,
// database paths, SQL and dataset descriptors are intentionally absent.
type AccountFlowExecutionInput struct {
	ThreadID string
	TurnID   string
	GrantID  string
	Intent   domainnative.AnalyzeAccountFlowsProviderIntentV1
}

// AccountFlowExecutor is composed only by the host after the native funds
// execution path is ready. The summary and row callbacks are synchronous and
// host-private; the implementation must complete them before returning.
type AccountFlowExecutor func(
	context.Context,
	AccountFlowExecutionInput,
	domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
	domainnative.AccountFlowHostEvidenceRowConsumerV1,
) (domainnative.AccountFlowProviderSemanticResultV1, error)

// hostFundsTransportClientV1 is the host-owned MCP protocol boundary for the
// packaged funds capability. The packaged JavaScript entrypoint remains
// provenance material, but no executable path, environment, or plugin file is
// consulted after this client is constructed and no child process is started.
type hostFundsTransportClientV1 struct {
	mu                  sync.RWMutex
	closed              bool
	sourceReadRequested bool
	accountFlowReady    bool
	identity            domainmcp.ServerIdentity
}

var (
	_ mcpTransportClient            = (*hostFundsTransportClientV1)(nil)
	_ mcpContextTransportClient     = (*hostFundsTransportClientV1)(nil)
	_ mcpHostContextTransportClient = (*hostFundsTransportClientV1)(nil)
	_ mcpNativeProbeClient          = (*hostFundsTransportClientV1)(nil)
	_ mcpToolCatalogClient          = (*hostFundsTransportClientV1)(nil)
	_ mcpNativeEvidenceClient       = (*hostFundsTransportClientV1)(nil)
	_ mcpObservedIdentityClient     = (*hostFundsTransportClientV1)(nil)
)

func newHostFundsTransportClientV1(
	binding *HostFundsServerSpecV1,
	spec ServerSpec,
	fingerprint string,
	accountFlowReadiness ...bool,
) (*hostFundsTransportClientV1, error) {
	accountFlowReady := len(accountFlowReadiness) == 0 ||
		(len(accountFlowReadiness) == 1 && accountFlowReadiness[0])
	if len(accountFlowReadiness) > 1 {
		return nil, errors.New("host funds in-process readiness is invalid")
	}
	if !binding.matches(spec, fingerprint) || spec.ID != "analytix_funds" ||
		spec.ExpectedServerName != "analytix_funds" ||
		!validHostFundsPackageVersionV1(spec.ExpectedServerVersion) {
		return nil, errors.New("host funds in-process binding is invalid")
	}
	sourceReadRequested := binding.sourceReadRequestedV1()
	tools := hostFundsRuntimeToolCatalogV1(sourceReadRequested, accountFlowReady)
	if !exactHostFundsRuntimeToolCatalogV1(tools, 0, sourceReadRequested, accountFlowReady) {
		return nil, errors.New("host funds in-process catalog is invalid")
	}
	return &hostFundsTransportClientV1{
		sourceReadRequested: sourceReadRequested,
		accountFlowReady:    accountFlowReady, identity: domainmcp.ServerIdentity{
			ProtocolVersion: MCPProtocolVersion,
			Name:            spec.ExpectedServerName,
			Version:         spec.ExpectedServerVersion,
		}}, nil
}

// hostFundsToolCatalogV1 is the exact packaged protocol catalog used by the
// immutable package-provenance check.
func hostFundsToolCatalogV1() []ToolSpec {
	return []ToolSpec{
		{
			Name:         hostFundsCountToolNameV1,
			Description:  hostFundsCountToolDescriptionV1,
			InputSchema:  json.RawMessage(hostFundsInputSchemaV1),
			OutputSchema: json.RawMessage(hostFundsOutputSchemaV1),
			ReadOnlyHint: true,
			TaskSupport:  domainmcp.ToolTaskSupportForbidden,
		},
		{
			Name:         hostFundsAccountFlowToolNameV1,
			Description:  hostFundsAccountFlowDescriptionV1,
			InputSchema:  json.RawMessage(hostFundsAccountFlowInputSchemaV1),
			OutputSchema: json.RawMessage(hostFundsAccountFlowOutputSchemaV1),
			ReadOnlyHint: true,
			TaskSupport:  domainmcp.ToolTaskSupportForbidden,
		},
	}
}

func hostFundsRuntimeToolCatalogV1(sourceReadReady bool, accountFlowReady bool) []ToolSpec {
	packaged := hostFundsToolCatalogV1()
	tools := make([]ToolSpec, 0, 2)
	if sourceReadReady {
		tools = append(tools, packaged[0])
	}
	if accountFlowReady {
		accountFlow := packaged[1]
		accountFlow.OutputSchema = json.RawMessage(hostFundsAccountFlowAnalysisOutputSchemaV1)
		tools = append(tools, accountFlow)
	}
	return tools
}

func (client *hostFundsTransportClientV1) begin(
	ctx context.Context,
) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}
	if client == nil {
		return ctx, nil, errHostFundsTransportClosedV1
	}
	client.mu.RLock()
	if client.closed {
		client.mu.RUnlock()
		return ctx, nil, errHostFundsTransportClosedV1
	}
	if err := ctx.Err(); err != nil {
		client.mu.RUnlock()
		return ctx, nil, err
	}
	return ctx, client.mu.RUnlock, nil
}

func (client *hostFundsTransportClientV1) ListTools() ([]ToolSpec, error) {
	return client.ListToolsContext(context.Background())
}

func (client *hostFundsTransportClientV1) ListToolsContext(ctx context.Context) ([]ToolSpec, error) {
	catalog, err := client.ListToolCatalogContext(ctx)
	if err != nil {
		return nil, err
	}
	return catalog.Tools, nil
}

func (client *hostFundsTransportClientV1) ListToolCatalogContext(ctx context.Context) (mcpprotocol.ToolCatalog, error) {
	ctx, done, err := client.begin(ctx)
	if err != nil {
		return mcpprotocol.ToolCatalog{}, err
	}
	defer done()
	if err := ctx.Err(); err != nil {
		return mcpprotocol.ToolCatalog{}, err
	}
	return mcpprotocol.ToolCatalog{Tools: hostFundsRuntimeToolCatalogV1(
		client.sourceReadRequested,
		client.accountFlowReady,
	)}, nil
}

func (client *hostFundsTransportClientV1) ListPrompts() ([]PromptSpec, error) {
	_, done, err := client.begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer done()
	return []PromptSpec{{
		Name:        hostFundsBoundaryPromptNameV1,
		Description: "返回固定资金来源不可用边界；不接受案件文本或敏感个人信息参数。",
		Arguments:   []PromptArgumentSpec{},
	}}, nil
}

func (client *hostFundsTransportClientV1) ListResources() ([]ResourceSpec, error) {
	_, done, err := client.begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer done()
	return []ResourceSpec{{
		URI:         hostFundsBoundaryResourceURIV1,
		Name:        "资金事实来源不可用边界",
		Description: "固定能力边界；不包含案件事实、敏感个人信息或报告内容。",
		MimeType:    "text/plain",
	}}, nil
}

func (client *hostFundsTransportClientV1) CallTool(name string, arguments map[string]any) (any, error) {
	return client.CallToolContext(context.Background(), name, arguments)
}

func (client *hostFundsTransportClientV1) CallToolContext(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (any, error) {
	ctx, done, err := client.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	return hostFundsDirectToolCallV1(ctx, name, arguments)
}

func (client *hostFundsTransportClientV1) CallToolWithHostContext(
	ctx context.Context,
	name string,
	arguments map[string]any,
	envelope domainmcp.HostContextEnvelope,
) (any, error) {
	_, grant, err := envelope.Authority()
	if err != nil || grant.ToolName != CanonicalToolName("analytix_funds", strings.TrimSpace(name)) {
		return nil, errHostFundsRequestInvalidV1
	}
	return client.CallToolContext(ctx, name, arguments)
}

func (client *hostFundsTransportClientV1) CallNativeContext(
	ctx context.Context,
	method string,
	params map[string]any,
) (any, error) {
	result, err := client.CallNativeLosslessContext(ctx, method, params)
	return result.Value, err
}

func (client *hostFundsTransportClientV1) CallNativeLosslessContext(
	ctx context.Context,
	method string,
	params map[string]any,
) (domainmcp.LosslessToolResult, error) {
	ctx, done, err := client.begin(ctx)
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	defer done()

	method = strings.TrimSpace(method)
	var value any
	switch method {
	case "analytix/sourceProbe":
		value, err = hostFundsSourceProbeResultV1(params, client.identity.Version)
	case "tools/call":
		return hostFundsNativeToolCallV1(ctx, params)
	case fundsEvidenceReadMethod:
		value, err = hostFundsEvidenceReadResultV1(params, client.identity.Version)
	default:
		err = errHostFundsRequestInvalidV1
	}
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
	}
	return mcpprotocol.DecodeLosslessJSONResult(body), nil
}

func (client *hostFundsTransportClientV1) ObservedServerIdentity() domainmcp.ServerIdentity {
	if client == nil {
		return domainmcp.ServerIdentity{}
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.closed {
		return domainmcp.ServerIdentity{}
	}
	return client.identity
}

func (client *hostFundsTransportClientV1) accountFlowExecutionReadyV1() bool {
	if client == nil {
		return false
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	return !client.closed && client.accountFlowReady
}

func (client *hostFundsTransportClientV1) Close() {
	if client == nil {
		return
	}
	client.mu.Lock()
	client.closed = true
	client.accountFlowReady = false
	client.identity = domainmcp.ServerIdentity{}
	client.mu.Unlock()
}

func hostFundsDirectToolCallV1(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (domainmcp.LosslessToolResult, error) {
	if err := ctx.Err(); err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	switch strings.TrimSpace(name) {
	case hostFundsCountToolNameV1:
		return domainmcp.LosslessToolResult{}, errors.New("host funds count requires an exact host projection")
	case hostFundsAccountFlowToolNameV1:
		return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
	default:
		return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
	}
}

func hostFundsNativeToolCallV1(
	ctx context.Context,
	params map[string]any,
) (domainmcp.LosslessToolResult, error) {
	if err := ctx.Err(); err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	name, ok := params["name"].(string)
	arguments, argumentsOK := params["arguments"].(map[string]any)
	if !ok || !argumentsOK {
		return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
	}
	switch name {
	case hostFundsCountToolNameV1:
		if !hostFundsMapHasExactKeysV1(params, "name", "arguments", "_meta") ||
			validateHostFundsArgumentsAgainstSchemaV1(arguments, hostFundsInputSchemaV1) != nil {
			return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
		}
		projection, err := hostFundsProjectionFromMetaV1(params["_meta"])
		if err != nil || arguments["table_name"] != projection.TableName {
			return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
		}
		record, err := domainsecurity.FundsCountProjectionV2Record(projection)
		if err != nil {
			return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
		}
		body, err := json.Marshal(map[string]any{
			"content": []any{},
			"structuredContent": map[string]any{
				"schemaVersion":  2,
				"purpose":        fundsCountToolOutcomePurposeV2,
				"semanticStatus": "success",
				"data":           record,
			},
		})
		if err != nil {
			return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
		}
		return mcpprotocol.ExtractLosslessToolResult(body), nil
	case hostFundsAccountFlowToolNameV1:
		return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
	default:
		return domainmcp.LosslessToolResult{}, errHostFundsRequestInvalidV1
	}
}

func hostFundsSourceProbeResultV1(params map[string]any, packageVersion string) (map[string]any, error) {
	if !hostFundsMapHasExactKeysV1(params, "_meta") {
		return nil, errHostFundsRequestInvalidV1
	}
	if !validHostFundsPackageVersionV1(packageVersion) {
		return nil, errHostFundsRequestInvalidV1
	}
	projection, err := hostFundsProjectionFromMetaV1(params["_meta"])
	if err != nil {
		return nil, errHostFundsRequestInvalidV1
	}
	return map[string]any{
		"schemaVersion":    fundsSourceProbeSchemaVersionV2,
		"purpose":          fundsSourceProbePurposeV2,
		"serverName":       "analytix_funds",
		"serverVersion":    packageVersion,
		"projectionDigest": projection.ProjectionDigest,
		"ready":            true,
		"readOnly":         true,
	}, nil
}

func hostFundsEvidenceReadResultV1(params map[string]any, packageVersion string) (map[string]any, error) {
	if !hostFundsMapHasExactKeysV1(params, "tool", "tableName", "noFilter", "_meta") ||
		params["tool"] != hostFundsCountToolNameV1 ||
		params["tableName"] != domainsecurity.FundsCountProjectionTableV2 ||
		!validHostFundsPackageVersionV1(packageVersion) ||
		params["noFilter"] != true {
		return nil, errHostFundsRequestInvalidV1
	}
	projection, err := hostFundsProjectionFromMetaV1(params["_meta"])
	if err != nil || projection.TableName != params["tableName"] {
		return nil, errHostFundsRequestInvalidV1
	}
	record, err := domainsecurity.FundsCountProjectionV2Record(projection)
	if err != nil {
		return nil, errHostFundsRequestInvalidV1
	}
	return map[string]any{
		"schemaVersion":      2,
		"purpose":            "analytix.funds.count-case-rows-evidence-candidate/v2",
		"serverName":         "analytix_funds",
		"serverVersion":      packageVersion,
		"toolName":           hostFundsCountToolNameV1,
		"projection":         record,
		"paginationComplete": true,
		"readOnly":           true,
	}, nil
}

func hostFundsProjectionFromMetaV1(value any) (domainsecurity.FundsCountProjectionV2, error) {
	meta, ok := value.(map[string]any)
	if !ok || !hostFundsMapHasExactKeysV1(meta, domainsecurity.FundsCountProjectionMetaKeyV2) {
		return domainsecurity.FundsCountProjectionV2{}, errHostFundsRequestInvalidV1
	}
	projection, err := domainsecurity.ParseFundsCountProjectionV2(
		meta[domainsecurity.FundsCountProjectionMetaKeyV2],
	)
	if err != nil {
		return domainsecurity.FundsCountProjectionV2{}, errHostFundsRequestInvalidV1
	}
	return projection, nil
}

func validateHostFundsArgumentsAgainstSchemaV1(arguments map[string]any, schema string) error {
	if arguments == nil {
		return errHostFundsRequestInvalidV1
	}
	body, err := json.Marshal(arguments)
	if err != nil || toolcatalogapp.ValidateJSONSchemaRawValue(
		body, json.RawMessage(schema), "arguments",
	) != nil {
		return errHostFundsRequestInvalidV1
	}
	return nil
}

func validateHostFundsAccountFlowArgumentsV1(arguments map[string]any) error {
	_, err := decodeHostFundsAccountFlowIntentV1(arguments)
	return err
}

func decodeHostFundsAccountFlowIntentV1(
	arguments map[string]any,
) (domainnative.AnalyzeAccountFlowsProviderIntentV1, error) {
	zero := domainnative.AnalyzeAccountFlowsProviderIntentV1{}
	if !hostFundsMapHasExactKeysV1(
		arguments,
		"subject_alias",
		"start_inclusive",
		"end_inclusive",
		"evidence_row_limit",
	) || validateHostFundsArgumentsAgainstSchemaV1(arguments, hostFundsAccountFlowInputSchemaV1) != nil {
		return zero, errHostFundsRequestInvalidV1
	}
	body, err := json.Marshal(arguments)
	if err != nil {
		return zero, errHostFundsRequestInvalidV1
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var intent domainnative.AnalyzeAccountFlowsProviderIntentV1
	if err := decoder.Decode(&intent); err != nil {
		return zero, errHostFundsRequestInvalidV1
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) ||
		domainnative.ValidateAnalyzeAccountFlowsProviderIntentV1(intent) != nil ||
		!hostFundsEntityAliasPatternV1.MatchString(intent.SubjectAlias) {
		return zero, errHostFundsRequestInvalidV1
	}
	canonical, err := json.Marshal(intent)
	if err != nil || domainsecurity.CanonicalJSONHash(body) == "" ||
		domainsecurity.CanonicalJSONHash(body) != domainsecurity.CanonicalJSONHash(canonical) {
		return zero, errHostFundsRequestInvalidV1
	}
	return intent, nil
}

type hostFundsAccountFlowEvidenceSummaryV1 struct {
	subjectRef           string
	datasetSnapshotID    string
	contextEpoch         uint64
	contextDigest        string
	caseBindingHash      string
	startInclusive       string
	endInclusive         string
	timezone             string
	currency             string
	minorUnitScale       uint8
	inflowMinor          string
	outflowMinor         string
	netMinor             string
	transactionCount     uint64
	aggregateComplete    bool
	evidenceRowsComplete bool
	evidenceRowCount     uint64
	coverage             domainnative.AccountFlowProviderSemanticCoverageV1
	queryHash            string
	resultHash           string
}

type hostFundsAccountFlowEvidenceRowV1 struct {
	subjectRef      string
	sourceRecordID  string
	sourceFileID    string
	sourceRowNumber uint64
	occurredAt      string
	direction       string
	amountMinor     string
	currency        string
	minorUnitScale  uint8
}

type hostFundsAccountFlowEvidenceSnapshotV1 struct {
	summary hostFundsAccountFlowEvidenceSummaryV1
	rows    []hostFundsAccountFlowEvidenceRowV1
}

type hostFundsAccountFlowCaptureV1 struct {
	mu           sync.Mutex
	active       bool
	invalid      bool
	summaryCalls uint32
	summary      hostFundsAccountFlowEvidenceSummaryV1
	rows         []hostFundsAccountFlowEvidenceRowV1
}

func newHostFundsAccountFlowCaptureV1() *hostFundsAccountFlowCaptureV1 {
	return &hostFundsAccountFlowCaptureV1{active: true}
}

func (capture *hostFundsAccountFlowCaptureV1) consumeSummary(
	subjectRef string,
	datasetSnapshotID string,
	contextEpoch uint64,
	contextDigest string,
	caseBindingHash string,
	startInclusive string,
	endInclusive string,
	timezone string,
	currency string,
	minorUnitScale uint8,
	inflowMinor string,
	outflowMinor string,
	netMinor string,
	transactionCount uint64,
	aggregateComplete bool,
	evidenceRowsComplete bool,
	evidenceRowCount uint64,
	coverage domainnative.AccountFlowProviderSemanticCoverageV1,
	queryHash string,
	resultHash string,
) error {
	if capture == nil {
		return errHostFundsRequestInvalidV1
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.summaryCalls++
	if !capture.active || capture.summaryCalls != 1 || len(capture.rows) != 0 {
		capture.invalid = true
		return errHostFundsRequestInvalidV1
	}
	coverage = cloneHostFundsAccountFlowCoverageV1(coverage)
	capture.summary = hostFundsAccountFlowEvidenceSummaryV1{
		subjectRef: strings.Clone(subjectRef), datasetSnapshotID: strings.Clone(datasetSnapshotID),
		contextEpoch: contextEpoch, contextDigest: strings.Clone(contextDigest),
		caseBindingHash: strings.Clone(caseBindingHash), startInclusive: strings.Clone(startInclusive),
		endInclusive: strings.Clone(endInclusive), timezone: strings.Clone(timezone),
		currency: strings.Clone(currency), minorUnitScale: minorUnitScale,
		inflowMinor: strings.Clone(inflowMinor), outflowMinor: strings.Clone(outflowMinor),
		netMinor: strings.Clone(netMinor), transactionCount: transactionCount,
		aggregateComplete: aggregateComplete, evidenceRowsComplete: evidenceRowsComplete,
		evidenceRowCount: evidenceRowCount, coverage: coverage,
		queryHash: strings.Clone(queryHash), resultHash: strings.Clone(resultHash),
	}
	return nil
}

func (capture *hostFundsAccountFlowCaptureV1) consumeRow(
	index int,
	subjectRef string,
	sourceRecordID string,
	sourceFileID string,
	sourceRowNumber uint64,
	occurredAt string,
	direction string,
	amountMinor string,
	currency string,
	minorUnitScale uint8,
) error {
	if capture == nil {
		return errHostFundsRequestInvalidV1
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if !capture.active || capture.summaryCalls != 1 || index != len(capture.rows) ||
		!hostFundsSourceFileIDPatternV1.MatchString(sourceFileID) || sourceRowNumber == 0 {
		capture.invalid = true
		return errHostFundsRequestInvalidV1
	}
	capture.rows = append(capture.rows, hostFundsAccountFlowEvidenceRowV1{
		subjectRef: strings.Clone(subjectRef), sourceRecordID: strings.Clone(sourceRecordID),
		sourceFileID: strings.Clone(sourceFileID), sourceRowNumber: sourceRowNumber,
		occurredAt: strings.Clone(occurredAt), direction: strings.Clone(direction),
		amountMinor: strings.Clone(amountMinor), currency: strings.Clone(currency),
		minorUnitScale: minorUnitScale,
	})
	return nil
}

func (capture *hostFundsAccountFlowCaptureV1) close() (hostFundsAccountFlowEvidenceSnapshotV1, bool) {
	if capture == nil {
		return hostFundsAccountFlowEvidenceSnapshotV1{}, false
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.active = false
	valid := !capture.invalid && capture.summaryCalls == 1 &&
		capture.summary.evidenceRowCount == uint64(len(capture.rows))
	snapshot := hostFundsAccountFlowEvidenceSnapshotV1{
		summary: capture.summary,
		rows:    append([]hostFundsAccountFlowEvidenceRowV1(nil), capture.rows...),
	}
	capture.summary = hostFundsAccountFlowEvidenceSummaryV1{}
	clear(capture.rows)
	capture.rows = nil
	return snapshot, valid
}

type hostFundsAccountFlowEvidenceCarrierV1 struct {
	mu        sync.Mutex
	active    bool
	rawSHA256 string
	evidence  hostFundsAccountFlowEvidenceSnapshotV1
}

func (*hostFundsAccountFlowEvidenceCarrierV1) MarshalJSON() ([]byte, error) {
	return nil, errHostFundsRequestInvalidV1
}

func (*hostFundsAccountFlowEvidenceCarrierV1) String() string {
	return "hostFundsAccountFlowEvidenceCarrierV1{private:[REDACTED]}"
}

func (carrier *hostFundsAccountFlowEvidenceCarrierV1) GoString() string {
	return carrier.String()
}

func (carrier *hostFundsAccountFlowEvidenceCarrierV1) validFor(rawSHA256 string) bool {
	if carrier == nil {
		return false
	}
	carrier.mu.Lock()
	defer carrier.mu.Unlock()
	return carrier.active && domainsecurity.IsSHA256Hex(carrier.rawSHA256) &&
		carrier.rawSHA256 == rawSHA256
}

func (carrier *hostFundsAccountFlowEvidenceCarrierV1) discard() {
	if carrier == nil {
		return
	}
	carrier.mu.Lock()
	carrier.active = false
	carrier.rawSHA256 = ""
	carrier.evidence.summary = hostFundsAccountFlowEvidenceSummaryV1{}
	clear(carrier.evidence.rows)
	carrier.evidence.rows = nil
	carrier.mu.Unlock()
}

func (carrier *hostFundsAccountFlowEvidenceCarrierV1) consume(
	rawSHA256 string,
	consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
	consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
) (err error) {
	if carrier == nil || consumeSummary == nil || consumeRow == nil {
		return errHostFundsRequestInvalidV1
	}
	carrier.mu.Lock()
	if !carrier.active || carrier.rawSHA256 != rawSHA256 ||
		!domainsecurity.IsSHA256Hex(rawSHA256) {
		carrier.mu.Unlock()
		return errHostFundsRequestInvalidV1
	}
	carrier.active = false
	evidence := carrier.evidence
	carrier.rawSHA256 = ""
	carrier.evidence.summary = hostFundsAccountFlowEvidenceSummaryV1{}
	carrier.evidence.rows = nil
	carrier.mu.Unlock()
	defer func() {
		if recover() != nil {
			err = errHostFundsRequestInvalidV1
		}
		evidence.summary = hostFundsAccountFlowEvidenceSummaryV1{}
		clear(evidence.rows)
	}()
	summary := evidence.summary
	if err := consumeSummary(
		summary.subjectRef, summary.datasetSnapshotID, summary.contextEpoch,
		summary.contextDigest, summary.caseBindingHash, summary.startInclusive,
		summary.endInclusive, summary.timezone, summary.currency, summary.minorUnitScale,
		summary.inflowMinor, summary.outflowMinor, summary.netMinor,
		summary.transactionCount, summary.aggregateComplete, summary.evidenceRowsComplete,
		summary.evidenceRowCount, cloneHostFundsAccountFlowCoverageV1(summary.coverage),
		summary.queryHash, summary.resultHash,
	); err != nil {
		return err
	}
	for index, row := range evidence.rows {
		if err := consumeRow(
			index, row.subjectRef, row.sourceRecordID, row.sourceFileID, row.sourceRowNumber, row.occurredAt,
			row.direction, row.amountMinor, row.currency, row.minorUnitScale,
		); err != nil {
			return err
		}
	}
	return nil
}

type hostFundsAccountFlowSealedResultV1 struct {
	client   *hostFundsTransportClientV1
	lossless domainmcp.LosslessToolResult
	carrier  *hostFundsAccountFlowEvidenceCarrierV1
}

func (sealed hostFundsAccountFlowSealedResultV1) valid() bool {
	return sealed.client != nil && sealed.client.accountFlowExecutionReadyV1() &&
		domainmcp.ValidLosslessToolResult(sealed.lossless) && sealed.lossless.HostPrivate == nil &&
		sealed.carrier.validFor(sealed.lossless.RawSHA256)
}

func (sealed hostFundsAccountFlowSealedResultV1) discard() {
	sealed.carrier.discard()
}

func discardHostFundsSealedResultV1(result any) {
	if sealed, ok := result.(hostFundsAccountFlowSealedResultV1); ok {
		sealed.discard()
	}
}

func executeHostFundsAccountFlowV1(
	ctx context.Context,
	client *hostFundsTransportClientV1,
	executor AccountFlowExecutor,
	securityContext domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	intent domainnative.AnalyzeAccountFlowsProviderIntentV1,
) (hostFundsAccountFlowSealedResultV1, error) {
	zero := hostFundsAccountFlowSealedResultV1{}
	if ctx == nil || ctx.Err() != nil || client == nil || !client.accountFlowExecutionReadyV1() ||
		executor == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil ||
		grant.ToolName != CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1) ||
		!grant.ReadOnly || (grant.ApprovalState != "not_required" && grant.ApprovalState != "approved") ||
		domainnative.ValidateAnalyzeAccountFlowsProviderIntentV1(intent) != nil ||
		grant.ArgsHash != domainnative.AnalyzeAccountFlowsProviderIntentHashV1(intent) {
		return zero, errHostFundsExecutionFailedV1
	}
	capture := newHostFundsAccountFlowCaptureV1()
	semantic, executeErr := callHostFundsAccountFlowExecutorV1(
		executor,
		ctx,
		AccountFlowExecutionInput{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
			GrantID: grant.GrantID, Intent: intent,
		},
		capture.consumeSummary,
		capture.consumeRow,
	)
	evidence, captureOK := capture.close()
	if executeErr != nil || !captureOK || ctx.Err() != nil ||
		validateHostFundsAccountFlowSemanticV1(semantic, intent) != nil ||
		validateHostFundsAccountFlowEvidenceV1(evidence, semantic, securityContext) != nil {
		clear(evidence.rows)
		return zero, errHostFundsExecutionFailedV1
	}
	semanticStatus := "partial"
	if semantic.AggregateComplete && semantic.EvidenceRowsComplete &&
		semantic.CounterpartySemanticsComplete {
		semanticStatus = "success"
	}
	body, err := json.Marshal(map[string]any{
		"content": []any{},
		"structuredContent": map[string]any{
			"schemaVersion":  3,
			"purpose":        "analytix.funds-account-flow-analysis/v3",
			"semanticStatus": semanticStatus,
			"data":           semantic,
		},
	})
	if err != nil {
		clear(evidence.rows)
		return zero, errHostFundsExecutionFailedV1
	}
	lossless := mcpprotocol.ExtractLosslessToolResult(body)
	if !domainmcp.ValidLosslessToolResult(lossless) {
		clear(evidence.rows)
		return zero, errHostFundsExecutionFailedV1
	}
	carrier := &hostFundsAccountFlowEvidenceCarrierV1{
		active: true, rawSHA256: lossless.RawSHA256, evidence: evidence,
	}
	sealed := hostFundsAccountFlowSealedResultV1{client: client, lossless: lossless, carrier: carrier}
	if !sealed.valid() {
		sealed.discard()
		return zero, errHostFundsExecutionFailedV1
	}
	return sealed, nil
}

func callHostFundsAccountFlowExecutorV1(
	executor AccountFlowExecutor,
	ctx context.Context,
	input AccountFlowExecutionInput,
	consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
	consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
) (semantic domainnative.AccountFlowProviderSemanticResultV1, err error) {
	defer func() {
		if recover() != nil {
			semantic = domainnative.AccountFlowProviderSemanticResultV1{}
			err = errHostFundsExecutionFailedV1
		}
	}()
	return executor(ctx, input, consumeSummary, consumeRow)
}

// ConsumeHostFundsAccountFlowEvidenceV1 consumes and burns the exact
// host-private account-flow evidence once. The LosslessToolResult Value and
// RawResult remain the provider-safe exact decode and never contain this data.
func ConsumeHostFundsAccountFlowEvidenceV1(
	result domainmcp.LosslessToolResult,
	consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
	consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
) error {
	recomputed := mcpprotocol.ExtractLosslessToolResult(result.RawResult)
	carrier, ok := result.HostPrivate.(*hostFundsAccountFlowEvidenceCarrierV1)
	if !ok || !domainmcp.ValidLosslessToolResult(result) ||
		!domainmcp.ValidLosslessToolResult(recomputed) || result.RawSHA256 != recomputed.RawSHA256 ||
		!reflect.DeepEqual(result.Value, recomputed.Value) || !carrier.validFor(result.RawSHA256) {
		if ok {
			carrier.discard()
		}
		return errHostFundsRequestInvalidV1
	}
	return carrier.consume(result.RawSHA256, consumeSummary, consumeRow)
}

// DiscardHostFundsAccountFlowEvidenceV1 burns an unused private carrier.
func DiscardHostFundsAccountFlowEvidenceV1(result domainmcp.LosslessToolResult) {
	if carrier, ok := result.HostPrivate.(*hostFundsAccountFlowEvidenceCarrierV1); ok {
		carrier.discard()
	}
}

// HostFundsAccountFlowProviderSemanticV1 extracts only the exact
// provider-safe semantic projection from a validated lossless result.
func HostFundsAccountFlowProviderSemanticV1(
	result domainmcp.LosslessToolResult,
) (domainnative.AccountFlowProviderSemanticResultV1, bool) {
	zero := domainnative.AccountFlowProviderSemanticResultV1{}
	carrier, ok := result.HostPrivate.(*hostFundsAccountFlowEvidenceCarrierV1)
	if !ok || !carrier.validFor(result.RawSHA256) {
		return zero, false
	}
	recomputed := mcpprotocol.ExtractLosslessToolResult(result.RawResult)
	if !domainmcp.ValidLosslessToolResult(result) || !domainmcp.ValidLosslessToolResult(recomputed) ||
		result.RawSHA256 != recomputed.RawSHA256 || !reflect.DeepEqual(result.Value, recomputed.Value) {
		return zero, false
	}
	record, ok := recomputed.Value.(map[string]any)
	if !ok || record["schemaVersion"] != json.Number("3") ||
		record["purpose"] != "analytix.funds-account-flow-analysis/v3" ||
		(record["semanticStatus"] != "success" && record["semanticStatus"] != "partial") {
		return zero, false
	}
	body, err := json.Marshal(record["data"])
	if err != nil {
		return zero, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var semantic domainnative.AccountFlowProviderSemanticResultV1
	if err := decoder.Decode(&semantic); err != nil {
		return zero, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return zero, false
	}
	if semantic.EvidenceRowLimit == 0 ||
		semantic.EvidenceRowLimit > domainnative.AccountFlowMaximumEvidenceRowsV1 ||
		semantic.EvidenceTransactionCount > uint64(domainnative.AccountFlowMaximumEvidenceRowsV1) ||
		(semantic.EvidenceRowsComplete && semantic.TransactionCount > uint64(domainnative.AccountFlowMaximumEvidenceRowsV1)) {
		return zero, false
	}
	if validateHostFundsAccountFlowSemanticV1(semantic, domainnative.AnalyzeAccountFlowsProviderIntentV1{
		SubjectAlias: semantic.SubjectAlias, StartInclusive: semantic.StartInclusive,
		EndInclusive: semantic.EndInclusive, EvidenceRowLimit: semantic.EvidenceRowLimit,
	}) != nil {
		return zero, false
	}
	wantSemanticStatus := "partial"
	if semantic.AggregateComplete && semantic.EvidenceRowsComplete &&
		semantic.CounterpartySemanticsComplete {
		wantSemanticStatus = "success"
	}
	if record["semanticStatus"] != wantSemanticStatus {
		return zero, false
	}
	return semantic, true
}

func validateHostFundsAccountFlowSemanticV1(
	semantic domainnative.AccountFlowProviderSemanticResultV1,
	intent domainnative.AnalyzeAccountFlowsProviderIntentV1,
) error {
	if domainnative.ValidateAnalyzeAccountFlowsProviderIntentV1(intent) != nil ||
		semantic.SubjectAlias != intent.SubjectAlias || semantic.StartInclusive != intent.StartInclusive ||
		semantic.EndInclusive != intent.EndInclusive || !validHostFundsAccountFlowTimezoneV1(semantic.Timezone) ||
		!validHostFundsCurrencyV1(semantic.Currency) ||
		semantic.MinorUnitScale != domainnative.AccountFlowMinorUnitScaleV1 ||
		semantic.TransactionCount > uint64(domainnative.AccountFlowMaximumScanRowsV1) ||
		semantic.EvidenceRowLimit != intent.EvidenceRowLimit ||
		!domainsecurity.IsSHA256Hex(semantic.QueryHash) ||
		!domainsecurity.IsSHA256Hex(semantic.ResultHash) ||
		domainnative.ValidateAccountFlowProviderOutcomeV1(semantic) != nil ||
		semantic.EvidenceTransactionCount != uint64(len(semantic.Transactions)) ||
		semantic.EvidenceTransactionCount > uint64(intent.EvidenceRowLimit) || semantic.Transactions == nil {
		return errHostFundsRequestInvalidV1
	}
	inflow, inflowOK := hostFundsCanonicalIntegerV1(semantic.InflowMinor, false)
	outflow, outflowOK := hostFundsCanonicalIntegerV1(semantic.OutflowMinor, false)
	net, netOK := hostFundsCanonicalIntegerV1(semantic.NetMinor, true)
	if !inflowOK || !outflowOK || !netOK || new(big.Int).Sub(inflow, outflow).Cmp(net) != 0 {
		return errHostFundsRequestInvalidV1
	}
	wantEvidenceCount := semantic.TransactionCount
	if wantEvidenceCount > uint64(intent.EvidenceRowLimit) {
		wantEvidenceCount = uint64(intent.EvidenceRowLimit)
	}
	if semantic.EvidenceTransactionCount != wantEvidenceCount ||
		semantic.EvidenceRowsComplete != (semantic.TransactionCount <= uint64(intent.EvidenceRowLimit)) {
		return errHostFundsRequestInvalidV1
	}
	start, _ := time.Parse("2006-01-02T15:04:05.000000Z", intent.StartInclusive)
	end, _ := time.Parse("2006-01-02T15:04:05.000000Z", intent.EndInclusive)
	previous := time.Time{}
	rowInflow := new(big.Int)
	rowOutflow := new(big.Int)
	seenEvidenceRefs := make(map[string]struct{}, len(semantic.Transactions))
	counterpartySemantics := make(map[domaincaseentity.ModelEntityAliasV1]domainnative.AccountFlowProviderCounterpartyV1, len(semantic.Transactions))
	wantCounterpartySemanticsComplete := true
	for _, row := range semantic.Transactions {
		occurredAt, timeErr := time.Parse("2006-01-02T15:04:05.000000Z", row.OccurredAt)
		amount, amountOK := hostFundsCanonicalIntegerV1(row.AmountMinor, false)
		if !hostFundsSourceRecordIDPatternV1.MatchString(row.EvidenceRef) ||
			timeErr != nil || occurredAt.Before(start) || occurredAt.After(end) ||
			(!previous.IsZero() && occurredAt.Before(previous)) || !amountOK ||
			row.Currency != semantic.Currency || row.MinorUnitScale != semantic.MinorUnitScale ||
			domainnative.ValidateAccountFlowProviderCounterpartyV1(row.Counterparty) != nil {
			return errHostFundsRequestInvalidV1
		}
		if row.Counterparty.Status != domainnative.AccountFlowCounterpartyResolvedV1 {
			wantCounterpartySemanticsComplete = false
		}
		if row.Counterparty.Alias != "" {
			if previousSemantics, exists := counterpartySemantics[row.Counterparty.Alias]; exists &&
				(previousSemantics.EntityType != row.Counterparty.EntityType ||
					previousSemantics.AccountType != row.Counterparty.AccountType) {
				return errHostFundsRequestInvalidV1
			}
			counterpartySemantics[row.Counterparty.Alias] = row.Counterparty
		}
		if _, duplicate := seenEvidenceRefs[row.EvidenceRef]; duplicate {
			return errHostFundsRequestInvalidV1
		}
		seenEvidenceRefs[row.EvidenceRef] = struct{}{}
		previous = occurredAt
		switch row.Direction {
		case domainnative.AccountFlowDirectionInflowV1:
			rowInflow.Add(rowInflow, amount)
		case domainnative.AccountFlowDirectionOutflowV1:
			rowOutflow.Add(rowOutflow, amount)
		default:
			return errHostFundsRequestInvalidV1
		}
	}
	if semantic.EvidenceRowsComplete && (rowInflow.Cmp(inflow) != 0 || rowOutflow.Cmp(outflow) != 0) {
		return errHostFundsRequestInvalidV1
	}
	if semantic.CounterpartySemanticsComplete != wantCounterpartySemanticsComplete {
		return errHostFundsRequestInvalidV1
	}
	coverage := semantic.Coverage
	if coverage.Gaps == nil || coverage.AcceptedSnapshotRows > coverage.NormalizedSnapshotRows ||
		coverage.RejectedSnapshotRows > coverage.NormalizedSnapshotRows-coverage.AcceptedSnapshotRows ||
		coverage.DuplicateSnapshotRows != coverage.NormalizedSnapshotRows-coverage.AcceptedSnapshotRows-coverage.RejectedSnapshotRows ||
		coverage.UntimedSubjectRows > coverage.AcceptedSnapshotRows ||
		coverage.ObservedMatchingRows != semantic.TransactionCount ||
		coverage.ObservedMatchingRows > coverage.AcceptedSnapshotRows {
		return errHostFundsRequestInvalidV1
	}
	wantAggregateComplete := coverage.RejectedSnapshotRows == 0 &&
		coverage.DuplicateSnapshotRows == 0 && coverage.UntimedSubjectRows == 0
	if semantic.AggregateComplete != wantAggregateComplete {
		return errHostFundsRequestInvalidV1
	}
	wantGaps := make([]string, 0, 5)
	if coverage.RejectedSnapshotRows != 0 {
		wantGaps = append(wantGaps, domainnative.AccountFlowGapRejectedSourceRowsV1)
	}
	if coverage.DuplicateSnapshotRows != 0 {
		wantGaps = append(wantGaps, domainnative.AccountFlowGapDuplicateSourceRowsV1)
	}
	if coverage.UntimedSubjectRows != 0 {
		wantGaps = append(wantGaps, domainnative.AccountFlowGapUntimedSubjectRowsV1)
	}
	if !semantic.EvidenceRowsComplete {
		wantGaps = append(wantGaps, domainnative.AccountFlowGapEvidenceRowLimitV1)
	}
	if !semantic.CounterpartySemanticsComplete {
		wantGaps = append(wantGaps, domainnative.AccountFlowGapCounterpartyResolutionV1)
	}
	wantState := domainnative.AccountFlowCoverageCompleteV1
	if !semantic.AggregateComplete || !semantic.EvidenceRowsComplete ||
		!semantic.CounterpartySemanticsComplete {
		wantState = domainnative.AccountFlowCoveragePartialV1
	} else if semantic.TransactionCount == 0 {
		wantState = domainnative.AccountFlowCoverageObservedNoHitPendingBindingV1
	}
	if coverage.State != wantState || !reflect.DeepEqual(coverage.Gaps, wantGaps) {
		return errHostFundsRequestInvalidV1
	}
	return nil
}

func validateHostFundsAccountFlowEvidenceV1(
	evidence hostFundsAccountFlowEvidenceSnapshotV1,
	semantic domainnative.AccountFlowProviderSemanticResultV1,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	summary := evidence.summary
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!hostFundsEntityReferencePatternV1.MatchString(summary.subjectRef) ||
		summary.datasetSnapshotID != securityContext.DatasetSnapshotID ||
		summary.contextEpoch != securityContext.ContextEpoch || summary.contextDigest != securityContext.ContextDigest ||
		summary.caseBindingHash != securityContext.CaseBindingHash || summary.startInclusive != semantic.StartInclusive ||
		summary.endInclusive != semantic.EndInclusive || summary.timezone != semantic.Timezone ||
		summary.currency != semantic.Currency || summary.minorUnitScale != semantic.MinorUnitScale ||
		summary.inflowMinor != semantic.InflowMinor || summary.outflowMinor != semantic.OutflowMinor ||
		summary.netMinor != semantic.NetMinor || summary.transactionCount != semantic.TransactionCount ||
		summary.aggregateComplete != semantic.AggregateComplete ||
		summary.evidenceRowsComplete != semantic.EvidenceRowsComplete ||
		summary.evidenceRowCount != semantic.EvidenceTransactionCount ||
		!reflect.DeepEqual(summary.coverage, semantic.Coverage) ||
		summary.queryHash != semantic.QueryHash || summary.resultHash != semantic.ResultHash ||
		len(evidence.rows) != len(semantic.Transactions) {
		return errHostFundsRequestInvalidV1
	}
	seen := make(map[string]struct{}, len(evidence.rows))
	for index, row := range evidence.rows {
		semanticRow := semantic.Transactions[index]
		if row.subjectRef != summary.subjectRef || !hostFundsSourceRecordIDPatternV1.MatchString(row.sourceRecordID) ||
			row.sourceRecordID != semanticRow.EvidenceRef ||
			row.occurredAt != semanticRow.OccurredAt || row.direction != semanticRow.Direction ||
			row.amountMinor != semanticRow.AmountMinor || row.currency != semanticRow.Currency ||
			row.minorUnitScale != semanticRow.MinorUnitScale {
			return errHostFundsRequestInvalidV1
		}
		if _, duplicate := seen[row.sourceRecordID]; duplicate {
			return errHostFundsRequestInvalidV1
		}
		seen[row.sourceRecordID] = struct{}{}
	}
	return nil
}

func cloneHostFundsAccountFlowCoverageV1(
	coverage domainnative.AccountFlowProviderSemanticCoverageV1,
) domainnative.AccountFlowProviderSemanticCoverageV1 {
	if coverage.Gaps != nil {
		coverage.Gaps = append(make([]string, 0, len(coverage.Gaps)), coverage.Gaps...)
	}
	return coverage
}

func validHostFundsAccountFlowTimezoneV1(value string) bool {
	if value == "Z" {
		return true
	}
	if len(value) != 6 || (value[0] != '+' && value[0] != '-') || value[3] != ':' {
		return false
	}
	hour := int(value[1]-'0')*10 + int(value[2]-'0')
	minute := int(value[4]-'0')*10 + int(value[5]-'0')
	return value[1] >= '0' && value[1] <= '9' && value[2] >= '0' && value[2] <= '9' &&
		value[4] >= '0' && value[4] <= '9' && value[5] >= '0' && value[5] <= '9' &&
		minute < 60 && (hour < 14 || (hour == 14 && minute == 0))
}

func validHostFundsCurrencyV1(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func hostFundsCanonicalIntegerV1(value string, signed bool) (*big.Int, bool) {
	maximumLength := 128
	if signed {
		maximumLength = 129
	}
	if value == "" || len(value) > maximumLength || value != strings.TrimSpace(value) {
		return nil, false
	}
	digits := value
	if signed && strings.HasPrefix(digits, "-") {
		digits = strings.TrimPrefix(digits, "-")
	}
	if digits == "" || (len(digits) > 1 && digits[0] == '0') {
		return nil, false
	}
	for _, character := range digits {
		if character < '0' || character > '9' {
			return nil, false
		}
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok || (!signed && parsed.Sign() < 0) || (value == "-0") {
		return nil, false
	}
	return parsed, true
}

func hostFundsMapHasExactKeysV1(record map[string]any, keys ...string) bool {
	if record == nil || len(record) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := record[key]; !ok {
			return false
		}
	}
	return true
}
