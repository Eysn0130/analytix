package reasoningmarkup

import (
	"errors"
	"strings"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

const (
	MaxInputBytes = 16 << 20
	MaxTagBytes   = 4 << 10
	MaxDepth      = 32
)

var (
	ErrIncomplete = errors.New("provider reasoning markup is incomplete")
	ErrLimit      = errors.New("provider reasoning markup exceeds a host limit")
	ErrLifecycle  = errors.New("provider reasoning markup decoder lifecycle is invalid")
)

type RecoveryKind uint8

const (
	RecoveryNone RecoveryKind = iota
	RecoveryOrphanClose
)

type Outcome struct {
	PublicText   string
	HadReasoning bool
	Recovery     RecoveryKind
	RemovedBytes uint64
}

// ProtocolError is a closed host blocker for malformed provider-originated
// reasoning markup. It deliberately forbids transport retry/recovery while
// retaining the typed parser cause for deterministic internal tests.
type ProtocolError struct {
	cause error
}

func NewProtocolError(cause error) error {
	return ProtocolError{cause: cause}
}

func (err ProtocolError) Error() string {
	return domainfailure.New(domainfailure.CodeProviderReasoningMarkupInvalid, nil).Message()
}

func (err ProtocolError) Unwrap() error {
	return err.cause
}

func (ProtocolError) ProviderRetryForbidden() bool {
	return true
}

func (ProtocolError) PublicFailureRecord() domainfailure.Record {
	return domainfailure.New(domainfailure.CodeProviderReasoningMarkupInvalid, nil)
}

// Decoder is an attempt-private transactional decoder. Push never returns
// publishable bytes: a later orphan closing marker can prove that arbitrary
// earlier content belonged to a reasoning fragment whose opening marker was
// lost upstream. Only Finish on a complete, bounded stream yields public text.
type Decoder struct {
	input    strings.Builder
	poisoned error
	finished bool
}

func NewDecoder() *Decoder { return &Decoder{} }

func (decoder *Decoder) Push(chunk string) error {
	if decoder == nil || decoder.finished {
		return ErrLifecycle
	}
	if decoder.poisoned != nil {
		return decoder.poisoned
	}
	if len(chunk) > MaxInputBytes-decoder.input.Len() {
		decoder.poisoned = ErrLimit
		decoder.input.Reset()
		return decoder.poisoned
	}
	decoder.input.WriteString(chunk)
	return nil
}

func (decoder *Decoder) Finish() (Outcome, error) {
	if decoder == nil || decoder.finished {
		return Outcome{}, ErrLifecycle
	}
	decoder.finished = true
	if decoder.poisoned != nil {
		decoder.input.Reset()
		return Outcome{}, decoder.poisoned
	}
	value := decoder.input.String()
	decoder.input.Reset()
	outcome, err := filter(value)
	if err != nil {
		return Outcome{}, err
	}
	if err := ValidateProviderNativeTextV1(outcome.PublicText); err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}

func Filter(value string) (Outcome, error) {
	if len(value) > MaxInputBytes {
		return Outcome{}, ErrLimit
	}
	if !strings.Contains(value, "<") {
		if err := ValidateProviderNativeTextV1(value); err != nil {
			return Outcome{}, err
		}
		return Outcome{PublicText: value}, nil
	}
	decoder := NewDecoder()
	if err := decoder.Push(value); err != nil {
		return Outcome{}, err
	}
	return decoder.Finish()
}

type parsedTag struct {
	name        string
	closing     bool
	selfClosing bool
	end         int
}

type tagParseState uint8

const (
	tagAbsent tagParseState = iota
	tagPresent
	tagIncomplete
	tagLimit
)

func filter(value string) (Outcome, error) {
	var committed strings.Builder
	var candidate strings.Builder
	outcome := Outcome{}
	stack := make([]string, 0, 4)
	for cursor := 0; cursor < len(value); {
		if value[cursor] != '<' {
			next := strings.IndexByte(value[cursor:], '<')
			if next < 0 {
				next = len(value) - cursor
			}
			segment := value[cursor : cursor+next]
			if len(stack) > 0 {
				outcome.RemovedBytes += uint64(len(segment))
			} else {
				candidate.WriteString(segment)
			}
			cursor += next
			continue
		}
		tag, state := parseTag(value, cursor)
		switch state {
		case tagIncomplete:
			return Outcome{}, ErrIncomplete
		case tagLimit:
			return Outcome{}, ErrLimit
		case tagAbsent:
			if len(stack) > 0 {
				outcome.RemovedBytes++
			} else {
				candidate.WriteByte('<')
			}
			cursor++
			continue
		}
		tagBytes := tag.end - cursor
		outcome.HadReasoning = true
		outcome.RemovedBytes += uint64(tagBytes)
		cursor = tag.end
		if tag.closing {
			if len(stack) == 0 {
				outcome.RemovedBytes += uint64(candidate.Len())
				candidate.Reset()
				outcome.Recovery = RecoveryOrphanClose
				continue
			}
			if stack[len(stack)-1] != tag.name {
				return Outcome{}, ErrIncomplete
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if tag.selfClosing {
			continue
		}
		if len(stack) == 0 {
			committed.WriteString(candidate.String())
			candidate.Reset()
		}
		stack = append(stack, tag.name)
		if len(stack) > MaxDepth {
			return Outcome{}, ErrLimit
		}
	}
	if len(stack) != 0 {
		return Outcome{}, ErrIncomplete
	}
	committed.WriteString(candidate.String())
	outcome.PublicText = committed.String()
	return outcome, nil
}

func parseTag(value string, start int) (parsedTag, tagParseState) {
	if start < 0 || start >= len(value) || value[start] != '<' {
		return parsedTag{}, tagAbsent
	}
	remaining := value[start:]
	closing := strings.HasPrefix(remaining, "</")
	nameStart := 1
	if closing {
		nameStart = 2
	}
	for _, name := range []string{"think", "analysis", "reasoning"} {
		marker := "<" + name
		if closing {
			marker = "</" + name
		}
		if len(remaining) < len(marker) {
			if len(remaining) > nameStart && asciiFoldHasPrefix(marker, remaining) {
				return parsedTag{}, tagIncomplete
			}
			continue
		}
		if !asciiFoldEqual(remaining[nameStart:nameStart+len(name)], name) {
			continue
		}
		if len(remaining) == len(marker) {
			return parsedTag{}, tagIncomplete
		}
		boundary := remaining[len(marker)]
		if boundary != '>' && boundary != '/' && !asciiWhitespace(boundary) {
			continue
		}
		quote := byte(0)
		for offset := len(marker); offset < len(remaining); offset++ {
			if offset+1 > MaxTagBytes {
				return parsedTag{}, tagLimit
			}
			character := remaining[offset]
			if quote != 0 {
				if character == quote {
					quote = 0
				}
				continue
			}
			switch character {
			case '\'', '"':
				quote = character
			case '>':
				body := strings.TrimSpace(remaining[len(marker):offset])
				selfClosing := !closing && strings.HasSuffix(body, "/")
				return parsedTag{name: name, closing: closing, selfClosing: selfClosing, end: start + offset + 1}, tagPresent
			}
		}
		if len(remaining) > MaxTagBytes {
			return parsedTag{}, tagLimit
		}
		return parsedTag{}, tagIncomplete
	}
	return parsedTag{}, tagAbsent
}

func asciiWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n' || value == '\f'
}

func asciiFoldHasPrefix(value string, prefix string) bool {
	if len(prefix) > len(value) {
		return false
	}
	return asciiFoldEqual(value[:len(prefix)], prefix)
}

func asciiFoldEqual(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range len(left) {
		leftByte, rightByte := left[index], right[index]
		if leftByte >= 'A' && leftByte <= 'Z' {
			leftByte += 'a' - 'A'
		}
		if rightByte >= 'A' && rightByte <= 'Z' {
			rightByte += 'a' - 'A'
		}
		if leftByte != rightByte {
			return false
		}
	}
	return true
}
