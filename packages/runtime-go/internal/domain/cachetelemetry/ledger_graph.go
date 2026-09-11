package cachetelemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	providerIntentSetDigestDomain     = "analytix/provider-attempt-intent-set/v1\x00"
	providerSettlementSetDigestDomain = "analytix/provider-attempt-settlement-set/v1\x00"
	providerAggregateDigestDomain     = "analytix/provider-cache-aggregate/v1\x00"
)

type ProviderLedgerTokenAggregateV1 struct {
	Complete bool   `json:"complete"`
	Value    uint64 `json:"value"`
}

type ProviderLedgerDispatchCountsV1 struct {
	NotSent       uint32 `json:"notSent"`
	Sent          uint32 `json:"sent"`
	Indeterminate uint32 `json:"indeterminate"`
}

type ProviderLedgerStatusCountsV1 struct {
	Succeeded          uint32 `json:"succeeded"`
	Failed             uint32 `json:"failed"`
	Cancelled          uint32 `json:"cancelled"`
	TimedOut           uint32 `json:"timedOut"`
	StreamAborted      uint32 `json:"streamAborted"`
	RestartInterrupted uint32 `json:"restartInterrupted"`
}

type ProviderLedgerUsageAggregateV1 struct {
	InputTokens     ProviderLedgerTokenAggregateV1 `json:"inputTokens"`
	OutputTokens    ProviderLedgerTokenAggregateV1 `json:"outputTokens"`
	CacheHitTokens  ProviderLedgerTokenAggregateV1 `json:"cacheHitTokens"`
	CacheMissTokens ProviderLedgerTokenAggregateV1 `json:"cacheMissTokens"`
	ReasoningTokens ProviderLedgerTokenAggregateV1 `json:"reasoningTokens"`
}

// ProviderLedgerNamespaceV1 is the smallest cache-compatible namespace. A
// cache rate must never combine observations whose namespace differs.
type ProviderLedgerNamespaceV1 struct {
	UsageSource         ProviderUsageSourceV1 `json:"usageSource"`
	ChildRunHMAC        string                `json:"childRunHmac"`
	Channel             ProviderChannelV1     `json:"channel"`
	ProviderFamily      ProviderFamilyV1      `json:"providerFamily"`
	ModelHMAC           string                `json:"modelHmac"`
	EndpointFormat      EndpointFormatV1      `json:"endpointFormat"`
	EndpointHMAC        string                `json:"endpointHmac"`
	CredentialScopeHMAC string                `json:"credentialScopeHmac"`
	ProviderConfigHMAC  string                `json:"providerConfigHmac"`
	DigestEpoch         uint64                `json:"digestEpoch"`
}

type ProviderLedgerNamespaceAggregateV1 struct {
	Namespace      ProviderLedgerNamespaceV1      `json:"namespace"`
	AttemptCount   uint32                         `json:"attemptCount"`
	DispatchCounts ProviderLedgerDispatchCountsV1 `json:"dispatchCounts"`
	StatusCounts   ProviderLedgerStatusCountsV1   `json:"statusCounts"`
	Usage          ProviderLedgerUsageAggregateV1 `json:"usage"`
}

type providerLogicalCallV1 struct {
	ordinal        uint32
	channelKey     string
	channelOrdinal uint32
	shape          CacheVisibleShapeV1
	wireHeaders    string
	attempts       []ProviderAttemptIntentV1
}

func ValidateProviderTurnClosureForInventoryV1(closure ProviderTurnClosureV1, intents []ProviderAttemptIntentV1, settlements []ProviderAttemptSettlementV1) error {
	if err := ValidateProviderTurnClosureV1(closure); err != nil {
		return err
	}
	closedAt, _ := parseCanonicalTimestamp(closure.ClosedAt)
	intentSetDigest, settlementSetDigest, aggregateDigest, eligible, err := providerLedgerInventoryDigestsV1(
		closure.TurnBindingHMAC, intents, settlements, closedAt,
	)
	if err != nil {
		return err
	}
	eligible = eligible && providerTurnTerminalReasonAllowsBenchmarkV1(closure.TerminalReasonCode)
	if closure.IntentCount != uint32(len(intents)) || closure.SettlementCount != uint32(len(settlements)) ||
		closure.IntentSetDigest != intentSetDigest || closure.SettlementSetDigest != settlementSetDigest ||
		closure.AggregateDigest != aggregateDigest || closure.BenchmarkEligible != eligible {
		return errors.New("provider turn closure does not bind the exact inventory")
	}
	for _, intent := range intents {
		if intent.AuthorityKeyID != closure.AuthorityKeyID || intent.AuthorityPublicKey != closure.AuthorityPublicKey {
			return errors.New("provider turn closure authority differs from intent inventory")
		}
	}
	for _, settlement := range settlements {
		if settlement.AuthorityKeyID != closure.AuthorityKeyID || settlement.AuthorityPublicKey != closure.AuthorityPublicKey {
			return errors.New("provider turn closure authority differs from settlement inventory")
		}
	}
	return nil
}

func BuildProviderLedgerNamespaceAggregatesV1(intents []ProviderAttemptIntentV1, settlements []ProviderAttemptSettlementV1) ([]ProviderLedgerNamespaceAggregateV1, error) {
	if len(intents) == 0 && len(settlements) == 0 {
		return []ProviderLedgerNamespaceAggregateV1{}, nil
	}
	turnBinding := ""
	if len(intents) > 0 {
		turnBinding = intents[0].TurnBindingHMAC
	} else {
		turnBinding = settlements[0].TurnBindingHMAC
	}
	_, _, _, _, aggregates, err := validateAndAggregateProviderLedgerInventoryV1(turnBinding, intents, settlements, time.Time{})
	return aggregates, err
}

func providerLedgerInventoryDigestsV1(turnBinding string, intents []ProviderAttemptIntentV1, settlements []ProviderAttemptSettlementV1, closedAt time.Time) (string, string, string, bool, error) {
	intentIDs, settlementIDs, eligible, _, aggregates, err := validateAndAggregateProviderLedgerInventoryV1(turnBinding, intents, settlements, closedAt)
	if err != nil {
		return "", "", "", false, err
	}
	intentBytes, _ := json.Marshal(intentIDs)
	settlementBytes, _ := json.Marshal(settlementIDs)
	aggregateBytes, _ := json.Marshal(aggregates)
	return domainsecurity.SHA256Hex(append([]byte(providerIntentSetDigestDomain), intentBytes...)),
		domainsecurity.SHA256Hex(append([]byte(providerSettlementSetDigestDomain), settlementBytes...)),
		domainsecurity.SHA256Hex(append([]byte(providerAggregateDigestDomain), aggregateBytes...)), eligible, nil
}

// ValidateProviderLedgerOpenInventoryV1 validates a crash-recoverable ledger
// before turn closure. The final physical attempt may be unsettled, but a
// later attempt cannot start until its predecessor has a settlement.
func ValidateProviderLedgerOpenInventoryV1(turnBinding string, intents []ProviderAttemptIntentV1, settlements []ProviderAttemptSettlementV1) error {
	_, _, _, err := validateProviderLedgerGraphV1(turnBinding, intents, settlements)
	return err
}

func validateAndAggregateProviderLedgerInventoryV1(turnBinding string, intents []ProviderAttemptIntentV1, settlements []ProviderAttemptSettlementV1, closedAt time.Time) ([]string, []string, bool, map[string]ProviderAttemptSettlementV1, []ProviderLedgerNamespaceAggregateV1, error) {
	intentIDs, settlementIDs, settlementByIntent, err := validateProviderLedgerGraphV1(turnBinding, intents, settlements)
	if err != nil {
		return nil, nil, false, nil, nil, err
	}
	if len(settlementByIntent) != len(intents) {
		return nil, nil, false, nil, nil, errors.New("provider turn closure requires every intent to be settled")
	}
	if !closedAt.IsZero() {
		for _, settlement := range settlementByIntent {
			settledAt, _ := parseCanonicalTimestamp(settlement.Observation.SettledAt)
			if settledAt.After(closedAt) {
				return nil, nil, false, nil, nil, errors.New("provider settlement follows turn closure")
			}
		}
	}
	aggregates, eligible, err := aggregateProviderLedgerInventoryV1(intents, settlementByIntent)
	if err != nil {
		return nil, nil, false, nil, nil, err
	}
	return intentIDs, settlementIDs, eligible, settlementByIntent, aggregates, nil
}

func validateProviderLedgerGraphV1(turnBinding string, intents []ProviderAttemptIntentV1, settlements []ProviderAttemptSettlementV1) ([]string, []string, map[string]ProviderAttemptSettlementV1, error) {
	if !isLowerHexDigest(turnBinding) {
		return nil, nil, nil, errors.New("provider ledger turn binding is invalid")
	}
	if len(intents) > int(maxProviderLedgerOrdinal) || len(settlements) > int(maxProviderLedgerOrdinal) {
		return nil, nil, nil, errors.New("provider ledger inventory exceeds its bound")
	}
	intentByID := make(map[string]ProviderAttemptIntentV1, len(intents))
	logicalCalls := make(map[string]*providerLogicalCallV1)
	logicalOrdinalOwner := make(map[uint32]string)
	channelOrdinalOwner := make(map[string]map[uint32]string)
	intentIDs := make([]string, 0, len(intents))
	for _, intent := range intents {
		if err := ValidateProviderAttemptIntentV1(intent); err != nil {
			return nil, nil, nil, err
		}
		if intent.TurnBindingHMAC != turnBinding {
			return nil, nil, nil, errors.New("provider intent belongs to another turn binding")
		}
		if _, exists := intentByID[intent.IntentID]; exists {
			return nil, nil, nil, errors.New("provider ledger contains a duplicate intent")
		}
		intentByID[intent.IntentID] = intent
		intentIDs = append(intentIDs, intent.IntentID)
		channelKey := providerLedgerChannelKeyV1(intent)
		logical, exists := logicalCalls[intent.Shape.LogicalCallHMAC]
		if !exists {
			logical = &providerLogicalCallV1{
				ordinal: intent.LogicalCallOrdinal, channelKey: channelKey, channelOrdinal: intent.ChannelOrdinal,
				shape: intent.Shape, wireHeaders: intent.WireHeadersHMAC,
			}
			logicalCalls[intent.Shape.LogicalCallHMAC] = logical
			if owner, occupied := logicalOrdinalOwner[intent.LogicalCallOrdinal]; occupied && owner != intent.Shape.LogicalCallHMAC {
				return nil, nil, nil, errors.New("provider logical call ordinal is reused")
			}
			logicalOrdinalOwner[intent.LogicalCallOrdinal] = intent.Shape.LogicalCallHMAC
			owners := channelOrdinalOwner[channelKey]
			if owners == nil {
				owners = make(map[uint32]string)
				channelOrdinalOwner[channelKey] = owners
			}
			if owner, occupied := owners[intent.ChannelOrdinal]; occupied && owner != intent.Shape.LogicalCallHMAC {
				return nil, nil, nil, errors.New("provider channel ordinal is reused")
			}
			owners[intent.ChannelOrdinal] = intent.Shape.LogicalCallHMAC
		} else if logical.ordinal != intent.LogicalCallOrdinal || logical.channelKey != channelKey ||
			logical.channelOrdinal != intent.ChannelOrdinal || logical.wireHeaders != intent.WireHeadersHMAC ||
			!sameProviderLogicalShapeV1(logical.shape, intent.Shape) {
			return nil, nil, nil, errors.New("provider physical attempts disagree on logical call authority")
		}
		logical.attempts = append(logical.attempts, intent)
	}
	if len(logicalCalls) != len(logicalOrdinalOwner) {
		return nil, nil, nil, errors.New("provider logical call inventory is inconsistent")
	}
	for ordinal := uint32(1); ordinal <= uint32(len(logicalCalls)); ordinal++ {
		if _, ok := logicalOrdinalOwner[ordinal]; !ok {
			return nil, nil, nil, errors.New("provider logical call ordinals are not contiguous")
		}
	}
	for _, owners := range channelOrdinalOwner {
		for ordinal := uint32(1); ordinal <= uint32(len(owners)); ordinal++ {
			if _, ok := owners[ordinal]; !ok {
				return nil, nil, nil, errors.New("provider channel ordinals are not contiguous")
			}
		}
	}
	settlementByIntent := make(map[string]ProviderAttemptSettlementV1, len(settlements))
	settlementIDSeen := make(map[string]struct{}, len(settlements))
	settlementIDs := make([]string, 0, len(settlements))
	for _, settlement := range settlements {
		intent, exists := intentByID[settlement.IntentID]
		if !exists {
			return nil, nil, nil, errors.New("provider settlement has no intent")
		}
		if err := ValidateProviderAttemptSettlementForIntentV1(settlement, intent); err != nil {
			return nil, nil, nil, err
		}
		if _, exists := settlementByIntent[settlement.IntentID]; exists {
			return nil, nil, nil, errors.New("provider intent has more than one settlement")
		}
		if _, exists := settlementIDSeen[settlement.SettlementID]; exists {
			return nil, nil, nil, errors.New("provider ledger contains a duplicate settlement")
		}
		settlementByIntent[settlement.IntentID] = settlement
		settlementIDSeen[settlement.SettlementID] = struct{}{}
		settlementIDs = append(settlementIDs, settlement.SettlementID)
	}
	for _, logical := range logicalCalls {
		sort.Slice(logical.attempts, func(i, j int) bool { return logical.attempts[i].Shape.Attempt < logical.attempts[j].Shape.Attempt })
		for index, intent := range logical.attempts {
			if intent.Shape.Attempt != uint32(index+1) {
				return nil, nil, nil, errors.New("provider physical attempt ordinals are not contiguous")
			}
			if index+1 < len(logical.attempts) {
				settlement, settled := settlementByIntent[intent.IntentID]
				if !settled {
					return nil, nil, nil, errors.New("provider retry starts before its prior attempt is settled")
				}
				settledAt, _ := parseCanonicalTimestamp(settlement.Observation.SettledAt)
				nextStartedAt, _ := parseCanonicalTimestamp(logical.attempts[index+1].Shape.StartedAt)
				if nextStartedAt.Before(settledAt) {
					return nil, nil, nil, errors.New("provider physical attempts overlap their settlements")
				}
			}
		}
	}
	sort.Strings(intentIDs)
	sort.Strings(settlementIDs)
	return intentIDs, settlementIDs, settlementByIntent, nil
}

func aggregateProviderLedgerInventoryV1(intents []ProviderAttemptIntentV1, settlementByIntent map[string]ProviderAttemptSettlementV1) ([]ProviderLedgerNamespaceAggregateV1, bool, error) {
	type group struct {
		key       string
		aggregate ProviderLedgerNamespaceAggregateV1
	}
	groups := make(map[string]*group)
	eligible := len(intents) > 0
	for _, intent := range intents {
		namespace := providerLedgerNamespaceV1(intent)
		namespaceBody, _ := json.Marshal(namespace)
		key := string(namespaceBody)
		current := groups[key]
		if current == nil {
			current = &group{key: key, aggregate: ProviderLedgerNamespaceAggregateV1{
				Namespace: namespace,
				Usage: ProviderLedgerUsageAggregateV1{
					InputTokens: providerLedgerEmptyTokenAggregateV1(), OutputTokens: providerLedgerEmptyTokenAggregateV1(),
					CacheHitTokens: providerLedgerEmptyTokenAggregateV1(), CacheMissTokens: providerLedgerEmptyTokenAggregateV1(),
					ReasoningTokens: providerLedgerEmptyTokenAggregateV1(),
				},
			}}
			groups[key] = current
		}
		settlement := settlementByIntent[intent.IntentID]
		current.aggregate.AttemptCount++
		addProviderDispatchCountV1(&current.aggregate.DispatchCounts, settlement.DispatchState)
		addProviderStatusCountV1(&current.aggregate.StatusCounts, settlement.Observation.Status)
		addProviderTokenAggregateV1(&current.aggregate.Usage.InputTokens, settlement.Observation.Usage.InputTokens)
		addProviderTokenAggregateV1(&current.aggregate.Usage.OutputTokens, settlement.Observation.Usage.OutputTokens)
		addProviderTokenAggregateV1(&current.aggregate.Usage.CacheHitTokens, settlement.Observation.Usage.CacheHitTokens)
		addProviderTokenAggregateV1(&current.aggregate.Usage.CacheMissTokens, settlement.Observation.Usage.CacheMissTokens)
		addProviderTokenAggregateV1(&current.aggregate.Usage.ReasoningTokens, settlement.Observation.Usage.ReasoningTokens)
		usage := settlement.Observation.Usage
		if settlement.DispatchState != ProviderDispatchStateSent || settlement.Observation.Status != ProviderCallStatusSucceeded ||
			!usage.InputTokens.Known || !usage.CacheHitTokens.Known || !usage.CacheMissTokens.Known {
			eligible = false
		}
	}
	ordered := make([]*group, 0, len(groups))
	for _, current := range groups {
		ordered = append(ordered, current)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].key < ordered[j].key })
	aggregates := make([]ProviderLedgerNamespaceAggregateV1, 0, len(ordered))
	for _, current := range ordered {
		if !current.aggregate.Usage.InputTokens.Complete || !current.aggregate.Usage.CacheHitTokens.Complete ||
			!current.aggregate.Usage.CacheMissTokens.Complete {
			eligible = false
		}
		aggregates = append(aggregates, current.aggregate)
	}
	return aggregates, eligible, nil
}

func providerLedgerNamespaceV1(intent ProviderAttemptIntentV1) ProviderLedgerNamespaceV1 {
	return ProviderLedgerNamespaceV1{
		UsageSource: intent.UsageSource, ChildRunHMAC: intent.ChildRunHMAC, Channel: intent.Channel,
		ProviderFamily: intent.Shape.ProviderFamily, ModelHMAC: intent.Shape.ModelHMAC,
		EndpointFormat: intent.Shape.Endpoint, EndpointHMAC: intent.Shape.EndpointHMAC,
		CredentialScopeHMAC: intent.Shape.CredentialScopeHMAC, ProviderConfigHMAC: intent.Shape.ProviderConfigHMAC,
		DigestEpoch: intent.Shape.DigestEpoch,
	}
}

func providerLedgerChannelKeyV1(intent ProviderAttemptIntentV1) string {
	body, _ := json.Marshal(struct {
		UsageSource  ProviderUsageSourceV1 `json:"usageSource"`
		ChildRunHMAC string                `json:"childRunHmac"`
		Channel      ProviderChannelV1     `json:"channel"`
	}{intent.UsageSource, intent.ChildRunHMAC, intent.Channel})
	return string(body)
}

func sameProviderLogicalShapeV1(left, right CacheVisibleShapeV1) bool {
	left.Attempt, right.Attempt = 0, 0
	left.StartedAt, right.StartedAt = "", ""
	return left == right
}

func providerLedgerEmptyTokenAggregateV1() ProviderLedgerTokenAggregateV1 {
	return ProviderLedgerTokenAggregateV1{Complete: true}
}

func addProviderTokenAggregateV1(aggregate *ProviderLedgerTokenAggregateV1, count TokenCountV1) {
	if !count.Known {
		aggregate.Complete = false
		return
	}
	if ^uint64(0)-aggregate.Value < count.Value {
		aggregate.Complete = false
		aggregate.Value = 0
		return
	}
	aggregate.Value += count.Value
}

func addProviderDispatchCountV1(counts *ProviderLedgerDispatchCountsV1, state ProviderDispatchStateV1) {
	switch state {
	case ProviderDispatchStateNotSent:
		counts.NotSent++
	case ProviderDispatchStateSent:
		counts.Sent++
	case ProviderDispatchStateIndeterminate:
		counts.Indeterminate++
	}
}

func addProviderStatusCountV1(counts *ProviderLedgerStatusCountsV1, status ProviderCallStatusV1) {
	switch status {
	case ProviderCallStatusSucceeded:
		counts.Succeeded++
	case ProviderCallStatusFailed:
		counts.Failed++
	case ProviderCallStatusCancelled:
		counts.Cancelled++
	case ProviderCallStatusTimedOut:
		counts.TimedOut++
	case ProviderCallStatusStreamAborted:
		counts.StreamAborted++
	case ProviderCallStatusRestartInterrupted:
		counts.RestartInterrupted++
	}
}

func (namespace ProviderLedgerNamespaceV1) Validate() error {
	if !namespace.UsageSource.valid() || !isLowerHexDigest(namespace.ChildRunHMAC) || !namespace.Channel.valid() ||
		!namespace.ProviderFamily.valid() || !isLowerHexDigest(namespace.ModelHMAC) || !namespace.EndpointFormat.valid() ||
		!isLowerHexDigest(namespace.EndpointHMAC) || !isLowerHexDigest(namespace.CredentialScopeHMAC) ||
		!isLowerHexDigest(namespace.ProviderConfigHMAC) || namespace.DigestEpoch == 0 {
		return fmt.Errorf("provider ledger namespace is invalid")
	}
	return nil
}
