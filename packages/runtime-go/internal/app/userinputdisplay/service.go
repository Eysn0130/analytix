package userinputdisplay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	port "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

type ContinuationReaderV1 interface {
	ReadUserInputLocalDisplayHost(context.Context, continuationapp.UserInputLocalDisplayBindingV1) (continuationapp.UserInputLocalDisplayV1, error)
}

// History retains the original signed binding; the selector identifies only
// the child display. It never grants an inherited execution capability.
type HistoryReaderV1 interface {
	ReadUserInputHistoryHost(context.Context, continuationapp.UserInputLocalDisplayBindingV1, port.SelectorV1) (HistoryRecordV1, error)
}
type HistoryReplyV1 struct {
	ItemID              string `json:"itemId"`
	ClientUserMessageID string `json:"clientUserMessageId"`
	TurnID              string `json:"turnId"`
}
type HistoryRecordV1 struct {
	Record continuationapp.UserInputLocalDisplayV1
	Reply  *HistoryReplyV1
}

type ServiceV1 struct {
	Reader     ContinuationReaderV1
	Authority  port.AccessAuthority
	Discarder  LateReplyDiscarderV1
	DraftSaver DraftSaverV1
}

type OptionV1 struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type PlanReviewIntentV1 struct {
	Kind            string `json:"kind"`
	ApproveOptionID string `json:"approveOptionId"`
}

type QuestionV1 struct {
	Intent      *PlanReviewIntentV1 `json:"intent,omitempty"`
	ID          string              `json:"id"`
	Header      string              `json:"header"`
	Question    string              `json:"question"`
	Detail      *string             `json:"detail,omitempty"`
	Options     []OptionV1          `json:"options"`
	MultiSelect bool                `json:"multiSelect"`
}

type DisplayV1 struct {
	HistoryReply    *HistoryReplyV1                               `json:"historyReply,omitempty"`
	ReadOnly        bool                                          `json:"readOnly,omitempty"`
	ContinuedReason *string                                       `json:"continuedReason"`
	DraftRevision   *string                                       `json:"draftRevision"`
	Draft           *domaincontinuation.UserInputDraftSnapshotV1  `json:"draft"`
	CallID          *string                                       `json:"callId"`
	LateReply       *continuationapp.UserInputLateReplyDeliveryV1 `json:"lateReply"`
	TimedWait       *appturn.TimedWaitViewV1                      `json:"timedWait"`
	SchemaVersion   int                                           `json:"schemaVersion"`
	ThreadID        string                                        `json:"threadId"`
	TurnID          string                                        `json:"turnId"`
	ItemID          string                                        `json:"itemId"`
	RequestID       string                                        `json:"requestId"`
	Status          string                                        `json:"status"`
	Questions       []QuestionV1                                  `json:"questions"`
	AnswerState     string                                        `json:"answerState"`
	Submission      *domaincontinuation.UserInputSubmissionV1     `json:"submission,omitempty"`
}

func (s *ServiceV1) Read(ctx context.Context, selector port.SelectorV1) (DisplayV1, error) {
	unavailable := func() (DisplayV1, error) { return DisplayV1{}, port.ErrUnavailableV1 }
	if s == nil || s.Reader == nil || s.Authority == nil || ctx == nil || ctx.Err() != nil || !port.ValidateSelectorV1(selector) {
		return unavailable()
	}
	authorized, release, err := s.Authority.AcquireCurrent(ctx, selector)
	if err != nil || release == nil {
		if release != nil {
			release()
		}
		return unavailable()
	}
	defer release()
	if authorized.Selector != selector || s.Authority.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	if authorized.ReadOnly {
		return s.readHistory(ctx, selector, authorized)
	}
	record, err := s.Reader.ReadUserInputLocalDisplayHost(ctx, continuationapp.UserInputLocalDisplayBindingV1{
		ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID,
		InputID: selector.RequestID, ReceiptID: authorized.ReceiptID, SecurityContext: authorized.Original,
	})
	if err != nil || record.ThreadID != selector.ThreadID || record.TurnID != selector.TurnID ||
		record.ItemID != selector.ItemID || record.InputID != selector.RequestID || record.ReceiptID != authorized.ReceiptID || record.Status != authorized.Status {
		return unavailable()
	}
	display, err := projectDisplayRecordV1(record, selector)
	if err != nil {
		return unavailable()
	}
	// Disposition CAS precedes the public item status patch. Re-read the
	// trusted pair under this same scope lease, so that intermediate window
	// cannot return a pending or stale answer snapshot.
	finalRecord, err := s.Reader.ReadUserInputLocalDisplayHost(ctx, continuationapp.UserInputLocalDisplayBindingV1{
		ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID,
		InputID: selector.RequestID, ReceiptID: authorized.ReceiptID, SecurityContext: authorized.Original,
	})
	if err != nil || !reflect.DeepEqual(finalRecord, record) || s.Authority.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
		return unavailable()
	}
	return display, nil
}

func (s *ServiceV1) readHistory(ctx context.Context, selector port.SelectorV1, authorized port.AuthorizedV1) (DisplayV1, error) {
	unavailable := func() (DisplayV1, error) { return DisplayV1{}, port.ErrUnavailableV1 }
	reader, ok := s.Reader.(HistoryReaderV1)
	if !ok || !port.ValidateSelectorV1(authorized.Origin) || authorized.Origin.ThreadID == selector.ThreadID ||
		authorized.Origin.RequestID != selector.RequestID || !domainsecurity.IsSHA256Hex(authorized.HistoryDigest) {
		return unavailable()
	}
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: authorized.Origin.ThreadID, TurnID: authorized.Origin.TurnID,
		ItemID: authorized.Origin.ItemID, InputID: authorized.Origin.RequestID, ReceiptID: authorized.ReceiptID, SecurityContext: authorized.Original}
	first, err := reader.ReadUserInputHistoryHost(ctx, binding, selector)
	record := first.Record
	if err != nil || record.ThreadID != selector.ThreadID || record.TurnID != selector.TurnID || record.ItemID != selector.ItemID ||
		record.InputID != binding.InputID || record.ReceiptID != binding.ReceiptID || record.Status != authorized.Status ||
		record.DraftRevision != nil || record.Draft != nil || record.LateReply != nil || record.TimedWait != nil || record.ContinuedReason != nil ||
		(record.Submission != nil) != (first.Reply != nil) {
		return unavailable()
	}
	final, err := reader.ReadUserInputHistoryHost(ctx, binding, selector)
	if err != nil || !reflect.DeepEqual(first, final) || ctx.Err() != nil {
		return unavailable()
	}
	display, err := projectDisplayRecordModeV1(record, selector, true, first.Reply)
	if err != nil {
		return unavailable()
	}
	encoded, encodeErr := json.Marshal(display)
	if !ValidateDisplayV1(display, selector) || encodeErr != nil || len(encoded) > port.MaxResponseBytesV1 ||
		s.Authority.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
		return unavailable()
	}
	return display, nil
}

func questionsV1(records []map[string]any) ([]QuestionV1, bool) {
	questions := make([]QuestionV1, 0, len(records))
	for _, record := range records {
		id, idOK := record["id"].(string)
		header, headerOK := record["header"].(string)
		question, questionOK := record["question"].(string)
		multi, multiOK := record["multiSelect"].(bool)
		options, optionsOK := record["options"].([]map[string]string)
		if !idOK || !headerOK || !questionOK || !multiOK || !optionsOK {
			return nil, false
		}
		projected := QuestionV1{ID: id, Header: header, Question: question, MultiSelect: multi, Options: make([]OptionV1, 0, len(options))}
		if value, exists := record["detail"]; exists {
			detail, ok := value.(string)
			if !ok {
				return nil, false
			}
			projected.Detail = &detail
		}
		for _, option := range options {
			projected.Options = append(projected.Options, OptionV1{ID: option["id"], Label: option["label"], Description: option["description"]})
		}
		questions = append(questions, projected)
	}
	return questions, true
}

func displayTextV1(value string) bool { return utf8.ValidString(value) && len(value) <= 4<<20 }

// ValidateDisplayV1 also guards the HTTP leaf against an invalid service
// implementation. All returned fields belong to this closed private DTO.
func ValidateDisplayV1(d DisplayV1, selector port.SelectorV1) bool {
	if d.ReadOnly && d.Status == "continued" && d.AnswerState == "available" && d.HistoryReply == nil {
		return false
	}
	if d.HistoryReply != nil && (!d.ReadOnly || d.Status != "continued" || d.AnswerState != "available" || d.Submission == nil ||
		!domainthread.IsCanonicalRecordID(d.HistoryReply.ItemID) || !domainthread.IsCanonicalRecordID(d.HistoryReply.TurnID) ||
		!controlapp.IsOpaqueClientUserMessageID(d.HistoryReply.ClientUserMessageID) || d.HistoryReply.ItemID == d.ItemID ||
		d.TimedWait != nil || d.ContinuedReason != nil) {
		return false
	}
	if d.ReadOnly && (d.Status == "pending" || d.DraftRevision != nil || d.Draft != nil || d.LateReply != nil) {
		return false
	}
	if d.ContinuedReason != nil && (d.Status != "continued" || d.TimedWait == nil || !domaincontinuation.ValidUserInputContinuedReasonV1(&domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: d.TimedWait.TimeoutSeconds}, *d.ContinuedReason)) {
		return false
	}
	if d.CallID != nil && (strings.TrimSpace(*d.CallID) != *d.CallID || *d.CallID == "" || len(*d.CallID) > 1024) {
		return false
	}
	if d.SchemaVersion != 1 || !port.ValidateSelectorV1(selector) || d.ThreadID != selector.ThreadID || d.TurnID != selector.TurnID ||
		d.ItemID != selector.ItemID || d.RequestID != selector.RequestID || len(d.Questions) < 1 || len(d.Questions) > 3 {
		return false
	}
	if d.DraftRevision != nil && !domaincontinuation.IsUserInputDraftRevisionV1(*d.DraftRevision) {
		return false
	}
	if d.Draft != nil && (d.DraftRevision == nil || (d.Status != "pending" && d.Status != "continued") || d.LateReply != nil && d.LateReply.State != "discarded" || !validateDraftQuestionsV1(*d.Draft, selector.RequestID, d.Questions)) {
		return false
	}
	if d.TimedWait != nil {
		if !domaincontinuation.ValidUserInputTimeoutSeconds(d.TimedWait.TimeoutSeconds) || ((d.TimedWait.TimeoutSeconds == -1) != (d.TimedWait.DeadlineAt == nil)) {
			return false
		}
		if d.TimedWait.DeadlineAt != nil {
			parsed, err := time.Parse(time.RFC3339Nano, *d.TimedWait.DeadlineAt)
			if err != nil || parsed.UTC().Format(time.RFC3339Nano) != *d.TimedWait.DeadlineAt {
				return false
			}
		}
	}
	questions := make([]map[string]any, 0, len(d.Questions))
	ids := make(map[string]bool, len(d.Questions))
	for index, question := range d.Questions {
		if question.ID != fmt.Sprintf("%s_%d", selector.RequestID, index+1) || !displayTextV1(question.Header) || !displayTextV1(question.Question) ||
			strings.TrimSpace(question.Question) == "" || question.Options == nil || len(question.Options) > 3 || (question.Detail != nil && !displayTextV1(*question.Detail)) {
			return false
		}
		if intent := question.Intent; intent != nil {
			if intent.Kind != "plan-review" || d.CallID == nil || question.Detail == nil || strings.TrimSpace(*question.Detail) == "" || question.MultiSelect {
				return false
			}
			found := false
			for _, option := range question.Options {
				found = found || option.ID == intent.ApproveOptionID
			}
			if !found {
				return false
			}
		}
		ids[question.ID] = true
		options := make([]map[string]string, 0, len(question.Options))
		for optionIndex, option := range question.Options {
			if option.ID != fmt.Sprintf("%s_option_%d", question.ID, optionIndex+1) || !displayTextV1(option.Label) || strings.TrimSpace(option.Label) == "" || !displayTextV1(option.Description) {
				return false
			}
			options = append(options, map[string]string{"id": option.ID, "label": option.Label, "description": option.Description})
		}
		questions = append(questions, map[string]any{"id": question.ID, "multiSelect": question.MultiSelect, "options": options})
	}
	if d.LateReply != nil {
		if d.Status != "continued" || !controlapp.IsOpaqueClientUserMessageID(d.LateReply.ClientUserMessageID) || strings.TrimSpace(d.LateReply.TurnID) == "" || (d.LateReply.State != "queued" && d.LateReply.State != "accepted" && d.LateReply.State != "discarded") || d.AnswerState != "available" || d.Submission == nil {
			return false
		}
	}
	if d.Status == "pending" || d.Status == "cancelled" {
		return d.AnswerState == d.Status && d.Submission == nil
	}
	if d.Status == "continued" {
		if !d.ReadOnly && (d.TimedWait == nil || d.TimedWait.TimeoutSeconds == -1 && (d.ContinuedReason == nil || *d.ContinuedReason != domaincontinuation.ReasonUserInputOutcomeUnknownAfterRestartV1)) {
			return false
		}
		if d.LateReply == nil && d.AnswerState == "unavailable" {
			return d.Submission == nil
		}
	} else if d.Status != "submitted" {
		return false
	}
	if d.AnswerState == "unavailable" {
		return d.Submission == nil
	}
	if d.AnswerState != "available" || d.Submission == nil || domaincontinuation.ValidateUserInputSubmissionV1(*d.Submission) != nil {
		return false
	}
	if d.Submission.Format == "structured" {
		var answers []controlapp.UserInputAnswer
		if json.Unmarshal(d.Submission.Answers, &answers) != nil {
			return false
		}
		_, err := controlapp.NormalizeUserInputAnswers(controlapp.UserInputResponse{StructuredAnswers: answers}, questions, "")
		return err == nil
	}
	var answers []map[string]string
	if json.Unmarshal(d.Submission.Answers, &answers) != nil || len(answers) != len(questions) {
		return false
	}
	seen := make(map[string]bool, len(answers))
	for _, answer := range answers {
		id := answer["questionId"]
		if !ids[id] || seen[id] || !displayTextV1(answer["value"]) {
			return false
		}
		seen[id] = true
	}
	return true
}

var ErrDiscardConflictV1 = errors.New("late Question reply was already accepted")

// Implemented by the existing Host store/receipt authority. Wire callers
// cannot supply an AuthorizedV1 or a cancellation proof.
type LateReplyDiscarderV1 interface {
	DiscardLateReply(context.Context, port.AuthorizedV1, string, string) (continuationapp.UserInputLateReplyDeliveryV1, error)
}

type DiscardLateReplyRequestV1 struct {
	port.SelectorV1
	Action              string `json:"action"`
	ClientUserMessageID string `json:"clientUserMessageId"`
	ExpectedTurnID      string `json:"expectedTurnId"`
}

type DiscardLateReplyResponseV1 struct {
	port.SelectorV1
	SchemaVersion int                                          `json:"schemaVersion"`
	Status        string                                       `json:"status"`
	LateReply     continuationapp.UserInputLateReplyDeliveryV1 `json:"lateReply"`
}

func ValidateDiscardLateReplyRequestV1(request DiscardLateReplyRequestV1) bool {
	return request.Action == "discardLateReply" && port.ValidateSelectorV1(request.SelectorV1) &&
		controlapp.IsOpaqueClientUserMessageID(request.ClientUserMessageID) && domainthread.IsCanonicalRecordID(request.ExpectedTurnID)
}

func ValidateDiscardLateReplyResponseV1(response DiscardLateReplyResponseV1, request DiscardLateReplyRequestV1) bool {
	return ValidateDiscardLateReplyRequestV1(request) && response.SelectorV1 == request.SelectorV1 && response.SchemaVersion == 1 &&
		response.Status == "continued" && response.LateReply.ClientUserMessageID == request.ClientUserMessageID &&
		response.LateReply.TurnID == request.ExpectedTurnID && response.LateReply.State == "discarded"
}

func (s *ServiceV1) DiscardLateReply(ctx context.Context, request DiscardLateReplyRequestV1) (DiscardLateReplyResponseV1, error) {
	unavailable := func() (DiscardLateReplyResponseV1, error) { return DiscardLateReplyResponseV1{}, port.ErrUnavailableV1 }
	if s == nil || s.Reader == nil || s.Authority == nil || s.Discarder == nil || ctx == nil || ctx.Err() != nil || !ValidateDiscardLateReplyRequestV1(request) {
		return unavailable()
	}
	authorized, release, err := s.Authority.AcquireCurrent(ctx, request.SelectorV1)
	if err != nil || release == nil {
		if release != nil {
			release()
		}
		return unavailable()
	}
	defer release()
	if authorized.ReadOnly || authorized.Selector != request.SelectorV1 || authorized.Status != "continued" || !domainsecurity.IsSHA256Hex(authorized.ReceiptID) || s.Authority.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	record, err := s.Reader.ReadUserInputLocalDisplayHost(ctx, continuationapp.UserInputLocalDisplayBindingV1{
		ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID,
		InputID: request.RequestID, ReceiptID: authorized.ReceiptID, SecurityContext: authorized.Original,
	})
	if err != nil || record.ThreadID != request.ThreadID || record.TurnID != request.TurnID || record.ItemID != request.ItemID ||
		record.InputID != request.RequestID || record.ReceiptID != authorized.ReceiptID || record.Status != "continued" ||
		record.ContinuedReason == nil || !domaincontinuation.ValidUserInputContinuedReasonV1(record.TimedWait, *record.ContinuedReason) || s.Authority.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
		return unavailable()
	}
	delivery, err := s.Discarder.DiscardLateReply(ctx, authorized, request.ClientUserMessageID, request.ExpectedTurnID)
	if err != nil {
		if s.Authority.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
			return unavailable()
		}
		if errors.Is(err, ErrDiscardConflictV1) {
			return DiscardLateReplyResponseV1{}, ErrDiscardConflictV1
		}
		return unavailable()
	}
	response := DiscardLateReplyResponseV1{SelectorV1: request.SelectorV1, SchemaVersion: 1, Status: "continued", LateReply: delivery}
	if !ValidateDiscardLateReplyResponseV1(response, request) || s.Authority.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
		return unavailable()
	}
	return response, nil
}

// DraftSaverV1 writes through the existing private continuation receipt CAS.
// Clients cannot supply a receipt, capture-set witness or authority context.
type DraftSaverV1 interface {
	SaveUserInputDraftHost(context.Context, continuationapp.UserInputLocalDisplayBindingV1, string, *string, domaincontinuation.UserInputDraftSnapshotV1) (domaincontinuation.Receipt, error)
}

type SaveDraftRequestV1 struct {
	port.SelectorV1
	Action           string                                      `json:"action"`
	Revision         string                                      `json:"revision"`
	ExpectedRevision *string                                     `json:"expectedRevision"`
	Draft            domaincontinuation.UserInputDraftSnapshotV1 `json:"draft"`
}

type SaveDraftResponseV1 struct {
	port.SelectorV1
	SchemaVersion int                                         `json:"schemaVersion"`
	Status        string                                      `json:"status"`
	DraftRevision string                                      `json:"draftRevision"`
	Draft         domaincontinuation.UserInputDraftSnapshotV1 `json:"draft"`
}

func ValidateSaveDraftRequestV1(request SaveDraftRequestV1) bool {
	return request.Action == "saveDraft" && port.ValidateSelectorV1(request.SelectorV1) && domaincontinuation.IsUserInputDraftRevisionV1(request.Revision) && (request.ExpectedRevision == nil || domaincontinuation.IsUserInputDraftRevisionV1(*request.ExpectedRevision) && *request.ExpectedRevision != request.Revision) && domaincontinuation.ValidateUserInputDraftSnapshotV1(request.Draft, request.RequestID) == nil
}

func ValidateSaveDraftResponseV1(response SaveDraftResponseV1, request SaveDraftRequestV1) bool {
	return ValidateSaveDraftRequestV1(request) && response.SelectorV1 == request.SelectorV1 && response.SchemaVersion == 1 && (response.Status == "pending" || response.Status == "continued") && response.DraftRevision == request.Revision && reflect.DeepEqual(response.Draft, request.Draft)
}

func validateDraftQuestionsV1(snapshot domaincontinuation.UserInputDraftSnapshotV1, requestID string, questions []QuestionV1) bool {
	if domaincontinuation.ValidateUserInputDraftSnapshotV1(snapshot, requestID) != nil || snapshot.CurrentPage >= len(questions) {
		return false
	}
	for _, answer := range snapshot.Answers {
		var question *QuestionV1
		for i := range questions {
			if questions[i].ID == answer.QuestionID {
				question = &questions[i]
				break
			}
		}
		if question == nil {
			return false
		}
		count := len(answer.OptionIDs)
		if strings.TrimSpace(answer.CustomText) != "" {
			count++
		}
		if !question.MultiSelect && count > 1 {
			return false
		}
		for _, id := range answer.OptionIDs {
			found := false
			for _, option := range question.Options {
				if option.ID == id {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func draftEditableRecordV1(record continuationapp.UserInputLocalDisplayV1) bool {
	return (record.Status == "pending" || record.Status == "continued") && (record.LateReply == nil || record.LateReply.State == "discarded")
}

func (s *ServiceV1) SaveDraft(ctx context.Context, request SaveDraftRequestV1) (SaveDraftResponseV1, error) {
	unavailable := func() (SaveDraftResponseV1, error) { return SaveDraftResponseV1{}, port.ErrUnavailableV1 }
	if s == nil || s.Reader == nil || s.Authority == nil || s.DraftSaver == nil || ctx == nil || ctx.Err() != nil || !ValidateSaveDraftRequestV1(request) {
		return unavailable()
	}
	authorized, release, err := s.Authority.AcquireCurrent(ctx, request.SelectorV1)
	if err != nil || release == nil {
		if release != nil {
			release()
		}
		return unavailable()
	}
	defer release()
	if authorized.ReadOnly || authorized.Selector != request.SelectorV1 || !domainsecurity.IsSHA256Hex(authorized.ReceiptID) || s.Authority.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, InputID: request.RequestID, ReceiptID: authorized.ReceiptID, SecurityContext: authorized.Original}
	record, err := s.Reader.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || record.ThreadID != request.ThreadID || record.TurnID != request.TurnID || record.ItemID != request.ItemID || record.InputID != request.RequestID || record.ReceiptID != authorized.ReceiptID || record.Status != authorized.Status || !draftEditableRecordV1(record) || s.Authority.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	baseDisplay, err := projectDisplayRecordV1(record, request.SelectorV1)
	if err != nil {
		return unavailable()
	}
	if !validateDraftQuestionsV1(request.Draft, request.RequestID, baseDisplay.Questions) {
		return SaveDraftResponseV1{}, continuationapp.ErrUserInputDraftInvalidV1
	}
	candidate := record
	revision := request.Revision
	candidate.DraftRevision, candidate.Draft = &revision, &request.Draft
	if _, err := projectDisplayRecordV1(candidate, request.SelectorV1); err != nil {
		return SaveDraftResponseV1{}, continuationapp.ErrUserInputDraftCapacityV1
	}
	receipt, err := s.DraftSaver.SaveUserInputDraftHost(ctx, binding, request.Revision, request.ExpectedRevision, request.Draft)
	if err != nil {
		if s.Authority.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
			return unavailable()
		}
		if errors.Is(err, continuationapp.ErrUserInputDraftConflictV1) || errors.Is(err, continuationapp.ErrUserInputDraftCapacityV1) || errors.Is(err, continuationapp.ErrUserInputDraftInvalidV1) {
			return SaveDraftResponseV1{}, err
		}
		return unavailable()
	}
	link := receipt.Payload.UserInputDraft
	if domaincontinuation.ValidateReceipt(receipt) != nil || link == nil || link.SourceInputID != request.RequestID || link.SourceReceiptID != authorized.ReceiptID || receipt.Payload.ThreadID != request.ThreadID || receipt.Payload.TurnID != request.TurnID || link.Revision != request.Revision || !equalDraftExpectedV1(link.ParentRevision, request.ExpectedRevision) || !reflect.DeepEqual(link.Snapshot, request.Draft) {
		return unavailable()
	}
	final, err := s.Reader.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || final.ThreadID != record.ThreadID || final.TurnID != record.TurnID || final.ItemID != record.ItemID || final.InputID != record.InputID || final.ReceiptID != record.ReceiptID || final.Status != authorized.Status || !draftEditableRecordV1(final) || final.Draft == nil || s.Authority.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
		return unavailable()
	}
	if _, err := projectDisplayRecordV1(final, request.SelectorV1); err != nil {
		return unavailable()
	}
	response := SaveDraftResponseV1{SelectorV1: request.SelectorV1, SchemaVersion: 1, Status: final.Status, DraftRevision: link.Revision, Draft: domaincontinuation.CloneUserInputDraftSnapshotV1(link.Snapshot)}
	if !ValidateSaveDraftResponseV1(response, request) {
		return unavailable()
	}
	return response, nil
}

func equalDraftExpectedV1(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

// Use the same closed projection and response cap before persisting a new
// snapshot and when reading it. A small save ACK cannot admit an unreadable
// combined question/submission/draft display.
func projectDisplayRecordV1(record continuationapp.UserInputLocalDisplayV1, selector port.SelectorV1) (DisplayV1, error) {
	return projectDisplayRecordModeV1(record, selector, false, nil)
}

func projectDisplayRecordModeV1(record continuationapp.UserInputLocalDisplayV1, selector port.SelectorV1, readOnly bool, historyReply *HistoryReplyV1) (DisplayV1, error) {
	unavailable := func() (DisplayV1, error) { return DisplayV1{}, port.ErrUnavailableV1 }
	args, err := domainjsonstrict.DecodeObject(record.Arguments, domainjsonstrict.Options{MaxBytes: 4 << 20})
	if err != nil {
		return unavailable()
	}
	args, err = toolcatalogapp.UserInputArgumentsForTool(record.ToolName, args)
	if err != nil {
		return unavailable()
	}
	prompt := "User input required"
	for _, key := range []string{"prompt", "question"} {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			prompt = value
			break
		}
	}
	questionMaps := toolcatalogapp.UserInputQuestionsForLocalDisplay(args, selector.RequestID, prompt)
	questions, ok := questionsV1(questionMaps)
	if !ok {
		return unavailable()
	}
	if record.ToolName == toolcatalogapp.ToolExitPlanModeName && len(questions) == 1 && len(questions[0].Options) == 2 {
		questions[0].Intent = &PlanReviewIntentV1{Kind: "plan-review", ApproveOptionID: questions[0].Options[0].ID}
	}
	var callID *string
	if record.CallID != "" {
		value := record.CallID
		callID = &value
	}
	display := DisplayV1{ReadOnly: readOnly, ContinuedReason: record.ContinuedReason, DraftRevision: record.DraftRevision, Draft: record.Draft, CallID: callID, LateReply: record.LateReply, TimedWait: appturn.UserInputTimedWaitViewV1(record.TimedWait, record.IssuedAt), SchemaVersion: 1, ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, RequestID: selector.RequestID, Status: record.Status, Questions: questions}
	if historyReply != nil {
		value := *historyReply
		display.HistoryReply = &value
	}
	if record.Draft != nil {
		value := domaincontinuation.CloneUserInputDraftSnapshotV1(*record.Draft)
		display.Draft = &value
	}
	if record.DraftRevision != nil {
		value := *record.DraftRevision
		display.DraftRevision = &value
	}
	switch record.Status {
	case "pending":
		if record.Submission != nil || record.AnswerState != "" {
			return unavailable()
		}
		display.AnswerState = "pending"
	case "cancelled":
		if record.Submission != nil || record.AnswerState != "" {
			return unavailable()
		}
		display.AnswerState = "cancelled"
	case "continued":
		if !readOnly && (record.ContinuedReason == nil || !domaincontinuation.ValidUserInputContinuedReasonV1(record.TimedWait, *record.ContinuedReason)) {
			return unavailable()
		}
		display.AnswerState = record.AnswerState
		if record.Submission != nil {
			copy := domaincontinuation.CloneUserInputSubmissionV1(*record.Submission)
			display.Submission = &copy
		}
	case "submitted":
		display.AnswerState = record.AnswerState
		if record.Submission != nil {
			cloned := domaincontinuation.CloneUserInputSubmissionV1(*record.Submission)
			display.Submission = &cloned
		}
	default:
		return unavailable()
	}
	if !ValidateDisplayV1(display, selector) {
		return unavailable()
	}
	encoded, err := json.Marshal(display)
	if err != nil || len(encoded) > port.MaxResponseBytesV1 {
		return unavailable()
	}
	return display, nil
}
