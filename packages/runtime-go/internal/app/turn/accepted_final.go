package turn

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

// parseAcceptedFinalForPublicValidationV1 validates the original JSON-like
// record, including its exact field spelling and binary encoding. A decoded
// struct alone cannot detect case-folded aliases or omitted/null fields.
func parseAcceptedFinalForPublicValidationV1(value any) (domainevidence.AcceptedFinalRecord, error) {
	if err := domainsecret.ValidateValueLimitsV1(value); err != nil {
		return domainevidence.AcceptedFinalRecord{}, err
	}
	raw, ok := value.(map[string]any)
	if !ok || raw == nil || !acceptedFinalPlainJSONV1(raw) {
		return domainevidence.AcceptedFinalRecord{}, errors.New("accepted final original JSON shape is invalid")
	}
	for key, size := range map[string]int{"authorityPublicKey": ed25519.PublicKeySize, "authoritySignature": ed25519.SignatureSize} {
		text, ok := raw[key].(string)
		if !ok || len(text) != base64.RawURLEncoding.EncodedLen(size) {
			return domainevidence.AcceptedFinalRecord{}, errors.New("accepted final original binary field is invalid")
		}
		decoded, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil || len(decoded) != size || base64.RawURLEncoding.EncodeToString(decoded) != text {
			return domainevidence.AcceptedFinalRecord{}, errors.New("accepted final binary encoding is not canonical")
		}
	}
	record, err := domainevidence.ParseAcceptedFinalRecord(raw)
	if err != nil {
		// Strict decoding errors may include an untrusted unknown field name.
		// Qualification precedes ordinary credential scanning, so keep this
		// boundary's failure independent of raw record keys and content.
		return domainevidence.AcceptedFinalRecord{}, errors.New("accepted final original record integrity is invalid")
	}
	original, err := json.Marshal(raw)
	if err != nil {
		return domainevidence.AcceptedFinalRecord{}, err
	}
	canonical, err := json.Marshal(domainevidence.AcceptedFinalRecordMap(record))
	if err != nil || !bytes.Equal(original, canonical) {
		return domainevidence.AcceptedFinalRecord{}, errors.New("accepted final original fields differ from its closed schema")
	}
	return record, nil
}

func acceptedFinalPlainJSONV1(value any) bool {
	switch typed := value.(type) {
	case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	case json.Number:
		return domainjsonstrict.ValidateNumberText(string(typed), 0, 0) == nil
	case map[string]any:
		for _, child := range typed {
			if !acceptedFinalPlainJSONV1(child) {
				return false
			}
		}
		return true
	case []any:
		for _, child := range typed {
			if !acceptedFinalPlainJSONV1(child) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func acceptedFinalBinaryValidationViewV1(value map[string]any) map[string]any {
	out := shallowAcceptedFinalMapV1(value)
	for _, key := range []string{"authorityPublicKey", "authoritySignature"} {
		out[key] = strings.Repeat("a", len(value[key].(string)))
	}
	return out
}

func shallowAcceptedFinalMapV1(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, child := range value {
		out[key] = child
	}
	return out
}

// AcceptedFinalHistoryValidationViewV1 only changes a validation copy after
// proving the complete original record and its unique durable item binding.
// This is read-compatible integrity validation, not installation trust or
// permission to mutate. Trusted projection and atomic CAS retain that authority.
func AcceptedFinalHistoryValidationViewV1(threadID string, turn map[string]any) (map[string]any, error) {
	if err := domainsecret.ValidateValueLimitsV1(turn); err != nil {
		return nil, err
	}
	if turn["acceptedFinal"] == nil {
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if item["acceptedFinal"] != nil {
				return nil, errors.New("accepted final item has no turn authority")
			}
		}
		return turn, nil
	}
	// The pre-authority V1 format has no binary signing fields. Preserve its
	// strict historical read path with ordinary scanning and no exemption.
	if raw, ok := turn["acceptedFinal"].(map[string]any); ok && acceptedFinalPlainJSONV1(raw) {
		if legacy, err := domainevidence.ParseLegacyAcceptedFinalRecord(raw); err == nil {
			original, marshalErr := json.Marshal(raw)
			canonical, canonicalErr := json.Marshal(domainevidence.LegacyAcceptedFinalRecordMap(legacy))
			if marshalErr == nil && canonicalErr == nil && bytes.Equal(original, canonical) {
				return turn, nil
			}
		}
	}
	record, err := parseAcceptedFinalForPublicValidationV1(turn["acceptedFinal"])
	if err != nil {
		return nil, err
	}
	expectedStatus, statusOK := domainevidence.FinalAnswerTerminalStatus(record.TerminalReason)
	frozen, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if !statusOK || threadID != record.ThreadID || stringField(turn, "id") != record.TurnID ||
		stringField(turn, "status") != expectedStatus || stringField(turn, "finishedAt") != record.AcceptedAt ||
		contextErr != nil || frozen.ThreadID != record.ThreadID || frozen.TurnID != record.TurnID ||
		frozen.ContextDigest != record.ContextDigest || frozen.ContextEpoch != record.ContextEpoch ||
		frozen.DatasetSnapshotID != record.DatasetSnapshotID {
		return nil, errors.New("accepted final history turn binding is invalid")
	}
	if value, present := turn["threadId"]; present && value != record.ThreadID {
		return nil, errors.New("accepted final history thread identity differs")
	}
	items, ok := turn["items"].([]any)
	if !ok {
		return nil, errors.New("accepted final history items are invalid")
	}
	matched := -1
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || item == nil {
			return nil, errors.New("accepted final history item is invalid")
		}
		if stringField(item, "kind") != "assistant_text" {
			if item["acceptedFinal"] != nil {
				return nil, errors.New("accepted final metadata is outside its assistant item")
			}
			continue
		}
		itemRecord, err := parseAcceptedFinalForPublicValidationV1(item["acceptedFinal"])
		text, textOK := item["text"].(string)
		if matched >= 0 || err != nil || !reflect.DeepEqual(record, itemRecord) ||
			stringField(item, "threadId") != record.ThreadID || stringField(item, "turnId") != record.TurnID ||
			stringField(item, "role") != "assistant" || stringField(item, "status") != "completed" ||
			stringField(item, "finishedAt") != record.AcceptedAt ||
			!textOK || domainsecurity.SHA256Hex([]byte(text)) != record.RenderedTextSHA256 {
			return nil, errors.New("accepted final history assistant binding is invalid")
		}
		matched = index
	}
	if matched < 0 {
		return nil, errors.New("accepted final history assistant is missing")
	}
	out := shallowAcceptedFinalMapV1(turn)
	out["acceptedFinal"] = acceptedFinalBinaryValidationViewV1(turn["acceptedFinal"].(map[string]any))
	viewItems := append([]any(nil), items...)
	item := shallowAcceptedFinalMapV1(items[matched].(map[string]any))
	item["acceptedFinal"] = acceptedFinalBinaryValidationViewV1(item["acceptedFinal"].(map[string]any))
	viewItems[matched] = item
	out["items"] = viewItems
	return out, nil
}

type AcceptedFinalCompletionStore interface {
	FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status string, items []map[string]any, fields map[string]any) (bool, string, error)
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

// FactFinalMutationAuthority is an opaque, callback-scoped capability for one
// exact fact-bearing private final. Implementations must fail after the
// authority callback returns.
type FactFinalMutationAuthority interface {
	UseExact(domainevidence.PrivateAcceptedFinalRecord, func() error) error
}

// AcceptedFinalAuthorizedCompletionStore is the only public-turn CAS that may
// receive accepted-final fields. The implementation must verify factAuthority
// while holding the durable turn lock immediately before mutation.
type AcceptedFinalAuthorizedCompletionStore interface {
	FinishTurnIfActiveWithAcceptedFinalAuthority(
		threadID, turnID, status string,
		items []map[string]any,
		fields map[string]any,
		privateFinal domainevidence.PrivateAcceptedFinalRecord,
		factAuthority FactFinalMutationAuthority,
	) (bool, string, error)
}

type PersistAcceptedFinalInput struct {
	Store             AcceptedFinalCompletionStore
	ThreadID          string
	TurnID            string
	RenderedText      string
	AcceptedFinal     domainevidence.AcceptedFinalRecord
	PublicationIntent domainevidence.TerminalPublicationIntent
	PrivateFinal      domainevidence.PrivateAcceptedFinalRecord
	FactAuthority     FactFinalMutationAuthority
}

type PersistAcceptedFinalResult struct {
	Timing           PublicationTiming `json:"-"`
	CompletionRecord CompletionRecord
	AcceptedFinal    domainevidence.AcceptedFinalRecord
	Publication      AcceptedFinalPublicationPlan
	Changed          bool
	Status           string
}

func AcceptedFinalFinishedAt(fields map[string]any, fallback time.Time) (string, error) {
	if fields["acceptedFinal"] == nil {
		if fields["generalTerminalPublication"] == nil {
			return fallback.UTC().Format(time.RFC3339Nano), nil
		}
		commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(fields["generalTerminalPublication"])
		if err != nil {
			return "", err
		}
		return commit.CommittedAt, nil
	}
	record, err := domainevidence.ParseAcceptedFinalRecord(fields["acceptedFinal"])
	if err != nil {
		return "", err
	}
	return record.AcceptedAt, nil
}

func PersistAcceptedFinalTerminal(input PersistAcceptedFinalInput) (PersistAcceptedFinalResult, error) {
	if input.Store == nil || strings.TrimSpace(input.ThreadID) == "" || strings.TrimSpace(input.TurnID) == "" {
		return PersistAcceptedFinalResult{}, errors.New("accepted final completion input is invalid")
	}
	terminalStatus := strings.TrimSpace(input.PublicationIntent.TerminalStatus)
	if terminalStatus != "completed" && terminalStatus != "failed" && terminalStatus != "aborted" {
		return PersistAcceptedFinalResult{}, errors.New("accepted final terminal status is invalid")
	}
	acceptedFinal := input.AcceptedFinal
	privateFinal := input.PrivateFinal
	if err := domainevidence.ValidateAcceptedFinalForCurrentWriteV1(acceptedFinal); err != nil ||
		ValidatePrivateAcceptedFinalMutationAuthority(privateFinal, input.FactAuthority) != nil ||
		privateFinal.AcceptedFinal.RecordDigest != acceptedFinal.RecordDigest ||
		privateFinal.RenderedText != input.RenderedText ||
		!reflect.DeepEqual(privateFinal.PublicationIntent, input.PublicationIntent) ||
		acceptedFinal.ThreadID != strings.TrimSpace(input.ThreadID) || acceptedFinal.TurnID != strings.TrimSpace(input.TurnID) ||
		domainsecurity.SHA256Hex([]byte(input.RenderedText)) != acceptedFinal.RenderedTextSHA256 ||
		domainevidence.ValidateTerminalPublicationIntent(input.PublicationIntent, acceptedFinal.TerminalReason) != nil {
		return PersistAcceptedFinalResult{}, errors.New("accepted final authority or rendered text is invalid")
	}
	publication, err := BuildAcceptedFinalPublicationPlan(acceptedFinal, input.RenderedText, input.PublicationIntent)
	if err != nil {
		return PersistAcceptedFinalResult{}, err
	}
	authorizedStore, ok := input.Store.(AcceptedFinalAuthorizedCompletionStore)
	if !ok {
		return PersistAcceptedFinalResult{}, errors.New("accepted final completion store lacks authorized CAS")
	}
	timing := PublicationTiming{ProjectionReadyAt: time.Now()}
	changed, status, err := authorizedStore.FinishTurnIfActiveWithAcceptedFinalAuthority(
		input.ThreadID, input.TurnID, terminalStatus, publication.TurnItems, publication.TurnFields,
		privateFinal, input.FactAuthority,
	)
	if err != nil || !changed {
		return PersistAcceptedFinalResult{CompletionRecord: publication.Completion, AcceptedFinal: acceptedFinal, Publication: publication, Changed: changed, Status: status}, err
	}
	timing.CommittedAt = time.Now()
	return PersistAcceptedFinalResult{Timing: timing, CompletionRecord: publication.Completion, AcceptedFinal: acceptedFinal, Publication: publication, Changed: true, Status: status}, nil
}

func ValidatePrivateAcceptedFinalMutationAuthority(
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
) error {
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		if factAuthority == nil {
			return errors.New("fact final mutation authority is unavailable")
		}
		return domainevidence.ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(
			privateFinal,
			func(record domainevidence.PrivateAcceptedFinalRecord) error {
				return factAuthority.UseExact(record, func() error { return nil })
			},
		)
	}
	if factAuthority != nil {
		return errors.New("boundary final must not carry fact mutation authority")
	}
	return domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(privateFinal)
}

func UsePrivateAcceptedFinalMutationAuthority(
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
	mutation func() error,
) error {
	if mutation == nil {
		return errors.New("private accepted-final mutation is unavailable")
	}
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		if factAuthority == nil {
			return errors.New("fact final mutation authority is unavailable")
		}
		return factAuthority.UseExact(privateFinal, mutation)
	}
	if factAuthority != nil {
		return errors.New("boundary final must not carry fact mutation authority")
	}
	if err := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(privateFinal); err != nil {
		return err
	}
	return mutation()
}

// ValidateAcceptedFinalCASAuthority binds the entire public CAS payload to the
// exact private final. It is side-effect free; durable adapters must call it
// again in their atomic-write prewrite callback.
func ValidateAcceptedFinalCASAuthority(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
) (bool, error) {
	hasAcceptedFinal, err := validateAcceptedFinalCASPayload(
		threadID, turnID, status, appendItems, fields, privateFinal,
	)
	if err != nil {
		return hasAcceptedFinal, err
	}
	if !hasAcceptedFinal {
		if factAuthority != nil {
			return false, errors.New("terminal CAS authority has no accepted final")
		}
		return false, nil
	}
	if err := ValidatePrivateAcceptedFinalMutationAuthority(privateFinal, factAuthority); err != nil {
		return true, errors.Join(errors.New("accepted final CAS lacks live mutation authority"), err)
	}
	return true, nil
}

// UseAcceptedFinalCASAuthority holds the fact-witness lease across the exact
// atomic mutation. The mutation must be the adapter's immediate durable write,
// not a preflight that returns before persistence.
func UseAcceptedFinalCASAuthority(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
	mutation func() error,
) error {
	if mutation == nil {
		return errors.New("accepted final CAS mutation is unavailable")
	}
	hasAcceptedFinal, err := validateAcceptedFinalCASPayload(
		threadID, turnID, status, appendItems, fields, privateFinal,
	)
	if err != nil || !hasAcceptedFinal {
		return errors.Join(errors.New("accepted final CAS payload is invalid at mutation"), err)
	}
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		return UsePrivateAcceptedFinalMutationAuthority(privateFinal, factAuthority, func() error {
			if _, err := validateAcceptedFinalCASPayload(
				threadID, turnID, status, appendItems, fields, privateFinal,
			); err != nil {
				return err
			}
			return mutation()
		})
	}
	return UsePrivateAcceptedFinalMutationAuthority(privateFinal, factAuthority, mutation)
}

func validateAcceptedFinalCASPayload(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
) (bool, error) {
	hasAcceptedFinal := fields["acceptedFinal"] != nil
	for _, item := range appendItems {
		hasAcceptedFinal = hasAcceptedFinal || item["acceptedFinal"] != nil
	}
	if !hasAcceptedFinal {
		if privateFinal.StoreDigest != "" {
			return false, errors.New("terminal CAS authority has no accepted final")
		}
		return false, nil
	}
	acceptedFinal, parseErr := domainevidence.ParseAcceptedFinalRecord(fields["acceptedFinal"])
	if parseErr != nil || domainevidence.ValidatePrivateAcceptedFinalRecord(privateFinal) != nil ||
		domainevidence.ValidateAcceptedFinalForCurrentWriteV1(privateFinal.AcceptedFinal) != nil ||
		privateFinal.SecurityContext.ThreadID != threadID || privateFinal.SecurityContext.TurnID != turnID ||
		!reflect.DeepEqual(privateFinal.AcceptedFinal, acceptedFinal) {
		return true, errors.Join(errors.New("accepted final CAS lacks exact private authority"), parseErr)
	}
	publication, err := BuildAcceptedFinalPublicationPlan(
		privateFinal.AcceptedFinal,
		privateFinal.RenderedText,
		privateFinal.PublicationIntent,
	)
	if err != nil || status != privateFinal.PublicationIntent.TerminalStatus ||
		!reflect.DeepEqual(appendItems, publication.TurnItems) || !reflect.DeepEqual(fields, publication.TurnFields) {
		return true, errors.Join(errors.New("accepted final CAS payload differs from the exact publication plan"), err)
	}
	return true, nil
}

// ValidateAcceptedFinalTerminalUpdate runs under the durable thread lock. It
// closes the gap between gate evaluation and persistence by rejecting a final
// produced for an older case/epoch or a different turn.
func ValidateAcceptedFinalTerminalUpdate(thread map[string]any, turnID, status string, items []map[string]any, fields map[string]any) error {
	turn, found := securityTurnByID(thread, strings.TrimSpace(turnID))
	if !found {
		return errors.New("accepted final turn does not exist")
	}
	turnContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if contextErr != nil {
		return errors.New("terminal update requires a valid frozen security context")
	}
	caseRequired := domainsecurity.TurnSecurityContextIsCaseSensitive(turnContext)
	value, hasAcceptedFinal := fields["acceptedFinal"]
	if !hasAcceptedFinal {
		if caseRequired {
			return errors.New("case-bound terminal update requires an accepted final")
		}
		_, hasBinding := fields["generalTerminalCASBinding"]
		_, hasPublication := fields["generalTerminalPublication"]
		if domainsecurity.TurnSecurityContextIsGeneral(turnContext) {
			if err := ValidateGeneralTerminalPublicationUpdateV1(thread, turnID, status, items, fields); err != nil {
				return err
			}
		} else if hasBinding || hasPublication {
			return errors.New("general terminal authority is invalid for this terminal path")
		} else {
			for _, item := range items {
				if stringField(item, "kind") == "assistant_text" {
					return errors.New("general assistant terminal update requires a context-bound publication outbox")
				}
			}
		}
		for _, item := range items {
			if item["acceptedFinal"] != nil {
				return errors.New("accepted final item is missing turn authority")
			}
		}
		return nil
	}
	record, err := domainevidence.ParseAcceptedFinalRecord(value)
	if err != nil {
		return err
	}
	if err := domainevidence.ValidateAcceptedFinalForCurrentWriteV1(record); err != nil {
		return errors.New("accepted final lacks current-write authority")
	}
	expectedStatus, ok := domainevidence.FinalAnswerTerminalStatus(record.TerminalReason)
	if !ok || strings.TrimSpace(status) != expectedStatus {
		return errors.New("accepted final terminal status contradicts terminal reason")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(current) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(turnContext) != nil ||
		(domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Variant) &&
			(domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(current) != nil ||
				domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(turnContext) != nil)) ||
		current.ThreadID != strings.TrimSpace(stringField(thread, "id")) || current.TurnID != strings.TrimSpace(turnID) ||
		current.ContextDigest != record.ContextDigest || current.ContextEpoch != record.ContextEpoch || current.DatasetSnapshotID != record.DatasetSnapshotID {
		return errors.New("accepted final does not match the current thread context")
	}
	if contextErr != nil || turnContext.ContextDigest != current.ContextDigest {
		return errors.New("accepted final does not match the frozen turn context")
	}
	matchedItem := false
	for _, item := range items {
		itemRecordValue := item["acceptedFinal"]
		if stringField(item, "kind") == "assistant_text" && itemRecordValue == nil {
			return errors.New("case terminal update contains an unaccepted assistant item")
		}
		if itemRecordValue == nil {
			continue
		}
		itemRecord, err := domainevidence.ParseAcceptedFinalRecord(itemRecordValue)
		text, _ := item["text"].(string)
		if err != nil || itemRecord.RecordDigest != record.RecordDigest ||
			domainsecurity.SHA256Hex([]byte(text)) != record.RenderedTextSHA256 || strings.TrimSpace(stringField(item, "turnId")) != strings.TrimSpace(turnID) {
			return errors.New("accepted final item integrity is invalid")
		}
		if matchedItem {
			return errors.New("accepted final update contains duplicate final items")
		}
		matchedItem = true
	}
	if !matchedItem {
		return errors.New("accepted final update is missing its rendered item")
	}
	return nil
}
