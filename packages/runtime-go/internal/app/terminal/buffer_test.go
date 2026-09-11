package terminal

import "testing"

func TestLimitedOutputBufferCapturesAndMarksTruncated(t *testing.T) {
	buffer := NewLimitedOutputBuffer(5)
	if n, err := buffer.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("first write mismatch n=%d err=%v", n, err)
	}
	if n, err := buffer.Write([]byte(" world")); err != nil || n != 6 {
		t.Fatalf("second write mismatch n=%d err=%v", n, err)
	}
	if got := buffer.String(); got != "hello\n[truncated]" {
		t.Fatalf("buffer string mismatch: %q", got)
	}
	if !buffer.Truncated() {
		t.Fatalf("buffer should report truncation")
	}
}

func TestLimitedOutputBufferNegativeLimitStillConsumesWrites(t *testing.T) {
	buffer := NewLimitedOutputBuffer(-1)
	if n, err := buffer.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("write mismatch n=%d err=%v", n, err)
	}
	if got := buffer.String(); got != "\n[truncated]" {
		t.Fatalf("negative-limit buffer mismatch: %q", got)
	}
}

func TestBackgroundOutputBufferUpdatesAndSnapshots(t *testing.T) {
	updates := []string{}
	buffer := NewBackgroundOutputBuffer(7, func(output string, truncated bool) {
		if truncated {
			output += "|truncated"
		}
		updates = append(updates, output)
	})
	if n, err := buffer.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("first write mismatch n=%d err=%v", n, err)
	}
	if n, err := buffer.Write([]byte(" world")); err != nil || n != 6 {
		t.Fatalf("second write mismatch n=%d err=%v", n, err)
	}
	output, truncated := buffer.Snapshot()
	if output != "hello w" || !truncated {
		t.Fatalf("snapshot mismatch output=%q truncated=%t", output, truncated)
	}
	if len(updates) != 2 || updates[0] != "hello" || updates[1] != "hello w|truncated" {
		t.Fatalf("updates mismatch: %#v", updates)
	}
}
