package plan

import "testing"

func TestIsClarifyingQuestion(t *testing.T) {
	if !IsClarifyingQuestion("Which implementation path do you prefer?") {
		t.Fatal("expected clarifying question")
	}
	if !IsClarifyingQuestion("请问你想要 A 还是 B？") {
		t.Fatal("expected Chinese clarifying question")
	}
	if IsClarifyingQuestion("# Which option?\n\nUse A.") {
		t.Fatal("heading should not count as a clarifying question")
	}
	if IsClarifyingQuestion("Which implementation path is chosen.\nNo question mark here.") {
		t.Fatal("question mark should be required in the tail")
	}
}
