package control

import (
	"errors"
	"strings"

	appturn "analytix.local/runtime-go/internal/app/turn"
)

type TurnStatusReader interface {
	TurnStatus(string, string) (string, error)
}

func TerminalTurnStatus(reader TurnStatusReader, threadID, turnID string) (string, bool, error) {
	if reader == nil || strings.TrimSpace(threadID) == "" || strings.TrimSpace(turnID) == "" {
		return "", false, errors.New("turn status reader input is invalid")
	}
	status, err := reader.TurnStatus(strings.TrimSpace(threadID), strings.TrimSpace(turnID))
	return status, err == nil && appturn.IsTerminalStatus(status), err
}
