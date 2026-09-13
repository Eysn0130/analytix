package privacyprojection

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrProviderPrivacyAuthorityUnavailable = errors.New("provider privacy projection authority is unavailable")
	ErrUninspectableProviderPart           = errors.New("provider request contains an uninspectable private part")
	ErrProviderSchemaContainsPII           = errors.New("provider tool schema contains restricted PII")
	ErrOrdinaryProviderContainsCaseEffect  = errors.New("ordinary provider request contains a protected case-data effect")
	ErrProviderLogicalEffectInvalid        = errors.New("provider logical effect is invalid")
)

const (
	maxProviderPrivacyJSONBytes = 4 << 20
	maxProviderProjectionPasses = 8
	maxProviderSchemaDepth      = 128
	maxProviderSchemaNodes      = 200_000
	fundsAccountFlowToolNameV1  = "mcp__analytix_funds__analyze_account_flows"
)

// boundAccountFlowProviderSemanticV1 is an unforgeable-by-deserialization,
// process-local provenance value. Only BindAccountFlowProviderSemanticV1 can
// construct the concrete type accepted by the final privacy boundary.
type boundAccountFlowProviderSemanticV1 struct {
	purpose         string
	contextDigest   string
	toolName        string
	toolCallID      string
	canonicalSHA256 string
}

// boundAccountFlowSafeHistoryV1 carries the existing account-flow provenance
// sidecar onto the exact user-role envelope produced when provider-private
// protocol bytes are unavailable. Its private concrete type cannot be created
// by history deserialization.
type boundAccountFlowSafeHistoryV1 struct {
	contextDigest       string
	messageSHA256       string
	sourceCanonicalHash string
	sourceOriginKind    validatedProviderSemanticKindV1
	currentAttempt      *providerCurrentAttemptUseV1
}

type boundCaseForegroundProviderSemanticV1 struct {
	contextDigest   string
	toolCallID      string
	canonicalSHA256 string
	attempt         *providerCurrentAttemptUseV1
}

type providerCurrentAttemptUseV1 struct{ consumed atomic.Bool }

type validatedAccountFlowProviderSemanticV1 struct {
	canonical           []byte
	digest              string
	references          []domaincaseentity.ReferenceV1
	kind                validatedProviderSemanticKindV1
	sourceOriginKind    validatedProviderSemanticKindV1
	sourceCanonicalHash string
	currentAttempt      *providerCurrentAttemptUseV1
}

type validatedProviderSemanticKindV1 uint8

const (
	validatedProviderSemanticAccountFlowV1 validatedProviderSemanticKindV1 = iota + 1
	validatedProviderSemanticSafeHistoryV1
	validatedProviderSemanticCaseForegroundV1
	validatedProviderSemanticHostChildPromptV1
)

// BindAccountFlowProviderSemanticV1 binds one exact, host-validated funds
// model output to the matching process-local tool-result message. Non-funds
// exact outputs are not applicable. An applicable but malformed output fails
// closed instead of receiving a generic privacy exception.
func BindAccountFlowProviderSemanticV1(
	context domainsecurity.TurnSecurityContext,
	call domainmodel.ToolCall,
	exactPrivateModelOutput any,
	message *domainmodel.Message,
) (bool, error) {
	if strings.TrimSpace(call.Name) != fundsAccountFlowToolNameV1 {
		return false, nil
	}
	if message == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!domainmodel.IsHostToolCallIDV1(call.ID) ||
		message.Role != "tool" || message.Name != call.Name ||
		message.ToolCallID != call.ID || len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
		message.PrivateProviderSemanticBinding != nil {
		return true, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(
		exactPrivateModelOutput,
	)
	if err != nil || !bytesEqualStringV1(canonical, message.Content) {
		return true, ErrProviderPrivacyAuthorityUnavailable
	}
	digest := domainsecurity.SHA256Hex(canonical)
	if !domainsecurity.IsSHA256Hex(digest) {
		return true, ErrProviderPrivacyAuthorityUnavailable
	}
	message.PrivateProviderSemanticBinding = boundAccountFlowProviderSemanticV1{
		purpose:         domainnative.AccountFlowProviderModelPurposeV1,
		contextDigest:   context.ContextDigest,
		toolName:        call.Name,
		toolCallID:      call.ID,
		canonicalSHA256: digest,
	}
	return true, nil
}

// AccountFlowProviderSemanticsForCaseDelegationV1 opens only exact
// process-local account-flow semantic bindings from the current parent turn.
// The returned values remain provider-safe but are not evidence authority;
// callers must immediately narrow them to the row-free delegated slot type.
func AccountFlowProviderSemanticsForCaseDelegationV1(
	context domainsecurity.TurnSecurityContext,
	messages []domainmodel.Message,
) ([]domainnative.AccountFlowProviderModelOutputV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	outputs := make([]domainnative.AccountFlowProviderModelOutputV1, 0, 1)
	seen := make(map[string]struct{})
	for _, message := range messages {
		if message.PrivateProviderReferenceBinding != nil {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		var validated validatedAccountFlowProviderSemanticV1
		var err error
		switch message.PrivateProviderSemanticBinding.(type) {
		case nil:
			continue
		case boundAccountFlowProviderSemanticV1:
			validated, err = validateBoundAccountFlowProviderSemanticV1(context, message, true)
		case boundAccountFlowSafeHistoryV1:
			validated, err = validateBoundAccountFlowSafeHistoryV1(context, message)
			if err == nil && validated.sourceOriginKind != validatedProviderSemanticAccountFlowV1 {
				continue
			}
		case boundProviderCaseAliasesV1:
			_, err = validateBoundProviderCaseAliasesV1(context, message)
			if err == nil {
				continue
			}
		default:
			err = ErrProviderPrivacyAuthorityUnavailable
		}
		if err != nil || len(validated.references) != 0 {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		accountFlowContent := validated.canonical
		expectedCanonicalHash := validated.digest
		if _, safeHistory := message.PrivateProviderSemanticBinding.(boundAccountFlowSafeHistoryV1); safeHistory {
			accountFlowText, opened := appmodel.CompletedPrivateProtocolSafeHistoryContentV1(
				message.Content, fundsAccountFlowToolNameV1,
			)
			if !opened || domainsecurity.SHA256Hex([]byte(accountFlowText)) != validated.sourceCanonicalHash {
				return nil, ErrProviderPrivacyAuthorityUnavailable
			}
			accountFlowContent = []byte(accountFlowText)
			expectedCanonicalHash = validated.sourceCanonicalHash
		}
		canonical, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(json.RawMessage(accountFlowContent))
		if err != nil || domainsecurity.SHA256Hex(canonical) != expectedCanonicalHash {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		var output domainnative.AccountFlowProviderModelOutputV1
		if json.Unmarshal(canonical, &output) != nil {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		key := output.Data.QueryHash + "\x00" + output.Data.ResultHash
		if _, duplicate := seen[key]; duplicate {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		seen[key] = struct{}{}
		outputs = append(outputs, output)
		if len(outputs) > 8 {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
	}
	return outputs, nil
}

// BindCaseForegroundProviderSemanticV1 binds the host-private typed handoff
// projection to one exact parent task result message. Public/durable task
// output omits caseResult, so deserialization cannot recreate this authority.
func BindCaseForegroundProviderSemanticV1(
	context domainsecurity.TurnSecurityContext,
	call domainmodel.ToolCall,
	exactPrivateModelOutput any,
	message *domainmodel.Message,
) (bool, error) {
	if strings.TrimSpace(call.Name) != toolcatalogapp.ForegroundTaskToolName {
		return false, nil
	}
	record, ok := exactPrivateModelOutput.(map[string]any)
	if !ok || record["typedResultAccepted"] != true {
		return false, nil
	}
	if message == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!domainmodel.IsHostToolCallIDV1(call.ID) || message.Role != "tool" || message.Name != call.Name ||
		message.ToolCallID != call.ID || len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
		message.PrivateProviderSemanticBinding != nil {
		return true, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := canonicalCaseForegroundProviderOutputV1(record)
	if err != nil || !bytesEqualStringV1(canonical, message.Content) {
		return true, ErrProviderPrivacyAuthorityUnavailable
	}
	digest := domainsecurity.SHA256Hex(canonical)
	message.PrivateProviderSemanticBinding = boundCaseForegroundProviderSemanticV1{
		contextDigest: context.ContextDigest, toolCallID: call.ID, canonicalSHA256: digest,
		attempt: &providerCurrentAttemptUseV1{},
	}
	return true, nil
}

func canonicalCaseForegroundProviderOutputV1(value any) ([]byte, error) {
	record, ok := value.(map[string]any)
	if !ok || len(record) != 17 {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	allowed := map[string]bool{
		"kind": true, "childRunId": true, "jobId": true, "status": true,
		"handoffReceiptDigest": true, "submissionDigest": true, "privacyProjectionDigest": true,
		"childCompletionReceiptDigest": true, "typedResultAccepted": true, "finalGateRequired": true,
		"answerSlotCount": true, "gapCount": true, "caseResult": true,
		"factAnswerAllowed": true, "evidenceAuthority": true,
		"parentGoalCompletionAllowed": true, "parentTodoCompletionAllowed": true,
	}
	for key := range record {
		if !allowed[key] {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
	}
	kind, kindOK := record["kind"].(string)
	status, statusOK := record["status"].(string)
	childRunID, childRunOK := record["childRunId"].(string)
	jobID, jobOK := record["jobId"].(string)
	if !kindOK || !statusOK || !childRunOK || !jobOK || kind != "subagent_task" || status != "completed" ||
		childRunID == "" || jobID != childRunID ||
		record["typedResultAccepted"] != true || record["finalGateRequired"] != true ||
		record["factAnswerAllowed"] != false || record["evidenceAuthority"] != false ||
		record["parentGoalCompletionAllowed"] != false || record["parentTodoCompletionAllowed"] != false {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	for _, key := range []string{"handoffReceiptDigest", "submissionDigest", "privacyProjectionDigest", "childCompletionReceiptDigest"} {
		text, textOK := record[key].(string)
		if !textOK || !domainsecurity.IsSHA256Hex(text) {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
	}
	result, resultErr := domainjob.ParseCaseForegroundChildResultShapeV1(record["caseResult"])
	answerSlotCount, answerSlotCountOK := exactProviderProjectionIntegerV1(record["answerSlotCount"])
	gapCount, gapCountOK := exactProviderProjectionIntegerV1(record["gapCount"])
	if resultErr != nil || !answerSlotCountOK || !gapCountOK ||
		answerSlotCount != len(result.AnswerSlots) || gapCount != len(result.Gaps) {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	normalized := make(map[string]any, len(record))
	for key, entry := range record {
		normalized[key] = entry
	}
	normalized["caseResult"] = result
	normalized["answerSlotCount"] = answerSlotCount
	normalized["gapCount"] = gapCount
	canonical, err := json.Marshal(normalized)
	if err != nil || domaincaseentity.ContainsReferenceCandidateV1(string(canonical)) {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	return canonical, nil
}

func canonicalCaseForegroundPublicOutputFromPrivateV1(content string) ([]byte, error) {
	var record map[string]any
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	if decoder.Decode(&record) != nil || !jsonDecoderAtEOFV1(decoder) {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	canonicalPrivate, err := canonicalCaseForegroundProviderOutputV1(record)
	if err != nil || !bytesEqualStringV1(canonicalPrivate, content) {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	delete(record, "caseResult")
	canonicalPublic, err := json.Marshal(record)
	if err != nil || len(record) != 16 || strings.Contains(string(canonicalPublic), `"caseResult"`) {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	return canonicalPublic, nil
}

func exactProviderProjectionIntegerV1(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed >= 0
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil && parsed >= 0 && int64(int(parsed)) == parsed
	case float64:
		parsed := int(typed)
		return parsed, parsed >= 0 && float64(parsed) == typed
	default:
		return 0, false
	}
}

func bytesEqualStringV1(value []byte, text string) bool {
	return len(value) == len(text) && string(value) == text
}

func ProjectOrdinaryText(text string) string {
	current := text
	for pass := 0; pass < maxProviderProjectionPasses; pass++ {
		next := domainprivacy.ProjectText(domainprivacy.ProjectPrivateSourceText(domainsecret.ProjectTextV1(current))).Text
		if next == current {
			if validateOrdinaryText(current) == nil {
				return current
			}
			return domainsecret.RedactedV1
		}
		current = next
	}
	return domainsecret.RedactedV1
}

type providerCaseReferenceSpanV1 struct {
	start int
	end   int
}

// projectProviderTextForEffectV1 preserves only canonical, case-scoped entity
// references present in the already-validated host provenance allowset.
// Syntax alone is never sufficient. Each non-reference or unbound segment is
// projected independently, so an adjacent account label cannot cause either a
// safe reference or an injected lookalike to escape the privacy boundary.
func projectProviderTextForEffectV1(
	text string,
	allowed providerCaseReferenceAllowsetV1,
) string {
	if len(allowed) == 0 {
		masked, _ := domaincaseentity.MaskReferenceCandidatesV1(text)
		return ProjectOrdinaryText(masked)
	}
	spans := providerCaseReferenceSpansV1(text)
	if len(spans) == 0 {
		masked, _ := domaincaseentity.MaskReferenceCandidatesV1(text)
		return ProjectOrdinaryText(masked)
	}
	var projected strings.Builder
	projected.Grow(len(text))
	cursor := 0
	for _, span := range spans {
		gap, _ := domaincaseentity.MaskReferenceCandidatesV1(text[cursor:span.start])
		projected.WriteString(ProjectOrdinaryText(gap))
		reference := domaincaseentity.ReferenceV1(text[span.start:span.end])
		if _, ok := allowed[reference]; ok {
			projected.WriteString(string(reference))
		} else {
			masked, _ := domaincaseentity.MaskReferenceCandidatesV1(string(reference))
			projected.WriteString(ProjectOrdinaryText(masked))
		}
		cursor = span.end
	}
	gap, _ := domaincaseentity.MaskReferenceCandidatesV1(text[cursor:])
	projected.WriteString(ProjectOrdinaryText(gap))
	return projected.String()
}

func projectProviderTextWithModelAliasesV1(
	text string,
	aliases []domaincaseentity.ModelEntityAliasV1,
) (string, error) {
	canonical, err := canonicalProviderCaseAliasesV1(aliases)
	if err != nil {
		return "", err
	}
	allowed := make(map[domaincaseentity.ModelEntityAliasV1]struct{}, len(canonical))
	for _, alias := range canonical {
		allowed[alias] = struct{}{}
	}
	seen := make(map[domaincaseentity.ModelEntityAliasV1]bool, len(canonical))
	spans := providerModelAliasSpansV1(text)
	var projected strings.Builder
	cursor := 0
	for _, span := range spans {
		nextAlias := domaincaseentity.ModelEntityAliasV1(text[span.start:span.end])
		if _, ok := allowed[nextAlias]; !ok {
			return "", ErrProviderPrivacyAuthorityUnavailable
		}
		projected.WriteString(projectProviderAliasGapV1(
			text[cursor:span.start], providerCaseReferenceAllowsetV1{},
		))
		projected.WriteString(string(nextAlias))
		seen[nextAlias] = true
		cursor = span.end
	}
	if len(text) == 0 || len(spans) == 0 {
		return "", ErrProviderPrivacyAuthorityUnavailable
	}
	projected.WriteString(projectProviderAliasGapV1(text[cursor:], providerCaseReferenceAllowsetV1{}))
	for _, alias := range canonical {
		if !seen[alias] {
			return "", ErrProviderPrivacyAuthorityUnavailable
		}
	}
	result := projected.String()
	if domaincaseentity.ContainsReferenceCandidateV1(result) {
		return "", ErrProviderPrivacyAuthorityUnavailable
	}
	return result, nil
}

type providerModelAliasSpanV1 struct {
	start int
	end   int
}

func providerModelAliasSpansV1(text string) []providerModelAliasSpanV1 {
	spans := make([]providerModelAliasSpanV1, 0, 4)
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		if domaincaseentity.ValidateModelEntityAliasV1(text[start:end]) == nil {
			spans = append(spans, providerModelAliasSpanV1{start: start, end: end})
		}
		start = -1
	}
	for index := 0; index < len(text); index++ {
		current := text[index]
		aliasToken := current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' ||
			current >= '0' && current <= '9' || current == ':' || current == '_' || current == '-'
		if aliasToken {
			if start < 0 {
				start = index
			}
			continue
		}
		flush(index)
	}
	flush(len(text))
	return spans
}

func projectProviderAliasGapV1(text string, allowed providerCaseReferenceAllowsetV1) string {
	leftTrimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	leading := len(text) - len(leftTrimmed)
	core := strings.TrimRightFunc(leftTrimmed, unicode.IsSpace)
	trailing := len(leftTrimmed) - len(core)
	if core == "" {
		return text
	}
	projected := projectProviderTextForEffectV1(core, allowed)
	return text[:leading] + projected + text[len(text)-trailing:]
}

func providerCaseReferenceSpansV1(text string) []providerCaseReferenceSpanV1 {
	const referenceLength = len(domaincaseentity.ReferencePrefixV1) + 64
	spans := make([]providerCaseReferenceSpanV1, 0, 2)
	for searchAt := 0; searchAt < len(text); {
		offset := strings.Index(text[searchAt:], domaincaseentity.ReferencePrefixV1)
		if offset < 0 {
			break
		}
		start := searchAt + offset
		end := start + referenceLength
		if end <= len(text) &&
			domaincaseentity.ValidateReferenceV1(text[start:end]) == nil &&
			providerCaseReferenceHasPrefixBoundaryV1(text, start) &&
			providerCaseReferenceHasSuffixBoundaryV1(text, end) {
			spans = append(spans, providerCaseReferenceSpanV1{start: start, end: end})
			searchAt = end
			continue
		}
		searchAt = start + len(domaincaseentity.ReferencePrefixV1)
	}
	return spans
}

func providerCaseReferenceHasPrefixBoundaryV1(text string, start int) bool {
	if start == 0 {
		return true
	}
	if start < 0 || start > len(text) {
		return false
	}
	previous, _ := utf8.DecodeLastRuneInString(text[:start])
	return previous != utf8.RuneError && previous != '_' &&
		!unicode.IsLetter(previous) && !unicode.IsDigit(previous) &&
		!unicode.IsMark(previous) && !unicode.Is(unicode.Pc, previous)
}

func providerCaseReferenceHasSuffixBoundaryV1(text string, end int) bool {
	if end == len(text) {
		return true
	}
	if end < 0 || end > len(text) {
		return false
	}
	next, _ := utf8.DecodeRuneInString(text[end:])
	return next != utf8.RuneError && next != '_' &&
		!unicode.IsLetter(next) && !unicode.IsDigit(next) &&
		!unicode.IsMark(next) && !unicode.Is(unicode.Pc, next)
}

func validateProviderCaseTextV1(
	text string,
	allowed providerCaseReferenceAllowsetV1,
) error {
	if err := domainsecret.ValidateValueV1(text); err != nil {
		return err
	}
	spans := providerCaseReferenceSpansV1(text)
	if len(spans) == 0 {
		if domaincaseentity.ContainsReferenceCandidateV1(text) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		return validateOrdinaryText(text)
	}
	cursor := 0
	for _, span := range spans {
		gap := text[cursor:span.start]
		reference := domaincaseentity.ReferenceV1(text[span.start:span.end])
		if domaincaseentity.ContainsReferenceCandidateV1(gap) ||
			validateOrdinaryText(gap) != nil {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		if _, ok := allowed[reference]; !ok {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		cursor = span.end
	}
	gap := text[cursor:]
	if domaincaseentity.ContainsReferenceCandidateV1(gap) ||
		validateOrdinaryText(gap) != nil {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	return nil
}

func ProjectOrdinaryValue(value any) (any, bool) {
	current := value
	changed := false
	for pass := 0; pass < maxProviderProjectionPasses; pass++ {
		credentialSafe := domainsecret.ProjectValueV1(current)
		credentialSafe = maskProviderReferenceCandidatesV1(
			credentialSafe,
			nil,
		)
		next, privacyChanged := domainprivacy.ProjectPublicValue(credentialSafe)
		if reflect.DeepEqual(next, current) {
			if domainsecret.ValidateValueV1(current) == nil && !privacyChanged {
				return current, changed
			}
			return domainsecret.RedactedV1, true
		}
		changed = true
		current = next
	}
	return domainsecret.RedactedV1, true
}

// ProjectProviderRequest is the mandatory strict case-model context projection. It
// runs after attempt-local attachment materialization and before any provider
// adapter receives the request. Binary/image parts are fail-closed because a
// text projection cannot prove that their pixels or bytes exclude full PII.
func ProjectProviderRequest(context domainsecurity.TurnSecurityContext, request domainmodel.Request) (domainmodel.Request, error) {
	return ProjectProviderRequestForEffect(context, request, false)
}

// ProjectProviderRequestForEffect binds provider serialization to the exact
// host-classified effect. Every ordinary attempt, whether the current context
// is general or case-sensitive, keeps the full credential/PII projection but
// may not carry a protected tool call or schema. This lets ordinary work
// continue without silently downgrading a case-data access into an ordinary
// provider request.
func ProjectProviderRequestForEffect(
	context domainsecurity.TurnSecurityContext,
	request domainmodel.Request,
	ordinaryEffect bool,
) (domainmodel.Request, error) {
	contextAbsent := reflect.DeepEqual(context, domainsecurity.TurnSecurityContext{})
	caseSensitive := domainsecurity.TurnSecurityContextIsCaseSensitive(context)
	if caseSensitive && !ordinaryEffect {
		if domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil || !domainsecurity.TurnSecurityContextAllowsCaseEvidence(context) {
			return domainmodel.Request{}, ErrProviderPrivacyAuthorityUnavailable
		}
	} else {
		if !contextAbsent && domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
			return domainmodel.Request{}, ErrProviderPrivacyAuthorityUnavailable
		}
		if !contextAbsent && !caseSensitive && !ordinaryEffect {
			return domainmodel.Request{}, ErrProviderPrivacyAuthorityUnavailable
		}
		if err := rejectCaseDataEffectsFromOrdinaryProvider(request); err != nil {
			return domainmodel.Request{}, err
		}
	}

	projected := request
	preserveCaseEntityReferences := caseSensitive && !ordinaryEffect
	allowedCaseEntityReferences, validatedSemantics, provenanceErr :=
		collectProviderCaseReferenceProvenanceV1(
			context,
			request,
			preserveCaseEntityReferences,
		)
	if provenanceErr != nil {
		return domainmodel.Request{}, provenanceErr
	}
	projected.PrivateProviderReferenceBinding = nil
	if containsUninspectableEncodedMediaEnvelope(request.SystemPrompt) {
		return domainmodel.Request{}, ErrUninspectableProviderPart
	}
	projected.SystemPrompt = projectProviderTextForEffectV1(
		request.SystemPrompt,
		allowedCaseEntityReferences,
	)
	projected.Messages = cloneMessages(request.Messages)
	for messageIndex := range projected.Messages {
		message := &projected.Messages[messageIndex]
		if containsUninspectableEncodedMediaEnvelope(message.Content) {
			return domainmodel.Request{}, ErrUninspectableProviderPart
		}
		if request.Messages[messageIndex].PrivateProviderSemanticBinding != nil {
			validated, ok := validatedSemantics[messageIndex]
			if !ok {
				return domainmodel.Request{}, ErrProviderPrivacyAuthorityUnavailable
			}
			message.Content = string(validated.canonical)
			message.PrivateProviderSemanticBinding = nil
		} else {
			if message.Role == "tool" && len(message.Content) <= maxProviderPrivacyJSONBytes && json.Valid([]byte(message.Content)) {
				content, _, err := projectProviderJSONValueForEffectV1(json.RawMessage(message.Content), false, allowedCaseEntityReferences)
				if err != nil {
					return domainmodel.Request{}, fmt.Errorf("%w: tool result content", ErrProviderPrivacyAuthorityUnavailable)
				}
				message.Content = projectProviderTextForEffectV1(string(content), allowedCaseEntityReferences)
			} else {
				message.Content = projectProviderTextForEffectV1(message.Content, allowedCaseEntityReferences)
			}
		}
		message.PrivateProviderReferenceBinding = nil
		for partIndex := range message.Parts {
			part := &message.Parts[partIndex]
			if strings.TrimSpace(part.ImageURL) != "" || strings.TrimSpace(part.Data) != "" {
				return domainmodel.Request{}, ErrUninspectableProviderPart
			}
			if containsUninspectableEncodedMediaEnvelope(part.Text) {
				return domainmodel.Request{}, ErrUninspectableProviderPart
			}
			part.Text = projectProviderTextForEffectV1(
				part.Text,
				allowedCaseEntityReferences,
			)
		}
		for callIndex := range message.ToolCalls {
			if providerIdentifierRequiresProjection(message.ToolCalls[callIndex].Name) {
				return domainmodel.Request{}, fmt.Errorf("%w: tool name", ErrProviderPrivacyAuthorityUnavailable)
			}
			arguments, _, err := projectProviderJSONForEffectV1(
				message.ToolCalls[callIndex].Arguments,
				allowedCaseEntityReferences,
			)
			if err != nil {
				return domainmodel.Request{}, fmt.Errorf("%w: tool arguments", ErrProviderPrivacyAuthorityUnavailable)
			}
			message.ToolCalls[callIndex].Arguments = arguments
		}
	}

	projected.Tools = cloneToolSchemas(request.Tools)
	for _, schema := range projected.Tools {
		if providerIdentifierRequiresProjection(schema.Name) || providerIdentifierRequiresProjection(schema.Description) {
			return domainmodel.Request{}, ErrProviderSchemaContainsPII
		}
		for _, item := range []struct {
			label string
			raw   json.RawMessage
		}{
			{label: "parameters", raw: schema.Parameters},
			{label: "output schema", raw: schema.OutputSchema},
		} {
			label, raw := item.label, item.raw
			if len(raw) == 0 {
				continue
			}
			changed, err := projectProviderSchemaJSON(raw)
			if err != nil {
				return domainmodel.Request{}, fmt.Errorf("%w: invalid %s", ErrProviderSchemaContainsPII, label)
			}
			if changed {
				return domainmodel.Request{}, fmt.Errorf("%w: %s", ErrProviderSchemaContainsPII, label)
			}
		}
	}
	if err := validateProviderRequestWithSemanticsV1(
		projected,
		allowedCaseEntityReferences,
		validatedSemantics,
	); err != nil {
		return domainmodel.Request{}, err
	}
	if err := consumeProviderCurrentAttemptSemanticsV1(request, validatedSemantics); err != nil {
		return domainmodel.Request{}, err
	}
	return projected, nil
}

func containsUninspectableEncodedMediaEnvelope(text string) bool {
	if !strings.Contains(text, "```base64") {
		return false
	}
	return strings.Contains(text, "[Attached file]") ||
		strings.Contains(text, "Image Reference Map:") ||
		strings.Contains(text, "The model may not support image input")
}

func consumeProviderCurrentAttemptSemanticsV1(
	request domainmodel.Request,
	validated map[int]validatedAccountFlowProviderSemanticV1,
) error {
	var attempt *providerCurrentAttemptUseV1
	for index, semantic := range validated {
		var current *providerCurrentAttemptUseV1
		if binding, ok := request.Messages[index].PrivateProviderSemanticBinding.(boundAccountFlowSafeHistoryV1); ok {
			current = binding.currentAttempt
			if current == nil {
				continue
			}
		} else {
			switch semantic.kind {
			case validatedProviderSemanticCaseForegroundV1:
				binding, ok := request.Messages[index].PrivateProviderSemanticBinding.(boundCaseForegroundProviderSemanticV1)
				if !ok {
					return ErrProviderPrivacyAuthorityUnavailable
				}
				current = binding.attempt
			case validatedProviderSemanticHostChildPromptV1:
				binding, ok := request.Messages[index].PrivateProviderSemanticBinding.(boundHostChildCasePromptV1)
				if !ok {
					return ErrProviderPrivacyAuthorityUnavailable
				}
				current = binding.attempt
			default:
				continue
			}
		}
		if current == nil || attempt != nil {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		attempt = current
	}
	if attempt != nil && !attempt.consumed.CompareAndSwap(false, true) {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	return nil
}

func rejectCaseDataEffectsFromOrdinaryProvider(request domainmodel.Request) error {
	for _, message := range request.Messages {
		for _, call := range message.ToolCalls {
			if executiongrantapp.CallUsesCaseDataAuthority(call) {
				return ErrOrdinaryProviderContainsCaseEffect
			}
		}
	}
	for _, schema := range request.Tools {
		if executiongrantapp.CallUsesCaseDataAuthority(domainmodel.ToolCall{
			Name: schema.Name, Arguments: json.RawMessage(`{}`),
		}) {
			return ErrOrdinaryProviderContainsCaseEffect
		}
	}
	return nil
}

func OrdinaryOnlyMCPAdvertisementsV1(
	advertisements []domainmcp.ToolAdvertisementV1,
) []domainmcp.ToolAdvertisementV1 {
	out := make([]domainmcp.ToolAdvertisementV1, 0, len(advertisements))
	for _, advertisement := range advertisements {
		if executiongrantapp.CallUsesCaseDataAuthority(domainmodel.ToolCall{
			Name: advertisement.Name, Arguments: json.RawMessage(`{}`),
		}) {
			continue
		}
		out = append(out, advertisement)
	}
	return out
}

func OrdinaryOnlyProviderToolSchemasV1(schemas []domainmodel.ToolSchema) []domainmodel.ToolSchema {
	out := make([]domainmodel.ToolSchema, 0, len(schemas))
	for _, schema := range schemas {
		if executiongrantapp.CallUsesCaseDataAuthority(domainmodel.ToolCall{
			Name: schema.Name, Arguments: json.RawMessage(`{}`),
		}) {
			continue
		}
		out = append(out, schema)
	}
	return out
}

func ProviderCatalogForEffectV1(
	caseDataEffect bool,
	advertisements []domainmcp.ToolAdvertisementV1,
	schemas []domainmodel.ToolSchema,
) ([]domainmcp.ToolAdvertisementV1, []domainmodel.ToolSchema) {
	if caseDataEffect {
		return advertisements, schemas
	}
	return OrdinaryOnlyMCPAdvertisementsV1(advertisements), OrdinaryOnlyProviderToolSchemasV1(schemas)
}

// ProviderCatalogForLogicalEffectV1 projects the permanent Agent catalog at
// the concrete data boundary for one provider step. A generic case-data step
// must not inherit funds tools merely because both effects require case
// authority; only an exact funds-data step may advertise them.
func ProviderCatalogForLogicalEffectV1(
	effect domainsecurity.LogicalEffect,
	advertisements []domainmcp.ToolAdvertisementV1,
	schemas []domainmodel.ToolSchema,
) ([]domainmcp.ToolAdvertisementV1, []domainmodel.ToolSchema, error) {
	if err := domainsecurity.ValidateLogicalEffect(effect); err != nil {
		return nil, nil, ErrProviderLogicalEffectInvalid
	}
	switch effect {
	case domainsecurity.LogicalEffectOrdinary:
		ordinaryAdvertisements, ordinarySchemas := ProviderCatalogForEffectV1(false, advertisements, schemas)
		return ordinaryAdvertisements, ordinarySchemas, nil
	case domainsecurity.LogicalEffectCaseData:
		caseAdvertisements := make([]domainmcp.ToolAdvertisementV1, 0, len(advertisements))
		for _, advertisement := range advertisements {
			if !toolcatalogapp.MCPToolNeedsAnalytixCaseContext(advertisement.Name) {
				caseAdvertisements = append(caseAdvertisements, advertisement)
			}
		}
		caseSchemas := make([]domainmodel.ToolSchema, 0, len(schemas))
		for _, schema := range schemas {
			if !toolcatalogapp.MCPToolNeedsAnalytixCaseContext(schema.Name) {
				caseSchemas = append(caseSchemas, schema)
			}
		}
		return caseAdvertisements, caseSchemas, nil
	case domainsecurity.LogicalEffectFundsData:
		return advertisements, schemas, nil
	default:
		return nil, nil, ErrProviderLogicalEffectInvalid
	}
}

// OrdinaryOnlyProviderMessagesV1 removes protected tool-call/result pairs
// from provider history while preserving the rest of the same thread. Host
// evidence remains durable; an ordinary attempt receives no protected effect.
func OrdinaryOnlyProviderMessagesV1(messages []domainmodel.Message) []domainmodel.Message {
	cloned := appmodel.CloneProviderMessages(messages)
	protectedCallIDs := make(map[string]bool)
	for _, message := range cloned {
		for _, call := range message.ToolCalls {
			if executiongrantapp.CallUsesCaseDataAuthority(call) {
				protectedCallIDs[call.ID] = true
			}
		}
	}
	filtered := make([]domainmodel.Message, 0, len(cloned))
	for _, message := range cloned {
		if message.Role == "tool" && protectedCallIDs[message.ToolCallID] {
			continue
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]domainmodel.ToolCall, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				if !protectedCallIDs[call.ID] {
					calls = append(calls, call)
				}
			}
			message.ToolCalls = calls
			if len(calls) == 0 && strings.TrimSpace(message.Content) == "" {
				continue
			}
		}
		filtered = append(filtered, message)
	}
	return appmodel.SanitizeToolPairing(filtered)
}

func ValidateProviderRequest(request domainmodel.Request) error {
	return validateProviderRequestForEffectV1(request, nil)
}

func validateProviderRequestForEffectV1(
	request domainmodel.Request,
	allowedCaseEntityReferences providerCaseReferenceAllowsetV1,
) error {
	return validateProviderRequestWithSemanticsV1(request, allowedCaseEntityReferences, nil)
}

func validateProviderRequestWithSemanticsV1(
	request domainmodel.Request,
	allowedCaseEntityReferences providerCaseReferenceAllowsetV1,
	validatedSemantics map[int]validatedAccountFlowProviderSemanticV1,
) error {
	if request.PrivateProviderReferenceBinding != nil {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	for _, text := range []string{request.SystemPrompt} {
		if containsUninspectableEncodedMediaEnvelope(text) {
			return ErrUninspectableProviderPart
		}
		if validateProviderTextForEffectV1(text, allowedCaseEntityReferences) != nil {
			return ErrProviderPrivacyAuthorityUnavailable
		}
	}
	for messageIndex, message := range request.Messages {
		if containsUninspectableEncodedMediaEnvelope(message.Content) {
			return ErrUninspectableProviderPart
		}
		if trusted, ok := validatedSemantics[messageIndex]; ok {
			if message.PrivateProviderSemanticBinding != nil ||
				validateFinalAccountFlowProviderSemanticV1(message, trusted) != nil {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		} else if message.PrivateProviderSemanticBinding != nil ||
			message.PrivateProviderReferenceBinding != nil ||
			validateProviderTextForEffectV1(message.Content, allowedCaseEntityReferences) != nil {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		for _, part := range message.Parts {
			if containsUninspectableEncodedMediaEnvelope(part.Text) {
				return ErrUninspectableProviderPart
			}
			if strings.TrimSpace(part.ImageURL) != "" || strings.TrimSpace(part.Data) != "" ||
				validateProviderTextForEffectV1(part.Text, allowedCaseEntityReferences) != nil {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		}
		for _, call := range message.ToolCalls {
			if providerIdentifierRequiresProjection(call.Name) {
				return ErrProviderPrivacyAuthorityUnavailable
			}
			_, changed, err := projectProviderJSONForEffectV1(
				call.Arguments,
				allowedCaseEntityReferences,
			)
			if err != nil || changed {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		}
	}
	for _, schema := range request.Tools {
		if providerIdentifierRequiresProjection(schema.Name) || providerIdentifierRequiresProjection(schema.Description) {
			return ErrProviderSchemaContainsPII
		}
		for _, raw := range []json.RawMessage{schema.Parameters, schema.OutputSchema} {
			if len(raw) == 0 {
				continue
			}
			changed, err := projectProviderSchemaJSON(raw)
			if err != nil || changed {
				return ErrProviderSchemaContainsPII
			}
		}
	}
	return nil
}

func validateBoundAccountFlowProviderSemanticV1(
	context domainsecurity.TurnSecurityContext,
	message domainmodel.Message,
	preserveCaseEntityReferences bool,
) (validatedAccountFlowProviderSemanticV1, error) {
	binding, ok := message.PrivateProviderSemanticBinding.(boundAccountFlowProviderSemanticV1)
	if !ok || !preserveCaseEntityReferences ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		binding.purpose != domainnative.AccountFlowProviderModelPurposeV1 ||
		binding.contextDigest != context.ContextDigest ||
		binding.toolName != fundsAccountFlowToolNameV1 ||
		binding.toolCallID == "" || !domainmodel.IsHostToolCallIDV1(binding.toolCallID) ||
		message.Role != "tool" || message.Name != binding.toolName ||
		message.ToolCallID != binding.toolCallID || len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
		!domainsecurity.IsSHA256Hex(binding.canonicalSHA256) {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(
		json.RawMessage(message.Content),
	)
	if err != nil || !bytesEqualStringV1(canonical, message.Content) ||
		domainsecurity.SHA256Hex(canonical) != binding.canonicalSHA256 {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	references, referenceErr := accountFlowProviderReferencesV1(canonical)
	if referenceErr != nil {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	return validatedAccountFlowProviderSemanticV1{
		canonical:  canonical,
		digest:     binding.canonicalSHA256,
		references: references,
		kind:       validatedProviderSemanticAccountFlowV1,
	}, nil
}

func validateBoundCaseForegroundProviderSemanticV1(
	context domainsecurity.TurnSecurityContext,
	message domainmodel.Message,
) (validatedAccountFlowProviderSemanticV1, error) {
	binding, ok := message.PrivateProviderSemanticBinding.(boundCaseForegroundProviderSemanticV1)
	if !ok || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		binding.contextDigest != context.ContextDigest || !domainmodel.IsHostToolCallIDV1(binding.toolCallID) ||
		!domainsecurity.IsSHA256Hex(binding.canonicalSHA256) || binding.attempt == nil || message.Role != "tool" ||
		message.Name != toolcatalogapp.ForegroundTaskToolName || message.ToolCallID != binding.toolCallID ||
		len(message.Parts) != 0 || len(message.ToolCalls) != 0 {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	var record map[string]any
	decoder := json.NewDecoder(strings.NewReader(message.Content))
	decoder.UseNumber()
	if decoder.Decode(&record) != nil || !jsonDecoderAtEOFV1(decoder) {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := canonicalCaseForegroundProviderOutputV1(record)
	if err != nil || !bytesEqualStringV1(canonical, message.Content) ||
		domainsecurity.SHA256Hex(canonical) != binding.canonicalSHA256 {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	return validatedAccountFlowProviderSemanticV1{
		canonical: canonical, digest: binding.canonicalSHA256, references: []domaincaseentity.ReferenceV1{},
		kind: validatedProviderSemanticCaseForegroundV1, currentAttempt: binding.attempt,
	}, nil
}

func validateFinalAccountFlowProviderSemanticV1(
	message domainmodel.Message,
	expected validatedAccountFlowProviderSemanticV1,
) error {
	switch expected.kind {
	case validatedProviderSemanticSafeHistoryV1:
		if message.Role != "user" || message.Name != "" || message.ToolCallID != "" ||
			len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
			!domainsecurity.IsSHA256Hex(expected.digest) ||
			!bytesEqualStringV1(expected.canonical, message.Content) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		return nil
	case validatedProviderSemanticAccountFlowV1:
		if message.Role != "tool" || message.Name != fundsAccountFlowToolNameV1 ||
			!domainmodel.IsHostToolCallIDV1(message.ToolCallID) ||
			len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
			!domainsecurity.IsSHA256Hex(expected.digest) ||
			!bytesEqualStringV1(expected.canonical, message.Content) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		canonical, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(
			json.RawMessage(message.Content),
		)
		if err != nil || !bytesEqualStringV1(canonical, message.Content) ||
			domainsecurity.SHA256Hex(canonical) != expected.digest {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		return nil
	case validatedProviderSemanticCaseForegroundV1:
		if message.Role != "tool" || message.Name != toolcatalogapp.ForegroundTaskToolName ||
			!domainmodel.IsHostToolCallIDV1(message.ToolCallID) || len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
			!domainsecurity.IsSHA256Hex(expected.digest) || !bytesEqualStringV1(expected.canonical, message.Content) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		var record map[string]any
		decoder := json.NewDecoder(strings.NewReader(message.Content))
		decoder.UseNumber()
		if decoder.Decode(&record) != nil || !jsonDecoderAtEOFV1(decoder) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		canonical, err := canonicalCaseForegroundProviderOutputV1(record)
		if err != nil || !bytesEqualStringV1(canonical, message.Content) ||
			domainsecurity.SHA256Hex(canonical) != expected.digest {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		return nil
	case validatedProviderSemanticHostChildPromptV1:
		if message.Role != "user" || message.Name != "" || message.ToolCallID != "" ||
			len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
			!domainsecurity.IsSHA256Hex(expected.digest) || !bytesEqualStringV1(expected.canonical, message.Content) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		return nil
	default:
		return ErrProviderPrivacyAuthorityUnavailable
	}
}

func jsonDecoderAtEOFV1(decoder *json.Decoder) bool {
	if decoder == nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func validateProviderTextForEffectV1(
	text string,
	allowedCaseEntityReferences providerCaseReferenceAllowsetV1,
) error {
	if len(allowedCaseEntityReferences) != 0 {
		return validateProviderCaseTextV1(text, allowedCaseEntityReferences)
	}
	if domaincaseentity.ContainsReferenceCandidateV1(text) {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	return validateOrdinaryText(text)
}

func projectProviderJSON(raw json.RawMessage) (json.RawMessage, bool, error) {
	return projectProviderJSONForEffectV1(raw, nil)
}

func projectProviderJSONForEffectV1(
	raw json.RawMessage,
	allowedCaseEntityReferences providerCaseReferenceAllowsetV1,
) (json.RawMessage, bool, error) {
	return projectProviderJSONValueForEffectV1(raw, true, allowedCaseEntityReferences)
}

func projectProviderJSONValueForEffectV1(
	raw json.RawMessage,
	requireObject bool,
	allowedCaseEntityReferences providerCaseReferenceAllowsetV1,
) (json.RawMessage, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	value, err := domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		RequireObject: requireObject, MaxBytes: maxProviderPrivacyJSONBytes, MaxDepth: 128,
		MaxTokens: 200000, MaxStringBytes: maxProviderPrivacyJSONBytes,
	})
	if err != nil {
		return nil, false, err
	}
	if err := validateProviderStructuralKeysV1(value, 0); err != nil {
		return nil, false, err
	}
	projected, changed, err := projectProviderValueForEffectV1(
		value,
		requireObject,
		allowedCaseEntityReferences,
	)
	if err != nil {
		return nil, false, err
	}
	body, err := json.Marshal(projected)
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(body), changed, nil
}

func validateProviderStructuralKeysV1(value any, depth int) error {
	if depth > maxProviderSchemaDepth {
		return errors.New("provider JSON structural keys exceed depth budget")
	}
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if providerStructuralKeyRequiresRejectionV1(key) {
				return errors.New("provider JSON structural key contains private content")
			}
			if err := validateProviderStructuralKeysV1(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := validateProviderStructuralKeysV1(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func providerStructuralKeyRequiresRejectionV1(key string) bool {
	if providerIdentifierRequiresProjection(key) ||
		domaincaseentity.ContainsReferenceCandidateV1(key) {
		return true
	}
	trimmed := strings.ToLower(strings.TrimSpace(key))
	if trimmed == "" {
		return false
	}
	if strings.ContainsAny(trimmed, `/\\`) || strings.Contains(trimmed, "://") ||
		strings.HasSuffix(trimmed, ".duckdb") || strings.HasSuffix(trimmed, ".db") ||
		strings.HasSuffix(trimmed, ".sqlite") || strings.HasSuffix(trimmed, ".csv") ||
		strings.HasSuffix(trimmed, ".parquet") {
		return true
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return false
	}
	switch fields[0] {
	case "select", "insert", "update", "delete", "drop", "alter", "create", "with", "pragma", "attach", "detach", "copy":
		return true
	default:
		return false
	}
}

func projectProviderValue(value any, requireObject bool) (any, bool, error) {
	return projectProviderValueForEffectV1(value, requireObject, nil)
}

func projectProviderValueForEffectV1(
	value any,
	requireObject bool,
	allowedCaseEntityReferences providerCaseReferenceAllowsetV1,
) (any, bool, error) {
	current := value
	changed := false
	for pass := 0; pass < maxProviderProjectionPasses; pass++ {
		credentialSafe := domainsecret.ProjectValueV1(current)
		credentialSafe = maskProviderReferenceCandidatesV1(
			credentialSafe,
			allowedCaseEntityReferences,
		)
		credentialSafe, _ = domainprivacy.ProjectProviderSourceValue(credentialSafe)
		if requireObject {
			if _, ok := credentialSafe.(map[string]any); !ok {
				return nil, false, errors.New("provider JSON projection lost its object shape")
			}
		}
		next, privacyChanged := domainprivacy.ProjectUntrustedValue(credentialSafe)
		if requireObject {
			if _, ok := next.(map[string]any); !ok {
				return nil, false, errors.New("provider JSON privacy projection lost its object shape")
			}
		}
		if len(allowedCaseEntityReferences) != 0 {
			var restoreErr error
			next, restoreErr = restoreProviderCaseReferenceValuesV1(
				credentialSafe,
				next,
				allowedCaseEntityReferences,
			)
			if restoreErr != nil {
				return nil, false, restoreErr
			}
		}
		if reflect.DeepEqual(next, current) {
			if domainsecret.ValidateValueV1(current) != nil ||
				(len(allowedCaseEntityReferences) == 0 && privacyChanged) {
				return nil, false, errors.New("provider JSON projection did not validate")
			}
			return current, changed, nil
		}
		changed = true
		current = next
	}
	return nil, false, errors.New("provider JSON projection did not converge")
}

func maskProviderReferenceCandidatesV1(
	value any,
	allowed providerCaseReferenceAllowsetV1,
) any {
	switch current := value.(type) {
	case string:
		if len(allowed) == 0 {
			masked, _ := domaincaseentity.MaskReferenceCandidatesV1(current)
			return masked
		}
		spans := providerCaseReferenceSpansV1(current)
		if len(spans) == 0 {
			masked, _ := domaincaseentity.MaskReferenceCandidatesV1(current)
			return masked
		}
		var masked strings.Builder
		masked.Grow(len(current))
		cursor := 0
		for _, span := range spans {
			gap, _ := domaincaseentity.MaskReferenceCandidatesV1(current[cursor:span.start])
			masked.WriteString(gap)
			reference := domaincaseentity.ReferenceV1(current[span.start:span.end])
			if _, ok := allowed[reference]; ok {
				masked.WriteString(string(reference))
			} else {
				projected, _ := domaincaseentity.MaskReferenceCandidatesV1(string(reference))
				masked.WriteString(projected)
			}
			cursor = span.end
		}
		gap, _ := domaincaseentity.MaskReferenceCandidatesV1(current[cursor:])
		masked.WriteString(gap)
		return masked.String()
	case map[string]any:
		next := make(map[string]any, len(current))
		for key, child := range current {
			next[key] = maskProviderReferenceCandidatesV1(child, allowed)
		}
		return next
	case []any:
		next := make([]any, len(current))
		for index, child := range current {
			next[index] = maskProviderReferenceCandidatesV1(child, allowed)
		}
		return next
	default:
		return current
	}
}

func restoreProviderCaseReferenceValuesV1(
	original any,
	projected any,
	allowed providerCaseReferenceAllowsetV1,
) (any, error) {
	switch current := original.(type) {
	case string:
		if !domaincaseentity.ContainsReferenceV1(current) {
			return projected, nil
		}
		projectedText := projectProviderTextForEffectV1(current, allowed)
		if validateProviderCaseTextV1(projectedText, allowed) != nil {
			return nil, errors.New("provider case reference projection is invalid")
		}
		return projectedText, nil
	case map[string]any:
		projectedMap, ok := projected.(map[string]any)
		if !ok || len(projectedMap) != len(current) {
			return nil, errors.New("provider case reference projection changed object shape")
		}
		next := make(map[string]any, len(projectedMap))
		for key, projectedChild := range projectedMap {
			next[key] = projectedChild
		}
		for key, child := range current {
			projectedChild, found := projectedMap[key]
			if !found {
				return nil, errors.New("provider case reference projection changed object keys")
			}
			restored, err := restoreProviderCaseReferenceValuesV1(child, projectedChild, allowed)
			if err != nil {
				return nil, err
			}
			next[key] = restored
		}
		return next, nil
	case []any:
		projectedList, ok := projected.([]any)
		if !ok || len(projectedList) != len(current) {
			return nil, errors.New("provider case reference projection changed list shape")
		}
		next := make([]any, len(current))
		for index := range current {
			restored, err := restoreProviderCaseReferenceValuesV1(current[index], projectedList[index], allowed)
			if err != nil {
				return nil, err
			}
			next[index] = restored
		}
		return next, nil
	default:
		return projected, nil
	}
}

type providerSchemaBudget struct {
	nodes     int
	textBytes int
}

func (budget *providerSchemaBudget) consumeNode() error {
	if budget.nodes >= maxProviderSchemaNodes {
		return errors.New("provider schema exceeds node budget")
	}
	budget.nodes++
	return nil
}

func (budget *providerSchemaBudget) consumeText(value string) error {
	if len(value) > maxProviderPrivacyJSONBytes || budget.textBytes > maxProviderPrivacyJSONBytes-len(value) {
		return errors.New("provider schema exceeds text budget")
	}
	budget.textBytes += len(value)
	return nil
}

// projectProviderSchemaJSON treats JSON Schema identifiers as identifiers,
// not as values of the cases they describe. Content-bearing annotations and
// examples still use the same credential plus PII fixed point as tool input.
// Any projection is reported to the caller, which rejects the schema instead
// of silently changing the contract advertised to the model.
func projectProviderSchemaJSON(raw json.RawMessage) (bool, error) {
	value, err := domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxProviderPrivacyJSONBytes, MaxDepth: maxProviderSchemaDepth,
		MaxTokens: maxProviderSchemaNodes, MaxStringBytes: maxProviderPrivacyJSONBytes,
	})
	if err != nil {
		return false, err
	}
	_, changed, err := projectProviderSchemaNode(value, &providerSchemaBudget{}, 0, "")
	return changed, err
}

func projectProviderSchemaNode(value any, budget *providerSchemaBudget, depth int, dataKey string) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	switch current := value.(type) {
	case bool:
		return current, false, nil
	case map[string]any:
		out := make(map[string]any, len(current))
		changed := false
		for key, child := range current {
			if err := budget.consumeText(key); err != nil {
				return nil, false, err
			}
			if providerIdentifierRequiresProjection(key) {
				changed = true
			}
			projected, childChanged, err := projectProviderSchemaKeyword(key, child, budget, depth+1, dataKey)
			if err != nil {
				return nil, false, err
			}
			out[key] = projected
			changed = changed || childChanged
		}
		return out, changed, nil
	default:
		return nil, false, errors.New("provider schema node must be an object or boolean")
	}
}

func projectProviderSchemaKeyword(key string, value any, budget *providerSchemaBudget, depth int, dataKey string) (any, bool, error) {
	switch key {
	case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas":
		return projectProviderSchemaMap(value, budget, depth)
	case "allOf", "anyOf", "oneOf", "prefixItems":
		return projectProviderSchemaArray(value, budget, depth, dataKey)
	case "items":
		if _, ok := value.([]any); ok {
			return projectProviderSchemaArray(value, budget, depth, dataKey)
		}
		return projectProviderSchemaNode(value, budget, depth, dataKey)
	case "additionalProperties", "unevaluatedProperties", "additionalItems", "unevaluatedItems", "contains", "not", "if", "then", "else", "propertyNames", "contentSchema":
		return projectProviderSchemaNode(value, budget, depth, dataKey)
	case "required":
		return projectProviderSchemaIdentifierArray(value, budget, depth)
	case "dependentRequired":
		return projectProviderDependentRequired(value, budget, depth)
	case "type":
		if _, ok := value.([]any); ok {
			return projectProviderSchemaIdentifierArray(value, budget, depth)
		}
		return projectProviderSchemaIdentifier(value, budget, depth)
	case "format", "$schema", "$id", "$ref", "$anchor", "$dynamicRef", "$dynamicAnchor", "contentEncoding", "contentMediaType":
		return projectProviderSchemaIdentifier(value, budget, depth)
	case "title", "description", "$comment":
		return projectProviderSchemaText(value, budget, depth)
	case "default", "examples", "example", "const":
		return projectProviderSchemaContent(value, budget, depth, dataKey)
	case "enum":
		return projectProviderSchemaContent(value, budget, depth, "")
	default:
		return projectProviderSchemaContent(value, budget, depth, dataKey)
	}
}

func projectProviderSchemaMap(value any, budget *providerSchemaBudget, depth int) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	values, ok := value.(map[string]any)
	if !ok {
		return nil, false, errors.New("provider schema map keyword must contain an object")
	}
	out := make(map[string]any, len(values))
	changed := false
	for key, child := range values {
		if err := budget.consumeText(key); err != nil {
			return nil, false, err
		}
		if providerIdentifierRequiresProjection(key) {
			changed = true
		}
		projected, childChanged, err := projectProviderSchemaNode(child, budget, depth+1, key)
		if err != nil {
			return nil, false, err
		}
		out[key] = projected
		changed = changed || childChanged
	}
	return out, changed, nil
}

func projectProviderSchemaArray(value any, budget *providerSchemaBudget, depth int, dataKey string) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	values, ok := value.([]any)
	if !ok {
		return nil, false, errors.New("provider schema array keyword must contain an array")
	}
	out := make([]any, len(values))
	changed := false
	for index, child := range values {
		projected, childChanged, err := projectProviderSchemaNode(child, budget, depth+1, dataKey)
		if err != nil {
			return nil, false, err
		}
		out[index] = projected
		changed = changed || childChanged
	}
	return out, changed, nil
}

func projectProviderSchemaIdentifier(value any, budget *providerSchemaBudget, depth int) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	text, ok := value.(string)
	if !ok {
		return nil, false, errors.New("provider schema identifier must be a string")
	}
	if err := budget.consumeText(text); err != nil {
		return nil, false, err
	}
	return text, providerIdentifierRequiresProjection(text), nil
}

func projectProviderSchemaIdentifierArray(value any, budget *providerSchemaBudget, depth int) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	values, ok := value.([]any)
	if !ok {
		return nil, false, errors.New("provider schema identifier list must be an array")
	}
	out := make([]any, len(values))
	changed := false
	for index, child := range values {
		projected, childChanged, err := projectProviderSchemaIdentifier(child, budget, depth+1)
		if err != nil {
			return nil, false, err
		}
		out[index] = projected
		changed = changed || childChanged
	}
	return out, changed, nil
}

func projectProviderDependentRequired(value any, budget *providerSchemaBudget, depth int) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	values, ok := value.(map[string]any)
	if !ok {
		return nil, false, errors.New("provider dependentRequired must be an object")
	}
	out := make(map[string]any, len(values))
	changed := false
	for key, child := range values {
		if err := budget.consumeText(key); err != nil {
			return nil, false, err
		}
		if providerIdentifierRequiresProjection(key) {
			changed = true
		}
		projected, childChanged, err := projectProviderSchemaIdentifierArray(child, budget, depth+1)
		if err != nil {
			return nil, false, err
		}
		out[key] = projected
		changed = changed || childChanged
	}
	return out, changed, nil
}

func projectProviderSchemaText(value any, budget *providerSchemaBudget, depth int) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	text, ok := value.(string)
	if !ok {
		return nil, false, errors.New("provider schema text keyword must be a string")
	}
	if err := budget.consumeText(text); err != nil {
		return nil, false, err
	}
	projected := ProjectOrdinaryText(text)
	return projected, projected != text, nil
}

func projectProviderSchemaContent(value any, budget *providerSchemaBudget, depth int, dataKey string) (any, bool, error) {
	if depth > maxProviderSchemaDepth {
		return nil, false, errors.New("provider schema exceeds depth budget")
	}
	if err := budget.consumeNode(); err != nil {
		return nil, false, err
	}
	if err := consumeProviderSchemaContentBudget(value, budget, depth); err != nil {
		return nil, false, err
	}
	if dataKey == "" {
		return projectProviderValue(value, false)
	}
	projected, changed, err := projectProviderValue(map[string]any{dataKey: value}, true)
	if err != nil {
		return nil, false, err
	}
	record, ok := projected.(map[string]any)
	if !ok {
		return nil, false, errors.New("provider schema content projection lost its object shape")
	}
	projectedValue, exists := record[dataKey]
	if !exists {
		return nil, false, errors.New("provider schema content projection lost its property")
	}
	return projectedValue, changed, nil
}

func consumeProviderSchemaContentBudget(value any, budget *providerSchemaBudget, depth int) error {
	if depth > maxProviderSchemaDepth {
		return errors.New("provider schema exceeds depth budget")
	}
	switch current := value.(type) {
	case string:
		return budget.consumeText(current)
	case map[string]any:
		for key, child := range current {
			if err := budget.consumeNode(); err != nil {
				return err
			}
			if err := budget.consumeText(key); err != nil {
				return err
			}
			if err := consumeProviderSchemaContentBudget(child, budget, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := budget.consumeNode(); err != nil {
				return err
			}
			if err := consumeProviderSchemaContentBudget(child, budget, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func providerIdentifierRequiresProjection(value string) bool {
	return domaincaseentity.ContainsReferenceCandidateV1(value) ||
		ProjectOrdinaryText(value) != value || validateOrdinaryText(value) != nil
}

func validateOrdinaryText(text string) error {
	if err := domainsecret.ValidateValueV1(text); err != nil {
		return err
	}
	if domainprivacy.ProjectPrivateSourceText(text) != text {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	return domainprivacy.ValidateOrdinaryText(text)
}

func cloneMessages(values []domainmodel.Message) []domainmodel.Message {
	if values == nil {
		return nil
	}
	out := make([]domainmodel.Message, len(values))
	for index, message := range values {
		out[index] = message
		out[index].Parts = append([]domainmodel.MessagePart(nil), message.Parts...)
		if message.ToolCalls != nil {
			out[index].ToolCalls = make([]domainmodel.ToolCall, len(message.ToolCalls))
			for callIndex, call := range message.ToolCalls {
				out[index].ToolCalls[callIndex] = call
				out[index].ToolCalls[callIndex].Arguments = append(json.RawMessage(nil), call.Arguments...)
			}
		}
	}
	return out
}

func cloneToolSchemas(values []domainmodel.ToolSchema) []domainmodel.ToolSchema {
	if values == nil {
		return nil
	}
	out := make([]domainmodel.ToolSchema, len(values))
	for index, schema := range values {
		out[index] = schema
		out[index].Parameters = append(json.RawMessage(nil), schema.Parameters...)
		out[index].OutputSchema = append(json.RawMessage(nil), schema.OutputSchema...)
	}
	return out
}
