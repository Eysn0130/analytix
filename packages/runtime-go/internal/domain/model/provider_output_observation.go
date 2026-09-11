package model

// ProviderOutputObservationV1 is a byte-free host observation of output
// classes seen on one physical provider stream. Values are flags because a
// single interrupted attempt may contain more than one class.
type ProviderOutputObservationV1 uint8

const (
	ProviderOutputObservationNoneV1    ProviderOutputObservationV1 = 0
	ProviderOutputObservationControlV1 ProviderOutputObservationV1 = 1 << iota
	ProviderOutputObservationPrivateReasoningV1
	ProviderOutputObservationPublicTextV1
	ProviderOutputObservationToolStartedV1
)

const providerOutputObservationKnownMaskV1 = ProviderOutputObservationControlV1 |
	ProviderOutputObservationPrivateReasoningV1 |
	ProviderOutputObservationPublicTextV1 |
	ProviderOutputObservationToolStartedV1

func (observation ProviderOutputObservationV1) MergeV1(other ProviderOutputObservationV1) ProviderOutputObservationV1 {
	return observation | other
}

func (observation ProviderOutputObservationV1) IncludesV1(class ProviderOutputObservationV1) bool {
	return class != ProviderOutputObservationNoneV1 && observation&class == class
}

func (observation ProviderOutputObservationV1) HasUnknownClassV1() bool {
	return observation & ^providerOutputObservationKnownMaskV1 != 0
}

func (observation ProviderOutputObservationV1) RetryUnsafeV1() bool {
	return observation.HasUnknownClassV1() ||
		observation.IncludesV1(ProviderOutputObservationPublicTextV1) ||
		observation.IncludesV1(ProviderOutputObservationToolStartedV1)
}
