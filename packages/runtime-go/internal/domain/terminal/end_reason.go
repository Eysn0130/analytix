package terminal

import "errors"

// EndReason is display metadata from the committed turn winner, not execution
// or evidence authority. Absence means no recorded output-cap terminal fact.
type EndReason struct {
	Kind string `json:"kind"`
}

const EndReasonMaxTokens = "max-tokens"

func ValidateEndReason(reason *EndReason) error {
	if reason != nil && reason.Kind != EndReasonMaxTokens {
		return errors.New("turn end reason is invalid")
	}
	return nil
}

func EndReasonMap(reason *EndReason) map[string]any {
	if reason == nil {
		return nil
	}
	return map[string]any{"kind": reason.Kind}
}

func ParseEndReason(value any) (*EndReason, error) {
	if value == nil {
		return nil, nil
	}
	record, ok := value.(map[string]any)
	if !ok || len(record) != 1 || record["kind"] != EndReasonMaxTokens {
		return nil, errors.New("turn end reason is invalid")
	}
	return &EndReason{Kind: EndReasonMaxTokens}, nil
}

func ValidEndReasonRecord(record map[string]any) bool {
	value, present := record["endReason"]
	if !present {
		return true
	}
	if value == nil {
		return false
	}
	reason, err := ParseEndReason(value)
	return err == nil && reason != nil && record["status"] == "completed" &&
		CandidateAllowedV1(stringFieldEndReason(record, "terminalReason"))
}

func stringFieldEndReason(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
