package terminal

import "sync"

type LimitedOutputBuffer struct {
	mu        sync.Mutex
	limit     int
	data      []byte
	truncated bool
}

type BackgroundOutputBuffer struct {
	mu        sync.Mutex
	limit     int
	data      []byte
	truncated bool
	onUpdate  func(output string, truncated bool)
}

func NewLimitedOutputBuffer(limit int) *LimitedOutputBuffer {
	if limit < 0 {
		limit = 0
	}
	return &LimitedOutputBuffer{limit: limit}
}

func NewBackgroundOutputBuffer(limit int, onUpdate func(output string, truncated bool)) *BackgroundOutputBuffer {
	if limit < 0 {
		limit = 0
	}
	return &BackgroundOutputBuffer{limit: limit, onUpdate: onUpdate}
}

func (b *LimitedOutputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		b.data = append(b.data, p[:remaining]...)
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

func (b *LimitedOutputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.truncated {
		return string(b.data)
	}
	return string(b.data) + "\n[truncated]"
}

func (b *LimitedOutputBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func (b *BackgroundOutputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		b.data = append(b.data, p[:remaining]...)
	}
	if len(p) > remaining {
		b.truncated = true
	}
	output := string(append([]byte(nil), b.data...))
	truncated := b.truncated
	onUpdate := b.onUpdate
	b.mu.Unlock()
	if onUpdate != nil {
		onUpdate(output, truncated)
	}
	return len(p), nil
}

func (b *BackgroundOutputBuffer) Snapshot() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data), b.truncated
}
