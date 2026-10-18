package usage

import (
	appcache "analytix.local/runtime-go/internal/app/cachetelemetry"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	"context"
	"errors"
	"reflect"
	"sort"
	"time"
)

const ProviderAttemptTelemetrySchemaV1 = domainterminaltelemetry.ProviderAttemptTelemetrySchemaV1

// ProviderAttemptDiagnostics validates and aggregates every physical provider
// attempt. It emits numeric counters and closed status only; HMAC identities,
// request bytes, endpoints, credentials, model names, provider text, and
// reasoning have no public representation here.
func ProviderAttemptDiagnostics(observations []domaincache.ProviderCallObservationV1) map[string]any {
	if len(observations) == 0 {
		return nil
	}
	service := appcache.NewService()
	observations = append([]domaincache.ProviderCallObservationV1(nil), observations...)
	sort.SliceStable(observations, func(i, j int) bool {
		if observations[i].Shape.LogicalCallHMAC == observations[j].Shape.LogicalCallHMAC {
			return observations[i].Shape.Attempt < observations[j].Shape.Attempt
		}
		return observations[i].Shape.LogicalCallHMAC < observations[j].Shape.LogicalCallHMAC
	})
	for _, observation := range observations {
		if err := service.Begin(observation.Shape); err != nil {
			return invalidProviderAttemptDiagnostics()
		}
		if err := service.Settle(observation); err != nil {
			return invalidProviderAttemptDiagnostics()
		}
	}
	aggregate, err := service.Aggregate()
	if err != nil {
		return invalidProviderAttemptDiagnostics()
	}
	return map[string]any{
		"providerAttemptTelemetrySchema":   ProviderAttemptTelemetrySchemaV1,
		"providerAttemptTelemetryValid":    true,
		"providerLogicalCallCount":         aggregate.LogicalCallCount,
		"providerAttemptCount":             aggregate.AttemptCount,
		"providerCostKnownAttemptCount":    aggregate.CostKnownAttempts,
		"providerKnownCostUsdAttemptCount": aggregate.CostKnownUSDAttempts,
		"providerKnownCostCnyAttemptCount": aggregate.CostKnownCNYAttempts,
		"providerKnownCostUsdNanos":        aggregate.CostUSDNanos,
		"providerKnownCostCnyNanos":        aggregate.CostCNYNanos,
		"providerCostEstimateComplete":     aggregate.AttemptCount > 0 && aggregate.CostKnownAttempts == aggregate.AttemptCount,
		"providerAttemptStatuses": map[string]any{
			"succeeded": aggregate.Statuses.Succeeded, "failed": aggregate.Statuses.Failed,
			"cancelled": aggregate.Statuses.Cancelled, "timedOut": aggregate.Statuses.TimedOut,
			"streamAborted": aggregate.Statuses.StreamAborted,
		},
		"providerAttemptInputTokens":     aggregateTokenDiagnostics(aggregate.InputTokens),
		"providerAttemptOutputTokens":    aggregateTokenDiagnostics(aggregate.OutputTokens),
		"providerAttemptCacheHitTokens":  aggregateTokenDiagnostics(aggregate.CacheHitTokens),
		"providerAttemptCacheMissTokens": aggregateTokenDiagnostics(aggregate.CacheMissTokens),
		"providerAttemptCacheRate": map[string]any{
			"known": aggregate.CacheRate.Known, "numerator": aggregate.CacheRate.Numerator,
			"denominator": aggregate.CacheRate.Denominator,
		},
	}
}

func invalidProviderAttemptDiagnostics() map[string]any {
	return map[string]any{
		"providerAttemptTelemetrySchema": ProviderAttemptTelemetrySchemaV1,
		"providerAttemptTelemetryValid":  false,
		"providerAttemptTelemetryError":  "invalid_or_incomplete",
	}
}

func aggregateTokenDiagnostics(count appcache.AggregatedTokenCountV1) map[string]any {
	result := map[string]any{
		"complete": count.Complete, "knownObservationCount": count.KnownObservationCount,
		"observationCount": count.ObservationCount,
	}
	if count.Complete {
		result["value"] = count.Value
	}
	return result
}

// ConversationUsageV1 is rebuilt from the canonical event log and existing
// signed provider ledger. It adds no persistence authority or telemetry engine.
type ConversationUsageV1 struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Billing       appcache.ProviderBillingV1 `json:"billing"`
	Turns         []ConversationTurnUsageV1  `json:"turns"`
	Metrics       ConversationMetricsV1      `json:"metrics"`
}

type ConversationTurnUsageV1 struct {
	TurnID  string                      `json:"turnId"`
	Billing *appcache.ProviderBillingV1 `json:"billing,omitempty"`
	Metrics ConversationMetricsV1       `json:"metrics"`
}

type StepWallTimeV1 struct {
	ReportedValue  uint64 `json:"reportedValue"`
	KnownStepCount uint64 `json:"knownStepCount"`
	Complete       bool   `json:"complete"`
}

type ToolWallTimeV1 struct {
	ReportedValue  uint64 `json:"reportedValue"`
	KnownCallCount uint64 `json:"knownCallCount"`
	CallCount      uint64 `json:"callCount"`
	Complete       bool   `json:"complete"`
}

type ConversationMetricsV1 struct {
	TurnCount    *uint64        `json:"turnCount"`
	StepCount    *uint64        `json:"stepCount"`
	LLMMs        StepWallTimeV1 `json:"llmMs"`
	ToolMs       ToolWallTimeV1 `json:"toolMs"`
	TTFTMs       uint64         `json:"ttftMs"`
	TTFTSteps    uint64         `json:"ttftSteps"`
	DecodeMs     uint64         `json:"decodeMs"`
	DecodeTokens uint64         `json:"decodeTokens"`
	DecodeSteps  uint64         `json:"decodeSteps"`
}

type TurnUsageInventoryV1 struct {
	TurnID    string
	Inventory appcache.TurnInventoryV1
}

// ProviderUsageObserverV1 reads the existing installation-signed provider
// ledger. It is the same authority used by provider execution and closure.
type ProviderUsageObserverV1 interface {
	ObserveTurnInventoriesV1(context.Context, []domainsecurity.TurnSecurityContext) ([]appcache.TurnInventoryV1, error)
}

// ObserveConversationUsageV1 composes the historical read view with sealed
// provider inventory and the existing conversation aggregator.
func ObserveConversationUsageV1(ctx context.Context, thread map[string]any, events []map[string]any, committedContexts []domainsecurity.TurnSecurityContext, observer ProviderUsageObserverV1) (ConversationUsageV1, error) {
	thread, err := ConversationUsageHistoryV1(thread, events, committedContexts)
	if err != nil {
		return ConversationUsageV1{}, err
	}
	contexts, err := ConversationUsageContextsV1(thread, events)
	if err != nil {
		return ConversationUsageV1{}, err
	}
	if observer == nil {
		return ConversationUsageV1{}, errors.New("conversation provider usage inventory is unavailable")
	}
	snapshots, err := observer.ObserveTurnInventoriesV1(ctx, contexts)
	if err != nil {
		return ConversationUsageV1{}, errors.New("conversation provider usage inventory is unavailable")
	}
	if len(snapshots) != len(contexts) {
		return ConversationUsageV1{}, errors.New("conversation provider usage inventory is incomplete")
	}
	inventories := make([]TurnUsageInventoryV1, 0, len(snapshots))
	for i, snapshot := range snapshots {
		inventories = append(inventories, TurnUsageInventoryV1{TurnID: contexts[i].TurnID, Inventory: snapshot})
	}
	return BuildConversationUsageV1(thread, events, inventories)
}

func ConversationUsageContextsV1(thread map[string]any, events []map[string]any) ([]domainsecurity.TurnSecurityContext, error) {
	threadID := stringField(thread, "id")
	turnIDs := map[string]bool{}
	contexts := []domainsecurity.TurnSecurityContext{}
	seen := map[string]bool{}
	add := func(turn map[string]any) error {
		turnID := stringField(turn, "id")
		raw := turn["securityContext"]
		if raw == nil {
			return nil
		} // Legacy turns remain explicitly unavailable.
		frozen, err := domainsecurity.ParseTurnSecurityContext(raw)
		if err != nil || frozen.ThreadID != threadID || frozen.TurnID != turnID {
			return errors.New("conversation usage scope is invalid")
		}
		if frozen.Version != domainsecurity.TurnSecurityContextVersionV2 {
			return nil
		}
		if !seen[frozen.ContextDigest] {
			seen[frozen.ContextDigest] = true
			contexts = append(contexts, frozen)
		}
		return nil
	}
	for _, value := range listAny(thread["turns"]) {
		turn, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("conversation usage turn is invalid")
		}
		turnIDs[stringField(turn, "id")] = true
		if err := add(turn); err != nil {
			return nil, err
		}
	}
	for _, event := range events {
		if stringField(event, "kind") != "turn_started" || stringField(event, "threadId") != threadID || !turnIDs[stringField(event, "turnId")] {
			continue
		}
		turn, _ := event["turn"].(map[string]any)
		if turn == nil {
			// Protected starts intentionally persist metadata only. Their frozen
			// context is read from the durable turn, never invented from labels.
			if event["securityContext"] == nil {
				continue
			}
			turn = map[string]any{"id": stringField(event, "turnId")}
		}
		if stringField(turn, "id") != stringField(event, "turnId") {
			return nil, errors.New("conversation usage start scope is invalid")
		}
		if raw := event["securityContext"]; raw != nil {
			if embedded := turn["securityContext"]; embedded != nil {
				a, aErr := domainsecurity.ParseTurnSecurityContext(raw)
				b, bErr := domainsecurity.ParseTurnSecurityContext(embedded)
				if aErr != nil || bErr != nil || a.ContextDigest != b.ContextDigest {
					return nil, errors.New("conversation usage start scopes conflict")
				}
			}
			turn = map[string]any{"id": stringField(event, "turnId"), "securityContext": raw}
		}
		if err := add(turn); err != nil {
			return nil, err
		}
	}
	return contexts, nil
}

// The usage read view includes historical turns removed by compaction. It does
// not write a thread, restore content, or add a second persistence mechanism.
func ConversationUsageHistoryV1(thread map[string]any, events []map[string]any, committedContexts []domainsecurity.TurnSecurityContext) (map[string]any, error) {
	threadID := stringField(thread, "id")
	view := contracts.CloneMap(thread)
	turns := []any{}
	byID := map[string]map[string]any{}
	add := func(turnID string) map[string]any {
		if turn := byID[turnID]; turn != nil {
			return turn
		}
		turn := map[string]any{"id": turnID, "threadId": threadID, "items": []any{}}
		byID[turnID] = turn
		turns = append(turns, turn)
		return turn
	}
	for _, value := range listAny(view["turns"]) {
		turn, ok := value.(map[string]any)
		if !ok || stringField(turn, "id") == "" || byID[stringField(turn, "id")] != nil {
			return nil, errors.New("conversation historical turn inventory is invalid")
		}
		byID[stringField(turn, "id")] = turn
		turns = append(turns, turn)
	}
	for _, event := range events {
		if stringField(event, "threadId") != threadID || stringField(event, "kind") != "turn_started" {
			continue
		}
		turnID := stringField(event, "turnId")
		if turnID == "" {
			return nil, errors.New("conversation historical start identity is invalid")
		}
		turn := add(turnID)
		if turn["securityContext"] == nil {
			if frozen := event["securityContext"]; frozen != nil {
				turn["securityContext"] = frozen
			} else if embedded, _ := event["turn"].(map[string]any); embedded != nil {
				turn["securityContext"] = embedded["securityContext"]
			}
		}
	}
	for _, frozen := range committedContexts {
		if frozen.ThreadID != threadID {
			continue
		}
		turn := add(frozen.TurnID)
		if turn["securityContext"] == nil {
			turn["securityContext"] = frozen
		}
	}
	if raw := thread["generalTerminalPublicationArchive"]; raw != nil {
		archive, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(raw)
		if err != nil {
			return nil, errors.New("conversation historical terminal membership is invalid")
		}
		for _, commit := range archive.Commits {
			if commit.ThreadID != threadID {
				return nil, errors.New("conversation historical terminal membership is foreign")
			}
			add(commit.TurnID)
		}
	}
	view["turns"] = turns
	return view, nil
}

func BuildConversationUsageV1(thread map[string]any, events []map[string]any, inventories []TurnUsageInventoryV1) (ConversationUsageV1, error) {
	if stringField(thread, "id") == "" {
		return ConversationUsageV1{}, errors.New("conversation thread identity is unavailable")
	}
	if _, ok := thread["turns"].([]any); !ok {
		return ConversationUsageV1{}, errors.New("conversation turn inventory is unavailable")
	}
	result := ConversationUsageV1{SchemaVersion: "conversation-usage.v1", Turns: []ConversationTurnUsageV1{}}
	byTurn := map[string][]appcache.TurnInventoryV1{}
	seenScopes := map[string]bool{}
	for _, entry := range inventories {
		threadID, turnID, binding, available := entry.Inventory.ProjectionScopeV1()
		if !available || threadID != stringField(thread, "id") || turnID != entry.TurnID || seenScopes[binding] {
			return ConversationUsageV1{}, errors.New("conversation provider inventory scope is invalid or duplicated")
		}
		seenScopes[binding] = true
		byTurn[entry.TurnID] = append(byTurn[entry.TurnID], entry.Inventory)
	}
	allComplete, allClosed := true, true
	stepCount := uint64(0)
	result.Metrics.StepCount = &stepCount
	turnCount := uint64(0)
	result.Metrics.TurnCount = &turnCount
	seenTurns := map[string]bool{}
	for _, raw := range listAny(thread["turns"]) {
		turn, ok := raw.(map[string]any)
		if !ok {
			return ConversationUsageV1{}, errors.New("conversation turn inventory is invalid")
		}
		turnID := stringField(turn, "id")
		if turnID == "" || seenTurns[turnID] {
			return ConversationUsageV1{}, errors.New("conversation turn identity is invalid")
		}
		seenTurns[turnID] = true
		entry := ConversationTurnUsageV1{TurnID: turnID}
		var err error
		entry.Metrics, err = logicalStepMetricsV1(stringField(thread, "id"), turnID, events)
		if err != nil {
			return ConversationUsageV1{}, err
		}
		if snapshots := byTurn[turnID]; len(snapshots) > 0 {
			billing := appcache.ProviderBillingV1{InventoryComplete: true, Closed: true}
			for _, snapshot := range snapshots {
				part, err := snapshot.BillingV1()
				if err != nil {
					return ConversationUsageV1{}, err
				}
				if err := addConversationBillingV1(&billing, part); err != nil {
					return ConversationUsageV1{}, err
				}
			}
			finishConversationBillingV1(&billing)
			entry.Billing = &billing
			if err := addConversationBillingV1(&result.Billing, billing); err != nil {
				return ConversationUsageV1{}, err
			}
			allComplete = allComplete && billing.InventoryComplete
			allClosed = allClosed && billing.Closed
		} else {
			allComplete, allClosed = false, false
		}
		tools, err := conversationToolTimeV1(thread, events, turnID)
		if err != nil {
			return ConversationUsageV1{}, err
		}
		entry.Metrics.ToolMs = tools
		if err := addConversationMetricsV1(&result.Metrics, entry.Metrics); err != nil {
			return ConversationUsageV1{}, err
		}
		result.Turns = append(result.Turns, entry)
	}
	result.Billing.InventoryComplete, result.Billing.Closed = allComplete, allClosed
	finishConversationBillingV1(&result.Billing)
	result.Metrics.LLMMs.Complete = result.Metrics.StepCount != nil && result.Metrics.LLMMs.KnownStepCount == *result.Metrics.StepCount
	result.Metrics.ToolMs.Complete = result.Metrics.ToolMs.KnownCallCount == result.Metrics.ToolMs.CallCount
	return result, nil
}

func billingTokenCountsV1(b *appcache.ProviderBillingV1) []*appcache.ReportedTokenCountV1 {
	return []*appcache.ReportedTokenCountV1{&b.InputTokens, &b.OutputTokens, &b.UncachedInputTokens, &b.CacheReadTokens, &b.CacheWriteTokens, &b.ReasoningTokens, &b.TotalTokens}
}

func addConversationBillingV1(target *appcache.ProviderBillingV1, part appcache.ProviderBillingV1) error {
	if !part.InventoryComplete {
		target.InventoryComplete = false
	}
	target.Closed = target.Closed && part.Closed
	routesComplete := (target.AttemptCount == 0 || len(target.Routes) > 0) && (part.AttemptCount == 0 || len(part.Routes) > 0)
	for _, route := range part.Routes {
		seen := false
		for _, existing := range target.Routes {
			if route == existing {
				seen = true
				break
			}
		}
		if !seen {
			target.Routes = append(target.Routes, route)
		}
	}
	sort.Slice(target.Routes, func(i, j int) bool {
		if target.Routes[i].Provider == target.Routes[j].Provider {
			return target.Routes[i].Model < target.Routes[j].Model
		}
		return target.Routes[i].Provider < target.Routes[j].Provider
	})
	if !routesComplete {
		target.Routes = nil
	}
	if err := addConversationUnsignedV1(&target.AttemptCount, part.AttemptCount); err != nil {
		return err
	}
	if err := addConversationUnsignedV1(&target.SettledAttemptCount, part.SettledAttemptCount); err != nil {
		return err
	}
	for i, count := range billingTokenCountsV1(target) {
		other := billingTokenCountsV1(&part)[i]
		if err := addConversationUnsignedV1(&count.ReportedValue, other.ReportedValue); err != nil {
			return err
		}
		if err := addConversationUnsignedV1(&count.KnownAttemptCount, other.KnownAttemptCount); err != nil {
			return err
		}
		if err := addConversationUnsignedV1(&count.QualifiedAttemptCount, other.QualifiedAttemptCount); err != nil {
			return err
		}
	}
	return nil
}

func finishConversationBillingV1(b *appcache.ProviderBillingV1) {
	if !b.InventoryComplete {
		b.Routes = nil
	}
	for _, count := range billingTokenCountsV1(b) {
		count.AttemptCount = b.AttemptCount
		count.Complete = b.InventoryComplete && count.QualifiedAttemptCount == b.AttemptCount && (b.AttemptCount > 0 || b.Closed)
	}
}

func addConversationUnsignedV1(target *uint64, value uint64) error {
	const max uint64 = 9007199254740991
	if value > max || *target > max-value {
		return errors.New("conversation usage projection overflows")
	}
	*target += value
	return nil
}

// LogicalStepMetricsVersionV1 marks executions whose event log can prove
// logical closes. A signed provider attempt is billing evidence, not a close.
const LogicalStepMetricsVersionV1 = domainevent.LogicalStepMetricsVersionV1

// ProjectLogicalStepDetailsV1 shares the domain-owned closed grammar with
// event publication and lifecycle validation.
func ProjectLogicalStepDetailsV1(stage string, details map[string]any) (map[string]any, bool) {
	return domainevent.ProjectLogicalStepDetailsV1(stage, details)
}

func logicalMetricNumberV1(value any) (uint64, bool) {
	number, ok := contracts.NumericSeq(value)
	return uint64(number), ok && number >= 0 && uint64(number) <= 9007199254740991
}

func logicalStepMetricsV1(threadID, turnID string, events []map[string]any) (ConversationMetricsV1, error) {
	type reading struct {
		start, close map[string]any
		invalid      bool
	}
	groups := map[string]*reading{}
	invalidateLogicalRecord := func(event map[string]any) {
		details, _ := event["details"].(map[string]any)
		id := stringField(details, "logicalStepId")
		if !contracts.IsCanonicalOpaqueUUIDV4(id) {
			return
		}
		if groups[id] == nil {
			groups[id] = &reading{}
		}
		groups[id].invalid = true
	}
	seqs := map[int]map[string]any{}
	ordered := []map[string]any{}
	coverageKnown, markerSeen, modern := true, false, false
	for _, event := range events {
		if stringField(event, "threadId") != threadID || stringField(event, "turnId") != turnID || stringField(event, "kind") != "pipeline_stage" {
			continue
		}
		stage := stringField(event, "stage")
		if stage != "post_start" && stage != "logical_step_started" && stage != "logical_step_closed" {
			continue
		}
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq <= 0 {
			coverageKnown = false
			invalidateLogicalRecord(event)
			continue
		}
		if previous := seqs[seq]; previous != nil {
			if !reflect.DeepEqual(previous, event) {
				coverageKnown = false
				invalidateLogicalRecord(previous)
				invalidateLogicalRecord(event)
			}
			continue
		}
		seqs[seq] = event
		ordered = append(ordered, event)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, _ := contracts.NumericSeq(ordered[i]["seq"])
		b, _ := contracts.NumericSeq(ordered[j]["seq"])
		return a < b
	})
	for _, event := range ordered {
		stage := stringField(event, "stage")
		details, _ := event["details"].(map[string]any)
		if stage == "post_start" {
			markerSeen = true
			version, valid := logicalMetricNumberV1(details["logicalStepMetricsVersion"])
			modern = valid && version == LogicalStepMetricsVersionV1
			coverageKnown = coverageKnown && modern
			continue
		}
		_, valid := ProjectLogicalStepDetailsV1(stage, details)
		id := stringField(details, "logicalStepId")
		at, atErr := time.Parse(time.RFC3339Nano, stringField(event, "timestamp"))
		if !valid || atErr != nil || at.IsZero() {
			coverageKnown = false
			invalidateLogicalRecord(event)
			continue
		}
		if !modern {
			coverageKnown = false
		}
		group := groups[id]
		if group == nil {
			group = &reading{}
			groups[id] = group
		}
		if stage == "logical_step_started" {
			if group.start != nil {
				group.invalid = true
				coverageKnown = false
			}
			group.start = event
		} else {
			if group.close != nil {
				group.invalid = true
				coverageKnown = false
			}
			group.close = event
		}
	}
	result := ConversationMetricsV1{}
	count, turns := uint64(0), uint64(0)
	for _, group := range groups {
		if group.close == nil {
			continue
		}
		startSeq, _ := contracts.NumericSeq(group.start["seq"])
		closeSeq, _ := contracts.NumericSeq(group.close["seq"])
		startAt, startErr := time.Parse(time.RFC3339Nano, stringField(group.start, "timestamp"))
		closeAt, closeErr := time.Parse(time.RFC3339Nano, stringField(group.close, "timestamp"))
		if group.invalid || group.start == nil || startSeq >= closeSeq || startErr != nil || closeErr != nil || closeAt.Before(startAt) {
			coverageKnown = false
			continue
		}
		count++
		details, _ := group.close["details"].(map[string]any)
		projected, valid := ProjectLogicalStepDetailsV1("logical_step_closed", details)
		if !valid || projected["assistantMessageAssembled"] != true {
			continue
		}
		if value, present := projected["llmMs"]; present {
			number, _ := logicalMetricNumberV1(value)
			if err := addConversationUnsignedV1(&result.LLMMs.ReportedValue, number); err != nil {
				return ConversationMetricsV1{}, err
			}
			result.LLMMs.KnownStepCount++
		}
		if value, present := projected["ttftMs"]; present {
			number, _ := logicalMetricNumberV1(value)
			if err := addConversationUnsignedV1(&result.TTFTMs, number); err != nil {
				return ConversationMetricsV1{}, err
			}
			result.TTFTSteps++
		}
		if value, present := projected["decodeMs"]; present {
			number, _ := logicalMetricNumberV1(value)
			output, _ := logicalMetricNumberV1(projected["decodeTokens"])
			if err := addConversationUnsignedV1(&result.DecodeMs, number); err != nil {
				return ConversationMetricsV1{}, err
			}
			if err := addConversationUnsignedV1(&result.DecodeTokens, output); err != nil {
				return ConversationMetricsV1{}, err
			}
			result.DecodeSteps++
		}
	}
	if coverageKnown && markerSeen {
		if count > 0 {
			turns = 1
		}
		result.StepCount = &count
		result.TurnCount = &turns
	}
	result.LLMMs.Complete = result.StepCount != nil && result.LLMMs.KnownStepCount == *result.StepCount
	return result, nil
}

func addConversationMetricsV1(target *ConversationMetricsV1, part ConversationMetricsV1) error {
	for _, pair := range [][2]**uint64{{&target.StepCount, &part.StepCount}, {&target.TurnCount, &part.TurnCount}} {
		if *pair[0] == nil || *pair[1] == nil {
			*pair[0] = nil
		} else if err := addConversationUnsignedV1(*pair[0], **pair[1]); err != nil {
			return err
		}
	}
	for _, pair := range [][2]*uint64{{&target.LLMMs.ReportedValue, &part.LLMMs.ReportedValue}, {&target.LLMMs.KnownStepCount, &part.LLMMs.KnownStepCount}, {&target.ToolMs.ReportedValue, &part.ToolMs.ReportedValue}, {&target.ToolMs.KnownCallCount, &part.ToolMs.KnownCallCount}, {&target.ToolMs.CallCount, &part.ToolMs.CallCount}, {&target.TTFTMs, &part.TTFTMs}, {&target.TTFTSteps, &part.TTFTSteps}, {&target.DecodeMs, &part.DecodeMs}, {&target.DecodeTokens, &part.DecodeTokens}, {&target.DecodeSteps, &part.DecodeSteps}} {
		if err := addConversationUnsignedV1(pair[0], *pair[1]); err != nil {
			return err
		}
	}
	target.LLMMs.Complete = target.StepCount != nil && target.LLMMs.KnownStepCount == *target.StepCount
	target.ToolMs.Complete = target.ToolMs.KnownCallCount == target.ToolMs.CallCount
	return nil
}

// Tool elapsed time is ready-to-finished wall time, including waits. A real
// ready must identify the durable tool_call item; partial preparation cannot.
func conversationToolTimeV1(thread map[string]any, events []map[string]any, turnID string) (ToolWallTimeV1, error) {
	type callTime struct {
		ready, finished                  time.Time
		readySeen, finishedSeen, invalid bool
		readySeq, finishedSeq            int
	}
	calls := map[string]*callTime{}
	seqs := map[int]map[string]any{}
	items := map[string]map[string]any{}
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		for _, value := range listAny(turn["items"]) {
			item, _ := value.(map[string]any)
			if item != nil {
				items[stringField(item, "id")] = item
				if stringField(item, "kind") == "tool_call" && stringField(item, "callId") != "" {
					calls[stringField(item, "callId")] = &callTime{}
				}
			}
		}
	}
	// Full-log durable item events retain identity even after compaction has
	// removed the turn from the renderer's current history window.
	for _, event := range events {
		if stringField(event, "threadId") != stringField(thread, "id") || stringField(event, "turnId") != turnID {
			continue
		}
		switch stringField(event, "kind") {
		case "tool_call_started", "tool_call_finished", "item_created", "item_updated", "item_completed":
		default:
			continue
		}
		item, _ := event["item"].(map[string]any)
		itemID, callID := stringField(item, "id"), stringField(item, "callId")
		kind := stringField(item, "kind")
		if itemID == "" || itemID != stringField(event, "itemId") || callID == "" || stringField(item, "threadId") != stringField(thread, "id") || stringField(item, "turnId") != turnID || (kind != "tool_call" && kind != "tool_result") {
			continue
		}
		items[itemID] = item
		if kind == "tool_call" && calls[callID] == nil {
			calls[callID] = &callTime{}
		}
	}
	for _, event := range events {
		if stringField(event, "threadId") != stringField(thread, "id") || stringField(event, "turnId") != turnID {
			continue
		}
		kind := stringField(event, "kind")
		if kind != "tool_call_ready" && kind != "tool_call_finished" {
			continue
		}
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq <= 0 {
			return ToolWallTimeV1{}, errors.New("conversation tool event sequence is invalid")
		}
		if previous := seqs[seq]; previous != nil {
			if !reflect.DeepEqual(previous, event) {
				return ToolWallTimeV1{}, errors.New("conversation tool event sequence conflicts")
			}
			continue
		}
		seqs[seq] = event
		callID := stringField(event, "callId")
		if kind == "tool_call_finished" {
			item, _ := event["item"].(map[string]any)
			callID = stringField(item, "callId")
		}
		if callID == "" {
			continue
		}
		if kind == "tool_call_ready" {
			if partial, exists := event["partial"]; exists && partial != false {
				continue
			}
			itemID := stringField(event, "itemId")
			item := items[itemID]
			// Legacy preparation may have lost its partial marker. Only the
			// canonical durable item proves ready; missing endpoints still leave
			// the real call seeded above unknown, without poisoning later ready.
			if itemID == "" || itemID != domaintoolcall.ToolCallItemIDV1(turnID, callID) || item == nil || stringField(item, "kind") != "tool_call" || stringField(item, "callId") != callID {
				continue
			}
		}
		call := calls[callID]
		if call == nil {
			call = &callTime{}
			calls[callID] = call
		}
		if kind == "tool_call_finished" {
			item, _ := event["item"].(map[string]any)
			itemID := stringField(event, "itemId")
			durable := items[itemID]
			status := stringField(item, "status")
			isError, hasError := item["isError"].(bool)
			if itemID == "" || stringField(item, "id") != itemID || stringField(item, "threadId") != stringField(thread, "id") || stringField(item, "turnId") != turnID || stringField(item, "kind") != "tool_result" || (status != "completed" && status != "failed") || !hasError || isError != (status == "failed") || durable == nil || stringField(durable, "kind") != "tool_result" || stringField(durable, "callId") != callID || stringField(durable, "status") != status {
				call.invalid = true
				continue
			}
		}
		at, err := time.Parse(time.RFC3339Nano, stringField(event, "timestamp"))
		if err != nil {
			call.invalid = true
		}
		if kind == "tool_call_ready" {
			if call.readySeen {
				call.invalid = true
			}
			call.readySeen = true
			call.ready = at
			call.readySeq = seq
		} else {
			if call.finishedSeen {
				call.invalid = true
			}
			call.finishedSeen = true
			call.finished = at
			call.finishedSeq = seq
		}
	}
	result := ToolWallTimeV1{CallCount: uint64(len(calls))}
	for _, call := range calls {
		if call.invalid || !call.readySeen || !call.finishedSeen || call.readySeq >= call.finishedSeq || call.finished.Before(call.ready) {
			continue
		}
		if err := addConversationUnsignedV1(&result.ReportedValue, uint64(call.finished.Sub(call.ready).Milliseconds())); err != nil {
			return ToolWallTimeV1{}, err
		}
		result.KnownCallCount++
	}
	result.Complete = result.KnownCallCount == result.CallCount
	return result, nil
}

// ProviderInventoryDiagnostics proves the observation list is the entire begun
// attempt inventory. Pending attempts invalidate old complete-only diagnostics;
// their reported numeric subtotals remain available through conversationUsage.
func ProviderInventoryDiagnostics(inventories []appcache.TurnInventoryV1) map[string]any {
	observations := []domaincache.ProviderCallObservationV1{}
	pending := false
	for _, inventory := range inventories {
		billing, err := inventory.BillingV1()
		if err != nil {
			result := invalidProviderAttemptDiagnostics()
			result["providerAttemptInventoryComplete"] = false
			return result
		}
		pending = pending || billing.SettledAttemptCount != billing.AttemptCount
		if err := inventory.VisitAttemptsV1(func(_ domaincache.ProviderAttemptIntentV1, settlement *domaincache.ProviderAttemptSettlementV1) error {
			if settlement != nil {
				output := settlement.Observation.Usage
				pending = pending || output.OutputTokens.Known && output.OutputTokensFinality != domaincache.ProviderOutputTokensFinalV1
				observations = append(observations, settlement.Observation)
			}
			return nil
		}); err != nil {
			result := invalidProviderAttemptDiagnostics()
			result["providerAttemptInventoryComplete"] = false
			return result
		}
	}
	if pending {
		result := invalidProviderAttemptDiagnostics()
		result["providerAttemptInventoryComplete"] = true
		return result
	}
	if len(observations) == 0 {
		return nil
	}
	result := ProviderAttemptDiagnostics(observations)
	// Coverage is independent of cache-namespace comparability and presence.
	result["providerAttemptInventoryComplete"] = true
	return result
}
