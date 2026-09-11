package usage

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	MaxDays        = 370
	SourceTurn     = "turn"
	SourceSubagent = "subagent"
)

type Snapshot struct {
	PromptTokens              int      `json:"promptTokens"`
	CompletionTokens          int      `json:"completionTokens"`
	ReasoningTokens           int      `json:"reasoningTokens"`
	TotalTokens               int      `json:"totalTokens"`
	CachedTokens              int      `json:"cachedTokens"`
	CacheHitTokens            int      `json:"cacheHitTokens"`
	CacheMissTokens           int      `json:"cacheMissTokens"`
	HasCacheHitTokens         bool     `json:"hasCacheHitTokens"`
	HasCacheMissTokens        bool     `json:"hasCacheMissTokens"`
	CacheHitRate              *float64 `json:"cacheHitRate"`
	CacheableTokenHitRate     *float64 `json:"cacheableTokenHitRate"`
	TotalInputTokenHitRate    *float64 `json:"totalInputTokenHitRate"`
	CacheMissReasons          []string `json:"cacheMissReasons,omitempty"`
	CacheSuggestions          []string `json:"cacheSuggestions,omitempty"`
	Turns                     int      `json:"turns"`
	CostUSD                   float64  `json:"costUsd"`
	CostCNY                   float64  `json:"costCny"`
	PriceConfigured           bool     `json:"priceConfigured"`
	CacheSavingsUSD           float64  `json:"cacheSavingsUsd"`
	CacheSavingsCNY           float64  `json:"cacheSavingsCny"`
	TokenEconomySavingsTokens int      `json:"tokenEconomySavingsTokens"`
	TokenEconomySavingsUSD    float64  `json:"tokenEconomySavingsUsd"`
	TokenEconomySavingsCNY    float64  `json:"tokenEconomySavingsCny"`
}

type Record struct {
	ThreadID    string
	TurnID      string
	Model       string
	Provider    string
	CompletedAt string
	UsageSource string
	ChildRunID  string
	Usage       Snapshot
}

type Window struct {
	From     string
	To       string
	Timezone string
	Location *time.Location
	Days     int
}

type WindowParams struct {
	From     string
	To       string
	Window   string
	Timezone string
	Label    string
	Now      time.Time
}

type counters struct {
	InputTokens               int
	OutputTokens              int
	ReasoningTokens           int
	CachedTokens              int
	CacheMissTokens           int
	TotalTokens               int
	CostUSD                   float64
	CostCNY                   float64
	PriceConfigured           bool
	CacheSavingsUSD           float64
	CacheSavingsCNY           float64
	TokenEconomySavingsTokens int
	TokenEconomySavingsUSD    float64
	TokenEconomySavingsCNY    float64
	Turns                     int
	ThreadIDs                 map[string]bool
	HasCacheTelemetry         bool
	LatestCompletedAt         string
	LastTurnCacheHitRate      *float64
	LastTurnCacheableHitRate  *float64
	LastTurnTotalInputHitRate *float64
	LastCacheMissReasons      []string
	LastCacheSuggestions      []string
	LatestProvider            string
	ProviderConflict          bool
}

func ParseWindow(params WindowParams) (Window, error) {
	timezone := strings.TrimSpace(params.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return Window{}, errors.New("invalid timezone: " + timezone)
	}
	label := strings.TrimSpace(params.Label)
	if label == "" {
		label = "usage"
	}
	from := strings.TrimSpace(params.From)
	to := strings.TrimSpace(params.To)
	if (from == "") != (to == "") {
		return Window{}, errors.New(label + " requires both from and to")
	}
	if from == "" && to == "" {
		window := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(params.Window), "-", "_"))
		if window == "" {
			return Window{}, errors.New(label + " requires from and to")
		}
		var days int
		switch window {
		case "today":
			days = 1
		case "week":
			days = 7
		case "month":
			days = 30
		case "all", "all_time", "alltime":
			days = MaxDays
		default:
			return Window{}, errors.New("unsupported usage window: " + window)
		}
		now := params.Now
		if now.IsZero() {
			now = time.Now()
		}
		toDate := now.In(location)
		from = toDate.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
		to = toDate.Format("2006-01-02")
	}
	if _, err := ParseDate(from, location, "from"); err != nil {
		return Window{}, err
	}
	if _, err := ParseDate(to, location, "to"); err != nil {
		return Window{}, err
	}
	days, err := InclusiveDayCount(from, to)
	if err != nil {
		return Window{}, err
	}
	if days <= 0 {
		return Window{}, errors.New("from must be on or before to")
	}
	if days > MaxDays {
		return Window{}, errors.New("daily usage range must be 370 days or less")
	}
	return Window{From: from, To: to, Timezone: timezone, Location: location, Days: days}, nil
}

func InclusiveDayCount(from string, to string) (int, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return 0, errors.New("from must be a valid calendar date")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return 0, errors.New("to must be a valid calendar date")
	}
	return int(end.Sub(start).Hours()/24) + 1, nil
}

func ParseDate(value string, location *time.Location, field string) (time.Time, error) {
	if location == nil {
		location = time.UTC
	}
	if len(value) != len("2006-01-02") {
		return time.Time{}, errors.New(field + " must use YYYY-MM-DD")
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, errors.New(field + " must be a valid calendar date")
	}
	return parsed, nil
}

func Day(isoTimestamp string, location *time.Location) string {
	if location == nil {
		location = time.UTC
	}
	timestamp, err := time.Parse(time.RFC3339Nano, isoTimestamp)
	if err != nil {
		return ""
	}
	return timestamp.In(location).Format("2006-01-02")
}

func RuntimeResponse(records []Record, threadIDs []string) map[string]any {
	total := SumRecords(records)
	recordsByThread := map[string][]Record{}
	for _, record := range records {
		recordsByThread[record.ThreadID] = append(recordsByThread[record.ThreadID], record)
	}
	perThread := make([]any, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		threadID = strings.TrimSpace(threadID)
		if threadID == "" {
			continue
		}
		perThread = append(perThread, map[string]any{
			"threadId": threadID,
			"usage":    SumRecords(recordsByThread[threadID]).Map(),
		})
	}
	return map[string]any{
		"total":     total.Map(),
		"perThread": perThread,
		"bySource":  BySourceResponse(records),
	}
}

func BySourceResponse(records []Record) []any {
	buckets := map[string][]Record{}
	childRunIDs := map[string]map[string]bool{}
	for _, record := range records {
		source := RecordSource(record)
		buckets[source] = append(buckets[source], record)
		childRunID := strings.TrimSpace(record.ChildRunID)
		if childRunID == "" {
			continue
		}
		if childRunIDs[source] == nil {
			childRunIDs[source] = map[string]bool{}
		}
		childRunIDs[source][childRunID] = true
	}
	sources := make([]string, 0, len(buckets))
	for source := range buckets {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	out := make([]any, 0, len(sources))
	for _, source := range sources {
		ids := make([]string, 0, len(childRunIDs[source]))
		for id := range childRunIDs[source] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		out = append(out, map[string]any{
			"source":      source,
			"usage":       SumRecords(buckets[source]).Map(),
			"childRunIds": stringListAny(ids),
		})
	}
	return out
}

func RecordSource(record Record) string {
	source := strings.TrimSpace(record.UsageSource)
	if source == "" {
		return SourceTurn
	}
	return source
}

func LooksCumulativeUsage(current Snapshot, previous Snapshot) bool {
	return current.Turns > previous.Turns &&
		current.TotalTokens >= previous.TotalTokens &&
		current.PromptTokens >= previous.PromptTokens &&
		current.CompletionTokens >= previous.CompletionTokens
}

func RecordModel(thread map[string]any, event map[string]any) string {
	turnID := strings.TrimSpace(stringField(event, "turnId"))
	if turnID != "" {
		for _, turn := range listAny(thread["turns"]) {
			turnMap, _ := turn.(map[string]any)
			if turnMap != nil && stringField(turnMap, "id") == turnID {
				if model := strings.TrimSpace(stringField(turnMap, "model")); model != "" {
					return model
				}
			}
		}
	}
	return firstNonEmptyAnyString(thread["model"], "unknown")
}

func RecordProvider(thread map[string]any, event map[string]any) string {
	if diagnostics, ok := event["cacheDiagnostics"].(map[string]any); ok {
		if provider := firstNonEmptyAnyString(diagnostics["providerId"], diagnostics["provider"]); provider != "" {
			return provider
		}
	}
	return firstNonEmptyAnyString(thread["providerId"], "unknown")
}

func DailyResponse(records []Record, window Window) map[string]any {
	window = normalizeWindow(window)
	buckets := map[string]*counters{}
	orderedDays := []string{}
	start, _ := ParseDate(window.From, window.Location, "from")
	for offset := 0; offset < window.Days; offset++ {
		day := start.AddDate(0, 0, offset).Format("2006-01-02")
		orderedDays = append(orderedDays, day)
		buckets[day] = emptyCounters()
	}
	for _, record := range records {
		day := Day(record.CompletedAt, window.Location)
		bucket := buckets[day]
		if bucket == nil {
			continue
		}
		bucket.Add(record)
	}
	out := make([]any, 0, len(orderedDays))
	total := emptyCounters()
	activeDays := 0
	for _, day := range orderedDays {
		bucket := buckets[day]
		if bucket.HasActivity() {
			activeDays++
		}
		total.AddCounters(bucket)
		out = append(out, bucket.DailyMap(day))
	}
	totals := total.DailyTotalsMap(window.Days, activeDays)
	return map[string]any{
		"group_by": "day",
		"from":     window.From,
		"to":       window.To,
		"timezone": window.Timezone,
		"buckets":  out,
		"totals":   totals,
	}
}

func ThreadResponse(records []Record) map[string]any {
	buckets := map[string]*counters{}
	for _, record := range records {
		bucket := buckets[record.ThreadID]
		if bucket == nil {
			bucket = emptyCounters()
			buckets[record.ThreadID] = bucket
		}
		bucket.Add(record)
		if record.CompletedAt >= bucket.LatestCompletedAt {
			bucket.LatestCompletedAt = record.CompletedAt
			bucket.LastTurnCacheHitRate = record.Usage.CacheHitRate
			bucket.LastTurnCacheableHitRate = record.Usage.CacheableTokenHitRate
			bucket.LastTurnTotalInputHitRate = record.Usage.TotalInputTokenHitRate
			bucket.LastCacheMissReasons = append([]string(nil), record.Usage.CacheMissReasons...)
			bucket.LastCacheSuggestions = append([]string(nil), record.Usage.CacheSuggestions...)
			bucket.LatestProvider = record.Provider
		}
	}
	threadIDs := make([]string, 0, len(buckets))
	for threadID := range buckets {
		threadIDs = append(threadIDs, threadID)
	}
	sort.Slice(threadIDs, func(i, j int) bool {
		left := buckets[threadIDs[i]]
		right := buckets[threadIDs[j]]
		if left.TotalTokens == right.TotalTokens {
			return threadIDs[i] < threadIDs[j]
		}
		return left.TotalTokens > right.TotalTokens
	})
	out := make([]any, 0, len(threadIDs))
	total := emptyCounters()
	for _, threadID := range threadIDs {
		bucket := buckets[threadID]
		total.AddCounters(bucket)
		out = append(out, bucket.ThreadMap(threadID))
	}
	totals := total.ThreadTotalsMap(len(threadIDs))
	return map[string]any{
		"group_by": "thread",
		"buckets":  out,
		"totals":   totals,
	}
}

func ModelResponse(records []Record, window Window) map[string]any {
	window = normalizeWindow(window)
	dayBuckets := map[string]*counters{}
	orderedDays := []string{}
	start, _ := ParseDate(window.From, window.Location, "from")
	for offset := 0; offset < window.Days; offset++ {
		day := start.AddDate(0, 0, offset).Format("2006-01-02")
		orderedDays = append(orderedDays, day)
		dayBuckets[day] = emptyCounters()
	}
	modelBuckets := map[string]*counters{}
	for _, record := range records {
		day := Day(record.CompletedAt, window.Location)
		dayBucket := dayBuckets[day]
		if dayBucket == nil {
			continue
		}
		dayBucket.Add(record)
		model := strings.TrimSpace(record.Model)
		if model == "" {
			model = "unknown"
		}
		modelBucket := modelBuckets[model]
		if modelBucket == nil {
			modelBucket = emptyCounters()
			modelBuckets[model] = modelBucket
		}
		modelBucket.Add(record)
	}
	models := make([]string, 0, len(modelBuckets))
	for model := range modelBuckets {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		left := modelBuckets[models[i]]
		right := modelBuckets[models[j]]
		if left.TotalTokens == right.TotalTokens {
			return models[i] < models[j]
		}
		return left.TotalTokens > right.TotalTokens
	})
	modelOut := make([]any, 0, len(models))
	for _, model := range models {
		modelOut = append(modelOut, modelBuckets[model].ModelMap(model))
	}
	dayOut := make([]any, 0, len(orderedDays))
	total := emptyCounters()
	activeDays := 0
	for _, day := range orderedDays {
		bucket := dayBuckets[day]
		if bucket.HasActivity() {
			activeDays++
		}
		total.AddCounters(bucket)
		dayOut = append(dayOut, bucket.DailyMap(day))
	}
	return map[string]any{
		"group_by": "model",
		"from":     window.From,
		"to":       window.To,
		"timezone": window.Timezone,
		"buckets":  modelOut,
		"days":     dayOut,
		"totals":   total.DailyTotalsMap(window.Days, activeDays),
	}
}

func SnapshotFromMap(raw map[string]any) Snapshot {
	snapshot := Snapshot{
		PromptTokens:              intNumber(raw["promptTokens"]),
		CompletionTokens:          intNumber(raw["completionTokens"]),
		ReasoningTokens:           intNumber(raw["reasoningTokens"]),
		TotalTokens:               intNumber(raw["totalTokens"]),
		CachedTokens:              intNumber(raw["cachedTokens"]),
		CacheHitTokens:            intNumber(raw["cacheHitTokens"]),
		CacheMissTokens:           intNumber(raw["cacheMissTokens"]),
		HasCacheHitTokens:         hasNumber(raw, "cacheHitTokens"),
		HasCacheMissTokens:        hasNumber(raw, "cacheMissTokens"),
		CacheMissReasons:          stringList(firstNonNilAny(raw["cacheMissReasons"], raw["cache_miss_reasons"])),
		CacheSuggestions:          stringList(firstNonNilAny(raw["cacheSuggestions"], raw["cache_suggestions"])),
		Turns:                     intNumber(raw["turns"]),
		CostUSD:                   floatNumber(raw["costUsd"]),
		CostCNY:                   floatNumber(raw["costCny"]),
		PriceConfigured:           boolFromAny(raw["priceConfigured"]) || boolFromAny(raw["price_configured"]),
		CacheSavingsUSD:           floatNumber(raw["cacheSavingsUsd"]),
		CacheSavingsCNY:           floatNumber(raw["cacheSavingsCny"]),
		TokenEconomySavingsTokens: intNumber(raw["tokenEconomySavingsTokens"]),
		TokenEconomySavingsUSD:    floatNumber(raw["tokenEconomySavingsUsd"]),
		TokenEconomySavingsCNY:    floatNumber(raw["tokenEconomySavingsCny"]),
	}
	if snapshot.TotalTokens == 0 {
		snapshot.TotalTokens = snapshot.PromptTokens + snapshot.CompletionTokens
	}
	if rate, ok := finiteFloat(raw["cacheHitRate"]); ok {
		snapshot.CacheHitRate = &rate
	} else if snapshot.HasCacheHitTokens || snapshot.HasCacheMissTokens {
		total := snapshot.CacheHitTokens + snapshot.CacheMissTokens
		if total > 0 {
			rate := float64(snapshot.CacheHitTokens) / float64(total)
			snapshot.CacheHitRate = &rate
		}
	}
	if rate, ok := finiteFloat(firstNonNilAny(raw["cacheableTokenHitRate"], raw["cacheable_token_hit_rate"])); ok {
		snapshot.CacheableTokenHitRate = &rate
	} else if snapshot.HasCacheHitTokens || snapshot.HasCacheMissTokens {
		total := snapshot.CacheHitTokens + snapshot.CacheMissTokens
		if total > 0 {
			rate := float64(snapshot.CacheHitTokens) / float64(total)
			snapshot.CacheableTokenHitRate = &rate
		}
	}
	if rate, ok := finiteFloat(firstNonNilAny(raw["totalInputTokenHitRate"], raw["total_input_token_hit_rate"])); ok {
		snapshot.TotalInputTokenHitRate = &rate
	} else if (snapshot.HasCacheHitTokens || snapshot.HasCacheMissTokens) && snapshot.PromptTokens > 0 {
		total := snapshot.CacheHitTokens + snapshot.CacheMissTokens
		if total > 0 {
			rate := float64(snapshot.CacheHitTokens) / float64(snapshot.PromptTokens)
			snapshot.TotalInputTokenHitRate = &rate
		}
	}
	return snapshot
}

func (u Snapshot) Map() map[string]any {
	return map[string]any{
		"promptTokens":                   u.PromptTokens,
		"completionTokens":               u.CompletionTokens,
		"reasoningTokens":                u.ReasoningTokens,
		"totalTokens":                    u.TotalTokens,
		"cachedTokens":                   u.CachedSnapshotTokens(),
		"cacheHitTokens":                 u.CacheHitForCounters(),
		"cacheMissTokens":                u.CacheMissTokens,
		"cacheHitRate":                   nullableFloat(u.CacheHitRate),
		"cacheableTokenHitRate":          nullableFloat(u.CacheableTokenHitRate),
		"totalInputTokenHitRate":         nullableFloat(u.TotalInputTokenHitRate),
		"cacheMissReasons":               stringListAny(u.CacheMissReasons),
		"cacheSuggestions":               stringListAny(u.CacheSuggestions),
		"turns":                          u.Turns,
		"costUsd":                        u.CostUSD,
		"costCny":                        u.CostCNY,
		"priceConfigured":                u.PriceConfigured,
		"cacheSavingsUsd":                u.CacheSavingsUSD,
		"cacheSavingsCny":                u.CacheSavingsCNY,
		"tokenEconomySavingsTokens":      u.TokenEconomySavingsTokens,
		"tokenEconomySavingsUsd":         u.TokenEconomySavingsUSD,
		"tokenEconomySavingsCny":         u.TokenEconomySavingsCNY,
		"last_turn_cache_hit_rate":       nullableFloat(u.CacheHitRate),
		"last_turn_cacheable_hit_rate":   nullableFloat(u.CacheableTokenHitRate),
		"last_turn_total_input_hit_rate": nullableFloat(u.TotalInputTokenHitRate),
		"last_cache_miss_reasons":        stringListAny(u.CacheMissReasons),
		"last_cache_suggestions":         stringListAny(u.CacheSuggestions),
		"lastTurnCacheHitRate":           nullableFloat(u.CacheHitRate),
		"cache_hit_tokens":               u.CacheHitForCounters(),
		"cache_miss_tokens":              u.CacheMissTokens,
		"input_tokens":                   u.PromptTokens,
		"output_tokens":                  u.CompletionTokens,
		"reasoning_tokens":               u.ReasoningTokens,
		"total_tokens":                   u.TotalTokens,
		"cost_usd":                       u.CostUSD,
		"cost_cny":                       u.CostCNY,
		"price_configured":               u.PriceConfigured,
		"token_economy_savings_tokens":   u.TokenEconomySavingsTokens,
	}
}

func (u Snapshot) CachedSnapshotTokens() int {
	if u.HasCacheHitTokens {
		return u.CacheHitTokens
	}
	return u.CachedTokens
}

func (u Snapshot) CacheHitForCounters() int {
	if u.HasCacheHitTokens {
		return u.CacheHitTokens
	}
	return 0
}

func (u Snapshot) HasUsage() bool {
	return u.PromptTokens > 0 ||
		u.CompletionTokens > 0 ||
		u.ReasoningTokens > 0 ||
		u.TotalTokens > 0 ||
		u.CachedSnapshotTokens() > 0 ||
		u.CacheMissTokens > 0 ||
		u.Turns > 0 ||
		u.CostUSD > 0 ||
		u.CostCNY > 0 ||
		u.CacheSavingsUSD > 0 ||
		u.CacheSavingsCNY > 0 ||
		u.TokenEconomySavingsTokens > 0 ||
		u.TokenEconomySavingsUSD > 0 ||
		u.TokenEconomySavingsCNY > 0
}

func (u Snapshot) Diff(previous Snapshot) Snapshot {
	delta := Snapshot{
		PromptTokens:              nonNegativeInt(u.PromptTokens - previous.PromptTokens),
		CompletionTokens:          nonNegativeInt(u.CompletionTokens - previous.CompletionTokens),
		ReasoningTokens:           nonNegativeInt(u.ReasoningTokens - previous.ReasoningTokens),
		TotalTokens:               nonNegativeInt(u.TotalTokens - previous.TotalTokens),
		CachedTokens:              nonNegativeInt(u.CachedTokens - previous.CachedTokens),
		CacheHitTokens:            nonNegativeInt(u.CacheHitTokens - previous.CacheHitTokens),
		CacheMissTokens:           nonNegativeInt(u.CacheMissTokens - previous.CacheMissTokens),
		HasCacheHitTokens:         u.HasCacheHitTokens || previous.HasCacheHitTokens,
		HasCacheMissTokens:        u.HasCacheMissTokens || previous.HasCacheMissTokens,
		CacheMissReasons:          append([]string(nil), u.CacheMissReasons...),
		CacheSuggestions:          append([]string(nil), u.CacheSuggestions...),
		Turns:                     nonNegativeInt(u.Turns - previous.Turns),
		CostUSD:                   nonNegativeFloat(u.CostUSD - previous.CostUSD),
		CostCNY:                   nonNegativeFloat(u.CostCNY - previous.CostCNY),
		PriceConfigured:           u.PriceConfigured || previous.PriceConfigured,
		CacheSavingsUSD:           nonNegativeFloat(u.CacheSavingsUSD - previous.CacheSavingsUSD),
		CacheSavingsCNY:           nonNegativeFloat(u.CacheSavingsCNY - previous.CacheSavingsCNY),
		TokenEconomySavingsTokens: nonNegativeInt(u.TokenEconomySavingsTokens - previous.TokenEconomySavingsTokens),
		TokenEconomySavingsUSD:    nonNegativeFloat(u.TokenEconomySavingsUSD - previous.TokenEconomySavingsUSD),
		TokenEconomySavingsCNY:    nonNegativeFloat(u.TokenEconomySavingsCNY - previous.TokenEconomySavingsCNY),
	}
	if delta.TotalTokens == 0 {
		delta.TotalTokens = delta.PromptTokens + delta.CompletionTokens
	}
	total := delta.CacheHitForCounters() + delta.CacheMissTokens
	if (delta.HasCacheHitTokens || delta.HasCacheMissTokens) && total > 0 {
		rate := float64(delta.CacheHitForCounters()) / float64(total)
		delta.CacheHitRate = &rate
		delta.CacheableTokenHitRate = &rate
	}
	if (delta.HasCacheHitTokens || delta.HasCacheMissTokens) && delta.PromptTokens > 0 && total > 0 {
		rate := float64(delta.CacheHitForCounters()) / float64(delta.PromptTokens)
		delta.TotalInputTokenHitRate = &rate
	}
	return delta
}

func SumRecords(records []Record) Snapshot {
	var total Snapshot
	hasTelemetry := false
	for _, record := range records {
		total.PromptTokens += record.Usage.PromptTokens
		total.CompletionTokens += record.Usage.CompletionTokens
		total.ReasoningTokens += record.Usage.ReasoningTokens
		total.TotalTokens += record.Usage.TotalTokens
		total.CachedTokens += record.Usage.CachedSnapshotTokens()
		total.CacheHitTokens += record.Usage.CacheHitForCounters()
		total.CacheMissTokens += record.Usage.CacheMissTokens
		total.CostUSD += record.Usage.CostUSD
		total.CostCNY += record.Usage.CostCNY
		total.PriceConfigured = total.PriceConfigured || record.Usage.PriceConfigured
		total.CacheSavingsUSD += record.Usage.CacheSavingsUSD
		total.CacheSavingsCNY += record.Usage.CacheSavingsCNY
		total.TokenEconomySavingsTokens += record.Usage.TokenEconomySavingsTokens
		total.TokenEconomySavingsUSD += record.Usage.TokenEconomySavingsUSD
		total.TokenEconomySavingsCNY += record.Usage.TokenEconomySavingsCNY
		total.Turns += record.Usage.Turns
		total.CacheMissReasons = unionStrings(total.CacheMissReasons, record.Usage.CacheMissReasons)
		total.CacheSuggestions = unionStrings(total.CacheSuggestions, record.Usage.CacheSuggestions)
		hasTelemetry = hasTelemetry || record.Usage.HasCacheHitTokens || record.Usage.HasCacheMissTokens
	}
	total.HasCacheHitTokens = hasTelemetry
	total.HasCacheMissTokens = hasTelemetry
	if hasTelemetry {
		cacheTotal := total.CacheHitForCounters() + total.CacheMissTokens
		if cacheTotal > 0 {
			rate := float64(total.CacheHitForCounters()) / float64(cacheTotal)
			total.CacheHitRate = &rate
			total.CacheableTokenHitRate = &rate
		}
		if total.PromptTokens > 0 && cacheTotal > 0 {
			rate := float64(total.CacheHitForCounters()) / float64(total.PromptTokens)
			total.TotalInputTokenHitRate = &rate
		}
	}
	return total
}

func normalizeWindow(window Window) Window {
	if window.Location == nil {
		window.Location = time.UTC
	}
	if strings.TrimSpace(window.Timezone) == "" {
		window.Timezone = "UTC"
	}
	return window
}

func emptyCounters() *counters {
	return &counters{ThreadIDs: map[string]bool{}}
}

func (c *counters) Add(record Record) {
	usage := record.Usage
	c.InputTokens += usage.PromptTokens
	c.OutputTokens += usage.CompletionTokens
	c.ReasoningTokens += usage.ReasoningTokens
	c.CachedTokens += usage.CacheHitForCounters()
	c.CacheMissTokens += usage.CacheMissTokens
	c.TotalTokens += usage.TotalTokens
	c.CostUSD += usage.CostUSD
	c.CostCNY += usage.CostCNY
	c.PriceConfigured = c.PriceConfigured || usage.PriceConfigured
	c.CacheSavingsUSD += usage.CacheSavingsUSD
	c.CacheSavingsCNY += usage.CacheSavingsCNY
	c.TokenEconomySavingsTokens += usage.TokenEconomySavingsTokens
	c.TokenEconomySavingsUSD += usage.TokenEconomySavingsUSD
	c.TokenEconomySavingsCNY += usage.TokenEconomySavingsCNY
	c.Turns += usage.Turns
	if record.ThreadID != "" {
		c.ThreadIDs[record.ThreadID] = true
	}
	c.HasCacheTelemetry = c.HasCacheTelemetry || usage.HasCacheHitTokens || usage.HasCacheMissTokens
	if record.Provider != "" {
		if c.LatestProvider == "" {
			c.LatestProvider = record.Provider
		} else if c.LatestProvider != record.Provider {
			c.ProviderConflict = true
		}
	}
}

func (c *counters) AddCounters(other *counters) {
	c.InputTokens += other.InputTokens
	c.OutputTokens += other.OutputTokens
	c.ReasoningTokens += other.ReasoningTokens
	c.CachedTokens += other.CachedTokens
	c.CacheMissTokens += other.CacheMissTokens
	c.TotalTokens += other.TotalTokens
	c.CostUSD += other.CostUSD
	c.CostCNY += other.CostCNY
	c.PriceConfigured = c.PriceConfigured || other.PriceConfigured
	c.CacheSavingsUSD += other.CacheSavingsUSD
	c.CacheSavingsCNY += other.CacheSavingsCNY
	c.TokenEconomySavingsTokens += other.TokenEconomySavingsTokens
	c.TokenEconomySavingsUSD += other.TokenEconomySavingsUSD
	c.TokenEconomySavingsCNY += other.TokenEconomySavingsCNY
	c.Turns += other.Turns
	for threadID := range other.ThreadIDs {
		c.ThreadIDs[threadID] = true
	}
	c.HasCacheTelemetry = c.HasCacheTelemetry || other.HasCacheTelemetry
}

func (c *counters) HasActivity() bool {
	return c.TotalTokens > 0 || c.Turns > 0 || c.CostUSD > 0 || c.CostCNY > 0 || c.TokenEconomySavingsTokens > 0
}

func (c *counters) cacheHitRate() any {
	if !c.HasCacheTelemetry {
		return nil
	}
	total := c.CachedTokens + c.CacheMissTokens
	if total <= 0 {
		return nil
	}
	return float64(c.CachedTokens) / float64(total)
}

func (c *counters) countersMap() map[string]any {
	return map[string]any{
		"input_tokens":                 c.InputTokens,
		"output_tokens":                c.OutputTokens,
		"reasoning_tokens":             c.ReasoningTokens,
		"cached_tokens":                c.CachedTokens,
		"cache_hit_tokens":             c.CachedTokens,
		"cache_miss_tokens":            c.CacheMissTokens,
		"total_tokens":                 c.TotalTokens,
		"cost_usd":                     c.CostUSD,
		"cost_cny":                     c.CostCNY,
		"price_configured":             c.PriceConfigured,
		"cache_savings_usd":            c.CacheSavingsUSD,
		"cache_savings_cny":            c.CacheSavingsCNY,
		"token_economy_savings_tokens": c.TokenEconomySavingsTokens,
		"token_economy_savings_usd":    c.TokenEconomySavingsUSD,
		"token_economy_savings_cny":    c.TokenEconomySavingsCNY,
		"turns":                        c.Turns,
		"thread_count":                 len(c.ThreadIDs),
		"cache_hit_rate":               c.cacheHitRate(),
	}
}

func (c *counters) DailyMap(day string) map[string]any {
	out := c.countersMap()
	out["date"] = day
	return out
}

func (c *counters) ThreadMap(threadID string) map[string]any {
	out := c.countersMap()
	delete(out, "thread_count")
	out["thread_id"] = threadID
	out["last_turn_cache_hit_rate"] = nullableFloat(c.LastTurnCacheHitRate)
	out["last_turn_cacheable_hit_rate"] = nullableFloat(c.LastTurnCacheableHitRate)
	out["last_turn_total_input_hit_rate"] = nullableFloat(c.LastTurnTotalInputHitRate)
	out["last_cache_miss_reasons"] = stringListAny(c.LastCacheMissReasons)
	out["last_cache_suggestions"] = stringListAny(c.LastCacheSuggestions)
	out["provider"] = c.providerValue()
	return out
}

func (c *counters) ModelMap(model string) map[string]any {
	out := c.countersMap()
	out["model"] = model
	out["provider"] = c.providerValue()
	return out
}

func (c *counters) DailyTotalsMap(days int, activeDays int) map[string]any {
	out := c.countersMap()
	out["days"] = days
	out["active_days"] = activeDays
	return out
}

func (c *counters) ThreadTotalsMap(threadCount int) map[string]any {
	out := c.countersMap()
	out["thread_count"] = threadCount
	return out
}

func (c *counters) providerValue() string {
	if c.ProviderConflict {
		return "mixed"
	}
	if strings.TrimSpace(c.LatestProvider) == "" {
		return "unknown"
	}
	return c.LatestProvider
}

func firstNonNilAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func unionStrings(left []string, right []string) []string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	out := make([]string, 0, len(left)+len(right))
	seen := map[string]bool{}
	for _, value := range append(append([]string{}, left...), right...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func intNumber(value any) int {
	number, ok := finiteFloat(value)
	if !ok || number <= 0 {
		return 0
	}
	return int(number)
}

func floatNumber(value any) float64 {
	number, ok := finiteFloat(value)
	if !ok || number <= 0 {
		return 0
	}
	return number
}

func finiteFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
			return typed, true
		}
	case float32:
		number := float64(typed)
		if !math.IsNaN(number) && !math.IsInf(number, 0) {
			return number, true
		}
	case json.Number:
		number, err := typed.Float64()
		if err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
			return number, true
		}
	}
	return 0, false
}

func hasNumber(record map[string]any, key string) bool {
	_, ok := finiteFloat(record[key])
	return ok
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func listAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return []any{}
}

func stringList(value any) []string {
	raw := []any{}
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		raw = make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}
