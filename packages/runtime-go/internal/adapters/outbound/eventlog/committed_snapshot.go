package eventlog

import (
	"bufio"
	"bytes"
	"context"
	"errors"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

// ParseCommittedEventLogBytesV1 is the strict committed subset of replay for
// authenticated semantic After bytes. Invalid records are rejected rather
// than projected or omitted; no filesystem or publication authority is minted.
func ParseCommittedEventLogBytesV1(ctx context.Context, threadID string, body []byte) ([]map[string]any, error) {
	if ctx == nil {
		return nil, errors.New("committed event snapshot context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateExactEventThreadID(threadID); err != nil {
		return nil, err
	}
	if len(body) == 0 || body[len(body)-1] != '\n' {
		return nil, errors.New("committed event snapshot is empty or not newline terminated")
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), maxEventJSONLLineBytesV1+1)
	result := []map[string]any{}
	last := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			return nil, errors.New("committed event snapshot contains a blank record")
		}
		event, err := domainjsonstrict.DecodeObject(line, domainjsonstrict.Options{MaxBytes: maxEventJSONLLineBytesV1})
		if err != nil {
			return nil, err
		}
		seq, ok := normalizeReplaySequence(event)
		if !ok || last > 0 && seq != last+1 || contracts.StringField(event, "threadId") != threadID {
			return nil, errors.New("committed event snapshot sequence or thread is invalid")
		}
		if err := domainevent.ValidatePublicRecord(event); err != nil {
			return nil, err
		}
		last = seq
		result = append(result, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, ctx.Err()
}
