package userinputdisplay

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	port "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

type displayAuthorityFixtureV1 struct {
	authorized port.AuthorizedV1
	checks     int
	failAt     int
	active     bool
	released   bool
}

func (a *displayAuthorityFixtureV1) AcquireCurrent(context.Context, port.SelectorV1) (port.AuthorizedV1, func(), error) {
	a.active = true
	return a.authorized, func() { a.active = false; a.released = true }, nil
}

func (a *displayAuthorityFixtureV1) ValidateCurrent(context.Context, port.AuthorizedV1) error {
	a.checks++
	if !a.active || a.checks == a.failAt {
		return port.ErrUnavailableV1
	}
	return nil
}

type displayContinuationFixtureV1 struct {
	authority            *displayAuthorityFixtureV1
	record               continuationapp.UserInputLocalDisplayV1
	called               bool
	calls                int
	changeAfterRead      bool
	binding              continuationapp.UserInputLocalDisplayBindingV1
	err                  error
	historyReply         *HistoryReplyV1
	historySelector      port.SelectorV1
	changeReplyAfterRead bool
}

func (f *displayContinuationFixtureV1) ReadUserInputLocalDisplayHost(_ context.Context, binding continuationapp.UserInputLocalDisplayBindingV1) (continuationapp.UserInputLocalDisplayV1, error) {
	f.called, f.binding = true, binding
	f.calls++
	if f.changeAfterRead && f.calls == 2 {
		f.record.Status = "submitted"
		f.record.AnswerState = "unavailable"
	}
	if !f.authority.active {
		return continuationapp.UserInputLocalDisplayV1{}, port.ErrUnavailableV1
	}
	return f.record, f.err
}

func (f *displayContinuationFixtureV1) ReadUserInputHistoryHost(ctx context.Context, binding continuationapp.UserInputLocalDisplayBindingV1, selector port.SelectorV1) (HistoryRecordV1, error) {
	record, err := f.ReadUserInputLocalDisplayHost(ctx, binding)
	f.historySelector = selector
	record.ThreadID, record.TurnID, record.ItemID = selector.ThreadID, selector.TurnID, selector.ItemID
	var reply *HistoryReplyV1
	if f.historyReply != nil {
		value := *f.historyReply
		reply = &value
	}
	if f.changeReplyAfterRead && f.calls == 2 {
		reply.ItemID = "changed-reply"
	}
	return HistoryRecordV1{Record: record, Reply: reply}, err
}

func TestInheritedQuestionDisplayUsesOriginalRequestButChildReadonlyTuple(t *testing.T) {
	for _, status := range []string{"cancelled", "submitted", "continued"} {
		service, authority, reader, origin := newDisplayFixtureV1("pending")
		child := origin
		child.ThreadID = "child"
		authority.authorized.Selector, authority.authorized.Origin = child, origin
		authority.authorized.ReadOnly, authority.authorized.Status = true, status
		authority.authorized.HistoryDigest = strings.Repeat("c", 64)
		reader.record.CallID = "original-call"
		reader.record.Status = status
		if status == "continued" || status == "submitted" {
			reader.record.AnswerState = "unavailable"
		}
		view, err := service.Read(context.Background(), child)
		if err != nil || !view.ReadOnly || view.ThreadID != child.ThreadID || view.Status != status || !view.Questions[0].MultiSelect || view.Questions[0].ID != origin.RequestID+"_1" || *view.CallID != "original-call" {
			t.Fatal("history lost original semantics or gained current tuple")
		}
		if reader.binding.ThreadID != origin.ThreadID || view.TimedWait != nil || view.Draft != nil || view.LateReply != nil || view.Submission != nil || !authority.released {
			t.Fatal("history rebound original receipt or exposed later state")
		}
	}
}

func TestInheritedQuestionDisplayRejectsLaterStateAndAuthorityRaces(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*displayAuthorityFixtureV1, *displayContinuationFixtureV1)
	}{
		{"changed_original", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) { r.changeAfterRead = true }},
		{"changed_lineage", func(a *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1) { a.failAt = 2 }},
		{"missing_digest", func(a *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1) { a.authorized.HistoryDigest = "" }},
		{"later_submission", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) {
			r.record.Submission = &domaincontinuation.UserInputSubmissionV1{}
		}},
		{"later_draft", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) {
			r.record.Draft = &domaincontinuation.UserInputDraftSnapshotV1{}
		}},
		{"later_delivery", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) {
			r.record.LateReply = &continuationapp.UserInputLateReplyDeliveryV1{}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, authority, reader, origin := newDisplayFixtureV1("pending")
			child := origin
			child.ThreadID = "child"
			authority.authorized.Selector, authority.authorized.Origin = child, origin
			authority.authorized.ReadOnly, authority.authorized.Status = true, "cancelled"
			authority.authorized.HistoryDigest = strings.Repeat("c", 64)
			reader.record.Status = "cancelled"
			test.change(authority, reader)
			if _, err := service.Read(context.Background(), child); !errors.Is(err, port.ErrUnavailableV1) || !authority.released {
				t.Fatal("history exposed later state or passed a changed authority")
			}
		})
	}
}

func newDisplayFixtureV1(status string) (*ServiceV1, *displayAuthorityFixtureV1, *displayContinuationFixtureV1, port.SelectorV1) {
	selector := port.SelectorV1{ThreadID: "thread-a", TurnID: "turn-a", ItemID: "item_input_" + strings.Repeat("a", 64), RequestID: "input_" + strings.Repeat("a", 64)}
	authority := &displayAuthorityFixtureV1{authorized: port.AuthorizedV1{Selector: selector, ReceiptID: strings.Repeat("b", 64), Status: status}}
	record := continuationapp.UserInputLocalDisplayV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, InputID: selector.RequestID, ReceiptID: authority.authorized.ReceiptID, Status: status,
		Arguments: json.RawMessage(`{"prompt":" Choose ","questions":[{"question":" First ","header":" Header ","detail":" detail\n ","multi_select":true,"options":[{"label":" Same ","description":" first description "},{"label":" Same ","description":" second description "}]},{"question":" Second ","options":[]}]}`)}
	if status == "continued" {
		reason := domaincontinuation.ReasonUserInputContinuedV1
		record.ContinuedReason = &reason
	}
	reader := &displayContinuationFixtureV1{authority: authority, record: record}
	return &ServiceV1{Reader: reader, Authority: authority}, authority, reader, selector
}

func TestUserInputDisplayUsesOriginalStringsCoreIDsAndBoundLease(t *testing.T) {
	service, authority, reader, selector := newDisplayFixtureV1("submitted")
	reader.record.CallID = "call_host_" + strings.Repeat("a", 64)
	answers, _ := json.Marshal([]map[string]any{
		{"id": selector.RequestID + "_1", "kind": "answer", "optionIds": []string{selector.RequestID + "_1_option_2"}, "customText": " original custom\n "},
		{"id": selector.RequestID + "_2", "kind": "skip"},
	})
	reader.record.AnswerState = "available"
	reader.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: answers}
	display, err := service.Read(context.Background(), selector)
	if err != nil || !authority.released || authority.checks != 2 || !reader.called || reader.binding.InputID != selector.RequestID || reader.binding.ReceiptID != authority.authorized.ReceiptID {
		t.Fatal("read did not retain exact receipt binding under the current-scope lease")
	}
	if display.CallID == nil || *display.CallID != reader.record.CallID {
		t.Fatal("private display lost the original receipt CallID")
	}
	first := display.Questions[0]
	if first.Question != " First " || first.Header != " Header " || first.Detail == nil || *first.Detail != " detail\n " || first.Options[1].Description != " second description " ||
		first.ID != selector.RequestID+"_1" || first.Options[1].ID != selector.RequestID+"_1_option_2" || !first.MultiSelect {
		t.Fatal("original presentation values or effective Core identities changed")
	}
	if display.Submission == reader.record.Submission || string(display.Submission.Answers) != string(answers) {
		t.Fatal("submission bytes changed or shared a mutable private record")
	}
	encoded, _ := json.Marshal(display)
	for _, forbidden := range []string{"receiptId", "arguments", "executionGrant", "turnSecurityContext", "providerId"} {
		if strings.Contains(string(encoded), `"`+forbidden+`"`) {
			t.Fatal("private authority entered display DTO")
		}
	}
}

func TestUserInputDisplayNeverFabricatesOldOrCancelledAnswers(t *testing.T) {
	for _, status := range []string{"pending", "cancelled", "submitted"} {
		service, _, reader, selector := newDisplayFixtureV1(status)
		expected := status
		if status == "submitted" {
			reader.record.AnswerState, expected = "unavailable", "unavailable"
		}
		display, err := service.Read(context.Background(), selector)
		if err != nil || display.AnswerState != expected || display.Submission != nil {
			t.Fatal("missing or cancelled submission fabricated answers")
		}
	}
}

func TestUserInputDisplayRejectsIdentityStatusAndAuthorityRaces(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*displayAuthorityFixtureV1, *displayContinuationFixtureV1)
	}{
		{"before_read", func(a *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1) { a.failAt = 1 }},
		{"after_read", func(a *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1) { a.failAt = 2 }},
		{"disposition_before_primary_patch", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) { r.changeAfterRead = true }},
		{"foreign_request", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) {
			r.record.InputID = "input_" + strings.Repeat("c", 64)
		}},
		{"foreign_receipt", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) {
			r.record.ReceiptID = strings.Repeat("c", 64)
		}},
		{"transitional_status", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) { r.record.Status = "cancelled" }},
		{"untrusted_record", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) {
			r.err = errors.New("private unsafe body")
		}},
		{"pending_answers", func(_ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1) {
			r.record.AnswerState = "available"
			r.record.Submission = &domaincontinuation.UserInputSubmissionV1{}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, authority, reader, selector := newDisplayFixtureV1("pending")
			test.change(authority, reader)
			if _, err := service.Read(context.Background(), selector); !errors.Is(err, port.ErrUnavailableV1) || !authority.released {
				t.Fatal("failed authority or binding escaped the private read lease")
			}
			if test.name == "before_read" && reader.called {
				t.Fatal("private record read after authority denial")
			}
		})
	}
}

func TestUserInputDisplayRejectsForeignAnswerIdentitiesAndMalformedOriginals(t *testing.T) {
	for _, mutate := range []func(*displayContinuationFixtureV1){
		func(r *displayContinuationFixtureV1) {
			r.record.Arguments = json.RawMessage(`{"prompt":"x","prompt":"y"}`)
		},
		func(r *displayContinuationFixtureV1) {
			r.record.Arguments = json.RawMessage(`{"questions":[{"question":"1"},{"question":"2"},{"question":"3"},{"question":"4"}]}`)
		},
		func(r *displayContinuationFixtureV1) {
			r.record.Arguments = json.RawMessage(`{"prompt":"x","options":["1","2","3","4"]}`)
			answers, _ := json.Marshal([]map[string]string{{"id": r.record.InputID + "_1", "kind": "skip"}})
			r.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: answers}
		},
		func(r *displayContinuationFixtureV1) {
			r.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: json.RawMessage(`[{"id":"q1","kind":"skip"},{"id":"q2","kind":"skip"}]`)}
		},
		func(r *displayContinuationFixtureV1) {
			r.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "legacy", Answers: json.RawMessage(`[{"id":"q1","value":"original","questionId":"foreign"},{"id":"q2","value":"other","questionId":"foreign2"}]`)}
		},
	} {
		service, authority, reader, selector := newDisplayFixtureV1("submitted")
		reader.record.AnswerState = "available"
		answers, _ := json.Marshal([]map[string]string{{"id": selector.RequestID + "_1", "kind": "skip"}, {"id": selector.RequestID + "_2", "kind": "skip"}})
		reader.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: answers}
		mutate(reader)
		if _, err := service.Read(context.Background(), selector); !errors.Is(err, port.ErrUnavailableV1) || !authority.released {
			t.Fatal("malformed original or foreign answer was displayed")
		}
	}
}

func TestUserInputDisplayPreservesOriginalLegacySingletonAnswer(t *testing.T) {
	service, _, reader, selector := newDisplayFixtureV1("submitted")
	reader.record.Arguments = json.RawMessage(`{"prompt":" original prompt "}`)
	answers, _ := json.Marshal([]map[string]string{{"id": "q1", "questionId": selector.RequestID + "_1", "value": " original value\n ", "label": " original label "}})
	reader.record.AnswerState = "available"
	reader.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "legacy", Answers: answers}
	display, err := service.Read(context.Background(), selector)
	if err != nil || display.Submission == nil || display.Submission.Format != "legacy" || string(display.Submission.Answers) != string(answers) {
		t.Fatal("original legacy alias/value was normalized or reconstructed")
	}
}

func TestUserInputDisplayRejectsLateDeliveryOnUncontinuedRequest(t *testing.T) {
	for _, status := range []string{"pending", "cancelled"} {
		service, _, _, selector := newDisplayFixtureV1(status)
		display, err := service.Read(context.Background(), selector)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(display)
		if !strings.Contains(string(encoded), `"timedWait":null`) || !strings.Contains(string(encoded), `"lateReply":null`) {
			t.Fatal("legacy private display lost real nullable metadata")
		}
		display.LateReply = &continuationapp.UserInputLateReplyDeliveryV1{ClientUserMessageID: "00000000-0000-4000-8000-000000000001", TurnID: selector.TurnID, State: "queued"}
		if ValidateDisplayV1(display, selector) {
			t.Fatal("uncontinued request accepted delivery metadata")
		}
	}
}

type displayDiscardFixtureV1 struct {
	authority               *displayAuthorityFixtureV1
	delivery                continuationapp.UserInputLateReplyDeliveryV1
	calls                   int
	authorized              port.AuthorizedV1
	attemptID, targetTurnID string
	err                     error
}

func (f *displayDiscardFixtureV1) DiscardLateReply(_ context.Context, authorized port.AuthorizedV1, attemptID, targetTurnID string) (continuationapp.UserInputLateReplyDeliveryV1, error) {
	f.calls++
	f.authorized, f.attemptID, f.targetTurnID = authorized, attemptID, targetTurnID
	if !f.authority.active {
		return continuationapp.UserInputLateReplyDeliveryV1{}, port.ErrUnavailableV1
	}
	return f.delivery, f.err
}

func newDiscardDisplayFixtureV1() (*ServiceV1, *displayAuthorityFixtureV1, *displayContinuationFixtureV1, *displayDiscardFixtureV1, DiscardLateReplyRequestV1) {
	service, authority, reader, selector := newDisplayFixtureV1("continued")
	reader.record.TimedWait = &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 1}
	request := DiscardLateReplyRequestV1{SelectorV1: selector, Action: "discardLateReply", ClientUserMessageID: "00000000-0000-4000-8000-000000000001", ExpectedTurnID: "turn-target"}
	discarder := &displayDiscardFixtureV1{authority: authority, delivery: continuationapp.UserInputLateReplyDeliveryV1{ClientUserMessageID: request.ClientUserMessageID, TurnID: request.ExpectedTurnID, State: "discarded"}}
	service.Discarder = discarder
	return service, authority, reader, discarder, request
}

func TestUserInputDiscardBoundaryKeepsOriginalAndTargetTurnsSeparate(t *testing.T) {
	service, authority, reader, discarder, request := newDiscardDisplayFixtureV1()
	response, err := service.DiscardLateReply(context.Background(), request)
	if err != nil || !ValidateDiscardLateReplyResponseV1(response, request) || !authority.released || discarder.calls != 1 ||
		reader.binding.TurnID != request.TurnID || discarder.targetTurnID != request.ExpectedTurnID || discarder.attemptID != request.ClientUserMessageID ||
		discarder.authorized.Selector != request.SelectorV1 || discarder.authorized.ReceiptID != authority.authorized.ReceiptID {
		t.Fatal("discard boundary changed the original receipt or exact queued attempt")
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"receiptId", "arguments", "submission", "executionGrant", "securityContext", "cancelledAt", "cancellationAuthoritySignature"} {
		if strings.Contains(string(encoded), `"`+forbidden+`"`) {
			t.Fatal("internal cancellation authority entered the private result DTO")
		}
	}
}

func TestUserInputDiscardBoundaryRejectsBeforeMutation(t *testing.T) {
	for name, change := range map[string]func(*ServiceV1, *displayAuthorityFixtureV1, *displayContinuationFixtureV1, *DiscardLateReplyRequestV1){
		"missing_discard_authority": func(s *ServiceV1, _ *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1, _ *DiscardLateReplyRequestV1) {
			s.Discarder = nil
		},
		"pending_question": func(_ *ServiceV1, a *displayAuthorityFixtureV1, r *displayContinuationFixtureV1, _ *DiscardLateReplyRequestV1) {
			a.authorized.Status = "pending"
			r.record.Status = "pending"
		},
		"missing_receipt": func(_ *ServiceV1, a *displayAuthorityFixtureV1, r *displayContinuationFixtureV1, _ *DiscardLateReplyRequestV1) {
			a.authorized.ReceiptID = ""
			r.record.ReceiptID = ""
		},
		"wrong_receipt": func(_ *ServiceV1, _ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1, _ *DiscardLateReplyRequestV1) {
			r.record.ReceiptID = strings.Repeat("c", 64)
		},
		"wrong_thread": func(_ *ServiceV1, _ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1, _ *DiscardLateReplyRequestV1) {
			r.record.ThreadID = "foreign"
		},
		"unlimited_question": func(_ *ServiceV1, _ *displayAuthorityFixtureV1, r *displayContinuationFixtureV1, _ *DiscardLateReplyRequestV1) {
			r.record.TimedWait.TimeoutSeconds = -1
		},
		"expired_scope": func(_ *ServiceV1, a *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1, _ *DiscardLateReplyRequestV1) {
			a.failAt = 2
		},
		"non_uuid_attempt": func(_ *ServiceV1, _ *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1, r *DiscardLateReplyRequestV1) {
			r.ClientUserMessageID = "arbitrary-text"
		},
		"noncanonical_target": func(_ *ServiceV1, _ *displayAuthorityFixtureV1, _ *displayContinuationFixtureV1, r *DiscardLateReplyRequestV1) {
			r.ExpectedTurnID = "../target"
		},
	} {
		t.Run(name, func(t *testing.T) {
			service, authority, reader, discarder, request := newDiscardDisplayFixtureV1()
			change(service, authority, reader, &request)
			if _, err := service.DiscardLateReply(context.Background(), request); !errors.Is(err, port.ErrUnavailableV1) || discarder.calls != 0 {
				t.Fatal("invalid discard boundary reached a mutation")
			}
		})
	}
}

func TestUserInputDiscardBoundaryRetainsConflictAndMissingDeliveryTruth(t *testing.T) {
	for _, state := range []string{"", "queued", "accepted"} {
		service, _, _, discarder, request := newDiscardDisplayFixtureV1()
		discarder.delivery.State = state
		if _, err := service.DiscardLateReply(context.Background(), request); !errors.Is(err, port.ErrUnavailableV1) {
			t.Fatal("non-discarded delivery became optimistic success")
		}
	}
	service, _, _, discarder, request := newDiscardDisplayFixtureV1()
	discarder.err = ErrDiscardConflictV1
	if _, err := service.DiscardLateReply(context.Background(), request); !errors.Is(err, ErrDiscardConflictV1) {
		t.Fatal("accepted race lost its real conflict")
	}
}

func TestPrivateQuestionDraftDisplayUsesHostIDsAndNullableCursor(t *testing.T) {
	service, _, reader, selector := newDisplayFixtureV1("pending")
	revision := "00000000-0000-4000-8000-000000000001"
	reader.record.DraftRevision = &revision
	reader.record.Draft = &domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 1, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: selector.RequestID + "_2", OptionIDs: []string{}, CustomText: " \n "}}}
	display, err := service.Read(context.Background(), selector)
	if err != nil || display.Draft == nil || display.Submission != nil || display.AnswerState != "pending" {
		t.Fatal("unfinished state became submission or disappeared", err)
	}
	for _, change := range []func(*DisplayV1){
		func(v *DisplayV1) { v.Draft.CurrentPage = 2 },
		func(v *DisplayV1) { v.Draft.Answers[0].QuestionID = selector.RequestID + "_3" },
		func(v *DisplayV1) { v.Draft.Answers[0].OptionIDs = []string{selector.RequestID + "_2_option_1"} },
		func(v *DisplayV1) { v.DraftRevision = nil },
		func(v *DisplayV1) { v.Status = "cancelled"; v.AnswerState = "cancelled" },
	} {
		candidate := display
		copy := domaincontinuation.CloneUserInputDraftSnapshotV1(*display.Draft)
		candidate.Draft = &copy
		change(&candidate)
		if ValidateDisplayV1(candidate, selector) {
			t.Fatal("unbound draft displayed")
		}
	}
	reader.record.Draft = nil
	display, err = service.Read(context.Background(), selector)
	if err != nil || display.Draft != nil || display.DraftRevision == nil {
		t.Fatal("cleared content lost parent cursor", err)
	}
	body, _ := json.Marshal(display)
	if !strings.Contains(string(body), `"draft":null`) {
		t.Fatal("cleared draft was not actually nullable")
	}
}

type draftSaveNeverFixtureV1 struct{ calls int }

func (f *draftSaveNeverFixtureV1) SaveUserInputDraftHost(context.Context, continuationapp.UserInputLocalDisplayBindingV1, string, *string, domaincontinuation.UserInputDraftSnapshotV1) (domaincontinuation.Receipt, error) {
	f.calls++
	return domaincontinuation.Receipt{}, continuationapp.ErrAuthorityUnavailable
}
func TestPrivateQuestionDraftRejectsUnboundOrUnreadableSnapshotBeforeWrite(t *testing.T) {
	service, _, reader, selector := newDisplayFixtureV1("pending")
	saver := &draftSaveNeverFixtureV1{}
	service.DraftSaver = saver
	request := SaveDraftRequestV1{SelectorV1: selector, Action: "saveDraft", Revision: "00000000-0000-4000-8000-000000000001", Draft: domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: selector.RequestID + "_1", OptionIDs: []string{selector.RequestID + "_1_option_3"}}}}}
	if _, err := service.SaveDraft(context.Background(), request); !errors.Is(err, continuationapp.ErrUserInputDraftInvalidV1) || saver.calls != 0 {
		t.Fatal("unbound option reached CAS", err)
	}
	// Existing question plus available submission plus unfinished draft must
	// fit the unchanged private response cap before any new record is written.
	reader.record.Status = "continued"
	service.Authority.(*displayAuthorityFixtureV1).authorized.Status = "continued"
	reason := domaincontinuation.ReasonUserInputContinuedV1
	reader.record.ContinuedReason = &reason
	reader.record.TimedWait = &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 120}
	reader.record.IssuedAt = "2026-10-08T00:00:00Z"
	reader.record.AnswerState = "available"
	large := strings.Repeat("x", 3<<20)
	answers, _ := json.Marshal([]map[string]any{{"id": selector.RequestID + "_1", "kind": "answer", "optionIds": []string{}, "customText": large}, {"id": selector.RequestID + "_2", "kind": "skip"}})
	reader.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: answers}
	var args map[string]any
	if json.Unmarshal(reader.record.Arguments, &args) != nil {
		t.Fatal("fixture args")
	}
	args["questions"].([]any)[0].(map[string]any)["question"] = large
	reader.record.Arguments, _ = json.Marshal(args)
	request.Draft.Answers[0].OptionIDs = []string{}
	request.Draft.Answers[0].CustomText = large
	if _, err := service.SaveDraft(context.Background(), request); !errors.Is(err, continuationapp.ErrUserInputDraftCapacityV1) || saver.calls != 0 {
		t.Fatal("unreadable combined display reached CAS", err)
	}
}

func TestInheritedAcceptedReplyRequiresFixedPairAndFreshReadsV1(t *testing.T) {
	for _, name := range []string{"accepted", "missing_reply", "missing_submission", "wrong_mode", "changed_reply", "invalid_uuid", "question_as_reply", "active_timing"} {
		t.Run(name, func(t *testing.T) {
			service, authority, reader, origin := newDisplayFixtureV1("pending")
			child := origin
			child.ThreadID = "child"
			authority.authorized.Selector, authority.authorized.Origin = child, origin
			authority.authorized.ReadOnly, authority.authorized.Status, authority.authorized.HistoryDigest = true, "continued", strings.Repeat("c", 64)
			reader.record.Status, reader.record.AnswerState = "continued", "available"
			reader.record.CallID = "original-call"
			reader.record.Submission = &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: json.RawMessage(`[ {"id":"` + origin.RequestID + `_1","kind":"skip"}, {"id":"` + origin.RequestID + `_2","kind":"skip"} ]`)}
			reader.historyReply = &HistoryReplyV1{ItemID: "actual-reply", ClientUserMessageID: "00000000-0000-4000-8000-000000000001", TurnID: "later-turn"}
			switch name {
			case "missing_reply":
				reader.historyReply = nil
			case "missing_submission":
				reader.record.Submission = nil
			case "wrong_mode":
				reader.record.Status = "submitted"
			case "changed_reply":
				reader.changeReplyAfterRead = true
			case "invalid_uuid":
				reader.historyReply.ClientUserMessageID = "fake"
			case "question_as_reply":
				reader.historyReply.ItemID = child.ItemID
			case "active_timing":
				reader.record.TimedWait = &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 1}
			}
			view, err := service.Read(context.Background(), child)
			if name != "accepted" {
				if !errors.Is(err, port.ErrUnavailableV1) {
					t.Fatal("unproven historical answer was admitted")
				}
				return
			}
			if err == nil {
				withoutPair := view
				withoutPair.HistoryReply = nil
				if ValidateDisplayV1(withoutPair, child) {
					t.Fatal("independent leaf accepted an unassociated historical answer")
				}
			}
			if err != nil || !view.ReadOnly || view.Submission == nil || view.HistoryReply == nil || *view.HistoryReply != *reader.historyReply || reader.binding.ThreadID != origin.ThreadID || reader.historySelector != child || reader.calls != 2 || !authority.released {
				t.Fatal("fixed readonly history pair lost", err)
			}
		})
	}
}
