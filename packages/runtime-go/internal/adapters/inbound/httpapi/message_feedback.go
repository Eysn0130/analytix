package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type MessageFeedbackServiceV1 interface {
	ListMessageFeedbackV1(context.Context, string) ([]domainthread.MessageFeedbackItemV1, error)
	PutMessageFeedbackV1(context.Context, string, string, domainthread.MessageFeedbackPutRequestV1) (domainthread.MessageFeedbackItemV1, error)
	DeleteMessageFeedbackV1(context.Context, string, string, string) error
}

type MessageFeedbackHandlersV1 struct{ Service MessageFeedbackServiceV1 }

func (h MessageFeedbackHandlersV1) Handle(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[1] != "message-feedback" || !domainthread.IsCanonicalRecordID(parts[0]) || len(parts) == 3 && !domainthread.IsCanonicalRecordID(parts[2]) || r.URL.RawQuery != "" {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if h.Service == nil {
		messageFeedbackHTTPErrorV1(w, errors.New("feedback service unavailable"))
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			MethodNotAllowed(w)
			return
		}
		items, err := h.Service.ListMessageFeedbackV1(r.Context(), parts[0])
		if err != nil {
			messageFeedbackHTTPErrorV1(w, err)
			return
		}
		if items == nil {
			items = []domainthread.MessageFeedbackItemV1{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		MethodNotAllowed(w)
		return
	}
	// Bound raw JSON independently of transport admission, including duplicate
	// keys, UTF-8, unknown fields and required nullable creation precondition.
	if r.Body == nil {
		messageFeedbackHTTPErrorV1(w, domainthread.ErrMessageFeedbackInvalidV1)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(raw) > 65536 {
		messageFeedbackHTTPErrorV1(w, domainthread.ErrMessageFeedbackInvalidV1)
		return
	}
	body, err := domainjsonstrict.DecodeObject(raw, domainjsonstrict.Options{MaxBytes: 65536, MaxDepth: 4, MaxTokens: 32, MaxStringBytes: domainthread.MessageFeedbackNoteMaxBytesV1})
	if err != nil {
		messageFeedbackHTTPErrorV1(w, domainthread.ErrMessageFeedbackInvalidV1)
		return
	}
	if _, exists := body["ifVersion"]; !exists {
		messageFeedbackHTTPErrorV1(w, domainthread.ErrMessageFeedbackInvalidV1)
		return
	}
	if r.Method == http.MethodPut {
		for _, key := range []string{"note", "category"} {
			if value, exists := body[key]; exists && value == nil {
				messageFeedbackHTTPErrorV1(w, domainthread.ErrMessageFeedbackInvalidV1)
				return
			}
		}
		var request domainthread.MessageFeedbackPutRequestV1
		if domainthread.DecodeMessageFeedbackV1(raw, &request) != nil || domainthread.ValidateMessageFeedbackPutV1(request) != nil {
			messageFeedbackHTTPErrorV1(w, domainthread.ErrMessageFeedbackInvalidV1)
			return
		}
		item, err := h.Service.PutMessageFeedbackV1(r.Context(), parts[0], parts[2], request)
		if err != nil {
			messageFeedbackHTTPErrorV1(w, err, parts[2])
			return
		}
		WriteJSON(w, http.StatusOK, item)
		return
	}
	var request struct {
		IfVersion string `json:"ifVersion"`
	}
	if domainthread.DecodeMessageFeedbackV1(raw, &request) != nil || domainthread.ValidateMessageFeedbackVersionV1(request.IfVersion) != nil {
		messageFeedbackHTTPErrorV1(w, domainthread.ErrMessageFeedbackInvalidV1)
		return
	}
	if err := h.Service.DeleteMessageFeedbackV1(r.Context(), parts[0], parts[2], request.IfVersion); err != nil {
		messageFeedbackHTTPErrorV1(w, err, parts[2])
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"absent": true})
}

func messageFeedbackHTTPErrorV1(w http.ResponseWriter, err error, messageID ...string) {
	var conflict *domainthread.MessageFeedbackVersionConflictV1
	if errors.As(err, &conflict) && len(messageID) == 1 && domainthread.IsCanonicalRecordID(messageID[0]) && writeMessageFeedbackConflictV1(w, messageID[0], conflict.Current) {
		return
	}
	status, code, message := http.StatusServiceUnavailable, "message_feedback_unavailable", "Message feedback is unavailable. Try again."
	switch {
	case errors.Is(err, domainthread.ErrMessageFeedbackInvalidV1), errors.Is(err, domainthread.ErrMessageFeedbackNoteBlankV1), errors.Is(err, domainthread.ErrMessageFeedbackNoteTooLargeV1):
		status, code, message = http.StatusBadRequest, "message_feedback_invalid", "Message feedback is invalid."
	case errors.Is(err, domainthread.ErrMessageFeedbackTargetNotFoundV1), errors.Is(err, threadapp.ErrThreadNotFound), errors.Is(err, os.ErrNotExist):
		status, code, message = http.StatusNotFound, "message_feedback_target_not_found", "The assistant message is unavailable."
	}
	WriteJSON(w, status, map[string]any{"code": code, "message": message})
}

func writeMessageFeedbackConflictV1(w http.ResponseWriter, messageID string, current *domainthread.MessageFeedbackItemV1) bool {
	var publicCurrent any
	if current != nil {
		if current.MessageID != messageID || domainthread.ValidateMessageFeedbackItemV1(*current) != nil {
			return false
		}
		raw, err := json.Marshal(current)
		if err != nil {
			return false
		}
		var item map[string]any
		if json.Unmarshal(raw, &item) != nil || domainevent.ValidatePublicRecord(item) != nil {
			return false
		}
		publicCurrent = item
	}
	// The generic failure projection intentionally discards arbitrary details.
	// This route owns only its fixed envelope and an independently closed,
	// map-scanned current item; it does not change the shared failure policy.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": "message_feedback_version_conflict", "message": "Message feedback changed. Review the current rating and submit again.", "current": publicCurrent})
	return true
}
