package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	admission "analytix.local/runtime-go/internal/domain/scheduleadmission"
)

const ScheduleAdmissionPathV1 = "/v1/local-display/schedule-admission"

type ScheduleAdmissionHandlerV1 struct {
	Admit func(context.Context, admission.RequestV1) admission.ResponseV1
}

func (h ScheduleAdmissionHandlerV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if r.Body == nil || r.ContentLength > admission.MaximumRequestBytesV1 {
		writeScheduleAdmissionV1(w, admission.ResultV1("invalid", nil))
		return
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, admission.MaximumRequestBytesV1+1))
	request, decodeErr := admission.DecodeRequestV1(b)
	if e != nil || decodeErr != nil {
		writeScheduleAdmissionV1(w, admission.ResultV1("invalid", nil))
		return
	}
	if h.Admit == nil {
		writeScheduleAdmissionV1(w, admission.ResultV1("unknown", nil))
		return
	}
	response := h.Admit(r.Context(), request)
	writeScheduleAdmissionV1(w, response)
}

// This private response contains only a closed status/code and a typed receipt,
// never an error cause or arbitrary caller/provider value. The public failure
// projector deliberately does not understand private admission discriminators.
func writeScheduleAdmissionV1(w http.ResponseWriter, response admission.ResponseV1) {
	switch response.Status {
	case "accepted", "not_found", "rejected", "unknown", "conflict", "invalid":
	default:
		response = admission.ResultV1("unknown", nil)
	}
	if response.Status != "accepted" {
		response.Receipt = nil
	}
	response = admission.ResultV1(response.Status, response.Receipt)
	body, err := json.Marshal(response)
	if err != nil || len(body) > 4096 {
		response = admission.ResultV1("unknown", nil)
		body, _ = json.Marshal(response)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.HTTPStatusV1())
	_, _ = w.Write(body)
}
