package toolresult

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PublicProjectionSchemaVersion = 1
	MetadataOnlyDisclosure        = "metadata_only"
	HostReportAdmissionVersion    = 1
	HostReportAdmissionPurpose    = "analytix.host-report-admission/v1"
)

type PublicProjectionKind string

const (
	ProjectionWithheld         PublicProjectionKind = "withheld"
	ProjectionHostStatus       PublicProjectionKind = "host_status"
	ProjectionWebFetchStatus   PublicProjectionKind = "web_fetch_status"
	ProjectionPlanStatus       PublicProjectionKind = "plan_status"
	ProjectionArtifactStatus   PublicProjectionKind = "artifact_status"
	ProjectionCaseSourceStatus PublicProjectionKind = "case_source_status"
	ProjectionMCPDiagnostic    PublicProjectionKind = "mcp_diagnostic"
	ProjectionTaskJobResult    PublicProjectionKind = "task_job_result"
)

type PublicToolResultProjectionV1 struct {
	SchemaVersion          int                    `json:"schemaVersion"`
	ProjectionKind         PublicProjectionKind   `json:"projectionKind"`
	Disclosure             string                 `json:"disclosure"`
	MessageKey             string                 `json:"messageKey"`
	Status                 string                 `json:"status"`
	Code                   string                 `json:"code,omitempty"`
	PrivatePayloadWithheld bool                   `json:"privatePayloadWithheld"`
	FactAnswerAllowed      bool                   `json:"factAnswerAllowed"`
	EvidenceAuthority      bool                   `json:"evidenceAuthority"`
	Plan                   *PlanStatusV1          `json:"plan,omitempty"`
	RPCError               *MCPRPCErrorDiagnostic `json:"rpcError,omitempty"`
	Artifact               *ArtifactStatusV1      `json:"artifact,omitempty"`
	Web                    *WebFetchStatusV1      `json:"web,omitempty"`
	TaskJob                *TaskJobResultV1       `json:"taskJob,omitempty"`
}

type ArtifactStatusV1 struct {
	ArtifactID  string `json:"artifactId"`
	Kind        string `json:"kind"`
	ContentHash string `json:"contentHash"`
	ByteSize    int64  `json:"byteSize"`
	SavedAt     string `json:"savedAt"`
}

// ForegroundHandoffInvalidPublicOutputV1 is the single closed failure shape
// used when a process-local foreground carrier is absent, malformed, replayed,
// or unavailable to a public projection owner.
func ForegroundHandoffInvalidPublicOutputV1() map[string]any {
	return map[string]any{
		"kind": "subagent_task", "status": "failed", "code": "foreground_handoff_projection_invalid",
		"factAnswerAllowed": false, "evidenceAuthority": false,
		"parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
	}
}

type PlanStatusV1 struct {
	PlanID       string `json:"planId"`
	RelativePath string `json:"relativePath"`
	Operation    string `json:"operation"`
	ContentHash  string `json:"contentHash"`
	ByteSize     int64  `json:"byteSize"`
	SavedAt      string `json:"savedAt"`
}

type MCPRPCErrorDiagnostic struct {
	Code        int    `json:"code"`
	Class       string `json:"class"`
	DataPresent bool   `json:"dataPresent"`
}

// HostReportAdmissionV1 is private settlement metadata. It binds the exact
// durable tool result to one signed report decision without carrying report
// bytes, paths, claim payloads, PII, provider text, or reasoning. Public tool
// projections deliberately omit it.
type HostReportAdmissionV1 struct {
	Version              int    `json:"version"`
	Purpose              string `json:"purpose"`
	Status               string `json:"status"`
	DecisionID           string `json:"decisionId"`
	DecisionRecordDigest string `json:"decisionRecordDigest"`
}

func NewHostReportAdmissionV1(decisionID, decisionRecordDigest string) HostReportAdmissionV1 {
	return HostReportAdmissionV1{
		Version: HostReportAdmissionVersion, Purpose: HostReportAdmissionPurpose, Status: "admitted",
		DecisionID: strings.TrimSpace(decisionID), DecisionRecordDigest: strings.TrimSpace(decisionRecordDigest),
	}
}

func ValidateHostReportAdmissionV1(admission HostReportAdmissionV1) error {
	if admission.Version != HostReportAdmissionVersion || admission.Purpose != HostReportAdmissionPurpose ||
		admission.Status != "admitted" || !domainsecurity.IsSHA256Hex(admission.DecisionID) ||
		!domainsecurity.IsSHA256Hex(admission.DecisionRecordDigest) {
		return errors.New("host report admission is invalid")
	}
	return nil
}

func ParseHostReportAdmissionV1(value any) (HostReportAdmissionV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return HostReportAdmissionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var admission HostReportAdmissionV1
	if err := decoder.Decode(&admission); err != nil {
		return HostReportAdmissionV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return HostReportAdmissionV1{}, errors.New("host report admission contains trailing data")
	}
	return admission, ValidateHostReportAdmissionV1(admission)
}

func HostReportAdmissionRecordV1(admission HostReportAdmissionV1) map[string]any {
	if ValidateHostReportAdmissionV1(admission) != nil {
		return nil
	}
	body, _ := json.Marshal(admission)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

// ValidatePrivateAdmittedReportResultItemV1 is the sole content-level check
// for a durable report-stage result eligible for grant settlement. It accepts
// only the canonically closed private host shape, a successful metadata-only
// public projection, and one exact host-issued decision admission. A generic
// successful tool result or a caller-supplied nonempty decision ID is never
// publication authority.
func ValidatePrivateAdmittedReportResultItemV1(
	item map[string]any,
	decisionID string,
	decisionRecordDigest string,
) error {
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(decisionID)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(decisionRecordDigest)) {
		return errors.New("private report result decision binding is invalid")
	}
	isError, isErrorOK := item["isError"].(bool)
	projected, projectedOK := PrivateDurableToolResultItemRecordV1(item)
	projectedBody, projectedErr := json.Marshal(projected)
	itemBody, itemErr := json.Marshal(item)
	projection, projectionErr := ParsePublicToolResultProjectionV1(item["output"])
	admission, admissionErr := ParseHostReportAdmissionV1(item["hostReportAdmission"])
	if !isErrorOK || isError || !projectedOK || projectedErr != nil || itemErr != nil ||
		domainsecurity.CanonicalJSONHash(projectedBody) != domainsecurity.CanonicalJSONHash(itemBody) ||
		projectionErr != nil || projection.Status != "completed" || !projection.PrivatePayloadWithheld ||
		projection.FactAnswerAllowed || projection.EvidenceAuthority || admissionErr != nil ||
		admission.DecisionID != strings.TrimSpace(decisionID) ||
		admission.DecisionRecordDigest != strings.TrimSpace(decisionRecordDigest) ||
		strings.TrimSpace(publicItemString(item, "role")) != "tool" ||
		strings.TrimSpace(publicItemString(item, "status")) != "completed" ||
		strings.TrimSpace(publicItemString(item, "toolName")) != "stage_case_report" {
		return errors.New("private admitted report result is invalid")
	}
	return nil
}

var publicCodePattern = regexp.MustCompile(`^[a-z0-9_-]{1,96}$`)

func ValidatePublicToolResultProjectionV1(projection PublicToolResultProjectionV1) error {
	if projection.SchemaVersion != PublicProjectionSchemaVersion || projection.Disclosure != MetadataOnlyDisclosure {
		return errors.New("public tool result projection version or disclosure is invalid")
	}
	if !projection.PrivatePayloadWithheld || projection.FactAnswerAllowed || projection.EvidenceAuthority {
		return errors.New("public tool result projection cannot carry private payload or evidence authority")
	}
	if !validPublicStatus(projection.Status) {
		return errors.New("public tool result projection status is invalid")
	}
	if projection.Code != "" && (!publicCodePattern.MatchString(projection.Code) || !validPublicProjectionCode(projection.ProjectionKind, projection.Code)) {
		return errors.New("public tool result projection code is invalid")
	}
	if projection.ProjectionKind != ProjectionWebFetchStatus && projection.Web != nil {
		return errors.New("tool result contains an incompatible web payload")
	}
	if projection.ProjectionKind != ProjectionArtifactStatus && projection.Artifact != nil {
		return errors.New("tool result contains an incompatible artifact payload")
	}
	if projection.ProjectionKind != ProjectionTaskJobResult && projection.TaskJob != nil {
		return errors.New("tool result contains incompatible job metadata")
	}
	switch projection.ProjectionKind {
	case ProjectionTaskJobResult:
		if projection.Status != "completed" || projection.MessageKey != "tool_completed" || projection.Code != "tool_completed" || projection.TaskJob == nil ||
			projection.Plan != nil || projection.RPCError != nil || projection.Web != nil || projection.Artifact != nil || validateTaskJobResultV1(*projection.TaskJob) != nil {
			return errors.New("task-job result projection is invalid")
		}
	case ProjectionWithheld:
		if projection.MessageKey != "tool_output_withheld" && projection.MessageKey != "legacy_output_withheld" {
			return errors.New("withheld tool result message key is invalid")
		}
		if projection.Plan != nil || projection.RPCError != nil {
			return errors.New("withheld tool result contains an incompatible payload")
		}
	case ProjectionHostStatus:
		if !oneOf(projection.MessageKey, "tool_completed", "tool_failed", "tool_cancelled", "tool_blocked", "tool_outcome_unknown") {
			return errors.New("host tool result message key is invalid")
		}
		unknownStatus := projection.Status == "unknown"
		unknownMessage := projection.MessageKey == "tool_outcome_unknown"
		unknownCode := projection.Code == "tool_outcome_unknown_after_restart"
		if (unknownStatus || unknownMessage || unknownCode) && !(unknownStatus && unknownMessage && unknownCode) {
			return errors.New("host outcome-unknown projection is not canonically paired")
		}
		if projection.Plan != nil || projection.RPCError != nil {
			return errors.New("host tool result contains an incompatible payload")
		}
	case ProjectionWebFetchStatus:
		if projection.Web == nil || projection.Plan != nil || projection.RPCError != nil ||
			ValidateWebFetchStatusV1(*projection.Web) != nil {
			return errors.New("web fetch tool result projection is invalid")
		}
		status, messageKey, code, ok := webFetchLifecycleV1(projection.Web.Outcome)
		if !ok || projection.Status != status || projection.MessageKey != messageKey || projection.Code != code {
			return errors.New("web fetch tool result lifecycle is not canonically paired")
		}
	case ProjectionArtifactStatus:
		if projection.MessageKey != "artifact_created" || projection.Code != "artifact_created" || projection.Status != "completed" ||
			projection.Artifact == nil || projection.Plan != nil || projection.RPCError != nil || validateArtifactStatus(*projection.Artifact) != nil {
			return errors.New("artifact tool result projection is invalid")
		}
	case ProjectionPlanStatus:
		if !oneOf(projection.MessageKey, "plan_updated", "plan_failed") || projection.Plan == nil ||
			projection.RPCError != nil || validatePlanStatus(*projection.Plan) != nil {
			return errors.New("plan tool result projection is invalid")
		}
	case ProjectionCaseSourceStatus:
		if !oneOf(projection.MessageKey, "case_source_private", "case_source_failed") ||
			projection.Plan != nil || projection.RPCError != nil {
			return errors.New("case source tool result projection is invalid")
		}
	case ProjectionMCPDiagnostic:
		if projection.MessageKey != "mcp_request_rejected" || projection.RPCError == nil ||
			projection.Plan != nil || validateRPCError(*projection.RPCError) != nil {
			return errors.New("MCP diagnostic projection is invalid")
		}
	default:
		return errors.New("public tool result projection kind is invalid")
	}
	return nil
}

func validateArtifactStatus(artifact ArtifactStatusV1) error {
	if !domainsecurity.IsSHA256Hex(artifact.ArtifactID) || !domainsecurity.IsSHA256Hex(artifact.ContentHash) ||
		!oneOf(artifact.Kind, "docx", "xlsx", "pptx") || artifact.ByteSize <= 0 || artifact.ByteSize > 16<<20 {
		return errors.New("artifact metadata is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, artifact.SavedAt); err != nil {
		return errors.New("artifact confirmation time is invalid")
	}
	return nil
}

func ParsePublicToolResultProjectionV1(value any) (PublicToolResultProjectionV1, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return PublicToolResultProjectionV1{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return PublicToolResultProjectionV1{}, err
	}
	var kind PublicProjectionKind
	_ = json.Unmarshal(fields["projectionKind"], &kind)
	if raw, present := fields["taskJob"]; present && (kind != ProjectionTaskJobResult || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
		return PublicToolResultProjectionV1{}, errors.New("job metadata is incompatible or null")
	}
	if kind == ProjectionTaskJobResult {
		required := []string{"schemaVersion", "projectionKind", "disclosure", "messageKey", "status", "code", "privatePayloadWithheld", "factAnswerAllowed", "evidenceAuthority", "taskJob"}
		if len(fields) != len(required) {
			return PublicToolResultProjectionV1{}, errors.New("job projection shape is invalid")
		}
		for _, key := range required {
			raw, present := fields[key]
			if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return PublicToolResultProjectionV1{}, errors.New("job projection field is missing or null")
			}
		}
	}
	if web, present := fields["web"]; present && bytes.Equal(bytes.TrimSpace(web), []byte("null")) {
		return PublicToolResultProjectionV1{}, errors.New("public web payload cannot be null")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var projection PublicToolResultProjectionV1
	if err := decoder.Decode(&projection); err != nil {
		return PublicToolResultProjectionV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return PublicToolResultProjectionV1{}, errors.New("public tool result projection contains trailing data")
	}
	if err := ValidatePublicToolResultProjectionV1(projection); err != nil {
		return PublicToolResultProjectionV1{}, err
	}
	return projection, nil
}

func PublicToolResultProjectionRecordV1(projection PublicToolResultProjectionV1) map[string]any {
	if ValidatePublicToolResultProjectionV1(projection) != nil {
		projection = WithheldProjectionV1("failed", "tool_projection_invalid")
	}
	data, _ := json.Marshal(projection)
	record := map[string]any{}
	_ = json.Unmarshal(data, &record)
	return record
}

// PublicToolResultItemRecordV1 is the only ordinary-public projection for a
// durable tool-result item. It keeps pairing and lifecycle metadata, replaces
// legacy/open output with a closed projection, and drops every other root
// field so content cannot bypass the output schema through summary, details,
// media, paths, or remote authority claims.
func PublicToolResultItemRecordV1(item map[string]any) map[string]any {
	turnID := strings.TrimSpace(publicItemString(item, "turnId"))
	callID := strings.TrimSpace(publicItemString(item, "callId"))
	itemID := ToolResultItemIDV1(turnID, callID)
	if !domainmodel.IsHostToolCallIDV1(callID) || itemID == "" {
		projected := toolResultLifecycleRecordV1(item)
		delete(projected, "id")
		delete(projected, "callId")
		delete(projected, "toolName")
		delete(projected, "toolKind")
		projected["status"] = "failed"
		projected["isError"] = true
		projected["legacyIdentityWithheld"] = true
		projected["output"] = PublicToolResultProjectionRecordV1(LegacyWithheldProjectionV1())
		return projected
	}
	projected := toolResultLifecycleRecordV1(item)
	projected["id"] = itemID
	projected["callId"] = callID
	if projected["toolKind"] == "skill" {
		projected["toolKind"] = "tool_call"
	}
	projected["output"] = publicToolResultOutputProjectionV1(item)
	if caseSourceReadDowngradedV1(item, projected["output"]) {
		isError, valid := item["isError"].(bool)
		if !valid {
			isError = true
		}
		projected["isError"] = isError
		if isError {
			projected["status"] = "failed"
		} else {
			projected["status"] = "completed"
		}
		delete(projected, "lifecycleStatusWithheld")
	}
	return projected
}

func toolResultLifecycleRecordV1(item map[string]any) map[string]any {
	projected := map[string]any{}
	for _, key := range []string{
		"id", "turnId", "threadId", "createdAt", "finishedAt",
		"toolName", "callId", "toolKind",
	} {
		if value, ok := item[key]; ok {
			projected[key] = value
		}
	}
	projected["kind"] = "tool_result"
	projected["role"] = "tool"
	isError, valid := item["isError"].(bool)
	if !valid {
		isError = true
		projected["lifecycleStatusWithheld"] = true
	}
	projected["isError"] = isError
	rawStatus := strings.TrimSpace(publicItemString(item, "status"))
	projection, projectionErr := ParsePublicToolResultProjectionV1(item["output"])
	status, lifecycleErr := SettlementLifecycleStatusV1(projection, isError)
	if !valid || projectionErr != nil || lifecycleErr != nil {
		status = "failed"
		isError = true
		projected["isError"] = true
		projected["lifecycleStatusWithheld"] = true
	} else if status != rawStatus {
		projected["lifecycleStatusWithheld"] = true
	}
	projected["status"] = status
	if toolKind := publicToolResultKindV1(publicItemString(item, "toolKind")); toolKind != "" {
		projected["toolKind"] = toolKind
	} else {
		delete(projected, "toolKind")
	}
	return projected
}

func publicToolResultKindV1(value string) string {
	switch strings.TrimSpace(value) {
	case "tool_call", "file_change", "command_execution", "skill", "subagent":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func publicToolResultOutputProjectionV1(item map[string]any) map[string]any {
	_, caseSourceProofPresent := item[CaseSourceBindingProofFieldV1]
	projection, err := ParsePublicToolResultProjectionV1(item["output"])
	if err != nil {
		if caseSourceProofPresent {
			projection = caseSourceLegacyWithheldProjectionV1(item)
		} else {
			projection = LegacyWithheldProjectionV1()
		}
	} else if !publicProjectionMatchesItem(projection, item) {
		if projection.ProjectionKind == ProjectionCaseSourceStatus || caseSourceProofPresent {
			projection = caseSourceLegacyWithheldProjectionV1(item)
		} else {
			projection = LegacyWithheldProjectionV1()
		}
	}
	return PublicToolResultProjectionRecordV1(projection)
}

func caseSourceLegacyWithheldProjectionV1(item map[string]any) PublicToolResultProjectionV1 {
	status := "failed"
	if isError, ok := item["isError"].(bool); ok && !isError {
		status = "completed"
	}
	projection := WithheldProjectionV1(status, "legacy_output_withheld")
	projection.MessageKey = "legacy_output_withheld"
	return projection
}

func caseSourceReadDowngradedV1(item map[string]any, output any) bool {
	_, caseSourceProofPresent := item[CaseSourceBindingProofFieldV1]
	original, originalErr := ParsePublicToolResultProjectionV1(item["output"])
	projected, projectedErr := ParsePublicToolResultProjectionV1(output)
	caseSourceInput := caseSourceProofPresent || (originalErr == nil && original.ProjectionKind == ProjectionCaseSourceStatus)
	return caseSourceInput &&
		projectedErr == nil && projected.ProjectionKind == ProjectionWithheld && projected.MessageKey == "legacy_output_withheld"
}

// PrivateDurableToolResultItemRecordV1 is the host-only persistence shape.
// It keeps the closed public output while retaining only the context/grant
// metadata needed to replay active -> settled membership. Optional evidence
// settlement and private-protocol observation markers remain opaque here.
// Neither marker is part of the public item projection or independent
// authority; the latter only selects the safe provider-history disposition for
// a host-bound tool pair after the exact integrity-bound grant fields are
// revalidated.
func PrivateDurableToolResultItemRecordV1(item map[string]any) (map[string]any, bool) {
	projected := toolResultLifecycleRecordV1(item)
	projected["output"] = publicToolResultOutputProjectionV1(item)
	rawProjection, rawProjectionErr := ParsePublicToolResultProjectionV1(item["output"])
	contextDigest := strings.TrimSpace(publicItemString(item, "contextDigest"))
	grantID := strings.TrimSpace(publicItemString(item, "executionGrantId"))
	contextEpoch := publicItemUint64(item["contextEpoch"])
	marker, markerPresent := item["hostEvidenceSettlement"]
	reportAdmissionValue, reportAdmissionPresent := item["hostReportAdmission"]
	privateProtocolObservedValue, privateProtocolObservedPresent := item["privateProtocolObserved"]
	caseSourceProofValue, caseSourceProofPresent := item[CaseSourceBindingProofFieldV1]
	snapshotValue, snapshotPresent := item[ProtectedSnapshotBindingFieldV1]
	isCaseProjection := rawProjectionErr == nil && rawProjection.ProjectionKind == ProjectionCaseSourceStatus
	if isCaseProjection {
		if !caseSourceProofPresent || validateCaseSourceBindingProofForItemV1(rawProjection, item) != nil {
			return projected, false
		}
	} else if caseSourceProofPresent {
		return projected, false
	}
	if contextDigest == "" && grantID == "" && contextEpoch == 0 && !markerPresent && !reportAdmissionPresent &&
		!privateProtocolObservedPresent && !caseSourceProofPresent && !snapshotPresent {
		return projected, true
	}
	if !domainsecurity.IsSHA256Hex(contextDigest) || !domainsecurity.IsSHA256Hex(grantID) || contextEpoch == 0 ||
		strings.TrimSpace(publicItemString(projected, "threadId")) == "" || strings.TrimSpace(publicItemString(projected, "turnId")) == "" ||
		strings.TrimSpace(publicItemString(projected, "toolName")) == "" || strings.TrimSpace(publicItemString(projected, "callId")) == "" {
		return projected, false
	}
	projected["contextDigest"] = contextDigest
	projected["contextEpoch"] = contextEpoch
	projected["executionGrantId"] = grantID
	if caseSourceProofPresent {
		proof, err := ParseCaseSourceBindingProofV1(caseSourceProofValue)
		if err != nil {
			return projected, false
		}
		projected[CaseSourceBindingProofFieldV1] = CaseSourceBindingProofRecordV1(proof)
	}
	if markerPresent {
		cloned := cloneToolResultJSONValue(marker)
		if cloned == nil {
			return projected, false
		}
		projected["hostEvidenceSettlement"] = cloned
	}
	if reportAdmissionPresent {
		admission, err := ParseHostReportAdmissionV1(reportAdmissionValue)
		if err != nil {
			return projected, false
		}
		projected["hostReportAdmission"] = HostReportAdmissionRecordV1(admission)
	}
	if snapshotPresent {
		binding, err := ParseProtectedSnapshotBindingV1(snapshotValue)
		name, kind := publicItemString(projected, "toolName"), publicItemString(projected, "toolKind")
		if err != nil || name == "" || strings.TrimSpace(name) != name || !utf8.ValidString(name) || len(name) > 256 || strings.ContainsRune(name, 0) || kind == "" ||
			oneOf(name, "web_fetch", "grep", "glob", "create_goal", "get_goal", "update_goal", "todo_write") && kind != "tool_call" || name == "run_skill" && kind != "skill" {
			return projected, false
		}
		projected[ProtectedSnapshotBindingFieldV1] = ProtectedSnapshotBindingRecordV1(binding)
	}
	if privateProtocolObservedPresent {
		observed, ok := privateProtocolObservedValue.(bool)
		if !ok || !observed {
			return projected, false
		}
		projected["privateProtocolObserved"] = true
	}
	return projected, true
}

// ClosedPlanToolResultDigestV1 returns the plan digest only for the exact
// host-bound durable create_plan result shape. It is a structural privacy
// classification helper, not execution, evidence, or publication authority.
func ClosedPlanToolResultDigestV1(item map[string]any) (string, bool) {
	threadID := publicItemString(item, "threadId")
	turnID := publicItemString(item, "turnId")
	callID := publicItemString(item, "callId")
	output, outputOK := item["output"].(map[string]any)
	plan, planOK := output["plan"].(map[string]any)
	if threadID == "" || strings.TrimSpace(threadID) != threadID || turnID == "" || strings.TrimSpace(turnID) != turnID ||
		!domainmodel.IsHostToolCallIDV1(callID) || publicItemString(item, "id") != ToolResultItemIDV1(turnID, callID) ||
		publicItemString(item, "toolName") != "create_plan" || publicItemString(item, "toolKind") != "file_change" ||
		!outputOK || output == nil || !planOK || plan == nil {
		return "", false
	}
	canonical, ok := PrivateDurableToolResultItemRecordV1(item)
	canonicalBody, canonicalErr := json.Marshal(canonical)
	itemBody, itemErr := json.Marshal(item)
	if !ok || canonicalErr != nil || itemErr != nil || !bytes.Equal(canonicalBody, itemBody) {
		return "", false
	}
	projection, err := ParsePublicToolResultProjectionV1(item["output"])
	if err != nil || projection.ProjectionKind != ProjectionPlanStatus || projection.Plan == nil {
		return "", false
	}
	return projection.Plan.ContentHash, true
}

func cloneToolResultJSONValue(value any) any {
	body, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned any
	if json.Unmarshal(body, &cloned) != nil {
		return nil
	}
	return cloned
}

func publicProjectionMatchesItem(projection PublicToolResultProjectionV1, item map[string]any) bool {
	isError, ok := item["isError"].(bool)
	if !ok {
		return false
	}
	status, err := SettlementLifecycleStatusV1(projection, isError)
	if err != nil || status != strings.TrimSpace(publicItemString(item, "status")) {
		return false
	}
	if projection.ProjectionKind == ProjectionWebFetchStatus &&
		(publicItemString(item, "toolName") != "web_fetch" || publicItemString(item, "toolKind") != "tool_call") {
		return false
	}
	if projection.ProjectionKind == ProjectionMCPDiagnostic && !isError {
		return false
	}
	if projection.ProjectionKind == ProjectionTaskJobResult && (projection.TaskJob == nil ||
		publicItemString(item, "toolName") != taskJobToolNameV1(projection.TaskJob.Operation) || publicItemString(item, "toolKind") != "command_execution" ||
		isError || !domainmodel.IsHostToolCallIDV1(publicItemString(item, "callId")) || publicItemString(item, "id") != ToolResultItemIDV1(publicItemString(item, "turnId"), publicItemString(item, "callId"))) {
		return false
	}
	_, caseSourceProofPresent := item[CaseSourceBindingProofFieldV1]
	if projection.ProjectionKind == ProjectionCaseSourceStatus {
		return caseSourceProofPresent && validateCaseSourceBindingProofForItemV1(projection, item) == nil
	}
	if caseSourceProofPresent {
		return false
	}
	return true
}

var taskJobPublicIDPatternV1 = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

type TaskJobSummaryV1 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Status            string `json:"status"`
	Background        bool   `json:"background"`
	Terminal          bool   `json:"terminal"`
	OutputWithheld    bool   `json:"outputWithheld"`
	OutputTrustStatus string `json:"outputTrustStatus"`
	FactAnswerAllowed bool   `json:"factAnswerAllowed"`
	EvidenceAuthority bool   `json:"evidenceAuthority"`
	CanReadOutput     bool   `json:"canReadOutput"`
	CanContinueParent bool   `json:"canContinueParent"`
}
type TaskJobOutputV1 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Availability      string `json:"availability"`
	JobID             string `json:"jobId"`
	Status            string `json:"status"`
	ReasonCode        string `json:"reasonCode"`
	OutputWithheld    bool   `json:"outputWithheld"`
	OutputTrustStatus string `json:"outputTrustStatus"`
	FactAnswerAllowed bool   `json:"factAnswerAllowed"`
	EvidenceAuthority bool   `json:"evidenceAuthority"`
	CanReadOutput     bool   `json:"canReadOutput"`
	CanContinueParent bool   `json:"canContinueParent"`
}
type TaskJobListV1 struct {
	Jobs  []TaskJobSummaryV1 `json:"jobs"`
	Count int                `json:"count"`
}
type TaskJobWaitV1 struct {
	Jobs []TaskJobSummaryV1 `json:"jobs"`
}
type TaskJobKillV1 struct {
	Job TaskJobSummaryV1 `json:"job"`
}

// One typed result per operation; no arbitrary result object is retained.
type TaskJobResultV1 struct {
	Operation string
	List      *TaskJobListV1
	Wait      *TaskJobWaitV1
	Output    *TaskJobOutputV1
	Kill      *TaskJobKillV1
}

func (s *TaskJobSummaryV1) UnmarshalJSON(body []byte) error {
	type record TaskJobSummaryV1
	if err := decodeClosedRequiredV1(body, (*record)(s), []string{"schemaVersion", "id", "kind", "status", "background", "terminal", "outputWithheld", "outputTrustStatus", "factAnswerAllowed", "evidenceAuthority", "canReadOutput", "canContinueParent"}, nil); err != nil {
		return err
	}
	return validateTaskJobSummaryV1(*s)
}
func (o *TaskJobOutputV1) UnmarshalJSON(body []byte) error {
	type record TaskJobOutputV1
	if err := decodeClosedRequiredV1(body, (*record)(o), []string{"schemaVersion", "availability", "jobId", "status", "reasonCode", "outputWithheld", "outputTrustStatus", "factAnswerAllowed", "evidenceAuthority", "canReadOutput", "canContinueParent"}, nil); err != nil {
		return err
	}
	return validateTaskJobOutputV1(*o)
}
func (l *TaskJobListV1) UnmarshalJSON(body []byte) error {
	type record TaskJobListV1
	return decodeClosedRequiredV1(body, (*record)(l), []string{"jobs", "count"}, nil)
}
func (w *TaskJobWaitV1) UnmarshalJSON(body []byte) error {
	type record TaskJobWaitV1
	return decodeClosedRequiredV1(body, (*record)(w), []string{"jobs"}, nil)
}
func (k *TaskJobKillV1) UnmarshalJSON(body []byte) error {
	type record TaskJobKillV1
	return decodeClosedRequiredV1(body, (*record)(k), []string{"job"}, nil)
}
func taskJobToolNameV1(operation string) string {
	switch operation {
	case "list":
		return "list_jobs"
	case "wait":
		return "wait"
	case "output":
		return "bash_output"
	case "kill":
		return "kill_shell"
	}
	return ""
}
func TaskJobOperationV1(toolName string) string {
	switch toolName {
	case "list_jobs":
		return "list"
	case "wait":
		return "wait"
	case "bash_output":
		return "output"
	case "kill_shell":
		return "kill"
	}
	return ""
}

// These are already-normalized wire enums from TaskJobStatusV1Schema and
// TaskJobKindV1Schema. Producers keep their domain/job normalization; this
// result boundary rejects arbitrary values without importing the producer
// domain, whose output filtering publishes through event/toolresult.
func validTaskJobWireStatusV1(value string) bool {
	return oneOf(value, "queued", "running", "pause_requested", "paused", "resume_requested", "resuming",
		"completed", "failed", "aborted", "interrupted", "killed", "canceled", "timeout", "unknown")
}
func taskJobWireTerminalV1(value string) bool {
	return oneOf(value, "completed", "failed", "aborted", "interrupted", "killed", "canceled", "timeout")
}
func validateTaskJobSummaryV1(s TaskJobSummaryV1) error {
	if s.SchemaVersion != 1 || !taskJobPublicIDPatternV1.MatchString(s.ID) ||
		!oneOf(s.Kind, "background-shell", "subagent", "child-run", "parallel-child-run", "task", "parallel_task", "bash", "unknown") || !validTaskJobWireStatusV1(s.Status) ||
		s.Terminal != taskJobWireTerminalV1(s.Status) || !s.OutputWithheld || s.OutputTrustStatus != "untrusted_child_output" || s.FactAnswerAllowed || s.EvidenceAuthority || s.CanReadOutput || s.CanContinueParent {
		return errors.New("task-job summary is invalid")
	}
	return nil
}
func validateTaskJobOutputV1(o TaskJobOutputV1) error {
	if o.SchemaVersion != 1 || o.Availability != "withheld" || !taskJobPublicIDPatternV1.MatchString(o.JobID) || !validTaskJobWireStatusV1(o.Status) ||
		o.ReasonCode != "security_bound_child_output" || !o.OutputWithheld || o.OutputTrustStatus != "untrusted_child_output" || o.FactAnswerAllowed || o.EvidenceAuthority || o.CanReadOutput || o.CanContinueParent {
		return errors.New("task-job output is invalid")
	}
	return nil
}
func validateTaskJobResultV1(r TaskJobResultV1) error {
	if (r.List != nil) != (r.Operation == "list") || (r.Wait != nil) != (r.Operation == "wait") || (r.Output != nil) != (r.Operation == "output") || (r.Kill != nil) != (r.Operation == "kill") || taskJobToolNameV1(r.Operation) == "" {
		return errors.New("task-job operation/result mismatch")
	}
	var jobs []TaskJobSummaryV1
	if r.List != nil {
		if r.List.Jobs == nil || r.List.Count != len(r.List.Jobs) {
			return errors.New("task-job list count mismatch")
		}
		jobs = r.List.Jobs
	}
	if r.Wait != nil {
		if r.Wait.Jobs == nil {
			return errors.New("task-job wait needs a list")
		}
		jobs = r.Wait.Jobs
	}
	if r.Kill != nil {
		jobs = []TaskJobSummaryV1{r.Kill.Job}
	}
	for _, job := range jobs {
		if err := validateTaskJobSummaryV1(job); err != nil {
			return err
		}
	}
	if r.Output != nil {
		return validateTaskJobOutputV1(*r.Output)
	}
	return nil
}
func (r TaskJobResultV1) MarshalJSON() ([]byte, error) {
	if err := validateTaskJobResultV1(r); err != nil {
		return nil, err
	}
	var result any
	switch r.Operation {
	case "list":
		result = r.List
	case "wait":
		result = r.Wait
	case "output":
		result = r.Output
	case "kill":
		result = r.Kill
	}
	return json.Marshal(struct {
		Operation string `json:"operation"`
		Result    any    `json:"result"`
	}{r.Operation, result})
}
func (r *TaskJobResultV1) UnmarshalJSON(body []byte) error {
	var envelope struct {
		Operation string          `json:"operation"`
		Result    json.RawMessage `json:"result"`
	}
	if err := decodeClosedRequiredV1(body, &envelope, []string{"operation", "result"}, nil); err != nil {
		return err
	}
	*r = TaskJobResultV1{Operation: envelope.Operation}
	var target any
	switch r.Operation {
	case "list":
		r.List = &TaskJobListV1{}
		target = r.List
	case "wait":
		r.Wait = &TaskJobWaitV1{}
		target = r.Wait
	case "output":
		r.Output = &TaskJobOutputV1{}
		target = r.Output
	case "kill":
		r.Kill = &TaskJobKillV1{}
		target = r.Kill
	default:
		return errors.New("unknown task-job operation")
	}
	if err := json.Unmarshal(envelope.Result, target); err != nil {
		return err
	}
	return validateTaskJobResultV1(*r)
}
func NewTaskJobResultV1(operation string, value any) (*TaskJobResultV1, error) {
	record, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("task-job producer did not return a closed record")
	}
	normalized := map[string]any{}
	if operation == "list" || operation == "wait" {
		expected := 1
		if operation == "list" {
			expected = 2
		}
		if len(record) != expected {
			return nil, errors.New("unknown task-job list fields")
		}
		rows, ok := record["jobs"].([]any)
		if !ok {
			return nil, errors.New("task-job producer list missing")
		}
		jobs := make([]any, 0, len(rows))
		for _, raw := range rows {
			summary, err := normalizeTaskJobProducerSummaryV1(raw)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, summary)
		}
		normalized["jobs"] = jobs
		if operation == "list" {
			count, present := record["count"]
			if !present {
				return nil, errors.New("task-job count missing")
			}
			normalized["count"] = count
		}
	} else if operation == "kill" {
		if len(record) != 1 {
			return nil, errors.New("unknown task-job kill fields")
		}
		summary, err := normalizeTaskJobProducerSummaryV1(record["job"])
		if err != nil {
			return nil, err
		}
		normalized["job"] = summary
	} else if operation == "output" {
		normalized = record
	} else {
		return nil, errors.New("unknown task-job operation")
	}
	body, err := json.Marshal(struct {
		Operation string `json:"operation"`
		Result    any    `json:"result"`
	}{operation, normalized})
	if err != nil {
		return nil, err
	}
	var result TaskJobResultV1
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func normalizeTaskJobProducerSummaryV1(value any) (map[string]any, error) {
	record, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("task-job summary is not a record")
	}
	copy_ := map[string]any{}
	for key, value := range record {
		if key == "active" {
			if _, ok := value.(bool); !ok {
				return nil, errors.New("task-job active is not a boolean")
			}
			continue
		}
		copy_[key] = value
	}
	body, err := json.Marshal(copy_)
	if err != nil {
		return nil, err
	}
	var summary TaskJobSummaryV1
	if err := json.Unmarshal(body, &summary); err != nil {
		return nil, err
	}
	return copy_, nil
}

func publicItemString(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func publicItemUint64(value any) uint64 {
	switch typed := value.(type) {
	case uint64:
		return typed
	case uint:
		return uint64(typed)
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case int64:
		if typed > 0 {
			return uint64(typed)
		}
	case float64:
		if typed > 0 && typed <= 9007199254740991 && typed == float64(uint64(typed)) {
			return uint64(typed)
		}
	case json.Number:
		raw := typed.String()
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err == nil && parsed > 0 && parsed <= 9007199254740991 && strconv.FormatUint(parsed, 10) == raw {
			return parsed
		}
	}
	return 0
}

func WithheldProjectionV1(status string, code string) PublicToolResultProjectionV1 {
	status = normalizeStatus(status)
	code = normalizeCode(code)
	return PublicToolResultProjectionV1{
		SchemaVersion:          PublicProjectionSchemaVersion,
		ProjectionKind:         ProjectionWithheld,
		Disclosure:             MetadataOnlyDisclosure,
		MessageKey:             "tool_output_withheld",
		Status:                 status,
		Code:                   code,
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
	}
}

func LegacyWithheldProjectionV1() PublicToolResultProjectionV1 {
	projection := WithheldProjectionV1("unknown", "legacy_output_withheld")
	projection.MessageKey = "legacy_output_withheld"
	return projection
}

// OutcomeUnknownAfterRestartProjectionV1 is host-authored metadata for an
// approved writable effect that may have reached the external system before a
// process crash. It is neither success nor cancellation and carries no
// evidence authority from which a case fact could be published.
func OutcomeUnknownAfterRestartProjectionV1() PublicToolResultProjectionV1 {
	return PublicToolResultProjectionV1{
		SchemaVersion:          PublicProjectionSchemaVersion,
		ProjectionKind:         ProjectionHostStatus,
		Disclosure:             MetadataOnlyDisclosure,
		MessageKey:             "tool_outcome_unknown",
		Status:                 "unknown",
		Code:                   "tool_outcome_unknown_after_restart",
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
	}
}

func normalizeStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if validPublicStatus(status) {
		return status
	}
	return "failed"
}

func normalizeCode(code string) string {
	code = strings.TrimSpace(strings.ToLower(code))
	if publicCodePattern.MatchString(code) && validPublicProjectionCode(ProjectionWithheld, code) {
		return code
	}
	return "tool_output_private"
}

func validPublicProjectionCode(kind PublicProjectionKind, code string) bool {
	switch kind {
	case ProjectionWebFetchStatus:
		return oneOf(code, "tool_completed", "tool_failed", "tool_blocked", "tool_cancelled", "tool_timeout")
	case ProjectionArtifactStatus:
		return code == "artifact_created"
	case ProjectionTaskJobResult:
		return code == "tool_completed"
	case ProjectionWithheld:
		return oneOf(code, "tool_output_private", "legacy_output_withheld", "tool_projection_invalid")
	case ProjectionHostStatus:
		return oneOf(code,
			"approval_cancelled", "approval_denied", "approval_policy_blocked", "cancelled",
			"execution_grant_context_mismatch", "execution_grant_expired", "execution_grant_invalid",
			"loop_guard", "not_found", "publication_receipt_required", "case_report_publication_receipt_required",
			"runtime_recovered_job_interrupted", "sandbox_blocked", "side_effect_duplicate", "tool_blocked", "tool_cancelled",
			"tool_completed", "tool_failed", "tool_not_advertised", "tool_source_unavailable", "tool_timeout",
			"tool_outcome_unknown_after_restart", "user_input_cancelled", "validation_error", "workspace_escape")
	case ProjectionPlanStatus:
		return code == "plan_updated"
	case ProjectionCaseSourceStatus:
		return code == "case_source_result_private"
	case ProjectionMCPDiagnostic:
		return code == "mcp_request_rejected"
	default:
		return false
	}
}

func validPublicStatus(status string) bool {
	return oneOf(status, "completed", "failed", "blocked", "cancelled", "unknown")
}

// ValidatePlanTargetV1 is shared by write admission and the public result. A
// physical write must not succeed with a target its result cannot represent.
func ValidatePlanTargetV1(planID, relativePath string) error {
	if strings.TrimSpace(planID) == "" || len(planID) > 256 || strings.TrimSpace(relativePath) == "" || len(relativePath) > 1024 ||
		path.IsAbs(strings.ReplaceAll(relativePath, "\\", "/")) || strings.HasPrefix(path.Clean(strings.ReplaceAll(relativePath, "\\", "/")), "../") {
		return errors.New("plan target exceeds the supported identity/path bounds or is not relative")
	}
	return nil
}

func validatePlanStatus(plan PlanStatusV1) error {
	if ValidatePlanTargetV1(plan.PlanID, plan.RelativePath) != nil || !oneOf(plan.Operation, "draft", "refine") || !domainsecurity.IsSHA256Hex(plan.ContentHash) || plan.ByteSize < 0 {
		return errors.New("plan status fields are invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, plan.SavedAt); err != nil {
		return errors.New("plan status timestamp is invalid")
	}
	return nil
}

func validateRPCError(diagnostic MCPRPCErrorDiagnostic) error {
	if diagnostic.Code != canonicalPublicRPCErrorCode(diagnostic.Class) {
		return errors.New("MCP RPC diagnostic fields are invalid")
	}
	return nil
}

func canonicalPublicRPCErrorCode(class string) int {
	switch class {
	case "parse_error":
		return -32700
	case "invalid_request":
		return -32600
	case "method_not_found":
		return -32601
	case "invalid_params":
		return -32602
	case "internal_error":
		return -32603
	case "server_error":
		return -32000
	default:
		return 0
	}
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
