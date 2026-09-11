package security

import "errors"

// LogicalEffect is the host-classified data boundary for one steering input.
// It describes the authority needed to process that input; it does not grant
// that authority and it does not replace the permanent ordinary Agent base.
type LogicalEffect string

const (
	LogicalEffectOrdinary  LogicalEffect = "ordinary"
	LogicalEffectCaseData  LogicalEffect = "case_data"
	LogicalEffectFundsData LogicalEffect = "funds_data"
)

func ValidateLogicalEffect(effect LogicalEffect) error {
	switch effect {
	case LogicalEffectOrdinary, LogicalEffectCaseData, LogicalEffectFundsData:
		return nil
	default:
		return errors.New("logical effect is invalid")
	}
}
