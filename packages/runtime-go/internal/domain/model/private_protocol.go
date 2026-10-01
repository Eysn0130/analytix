package model

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const anthropicThinkingCapsuleVersion = 1

// PrivateProtocolSession is a process-local authority for provider protocol
// state that must never be persisted. A session is created for one logical
// agent-loop lifetime and may be carried across an in-memory approval resume.
// Restart intentionally destroys the authority and makes old capsules
// unusable.
type PrivateProtocolSession struct {
	mu                sync.Mutex
	key               [32]byte
	toolResultDigests map[string]string
}

type AnthropicThinkingCapsuleInput struct {
	Protocol              string
	ContextDigest         string
	ProviderRouteHash     string
	PromptRoute           string
	ToolManifestHash      string
	Sequence              uint64
	AssistantMessageIndex int
	PrefixMessages        []Message
	AssistantMessage      Message
	Thinking              string
	Signature             string
}

// AnthropicThinkingCapsule is the legacy-named volatile capsule used for exact
// Anthropic signed-thinking and DeepSeek reasoning continuation bytes. Its
// provider protocol, route, message shape, and tool-result pairing are bound
// behind an unexported process-local HMAC seal. It has no JSON representation.
type AnthropicThinkingCapsule struct {
	payload anthropicThinkingCapsulePayload
	seal    [sha256.Size]byte
}

type anthropicThinkingCapsulePayload struct {
	Version               int      `json:"version"`
	Nonce                 string   `json:"nonce"`
	Protocol              string   `json:"protocol"`
	ContextDigest         string   `json:"contextDigest"`
	ProviderRouteHash     string   `json:"providerRouteHash"`
	PromptRoute           string   `json:"promptRoute"`
	ToolManifestHash      string   `json:"toolManifestHash"`
	Sequence              uint64   `json:"sequence"`
	AssistantMessageIndex int      `json:"assistantMessageIndex"`
	PrefixDigest          string   `json:"prefixDigest"`
	AssistantShapeDigest  string   `json:"assistantShapeDigest"`
	ToolCallIDs           []string `json:"toolCallIds"`
	Thinking              string   `json:"thinking"`
	Signature             string   `json:"signature"`
}

// AnthropicThinkingReplay is the legacy-named immutable replay permit.
// Anthropic uses it only for the immediate continuation and its transport
// retries. DeepSeek reuses the exact permit for each later request that still
// contains the bound assistant tool-call message in the same volatile loop.
// Its private fields cannot be constructed by provider-originated data.
type AnthropicThinkingReplay struct {
	protocol              string
	assistantMessageIndex int
	assistantShapeDigest  string
	thinking              string
	signature             string
}

type AnthropicThinkingConsumeInput struct {
	Protocol          string
	ContextDigest     string
	ProviderRouteHash string
	PromptRoute       string
	ToolManifestHash  string
	Sequence          uint64
	Messages          []Message
}

func (*PrivateProtocolSession) GoString() string {
	return "model.PrivateProtocolSession{redacted}"
}

func (*PrivateProtocolSession) String() string {
	return "model.PrivateProtocolSession{redacted}"
}

func (*PrivateProtocolSession) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "model.PrivateProtocolSession{redacted}")
}

func (*AnthropicThinkingCapsule) GoString() string {
	return "model.AnthropicThinkingCapsule{redacted}"
}

func (*AnthropicThinkingCapsule) String() string {
	return "model.AnthropicThinkingCapsule{redacted}"
}

func (*AnthropicThinkingCapsule) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "model.AnthropicThinkingCapsule{redacted}")
}

func (*AnthropicThinkingReplay) GoString() string {
	return "model.AnthropicThinkingReplay{redacted}"
}

func (*AnthropicThinkingReplay) String() string {
	return "model.AnthropicThinkingReplay{redacted}"
}

func (*AnthropicThinkingReplay) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "model.AnthropicThinkingReplay{redacted}")
}

func NewPrivateProtocolSession() (*PrivateProtocolSession, error) {
	return newPrivateProtocolSession(rand.Reader)
}

func newPrivateProtocolSession(random io.Reader) (*PrivateProtocolSession, error) {
	if random == nil {
		return nil, errors.New("private protocol randomness is unavailable")
	}
	session := &PrivateProtocolSession{toolResultDigests: map[string]string{}}
	if _, err := io.ReadFull(random, session.key[:]); err != nil {
		return nil, errors.New("private protocol session key generation failed")
	}
	return session, nil
}

func (session *PrivateProtocolSession) IssueAnthropicThinking(input AnthropicThinkingCapsuleInput) (*AnthropicThinkingCapsule, error) {
	if session == nil {
		return nil, errors.New("private protocol session is unavailable")
	}
	contextDigest := strings.TrimSpace(input.ContextDigest)
	protocol := privateThinkingProtocol(input.Protocol)
	providerRouteHash := strings.TrimSpace(input.ProviderRouteHash)
	promptRoute := strings.TrimSpace(input.PromptRoute)
	toolManifestHash := strings.TrimSpace(input.ToolManifestHash)
	thinking := input.Thinking
	signature := strings.TrimSpace(input.Signature)
	if protocol == "" || !domainsecurity.IsSHA256Hex(contextDigest) || !domainsecurity.IsSHA256Hex(providerRouteHash) ||
		promptRoute == "" || !domainsecurity.IsSHA256Hex(toolManifestHash) || input.Sequence == 0 ||
		input.AssistantMessageIndex < 0 ||
		(protocol == "anthropic-thinking" && signature == "") ||
		(protocol == "deepseek-messages" && strings.TrimSpace(thinking) == "") ||
		(protocol == "deepseek-chat-completions" &&
			(strings.TrimSpace(thinking) == "" || signature != "")) {
		return nil, errors.New("private provider protocol capsule binding is invalid")
	}
	shapeDigest, toolCallIDs, err := anthropicAssistantShape(input.AssistantMessage)
	if err != nil {
		return nil, err
	}
	if len(input.PrefixMessages) != input.AssistantMessageIndex {
		return nil, errors.New("private provider protocol prefix length is invalid")
	}
	prefixDigest, err := privateProtocolMessagesDigest(input.PrefixMessages)
	if err != nil {
		return nil, err
	}
	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return nil, errors.New("private protocol capsule nonce generation failed")
	}
	payload := anthropicThinkingCapsulePayload{
		Version: anthropicThinkingCapsuleVersion, Nonce: hex.EncodeToString(nonceBytes),
		Protocol:      protocol,
		ContextDigest: contextDigest, ProviderRouteHash: providerRouteHash, PromptRoute: promptRoute,
		ToolManifestHash: toolManifestHash, Sequence: input.Sequence, AssistantMessageIndex: input.AssistantMessageIndex,
		PrefixDigest: prefixDigest, AssistantShapeDigest: shapeDigest, ToolCallIDs: toolCallIDs,
		Thinking: thinking, Signature: signature,
	}
	seal, err := session.seal(payload)
	if err != nil {
		return nil, err
	}
	return &AnthropicThinkingCapsule{payload: payload, seal: seal}, nil
}

// AnthropicThinkingRequestShapeChanged reports whether an authenticated
// Anthropic capsule was issued for a different prompt route or tool manifest.
// DeepSeek continuation permits catalog changes inside the same volatile
// chain, so an authenticated DeepSeek capsule never reports shape drift.
func (session *PrivateProtocolSession) AnthropicThinkingRequestShapeChanged(
	capsule *AnthropicThinkingCapsule,
	promptRoute string,
	toolManifestHash string,
) (bool, error) {
	if session == nil || capsule == nil {
		return false, errors.New("anthropic private protocol capsule is unavailable")
	}
	expectedSeal, err := session.seal(capsule.payload)
	if err != nil || subtle.ConstantTimeCompare(expectedSeal[:], capsule.seal[:]) != 1 {
		return false, errors.New("anthropic private protocol capsule seal is invalid")
	}
	promptRoute = strings.TrimSpace(promptRoute)
	toolManifestHash = strings.TrimSpace(toolManifestHash)
	if promptRoute == "" || !domainsecurity.IsSHA256Hex(toolManifestHash) {
		return false, errors.New("private provider protocol request shape is invalid")
	}
	switch capsule.payload.Protocol {
	case "anthropic-thinking":
		return capsule.payload.PromptRoute != promptRoute ||
			capsule.payload.ToolManifestHash != toolManifestHash, nil
	case "deepseek-chat-completions", "deepseek-messages":
		return false, nil
	default:
		return false, errors.New("private provider protocol capsule binding is invalid")
	}
}

func (session *PrivateProtocolSession) ConsumeAnthropicThinking(
	capsule *AnthropicThinkingCapsule,
	input AnthropicThinkingConsumeInput,
) (*AnthropicThinkingReplay, error) {
	if session == nil || capsule == nil {
		return nil, errors.New("anthropic private protocol capsule is unavailable")
	}
	expectedSeal, err := session.seal(capsule.payload)
	if err != nil || subtle.ConstantTimeCompare(expectedSeal[:], capsule.seal[:]) != 1 {
		return nil, errors.New("anthropic private protocol capsule seal is invalid")
	}
	payload := capsule.payload
	// Every thinking/reasoning block that remains in one uninterrupted tool-use
	// chain must be replayed on every later provider request in that chain.
	// Exact prefix, assistant shape, tool-result order, route, manifest, context,
	// and monotonic sequence bindings prevent a capsule from authorizing a
	// different branch even though the same bound block can be replayed more
	// than once.
	sequenceMatches := input.Sequence >= payload.Sequence
	if payload.Protocol != privateThinkingProtocol(input.Protocol) ||
		payload.ContextDigest != strings.TrimSpace(input.ContextDigest) ||
		payload.ProviderRouteHash != strings.TrimSpace(input.ProviderRouteHash) ||
		!sequenceMatches {
		return nil, errors.New("anthropic private protocol capsule binding changed")
	}
	if payload.Protocol == "anthropic-thinking" &&
		(payload.PromptRoute != strings.TrimSpace(input.PromptRoute) ||
			payload.ToolManifestHash != strings.TrimSpace(input.ToolManifestHash)) {
		return nil, errors.New("anthropic private protocol capsule binding changed")
	}
	if err := ValidateNoHistoricalPrivateProtocolParts(input.Messages); err != nil {
		return nil, err
	}
	if payload.AssistantMessageIndex < 0 || payload.AssistantMessageIndex >= len(input.Messages) {
		return nil, errors.New("anthropic private protocol assistant position changed")
	}
	prefixDigest, err := privateProtocolMessagesDigest(input.Messages[:payload.AssistantMessageIndex])
	if err != nil || prefixDigest != payload.PrefixDigest {
		return nil, errors.New("anthropic private protocol message prefix changed")
	}
	shapeDigest, toolCallIDs, err := anthropicAssistantShape(input.Messages[payload.AssistantMessageIndex])
	if err != nil || shapeDigest != payload.AssistantShapeDigest || !equalStrings(toolCallIDs, payload.ToolCallIDs) {
		return nil, errors.New("anthropic private protocol assistant tool-call shape changed")
	}
	toolResultDigest, ok := anthropicToolResultsDigest(
		input.Messages[payload.AssistantMessageIndex+1:],
		payload.ToolCallIDs,
	)
	if !ok {
		return nil, errors.New("anthropic private protocol tool-result order changed")
	}
	sealID := hex.EncodeToString(capsule.seal[:])
	session.mu.Lock()
	observedDigest, observed := session.toolResultDigests[sealID]
	if !observed {
		session.toolResultDigests[sealID] = toolResultDigest
	}
	session.mu.Unlock()
	if observed && observedDigest != toolResultDigest {
		return nil, errors.New("anthropic private protocol tool-result content changed")
	}
	return &AnthropicThinkingReplay{
		protocol:              payload.Protocol,
		assistantMessageIndex: payload.AssistantMessageIndex,
		assistantShapeDigest:  payload.AssistantShapeDigest,
		thinking:              payload.Thinking,
		signature:             payload.Signature,
	}, nil
}

func privateProtocolMessagesDigest(messages []Message) (string, error) {
	if err := ValidateNoHistoricalPrivateProtocolParts(messages); err != nil {
		return "", err
	}
	body, err := json.Marshal(messages)
	if err != nil {
		return "", errors.New("private provider protocol message prefix is not canonical")
	}
	return domainsecurity.SHA256Hex(body), nil
}

// ThinkingBlock returns the provider-private block only for the exact
// assistant message position and shape bound by the consumed capsule.
func (replay *AnthropicThinkingReplay) ThinkingBlock(index int, message Message) (string, string, bool) {
	if replay == nil || (replay.protocol != "anthropic-thinking" && replay.protocol != "deepseek-messages") || index != replay.assistantMessageIndex {
		return "", "", false
	}
	digest, _, err := anthropicAssistantShape(message)
	if err != nil || digest != replay.assistantShapeDigest {
		return "", "", false
	}
	return replay.thinking, replay.signature, true
}

// ReasoningContent returns exact DeepSeek continuation bytes only for the
// assistant tool-call message bound by the consumed process-local capsule.
func (replay *AnthropicThinkingReplay) ReasoningContent(index int, message Message) (string, bool) {
	if replay == nil || replay.protocol != "deepseek-chat-completions" ||
		index != replay.assistantMessageIndex {
		return "", false
	}
	digest, _, err := anthropicAssistantShape(message)
	if err != nil || digest != replay.assistantShapeDigest || strings.TrimSpace(replay.thinking) == "" {
		return "", false
	}
	return replay.thinking, true
}

// ValidateNoHistoricalPrivateProtocolParts rejects private provider protocol
// bytes in generic history. The only supported replay path is a consumed,
// volatile AnthropicThinkingReplay on the outbound request.
func ValidateNoHistoricalPrivateProtocolParts(messages []Message) error {
	for _, message := range messages {
		for _, part := range message.Parts {
			switch strings.ToLower(strings.TrimSpace(part.Type)) {
			case "thinking", "reasoning":
				return errors.New("private provider protocol content is not allowed in message history")
			}
		}
	}
	return nil
}

func (session *PrivateProtocolSession) seal(payload anthropicThinkingCapsulePayload) ([sha256.Size]byte, error) {
	var out [sha256.Size]byte
	body, err := json.Marshal(payload)
	if err != nil {
		return out, errors.New("anthropic private protocol capsule is not canonical")
	}
	mac := hmac.New(sha256.New, session.key[:])
	_, _ = mac.Write(body)
	copy(out[:], mac.Sum(nil))
	return out, nil
}

func privateThinkingProtocol(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "anthropic-thinking":
		// The empty value keeps compatibility with direct callers of the
		// historically Anthropic-named process-local capsule API.
		return "anthropic-thinking"
	case "deepseek-chat-completions", "deepseek-messages":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func anthropicAssistantShape(message Message) (string, []string, error) {
	if strings.TrimSpace(message.Role) != "assistant" || len(message.ToolCalls) == 0 {
		return "", nil, errors.New("anthropic private protocol requires an assistant tool-call message")
	}
	if err := ValidateNoHistoricalPrivateProtocolParts([]Message{message}); err != nil {
		return "", nil, err
	}
	type shapeCall struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		ArgsHash string `json:"argsHash"`
	}
	type shapePart struct {
		Type      string `json:"type"`
		TextHash  string `json:"textHash,omitempty"`
		ImageHash string `json:"imageHash,omitempty"`
	}
	shape := struct {
		Role        string      `json:"role"`
		ContentHash string      `json:"contentHash"`
		Parts       []shapePart `json:"parts"`
		ToolCalls   []shapeCall `json:"toolCalls"`
	}{Role: "assistant", ContentHash: domainsecurity.SHA256Hex([]byte(message.Content))}
	for _, part := range message.Parts {
		partType := strings.ToLower(strings.TrimSpace(part.Type))
		if partType != "text" && partType != "image" {
			return "", nil, errors.New("anthropic private protocol assistant part is unsupported")
		}
		shape.Parts = append(shape.Parts, shapePart{
			Type: partType, TextHash: domainsecurity.SHA256Hex([]byte(part.Text)),
			ImageHash: domainsecurity.SHA256Hex([]byte(part.ImageURL + "\x00" + part.MediaType + "\x00" + part.Data)),
		})
	}
	toolCallIDs := make([]string, 0, len(message.ToolCalls))
	seen := map[string]struct{}{}
	for _, call := range message.ToolCalls {
		id := strings.TrimSpace(call.ID)
		name := strings.TrimSpace(call.Name)
		if id == "" || name == "" {
			return "", nil, errors.New("anthropic private protocol tool-call identity is invalid")
		}
		if _, duplicate := seen[id]; duplicate {
			return "", nil, errors.New("anthropic private protocol tool-call identity is duplicated")
		}
		seen[id] = struct{}{}
		toolCallIDs = append(toolCallIDs, id)
		shape.ToolCalls = append(shape.ToolCalls, shapeCall{ID: id, Name: name, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments)})
	}
	body, err := json.Marshal(shape)
	if err != nil {
		return "", nil, errors.New("anthropic private protocol assistant shape is not canonical")
	}
	return domainsecurity.SHA256Hex(body), toolCallIDs, nil
}

func anthropicToolResultsDigest(messages []Message, expected []string) (string, bool) {
	actual := make([]Message, 0, len(expected))
	for _, message := range messages {
		if strings.TrimSpace(message.Role) != "tool" {
			break
		}
		actual = append(actual, message)
	}
	if len(actual) != len(expected) {
		return "", false
	}
	for index := range actual {
		if strings.TrimSpace(actual[index].ToolCallID) != expected[index] {
			return "", false
		}
	}
	digest, err := privateProtocolMessagesDigest(actual)
	return digest, err == nil && domainsecurity.IsSHA256Hex(digest)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
