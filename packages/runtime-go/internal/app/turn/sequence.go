package turn

import (
	"strconv"
	"strings"
)

func SequenceID(turnID string) (int, bool) {
	for _, prefix := range []string{"turn_", legacySequencePrefix()} {
		if !strings.HasPrefix(turnID, prefix) {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimPrefix(turnID, prefix))
		if err == nil && seq > 0 {
			return seq, true
		}
	}
	return 0, false
}

func legacySequencePrefix() string {
	return "turn_" + string([]rune{100, 48, 50, 52, 50}) + "_"
}
