package protocol

import "encoding/json"

type G2RouteReplayCase struct {
	ID           string          `json:"id"`
	Setup        string          `json:"setup"`
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Auth         string          `json:"auth"`
	ResponseKind string          `json:"responseKind"`
	Body         json.RawMessage `json:"body,omitempty"`
	Response     G2RouteResponse `json:"response"`
	SSEFrames    []string        `json:"sseFrames,omitempty"`
}

type G2RouteResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}
