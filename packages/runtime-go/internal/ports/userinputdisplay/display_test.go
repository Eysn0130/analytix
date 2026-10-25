package userinputdisplay

import (
	"strings"
	"testing"
)

func TestUserInputDisplaySelectorRequiresExactHostIDs(t *testing.T) {
	valid := SelectorV1{ThreadID: "thread-a", TurnID: "turn-a", ItemID: "item_input_" + strings.Repeat("a", 64), RequestID: "input_" + strings.Repeat("a", 64)}
	if !ValidateSelectorV1(valid) {
		t.Fatal("valid exact selector rejected")
	}
	for _, mutate := range []func(*SelectorV1){
		func(s *SelectorV1) { s.ThreadID = " thread-a" },
		func(s *SelectorV1) { s.TurnID = "turn-a " },
		func(s *SelectorV1) { s.ThreadID = "../thread-a" },
		func(s *SelectorV1) { s.ItemID = "item/a" },
		func(s *SelectorV1) { s.ItemID = "" },
		func(s *SelectorV1) { s.ItemID = "item_input_" + strings.Repeat("b", 64) },
		func(s *SelectorV1) { s.RequestID = "q1" },
		func(s *SelectorV1) { s.RequestID = "input_" + strings.Repeat("a", 63) },
		func(s *SelectorV1) { s.RequestID += " " },
	} {
		selector := valid
		mutate(&selector)
		if ValidateSelectorV1(selector) {
			t.Fatal("alias or malformed selector accepted")
		}
	}
}
