package monotonichead

import (
	"errors"
	"fmt"
	"testing"
)

func TestIntegrityAndIndeterminateErrorsRemainDistinctTypedSentinels(t *testing.T) {
	indeterminate := fmt.Errorf("advance transport closed: %w", ErrIndeterminate)
	if !errors.Is(indeterminate, ErrIndeterminate) || errors.Is(indeterminate, ErrUnavailable) || errors.Is(indeterminate, ErrEquivocation) {
		t.Fatal("indeterminate advance was collapsed into another witness error class")
	}
	equivocation := fmt.Errorf("generation conflict: %w", ErrEquivocation)
	if !errors.Is(equivocation, ErrEquivocation) || errors.Is(equivocation, ErrInvalidReceipt) || errors.Is(equivocation, ErrUnavailable) {
		t.Fatal("witness equivocation was collapsed into another witness error class")
	}
}
