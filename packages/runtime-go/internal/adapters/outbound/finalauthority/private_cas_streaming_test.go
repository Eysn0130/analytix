package finalauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"strconv"
	"testing"
)

func TestPrivateCASExactStreamingHashAndBody(t *testing.T) {
	for _, size := range []int{0, 1, 64 << 10, 16 << 20} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			body := bytes.Repeat([]byte{byte(size%251 + 1)}, size)
			want := sha256.Sum256(body)
			digest, err := privateCASHashExact(context.Background(), bytes.NewReader(body), int64(len(body)))
			if err != nil || digest != want {
				t.Fatalf("streaming hash = %x, want %x: %v", digest, want, err)
			}
			read, readDigest, err := privateCASReadBodyExact(
				context.Background(), bytes.NewReader(body), int64(len(body)),
			)
			if err != nil || !bytes.Equal(read, body) || readDigest != want {
				t.Fatalf("streaming body mismatch: bytes=%d digest=%x err=%v", len(read), readDigest, err)
			}
		})
	}
}

func TestPrivateCASExactStreamingRejectsShortExtraAndNoProgress(t *testing.T) {
	if _, err := privateCASHashExact(context.Background(), bytes.NewReader([]byte("short")), 6); err == nil {
		t.Fatal("streaming hash accepted a short read")
	}
	if _, err := privateCASHashExact(context.Background(), bytes.NewReader([]byte("extra")), 4); err == nil {
		t.Fatal("streaming hash accepted an extra byte")
	}
	if _, _, err := privateCASReadBodyExact(context.Background(), bytes.NewReader([]byte("short")), 6); err == nil {
		t.Fatal("streaming body accepted a short read")
	}
	if _, _, err := privateCASReadBodyExact(context.Background(), bytes.NewReader([]byte("extra")), 4); err == nil {
		t.Fatal("streaming body accepted an extra byte")
	}
	if _, err := privateCASHashExact(context.Background(), zeroProgressPrivateCASReader{}, 1); err == nil {
		t.Fatal("streaming hash accepted a zero-progress reader")
	}
	if _, _, err := privateCASReadBodyExact(context.Background(), zeroProgressPrivateCASReader{}, 1); err == nil {
		t.Fatal("streaming body accepted a zero-progress reader")
	}
}

func TestPrivateCASExactStreamingPreservesCancellation(t *testing.T) {
	for _, readBody := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &cancellingPrivateCASReader{
			body: bytes.Repeat([]byte("x"), 128<<10), cancel: cancel,
		}
		var err error
		if readBody {
			_, _, err = privateCASReadBodyExact(ctx, reader, int64(len(reader.body)))
		} else {
			_, err = privateCASHashExact(ctx, reader, int64(len(reader.body)))
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("streaming cancellation error = %v, want context.Canceled", err)
		}
	}
}

type zeroProgressPrivateCASReader struct{}

func (zeroProgressPrivateCASReader) Read([]byte) (int, error) {
	return 0, nil
}

type cancellingPrivateCASReader struct {
	body   []byte
	offset int
	cancel context.CancelFunc
}

func (reader *cancellingPrivateCASReader) Read(buffer []byte) (int, error) {
	if reader.offset >= len(reader.body) {
		return 0, io.EOF
	}
	limit := 1024
	if limit > len(buffer) {
		limit = len(buffer)
	}
	if remaining := len(reader.body) - reader.offset; limit > remaining {
		limit = remaining
	}
	copy(buffer[:limit], reader.body[reader.offset:reader.offset+limit])
	reader.offset += limit
	reader.cancel()
	return limit, nil
}
