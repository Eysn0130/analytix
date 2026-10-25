package processdisplay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const RawCaptureByteLimitV1 = 1 << 20
const CaptureBodyByteLimitV1 = 32 << 10
const ReasoningPreviewByteLimitV1 = 1024
const ToolInputJSONByteLimitV2 = 64 << 10

// InputV2 is a complete credential-safe copy of the actual accepted call.
// It has no effect, replay or export authority and is expanded-only.
type InputV2 struct {
	Availability string `json:"availability"`
	InputJSON    string `json:"inputJSON,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func ProjectToolInputV2(raw json.RawMessage, secrets ...string) (InputV2, string) {
	unavailable := InputV2{Availability: "unavailable", Reason: "missing"}
	if len(raw) == 0 {
		return unavailable, "withheld"
	}
	if len(raw) > ToolInputJSONByteLimitV2 {
		unavailable.Reason = "limit"
		return unavailable, "withheld"
	}
	if !utf8.Valid(raw) {
		return unavailable, "withheld"
	}
	decoded, err := domainsecurity.DecodeCanonicalJSONObject(raw)
	if err != nil {
		return unavailable, "withheld"
	}
	if len(secrets) > 64 {
		return unavailable, "withheld"
	}
	for _, secret := range secrets {
		if secret != "" && (len(strings.TrimSpace(secret)) < 4 || len(secret) > RawCaptureByteLimitV1) {
			return unavailable, "withheld"
		}
	}
	// This existing production function removes credentials only. It does not
	// apply public/model PII projection or parse and reserialize the source.
	text := secretprojection.ProjectTextV1(string(raw), secrets...)
	if secretprojection.ContainsCredentialV1(text) {
		return unavailable, "withheld"
	}
	if len(text) > ToolInputJSONByteLimitV2 {
		unavailable.Reason = "limit"
		return unavailable, "withheld"
	}
	if text != string(raw) {
		decoded, err = domainsecurity.DecodeCanonicalJSONObject([]byte(text))
		if err != nil {
			return unavailable, "withheld"
		}
	}
	// JSON escapes may conceal credentials from the literal text projection.
	// Inspect the already bounded decoded tree without rebuilding its output.
	var credentialSafe func(any) bool
	credentialSafe = func(value any) bool {
		switch value := value.(type) {
		case string:
			return strings.TrimSpace(value) == "" || secretprojection.ProjectTextV1(value, secrets...) == value
		case map[string]any:
			for key, child := range value {
				if strings.TrimSpace(key) != "" {
					if secretprojection.ProjectTextV1(key, secrets...) != key {
						return false
					}
					// Reuse existing credential-key semantics, including boolean
					// metadata exceptions, without imposing its full-tree limits.
					if secretprojection.ValidateValueV1(map[string]any{key: "present"}) != nil {
						material := child
						if text, ok := child.(string); ok && strings.TrimSpace(text) == "" {
							material = nil
						}
						if secretprojection.ValidateValueV1(map[string]any{key: material}) != nil {
							return false
						}
					}
				}
				if !credentialSafe(child) {
					return false
				}
			}
		case []any:
			for _, child := range value {
				if !credentialSafe(child) {
					return false
				}
			}
		}
		return true
	}
	if !credentialSafe(decoded) {
		return unavailable, "withheld"
	}
	classification := "source_exact_local"
	if text != string(raw) {
		classification = "credential_redacted"
	}
	return InputV2{Availability: "available", InputJSON: text}, classification
}

// An active Web input is retained whole or withheld, never prefix-truncated.
const WebRequestURLByteLimitV1 = 8192

// The protected numeric DTO must remain exact in JavaScript. This is not an
// input or file-effect quota.
const MaxInputUtf16CodeUnitsV1 int64 = 1<<53 - 1

// RawCaptureV1 is a process-local producer value. It is never a persisted,
// public event, provider request or JSON response. Complete means the real
// producer has closed the capture, not that the tool effect succeeded.
type RawCaptureV1 struct {
	InputUtf16CodeUnits *int64 `json:"-"`

	Kind             string        `json:"-"`
	Complete         bool          `json:"-"`
	Overflow         bool          `json:"-"`
	Body             string        `json:"-"`
	Diff             string        `json:"-"`
	AddedLines       int           `json:"-"`
	RemovedLines     int           `json:"-"`
	Approximate      bool          `json:"-"`
	ToolBinding      ToolBindingV1 `json:"-"`
	Title            string        `json:"-"`
	Path             string        `json:"-"`
	Before           string        `json:"-"`
	After            string        `json:"-"`
	Error            string        `json:"-"`
	Status           string        `json:"-"`
	SettlementStatus string        `json:"-"`
	EffectApplied    *bool         `json:"-"`
	ExitCode         *int          `json:"-"`
	DurationMs       int64         `json:"-"`
}

type DisplayCaptureV1 struct {
	Body, Preview, Title, Path, Before, After, Error string `json:"-"`
	Classification                                   string `json:"-"`
	Truncated                                        bool   `json:"-"`
}

type BodyV1 struct {
	Text         string `json:"text"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Error        string `json:"error"`
	AddedLines   int    `json:"addedLines"`
	RemovedLines int    `json:"removedLines"`
	Approximate  bool   `json:"approximate"`
}

type ToolBindingV1 struct {
	CallID        string `json:"-"`
	ResultItemID  string `json:"-"`
	ToolName      string `json:"-"`
	GrantID       string `json:"-"`
	InputArgsHash string `json:"-"`
}

// V2 identifies real host sources without exporting grants or protocol envelopes.
type CallAnchorV2 struct {
	CallID           string `json:"callId"`
	ToolCallItemID   string `json:"toolCallItemId"`
	ToolResultItemID string `json:"toolResultItemId,omitempty"`
	StartedEventSeq  string `json:"startedEventSeq,omitempty"`
}
type UnitV2 struct {
	UnitID           string         `json:"unitId"`
	ProducerID       string         `json:"producerId"`
	ProviderSequence string         `json:"providerSequence"`
	Placement        string         `json:"placement"`
	Calls            []CallAnchorV2 `json:"calls"`
	PrecedingCalls   []CallAnchorV2 `json:"precedingCalls"`
}
type SourceV2 struct {
	Kind, UnitID, ProducerID, ProviderSequence, SourceOrdinal string
	ReasoningForm                                             string
	Attempt                                                   uint64
	NativeIndex                                               int
	CallID, ToolCallItemID, ToolResultItemID, StartedEventSeq string
}

func (s SourceV2) MarshalJSON() ([]byte, error) {
	if s.Kind == "assistant_draft" {
		return json.Marshal(struct {
			Kind             string `json:"kind"`
			UnitID           string `json:"unitId"`
			ProducerID       string `json:"producerId"`
			ProviderSequence string `json:"providerSequence"`
			Attempt          uint64 `json:"attempt"`
		}{s.Kind, s.UnitID, s.ProducerID, s.ProviderSequence, s.Attempt})
	}
	if s.Kind == "provider_reasoning" {
		return json.Marshal(struct {
			Kind             string `json:"kind"`
			UnitID           string `json:"unitId"`
			ProducerID       string `json:"producerId"`
			ProviderSequence string `json:"providerSequence"`
			Attempt          uint64 `json:"attempt"`
			SourceOrdinal    string `json:"sourceOrdinal"`
			ReasoningForm    string `json:"reasoningForm,omitempty"`
		}{s.Kind, s.UnitID, s.ProducerID, s.ProviderSequence, s.Attempt, s.SourceOrdinal, s.ReasoningForm})
	}
	if s.Kind == "tool_preparation" {
		return json.Marshal(struct {
			Kind             string `json:"kind"`
			UnitID           string `json:"unitId"`
			ProducerID       string `json:"producerId"`
			ProviderSequence string `json:"providerSequence"`
			Attempt          uint64 `json:"attempt"`
			CallID           string `json:"callId"`
			SourceOrdinal    string `json:"sourceOrdinal"`
		}{s.Kind, s.UnitID, s.ProducerID, s.ProviderSequence, s.Attempt, s.CallID, s.SourceOrdinal})
	}
	if s.Kind == "native" {
		return json.Marshal(struct {
			Kind             string `json:"kind"`
			UnitID           string `json:"unitId"`
			ProducerID       string `json:"producerId"`
			ProviderSequence string `json:"providerSequence"`
			Attempt          uint64 `json:"attempt"`
			NativeIndex      int    `json:"nativeIndex"`
			SourceOrdinal    string `json:"sourceOrdinal"`
			ReasoningForm    string `json:"reasoningForm,omitempty"`
		}{s.Kind, s.UnitID, s.ProducerID, s.ProviderSequence, s.Attempt, s.NativeIndex, s.SourceOrdinal, s.ReasoningForm})
	}
	return json.Marshal(struct {
		Kind             string `json:"kind"`
		UnitID           string `json:"unitId"`
		CallID           string `json:"callId"`
		ToolCallItemID   string `json:"toolCallItemId"`
		ToolResultItemID string `json:"toolResultItemId,omitempty"`
		StartedEventSeq  string `json:"startedEventSeq,omitempty"`
	}{s.Kind, s.UnitID, s.CallID, s.ToolCallItemID, s.ToolResultItemID, s.StartedEventSeq})
}

type DraftSegmentV1 struct {
	// Offsets count projected UTF-8 bytes, within one complete revision.
	OffsetBytes string `json:"offsetBytes"`
	Text        string `json:"text"`
}

type PresenceV2 struct {
	AssistantDraftCount int `json:"assistantDraftCount,omitempty"`
	NativeBlockCount    int `json:"nativeBlockCount"`
	ToolCaptureCount    int `json:"toolCaptureCount"`
}
type BlockV1 struct {
	Input                 *InputV2         `json:"input,omitempty"`
	ToolName              string           `json:"toolName,omitempty"`
	ContentClassification string           `json:"contentClassification,omitempty"`
	DraftSegments         []DraftSegmentV1 `json:"draftSegments,omitempty"`
	DraftBytes            string           `json:"draftBytes,omitempty"`
	InputUtf16CodeUnits   *int64           `json:"inputUtf16CodeUnits,omitempty"`

	Source           SourceV2      `json:"source"`
	ToolBinding      ToolBindingV1 `json:"-"`
	BlockID          string        `json:"blockId"`
	Kind             string        `json:"kind"`
	State            string        `json:"state"`
	Attempt          uint64        `json:"attempt"`
	NativeIndex      *int          `json:"nativeIndex,omitempty"`
	Title            string        `json:"title,omitempty"`
	Preview          string        `json:"preview,omitempty"`
	Path             string        `json:"path,omitempty"`
	CaptureDigest    string        `json:"captureDigest,omitempty"`
	RequestURL       string        `json:"requestUrl,omitempty"`
	InputDigest      string        `json:"inputDigest,omitempty"`
	DurationMs       int64         `json:"durationMs"`
	Body             *BodyV1       `json:"body,omitempty"`
	EffectApplied    *bool         `json:"effectApplied,omitempty"`
	ExitCode         *int          `json:"exitCode,omitempty"`
	SettlementStatus string        `json:"settlementStatus,omitempty"`
	Truncated        bool          `json:"truncated"`
	Classification   string        `json:"classification"`
}

type ScopeV1 struct {
	ContextEpoch       string `json:"contextEpoch"`
	ProcessIncarnation string `json:"processIncarnation"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	ContextDigest      string `json:"contextDigest"`
}

// ProjectCaptureV1 makes a separate credential-safe local copy. Calling it
// supplies no viewer authority. Original source access and current viewer
// authority must both pass at the read owner before this copy is returned.
func ProjectCaptureV1(raw RawCaptureV1, knownSecrets ...string) DisplayCaptureV1 {
	withheld := DisplayCaptureV1{Classification: "withheld"}
	if !raw.Complete || raw.Overflow {
		return withheld
	}
	fields := []string{raw.Body, raw.Title, raw.Path, raw.Before, raw.After, raw.Error, raw.Diff}
	total := 0
	for i, field := range fields {
		total += len(field)
		if total > RawCaptureByteLimitV1 || !utf8.ValidString(field) {
			return withheld
		}
		clean, ok := plainTerminalV1(field)
		if !ok {
			return withheld
		}
		fields[i] = clean
	}
	if len(knownSecrets) > 64 {
		return withheld
	}
	secrets := append([]string(nil), knownSecrets...)
	for _, secret := range secrets {
		if secret != "" && (len(strings.TrimSpace(secret)) < 4 || len(secret) > RawCaptureByteLimitV1) {
			return withheld
		}
	}
	// Cross-field credentials are discovered from the complete capture before
	// any label, preview, body or diff response limit is applied.
	for _, field := range fields {
		secrets = append(secrets, secretprojection.ExtractExplicitSecretsV1(field)...)
	}
	classification := "source_exact_local"
	for i, field := range fields {
		// Complete whitespace-only source fields cannot contain a valid
		// known credential. Preserve their exact normalized bytes rather
		// than adopting the generic projector's empty-text normalization.
		if strings.TrimSpace(field) == "" {
			continue
		}
		projected := secretprojection.ProjectTextV1(field, secrets...)
		if secretprojection.ContainsCredentialV1(projected) {
			return withheld
		}
		if projected != field {
			classification = "credential_redacted"
		}
		fields[i] = projected
	}
	if raw.Kind == "write" || raw.Kind == "edit" {
		fields[0] = fields[6]
	}
	body, bodyCut := boundTextV1(fields[0], CaptureBodyByteLimitV1)
	title, titleCut := boundTextV1(fields[1], 1024)
	path, pathCut := boundTextV1(fields[2], 4096)
	before, beforeCut := boundTextV1(fields[3], CaptureBodyByteLimitV1)
	after, afterCut := boundTextV1(fields[4], CaptureBodyByteLimitV1)
	errText, errorCut := boundTextV1(fields[5], CaptureBodyByteLimitV1)
	preview := strings.SplitN(body, "\n", 2)[0]
	previewLimit := 256
	if raw.Kind == "native_reasoning" {
		previewLimit = ReasoningPreviewByteLimitV1
	}
	preview, _ = boundTextV1(preview, previewLimit)
	return DisplayCaptureV1{Body: body, Preview: preview, Title: title, Path: path, Before: before, After: after, Error: errText, Classification: classification, Truncated: bodyCut || titleCut || pathCut || beforeCut || afterCut || errorCut}
}

// ProjectAssistantDraftV1 projects a still-open, explicitly attributed source.
// It is not a complete capture or answer. The final token, an open quoted value
// and every suffix that could complete a host-known credential stay withheld.
// Every update replaces the whole projected prefix; raw offsets never become
// display offsets. Future undisclosed credentials are not guessed.
func ProjectAssistantDraftV1(raw string, knownSecrets ...string) DisplayCaptureV1 {
	withheld := DisplayCaptureV1{Classification: "withheld"}
	if len(raw) > RawCaptureByteLimitV1 || !utf8.ValidString(raw) || len(knownSecrets) > 64 {
		return withheld
	}
	clean, ok := plainTerminalV1(raw)
	if !ok {
		return withheld
	}
	for _, secret := range knownSecrets {
		if secret != "" && (len(strings.TrimSpace(secret)) < 4 || len(secret) > RawCaptureByteLimitV1) {
			return withheld
		}
	}
	cut := len(clean)
	// A token is withheld until its lexical delimiter, including incomplete
	// token-shaped keys, URL values, schemes and assignment values.
	for cut > 0 {
		r, size := utf8.DecodeLastRuneInString(clean[:cut])
		if unicode.IsSpace(r) {
			break
		}
		cut -= size
	}
	// The projector scans the complete current source before output limits.
	// Projecting only a token-cut prefix could cut away a closing quote or the
	// end of a known credential and accidentally expose its earlier words.
	projected := secretprojection.ProjectTextV1(clean, knownSecrets...)
	secrets := append(append([]string(nil), knownSecrets...), secretprojection.ExtractExplicitSecretsV1(clean)...)
	cut, ok = assistantDraftCandidateCutV1(clean, cut, secrets)
	if !ok {
		return withheld
	}
	if cut == 0 {
		return withheld
	}
	prefix := clean[:cut]
	projectedPrefix := secretprojection.ProjectTextV1(prefix, secrets...)
	// The full-source projection is deliberately computed before prefix bounds.
	// A credential introduced by normalization must not be missed.
	if secretprojection.ContainsCredentialV1(projected) {
		return withheld
	}
	projected = projectedPrefix
	if !utf8.ValidString(projected) || secretprojection.ContainsCredentialV1(projected) {
		return withheld
	}
	classification := "source_exact_local"
	if projected != prefix {
		classification = "credential_redacted"
	}
	return DisplayCaptureV1{Body: projected, Classification: classification}
}

// Candidate guards reach a monotone fixed point: withdrawing one key suffix
// can expose another key suffix or an unfinished quote. A bounded scan budget
// fails closed for adversarial overlapping keys instead of quadratic work.
func assistantDraftCandidateCutV1(text string, cut int, secrets []string) (int, bool) {
	type pattern struct {
		value  string
		prefix []int
	}
	patterns := []pattern{}
	budget := 4 * RawCaptureByteLimitV1
	for _, value := range secrets {
		value = strings.TrimSpace(value)
		if len(value) < 4 {
			continue
		}
		budget -= len(value)
		if budget < 0 {
			return 0, false
		}
		prefix := make([]int, len(value))
		for i, q := 1, 0; i < len(value); i++ {
			for q > 0 && value[i] != value[q] {
				q = prefix[q-1]
			}
			if value[i] == value[q] {
				q++
			}
			prefix[i] = q
		}
		patterns = append(patterns, pattern{value, prefix})
	}
	for {
		previous := cut
		budget -= cut
		if budget < 0 {
			return 0, false
		}
		quote, quoteStart := byte(0), -1
		for i := 0; i < cut; i++ {
			c := text[i]
			if c != '\'' && c != '"' {
				continue
			}
			// Ambiguous escaping is not admitted as a partial credential.
			if i > 0 && text[i-1] == '\\' {
				return 0, false
			}
			if quote != 0 {
				if c == quote {
					quote, quoteStart = 0, -1
				}
				continue
			}
			if c == '\'' && i > 0 && i+1 < cut && asciiDraftLetterV1(text[i-1]) && asciiDraftLetterV1(text[i+1]) {
				continue
			}
			quote, quoteStart = c, i
		}
		if quoteStart >= 0 {
			cut = quoteStart
		}
		for _, p := range patterns {
			start := cut - (len(p.value) - 1)
			if start < 0 {
				start = 0
			}
			budget -= cut - start
			if budget < 0 {
				return 0, false
			}
			q := 0
			for i := start; i < cut; i++ {
				for q > 0 && text[i] != p.value[q] {
					q = p.prefix[q-1]
				}
				if text[i] == p.value[q] {
					q++
				}
			}
			cut -= q
		}
		if cut == previous || cut == 0 {
			return cut, true
		}
	}
}

func asciiDraftLetterV1(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// SegmentAssistantDraftV1 preserves every projected byte without adopting the
// complete-capture 32KiB truncation. Owner quotas still bound the full draft.
func SegmentAssistantDraftV1(text string) []DraftSegmentV1 {
	if !utf8.ValidString(text) {
		return nil
	}
	segments := []DraftSegmentV1{}
	for offset := 0; offset < len(text); {
		end := offset + CaptureBodyByteLimitV1
		if end > len(text) {
			end = len(text)
		}
		for end > offset && !utf8.ValidString(text[offset:end]) {
			end--
		}
		segments = append(segments, DraftSegmentV1{OffsetBytes: strconv.Itoa(offset), Text: text[offset:end]})
		offset = end
	}
	return segments
}

func boundTextV1(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	end := limit
	for end > 0 && !utf8.ValidString(text[:end]) {
		end--
	}
	return text[:end], true
}

// Only terminal formatting is removed. Cursor controls, incomplete escape
// sequences and other control protocols are withheld instead of interpreted.
func plainTerminalV1(text string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(text); {
		c := text[i]
		if c == 0x1b {
			if i+1 >= len(text) {
				return "", false
			}
			switch text[i+1] {
			case '[':
				j := i + 2
				for j < len(text) && (text[j] >= '0' && text[j] <= '9' || text[j] == ';' || text[j] == ':') {
					j++
				}
				if j == len(text) || text[j] != 'm' {
					return "", false
				}
				i = j + 1
			case ']':
				if !strings.HasPrefix(text[i+2:], "8;") {
					return "", false
				}
				j := i + 2
				for j < len(text) && text[j] != 7 && !(text[j] == 0x1b && j+1 < len(text) && text[j+1] == '\\') {
					j++
				}
				if j == len(text) {
					return "", false
				}
				if text[j] == 7 {
					i = j + 1
				} else {
					i = j + 2
				}
			default:
				return "", false
			}
			continue
		}
		if c < 32 && c != '\n' && c != '\t' && c != '\r' || c == 127 {
			return "", false
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r >= 0x80 && r <= 0x9f {
			return "", false
		}
		out.WriteString(text[i : i+size])
		i += size
	}
	return out.String(), true
}

// ValidWebRequestURLV1 checks a sealed host source, not renderer authority.
func ValidWebRequestURLV1(raw string) bool {
	if raw == "" || len(raw) > WebRequestURLByteLimitV1 || !utf8.ValidString(raw) || strings.TrimSpace(raw) != raw {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
	}
	parsed, err := url.Parse(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil
}

// WebInputDigestV1 binds the real input to one active host call/grant. It grants
// no viewer authority and contains neither provider output nor a public value.
func WebInputDigestV1(tool ToolBindingV1, requestURL string) string {
	encoded, _ := json.Marshal(struct{ CallID, ToolName, GrantID, ArgsHash, RequestURL string }{tool.CallID, tool.ToolName, tool.GrantID, tool.InputArgsHash, requestURL})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func CaptureDigestV1(block BlockV1) string {
	block.BlockID = ""
	block.CaptureDigest = ""
	encoded, _ := json.Marshal(block)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// CollectorV1 admits one bounded capture from the real tool invocation.
// Duplicate producers invalidate it rather than selecting a different body.
type CollectorV1 struct {
	mu       sync.Mutex
	capture  *RawCaptureV1
	consumed bool
}
type collectorKeyV1 struct{}
type secretsKeyV1 struct{}

func WithCollectorV1(ctx context.Context, collector *CollectorV1) context.Context {
	return context.WithValue(ctx, collectorKeyV1{}, collector)
}

// knownSecretsRegistryV1 is a bounded process-local projection input shared by
// contexts within one turn. It grants no source, provider or viewer authority.
// Closing it invalidates future captures instead of retaining credentials for
// a late settlement or creating another history owner.
type knownSecretsRegistryV1 struct {
	mu       sync.Mutex
	values   []string
	seen     map[string]struct{}
	bytes    int
	overflow bool
	closed   bool
}

func WithKnownSecretsV1(ctx context.Context, secrets ...string) context.Context {
	ctx = context.WithValue(ctx, secretsKeyV1{}, &knownSecretsRegistryV1{seen: make(map[string]struct{})})
	RecordKnownSecretsV1(ctx, secrets...)
	return ctx
}

// RecordKnownSecretsV1 records only host-known actual request credentials. The
// caller must not use provider response values to populate this registry.
func RecordKnownSecretsV1(ctx context.Context, secrets ...string) {
	if ctx == nil {
		return
	}
	registry, _ := ctx.Value(secretsKeyV1{}).(*knownSecretsRegistryV1)
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed || registry.overflow {
		return
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if _, exists := registry.seen[secret]; exists {
			continue
		}
		if len(strings.TrimSpace(secret)) < 4 || len(secret) > RawCaptureByteLimitV1 || len(registry.values) >= 64 || len(secret) > RawCaptureByteLimitV1-registry.bytes {
			registry.overflow = true
			return
		}
		registry.seen[secret] = struct{}{}
		registry.values = append(registry.values, secret)
		registry.bytes += len(secret)
	}
}

// An actual request whose credential encoding cannot be observed safely
// retires text projection through the existing fail-closed quota state.
func InvalidateKnownSecretsV1(ctx context.Context) {
	if ctx == nil {
		return
	}
	registry, _ := ctx.Value(secretsKeyV1{}).(*knownSecretsRegistryV1)
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.overflow = true
}

func KnownSecretsV1(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	registry, _ := ctx.Value(secretsKeyV1{}).(*knownSecretsRegistryV1)
	if registry == nil {
		return nil
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed || registry.overflow {
		// A count-only sentinel makes existing producer and projector quotas
		// withhold the capture while containing no credential bytes itself.
		return make([]string, 65)
	}
	return append([]string(nil), registry.values...)
}

func ClearKnownSecretsV1(ctx context.Context) {
	if ctx == nil {
		return
	}
	registry, _ := ctx.Value(secretsKeyV1{}).(*knownSecretsRegistryV1)
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for i := range registry.values {
		registry.values[i] = ""
	}
	registry.values = nil
	registry.seen = nil
	registry.bytes = 0
	registry.closed = true
}
func CaptureV1(ctx context.Context, raw RawCaptureV1) {
	if ctx == nil {
		return
	}
	c, _ := ctx.Value(collectorKeyV1{}).(*CollectorV1)
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.consumed || c.capture != nil {
		c.capture = nil
		c.consumed = true
		return
	}
	total := len(raw.Body) + len(raw.Title) + len(raw.Path) + len(raw.Before) + len(raw.After) + len(raw.Error) + len(raw.Diff)
	if total > RawCaptureByteLimitV1 {
		raw.Body = ""
		raw.Title = ""
		raw.Path = ""
		raw.Before = ""
		raw.After = ""
		raw.Error = ""
		raw.Diff = ""
		raw.Overflow = true
	}
	if raw.InputUtf16CodeUnits != nil {
		value := *raw.InputUtf16CodeUnits
		raw.InputUtf16CodeUnits = &value
	}
	if raw.EffectApplied != nil {
		value := *raw.EffectApplied
		raw.EffectApplied = &value
	}
	if raw.ExitCode != nil {
		value := *raw.ExitCode
		raw.ExitCode = &value
	}
	c.capture = &raw
}

// MutationCaptureReadyV1 is a non-consuming branch selector, never settlement,
// source or viewer authority. Only the real matching Write/Edit producer may
// select ordinary atomic settlement; other tools retain their existing owner.
func MutationCaptureReadyV1(ctx context.Context, toolName string) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	var kind string
	switch strings.TrimSpace(toolName) {
	case "write", "write_file":
		kind = "write"
	case "edit", "edit_file", "multi_edit":
		kind = "edit"
	default:
		return false
	}
	collector, _ := ctx.Value(collectorKeyV1{}).(*CollectorV1)
	if collector == nil {
		return false
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return !collector.consumed && collector.capture != nil && collector.capture.Kind == kind
}

func ConsumeV1(ctx context.Context) (RawCaptureV1, bool) {
	if ctx == nil {
		return RawCaptureV1{}, false
	}
	c, _ := ctx.Value(collectorKeyV1{}).(*CollectorV1)
	if c == nil {
		return RawCaptureV1{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.consumed || c.capture == nil {
		return RawCaptureV1{}, false
	}
	raw := *c.capture
	c.capture = nil
	c.consumed = true
	return raw, true
}
