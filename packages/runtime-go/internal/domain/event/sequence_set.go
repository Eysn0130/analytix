package event

import "errors"

type SequenceSetV1 struct {
	seen map[int64]struct{}
	min  int64
	max  int64
}

func (set *SequenceSetV1) AddPositiveUnique(sequence int64) error {
	if set == nil || sequence <= 0 {
		return errors.New("event seq must be a positive integer")
	}
	if set.seen == nil {
		set.seen = map[int64]struct{}{}
	}
	if _, exists := set.seen[sequence]; exists {
		return errors.New("event seq is duplicated")
	}
	set.seen[sequence] = struct{}{}
	if set.min == 0 || sequence < set.min {
		set.min = sequence
	}
	if sequence > set.max {
		set.max = sequence
	}
	return nil
}

func (set SequenceSetV1) ValidateContiguous() error {
	if len(set.seen) == 0 {
		return nil
	}
	if set.min <= 0 || set.max < set.min || set.max-set.min+1 != int64(len(set.seen)) {
		return errors.New("event seq set is not contiguous")
	}
	return nil
}
