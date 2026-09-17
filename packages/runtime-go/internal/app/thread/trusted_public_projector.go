package thread

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const (
	CasePublicAuthorityRejectedCode    = "case_public_authority_rejected"
	CasePublicAuthorityUnavailableCode = "case_public_authority_unavailable"
)

type PublicProjector interface {
	ProjectThread(map[string]any) (map[string]any, error)
	ProjectEvent(string, map[string]any, map[string]any) (map[string]any, bool, error)
}

type AcceptedFinalDeliveryAuthority interface {
	SealAcceptedFinalDelivery(context.Context, []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error)
	ValidateAcceptedFinalDelivery(context.Context, domainevent.AcceptedFinalDeliveryBatchV2) error
}

func NewAcceptedFinalDeliverySealer(
	ctx context.Context,
	projector PublicProjector,
) func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
	return func(events []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
		authority, ok := projector.(AcceptedFinalDeliveryAuthority)
		if !ok {
			return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery authority is unavailable")
		}
		return authority.SealAcceptedFinalDelivery(ctx, events)
	}
}

func NewAcceptedFinalDeliveryValidator(
	ctx context.Context,
	projector PublicProjector,
) func(domainevent.AcceptedFinalDeliveryBatchV2) error {
	return func(batch domainevent.AcceptedFinalDeliveryBatchV2) error {
		authority, ok := projector.(AcceptedFinalDeliveryAuthority)
		if !ok {
			return errors.New("accepted final delivery authority is unavailable")
		}
		return authority.ValidateAcceptedFinalDelivery(ctx, batch)
	}
}

type CaseThreadAuthority interface {
	IsCaseThread(string) bool
	ContainsContext(domainsecurity.TurnSecurityContext) bool
	RestartPreservesThreadV1(string) bool
}

// PreservedDerivedHistoryV1 only observes an exact inherited turn in a
// previously qualified whole-primary hold. It grants no public content or
// execution authority.
type PreservedDerivedHistoryV1 interface {
	ValidateInheritedTurnV1(context.Context, map[string]any, map[string]any) error
}

type TrustedPublicProjector struct {
	index            *gateprojection.TrustedFinalProjectionIndex
	caseThreads      CaseThreadAuthority
	currentAuthority CurrentCaseThreadAuthorityValidator
	primaryCAS       finalauthorityport.AcceptedFinalCASReader
	preservedHistory PreservedDerivedHistoryV1
}

func NewTrustedPublicProjector(index *gateprojection.TrustedFinalProjectionIndex) *TrustedPublicProjector {
	return NewTrustedPublicProjectorWithCaseThreads(index, nil)
}

func NewTrustedPublicProjectorWithCaseThreads(index *gateprojection.TrustedFinalProjectionIndex, caseThreads CaseThreadAuthority) *TrustedPublicProjector {
	return NewTrustedPublicProjectorWithCurrentCaseAuthority(index, caseThreads, nil)
}

func NewTrustedPublicProjectorWithCurrentCaseAuthority(index *gateprojection.TrustedFinalProjectionIndex, caseThreads CaseThreadAuthority, currentAuthority CurrentCaseThreadAuthorityValidator) *TrustedPublicProjector {
	return NewTrustedPublicProjectorWithPrimaryCAS(index, caseThreads, currentAuthority, nil)
}

// NewTrustedPublicProjectorWithPrimaryCAS binds public snapshot and SSE
// admission to the same strict, primary-only CAS reader used by restart
// recovery. The normalized thread remains a rendering view, never CAS
// authority.
func NewTrustedPublicProjectorWithPrimaryCAS(
	index *gateprojection.TrustedFinalProjectionIndex,
	caseThreads CaseThreadAuthority,
	currentAuthority CurrentCaseThreadAuthorityValidator,
	primaryCAS finalauthorityport.AcceptedFinalCASReader,
) *TrustedPublicProjector {
	return NewTrustedPublicProjectorWithPreservedHistoryV1(index, caseThreads, currentAuthority, primaryCAS, nil)
}

func NewTrustedPublicProjectorWithPreservedHistoryV1(
	index *gateprojection.TrustedFinalProjectionIndex,
	caseThreads CaseThreadAuthority,
	currentAuthority CurrentCaseThreadAuthorityValidator,
	primaryCAS finalauthorityport.AcceptedFinalCASReader,
	history PreservedDerivedHistoryV1,
) *TrustedPublicProjector {
	if index == nil {
		index = gateprojection.NewTrustedFinalProjectionIndex(nil)
	}
	if currentAuthority != nil {
		caseThreads = currentAuthority
	}
	return &TrustedPublicProjector{
		index: index, caseThreads: caseThreads, currentAuthority: currentAuthority, primaryCAS: primaryCAS, preservedHistory: history,
	}
}

func EnsurePublicProjector(projector PublicProjector) PublicProjector {
	if projector != nil {
		return projector
	}
	return NewTrustedPublicProjector(nil)
}

func (projector *TrustedPublicProjector) ProjectThread(thread map[string]any) (map[string]any, error) {
	projected, err := projectPublicThreadWithAuthority(thread, projector.index, projector.caseThreads, projector.currentAuthority, projector.primaryCAS, projector.preservedHistory)
	if err != nil {
		return nil, err
	}
	ordinary := projectTrustedStructuredOrdinaryV1(projected, trustedProjectionThreadRootV1)
	out, _ := ordinary.(map[string]any)
	if out == nil || !domainstartup.FrozenEventOrderAuthorityStableV1(projected, out) {
		return nil, errors.New("ordinary public thread privacy projection failed")
	}
	if err := domainprivacy.ValidatePublicSourceProse(out); err != nil {
		return nil, err
	}
	if !projectedOrdinaryResultIdentitiesV1(out, trustedProjectionThreadRootV1) {
		return nil, errors.New("ordinary public result identity changed during projection")
	}
	if err := domainevent.ValidatePublicRecord(trustedStructuredValidationViewV1(out, trustedProjectionThreadRootV1)); err != nil {
		return nil, errors.Join(errors.New("ordinary public thread privacy projection failed"), err)
	}
	return out, nil
}

func (projector *TrustedPublicProjector) ProjectEvent(routeThreadID string, thread, event map[string]any) (map[string]any, bool, error) {
	projected, visible, err := projectPublicThreadEvent(routeThreadID, thread, event, projector.index, projector.caseThreads, projector.currentAuthority, projector.primaryCAS)
	if err != nil || !visible {
		return projected, visible, err
	}
	ordinary := projectTrustedStructuredOrdinaryV1(projected, trustedProjectionEventRootV1)
	out, _ := ordinary.(map[string]any)
	if out == nil {
		return nil, false, nil
	}
	if !domainstartup.FrozenEventOrderAuthorityStableV1(projected, out) {
		return nil, false, errors.New("ordinary public event projection changed frozen authority")
	}
	if err := domainprivacy.ValidatePublicSourceProse(out); err != nil {
		return nil, false, err
	}
	if !projectedOrdinaryResultIdentitiesV1(out, trustedProjectionEventRootV1) {
		return nil, false, errors.New("ordinary public result identity changed during projection")
	}
	if domainevent.ValidatePublicRecord(trustedStructuredValidationViewV1(out, trustedProjectionEventRootV1)) != nil {
		return nil, false, nil
	}
	return out, true, nil
}

// projectTrustedStructuredOrdinaryV1 projects only ordinary content leaves.
// projectPublicThread/projectPublicThreadEvent have already closed and verified
// the authority structures supplied here, so frozen values and complete
// terminal containers are exact-cloned instead of being rewritten by generic
// credential heuristics.
type trustedProjectionScopeV1 uint8

// A historical result is verified under its original v1 identity rules. If a
// new output rule changes its hash-bound text, withhold that projection rather
// than emit mismatched hashes or mint a replacement authority during a read.
func projectedOrdinaryResultIdentitiesV1(record map[string]any, scope trustedProjectionScopeV1) bool {
	if scope == trustedProjectionItemV1 {
		if raw, present := record["ordinaryResult"]; present {
			slot, err := domainordinaryresult.ParseResultSlotV1(raw)
			return err == nil && contracts.StringField(record, "text") == slot.Text
		}
		return true
	}
	if scope == trustedProjectionEventRootV1 {
		if item, ok := record["item"].(map[string]any); ok {
			return projectedOrdinaryResultIdentitiesV1(item, trustedProjectionItemV1)
		}
		return true
	}
	key, childScope := "turns", trustedProjectionTurnV1
	if scope == trustedProjectionTurnV1 {
		key, childScope = "items", trustedProjectionItemV1
	}
	children, _ := record[key].([]any)
	for _, raw := range children {
		if child, ok := raw.(map[string]any); ok && !projectedOrdinaryResultIdentitiesV1(child, childScope) {
			return false
		}
	}
	return true
}

const (
	trustedProjectionOrdinaryV1 trustedProjectionScopeV1 = iota
	trustedProjectionThreadRootV1
	trustedProjectionEventRootV1
	trustedProjectionTurnV1
	trustedProjectionItemV1
)

func projectTrustedStructuredOrdinaryV1(value any, scope trustedProjectionScopeV1) any {
	terminalEntries := []trustedTerminalProjectionEntryV1{}
	staged := stageTrustedTerminalAuthorityV1(value, scope, nil, &terminalEntries)
	bindTrustedTerminalProjectionCountV1(terminalEntries)
	planDigestEntries := []trustedTerminalProjectionEntryV1{}
	staged = stageTrustedPlanDigestV1(staged, nil, &planDigestEntries)
	bindTrustedTerminalProjectionCountV1(planDigestEntries)
	projected := domainordinary.ProjectValueV1(staged)
	restored, ok := restoreTrustedTerminalAuthorityV1(projected, planDigestEntries)
	if !ok {
		return nil
	}
	restored, ok = restoreTrustedTerminalAuthorityV1(restored, terminalEntries)
	if !ok {
		return nil
	}
	return restored
}

// stageTrustedPlanDigestV1 protects the SHA-256 and RFC3339Nano metadata of an
// exact, canonical plan or artifact result while the generic ordinary projector
// scans display-bearing fields. Without this typed staging, a legitimate digest
// or timestamp containing a long decimal run can invalidate the closed result.
func stageTrustedPlanDigestV1(
	value any,
	path []trustedProjectionPathSegmentV1,
	entries *[]trustedTerminalProjectionEntryV1,
) any {
	switch current := value.(type) {
	case map[string]any:
		if staged, ok := stageCanonicalPlanToolResultDigestV1(current, path, entries); ok {
			return staged
		}
		out := make(map[string]any, len(current))
		for key, child := range current {
			childPath := append(append([]trustedProjectionPathSegmentV1(nil), path...),
				trustedProjectionPathSegmentV1{Kind: "key", Key: key},
			)
			out[key] = stageTrustedPlanDigestV1(child, childPath, entries)
		}
		return out
	case []any:
		out := make([]any, len(current))
		for index, child := range current {
			childPath := append(append([]trustedProjectionPathSegmentV1(nil), path...),
				trustedProjectionPathSegmentV1{Kind: "index", Index: index},
			)
			out[index] = stageTrustedPlanDigestV1(child, childPath, entries)
		}
		return out
	default:
		return value
	}
}

func stageCanonicalPlanToolResultDigestV1(
	item map[string]any,
	path []trustedProjectionPathSegmentV1,
	entries *[]trustedTerminalProjectionEntryV1,
) (map[string]any, bool) {
	container, fields, ok := domainevent.ClosedToolResultPrivacyMetadataV1(item)
	if !ok {
		return nil, false
	}
	staged, _ := cloneTrustedAuthorityValueV1(item).(map[string]any)
	output, _ := staged["output"].(map[string]any)
	metadata, _ := output[container].(map[string]any)
	if metadata == nil {
		return nil, false
	}
	for _, field := range fields {
		placeholder := map[string]any{
			"kind": "trusted_plan_digest_placeholder_v1", "ordinal": float64(len(*entries)),
		}
		stagedPlaceholder, _ := cloneTrustedAuthorityValueV1(placeholder).(map[string]any)
		metadataPath := append(append([]trustedProjectionPathSegmentV1(nil), path...),
			trustedProjectionPathSegmentV1{Kind: "key", Key: "output"},
			trustedProjectionPathSegmentV1{Kind: "key", Key: container},
			trustedProjectionPathSegmentV1{Kind: "key", Key: field},
		)
		*entries = append(*entries, trustedTerminalProjectionEntryV1{
			path: metadataPath, authority: metadata[field].(string),
			placeholder: placeholder, stagedPlaceholder: stagedPlaceholder,
		})
		metadata[field] = stagedPlaceholder
	}
	return staged, true
}

type trustedProjectionPathSegmentV1 struct {
	Kind  string `json:"kind"`
	Key   string `json:"key,omitempty"`
	Index int    `json:"index,omitempty"`
}

type trustedTerminalProjectionEntryV1 struct {
	path              []trustedProjectionPathSegmentV1
	authority         any
	placeholder       map[string]any
	stagedPlaceholder map[string]any
}

func bindTrustedTerminalProjectionCountV1(entries []trustedTerminalProjectionEntryV1) {
	for index := range entries {
		count := float64(len(entries))
		entries[index].placeholder["total"] = count
		entries[index].stagedPlaceholder["total"] = count
	}
}

func stageTrustedTerminalAuthorityV1(
	value any,
	scope trustedProjectionScopeV1,
	path []trustedProjectionPathSegmentV1,
	entries *[]trustedTerminalProjectionEntryV1,
) any {
	record, structured := value.(map[string]any)
	if !structured {
		return cloneTrustedAuthorityValueV1(value)
	}
	if (scope == trustedProjectionEventRootV1 || scope == trustedProjectionTurnV1 || scope == trustedProjectionItemV1) &&
		trustedDirectTerminalAuthorityV1(record) {
		authority := cloneTrustedAuthorityValueV1(record)
		pathBody, _ := json.Marshal(path)
		authorityBody, _ := json.Marshal(authority)
		placeholder := map[string]any{
			"kind":            "trusted_terminal_projection_placeholder_v1",
			"ordinal":         float64(len(*entries)),
			"pathDigest":      domainsecurity.SHA256Hex(pathBody),
			"authorityDigest": domainsecurity.SHA256Hex(authorityBody),
		}
		stagedPlaceholder, _ := cloneTrustedAuthorityValueV1(placeholder).(map[string]any)
		*entries = append(*entries, trustedTerminalProjectionEntryV1{
			path: append([]trustedProjectionPathSegmentV1(nil), path...), authority: authority,
			placeholder: placeholder, stagedPlaceholder: stagedPlaceholder,
		})
		return stagedPlaceholder
	}
	out := make(map[string]any, len(record))
	for key, child := range record {
		childScope := trustedProjectionOrdinaryV1
		if scope == trustedProjectionThreadRootV1 && key == "turns" {
			childScope = trustedProjectionTurnV1
		} else if scope == trustedProjectionTurnV1 && key == "items" {
			childScope = trustedProjectionItemV1
		}
		if childScope == trustedProjectionOrdinaryV1 {
			out[key] = cloneTrustedAuthorityValueV1(child)
			continue
		}
		values, ok := child.([]any)
		if !ok {
			out[key] = cloneTrustedAuthorityValueV1(child)
			continue
		}
		projected := make([]any, len(values))
		for index := range values {
			childPath := append(append([]trustedProjectionPathSegmentV1(nil), path...),
				trustedProjectionPathSegmentV1{Kind: "key", Key: key},
				trustedProjectionPathSegmentV1{Kind: "index", Index: index},
			)
			projected[index] = stageTrustedTerminalAuthorityV1(values[index], childScope, childPath, entries)
		}
		out[key] = projected
	}
	return out
}

func restoreTrustedTerminalAuthorityV1(value any, entries []trustedTerminalProjectionEntryV1) (any, bool) {
	current := value
	for index := range entries {
		var ok bool
		current, ok = restoreTrustedTerminalAuthorityAtPathV1(current, entries[index].path, entries[index])
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func restoreTrustedTerminalAuthorityAtPathV1(
	value any,
	path []trustedProjectionPathSegmentV1,
	entry trustedTerminalProjectionEntryV1,
) (any, bool) {
	if len(path) == 0 {
		if !reflect.DeepEqual(value, entry.placeholder) {
			return nil, false
		}
		return cloneTrustedAuthorityValueV1(entry.authority), true
	}
	segment := path[0]
	switch segment.Kind {
	case "key":
		record, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		child, present := record[segment.Key]
		if !present {
			return nil, false
		}
		restored, ok := restoreTrustedTerminalAuthorityAtPathV1(child, path[1:], entry)
		if !ok {
			return nil, false
		}
		record[segment.Key] = restored
		return record, true
	case "index":
		values, ok := value.([]any)
		if !ok || segment.Index < 0 || segment.Index >= len(values) {
			return nil, false
		}
		restored, ok := restoreTrustedTerminalAuthorityAtPathV1(values[segment.Index], path[1:], entry)
		if !ok {
			return nil, false
		}
		values[segment.Index] = restored
		return values, true
	default:
		return nil, false
	}
}

func trustedStructuredValidationViewV1(value any, scope trustedProjectionScopeV1) any {
	record, structured := value.(map[string]any)
	if !structured {
		return value
	}
	if (scope == trustedProjectionEventRootV1 || scope == trustedProjectionTurnV1 || scope == trustedProjectionItemV1) &&
		trustedDirectTerminalAuthorityV1(record) {
		return map[string]any{"kind": "trusted_terminal_authority"}
	}
	if scope != trustedProjectionThreadRootV1 && scope != trustedProjectionTurnV1 {
		return value
	}
	out := make(map[string]any, len(record))
	for key, child := range record {
		childScope := trustedProjectionOrdinaryV1
		if scope == trustedProjectionThreadRootV1 && key == "turns" {
			childScope = trustedProjectionTurnV1
		} else if scope == trustedProjectionTurnV1 && key == "items" {
			childScope = trustedProjectionItemV1
		}
		if childScope == trustedProjectionOrdinaryV1 {
			out[key] = child
			continue
		}
		values, ok := child.([]any)
		if !ok {
			out[key] = child
			continue
		}
		projected := make([]any, len(values))
		for index := range values {
			projected[index] = trustedStructuredValidationViewV1(values[index], childScope)
		}
		out[key] = projected
	}
	return out
}

func cloneTrustedAuthorityValueV1(value any) any {
	switch current := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, child := range current {
			out[key] = cloneTrustedAuthorityValueV1(child)
		}
		return out
	case []any:
		out := make([]any, len(current))
		for index, child := range current {
			out[index] = cloneTrustedAuthorityValueV1(child)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(current))
		for index, child := range current {
			out[index], _ = cloneTrustedAuthorityValueV1(child).(map[string]any)
		}
		return out
	default:
		return value
	}
}

func trustedDirectTerminalAuthorityV1(record map[string]any) bool {
	for key := range record {
		if domainevent.ContainsTerminalPublicationAuthority(map[string]any{key: nil}) {
			return true
		}
	}
	kind := strings.TrimSpace(contracts.StringField(record, "kind"))
	purpose := strings.TrimSpace(contracts.StringField(record, "purpose"))
	return kind == "accepted_final_batch" || kind == "general_terminal_batch" ||
		purpose == "analytix.accepted-final-delivery-batch/v1" ||
		purpose == "analytix.accepted-final-delivery-batch/v2" ||
		purpose == "analytix.general-terminal-delivery-batch/v1"
}

func (projector *TrustedPublicProjector) SealAcceptedFinalDelivery(
	ctx context.Context,
	events []map[string]any,
) (domainevent.AcceptedFinalDeliverySealV1, error) {
	if projector == nil || projector.index == nil {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery projector is unavailable")
	}
	return projector.index.SealAcceptedFinalDelivery(ctx, events)
}

func (projector *TrustedPublicProjector) ValidateAcceptedFinalDelivery(
	ctx context.Context,
	batch domainevent.AcceptedFinalDeliveryBatchV2,
) error {
	if projector == nil || projector.index == nil {
		return errors.New("accepted final delivery projector is unavailable")
	}
	return projector.index.ValidateAcceptedFinalDelivery(ctx, batch)
}

type PublicThreadReader interface {
	GetThread(string) (map[string]any, error)
}

func NewPublicProjectionPreflight(reader PublicThreadReader, projector PublicProjector) func(string) string {
	return func(threadID string) string {
		threadID = strings.TrimSpace(threadID)
		if reader == nil || projector == nil {
			return CasePublicAuthorityUnavailableCode
		}
		thread, err := reader.GetThread(threadID)
		if err != nil || thread == nil {
			return CasePublicAuthorityUnavailableCode
		}
		if contracts.StringField(thread, "id") != threadID {
			return CasePublicAuthorityRejectedCode
		}
		projected, err := projector.ProjectThread(thread)
		if err != nil || projected == nil || contracts.StringField(projected, "id") != threadID {
			return CasePublicAuthorityRejectedCode
		}
		return ""
	}
}

func NewPublicEventProjector(reader PublicThreadReader, projector PublicProjector) func(string, map[string]any) (map[string]any, bool, string) {
	return func(threadID string, event map[string]any) (map[string]any, bool, string) {
		threadID = strings.TrimSpace(threadID)
		if reader == nil || projector == nil {
			return nil, false, CasePublicAuthorityUnavailableCode
		}
		thread, err := reader.GetThread(threadID)
		if err != nil || thread == nil {
			return nil, false, CasePublicAuthorityUnavailableCode
		}
		if contracts.StringField(thread, "id") != threadID {
			return nil, false, CasePublicAuthorityRejectedCode
		}
		projected, visible, err := projector.ProjectEvent(threadID, thread, event)
		if err != nil {
			return nil, false, CasePublicAuthorityRejectedCode
		}
		return projected, visible, ""
	}
}
