package turn

import "errors"

// General terminal details describe only the failing call site. They are
// in-process diagnostics, not public failure codes or a persistence contract.
const (
	GeneralTerminalDetailFinishV1           = "terminal_finish"
	GeneralTerminalDetailOutboxV1           = "outbox"
	GeneralTerminalDetailIdentityV1         = "outbox_identity"
	GeneralTerminalDetailReservedV1         = "outbox_reserved"
	GeneralTerminalDetailPrimaryV1          = "outbox_primary"
	GeneralTerminalDetailViewV1             = "outbox_view"
	GeneralTerminalDetailLogV1              = "outbox_log"
	GeneralTerminalDetailInventoryV1        = "outbox_inventory"
	GeneralTerminalDetailEarlierUnsettledV1 = "outbox_earlier_unsettled"
	GeneralTerminalDetailFrontierV1         = "outbox_frontier"
	GeneralTerminalDetailPrepareV1          = "outbox_prepare"
	GeneralTerminalDetailAppendV1           = "outbox_append"
	GeneralTerminalDetailReadbackV1         = "outbox_readback"
	GeneralTerminalDetailExactBundleV1      = "outbox_exact_bundle"
	GeneralTerminalDetailUsageSettleV1      = "outbox_usage_settle"
	GeneralTerminalDetailLivePublicationV1  = "outbox_live_publication"
)

type generalTerminalDiagnosticErrorV1 struct {
	detail string
	cause  error
}

func (err *generalTerminalDiagnosticErrorV1) Error() string {
	return "general terminal failure: " + closedGeneralTerminalDetailV1(err.detail)
}

func (err *generalTerminalDiagnosticErrorV1) Unwrap() error { return err.cause }

// WithGeneralTerminalDetailV1 preserves the error chain and any more precise
// inner call-site detail. Error text never includes the underlying cause.
func WithGeneralTerminalDetailV1(err error, detail string) error {
	if err == nil {
		return nil
	}
	var inner *generalTerminalDiagnosticErrorV1
	if errors.As(err, &inner) {
		detail = inner.detail
	}
	return &generalTerminalDiagnosticErrorV1{detail: closedGeneralTerminalDetailV1(detail), cause: err}
}

func GeneralTerminalDetailClassV1(err error) string {
	if err == nil {
		return "none"
	}
	var diagnostic *generalTerminalDiagnosticErrorV1
	if errors.As(err, &diagnostic) {
		return closedGeneralTerminalDetailV1(diagnostic.detail)
	}
	return "unknown"
}

func closedGeneralTerminalDetailV1(detail string) string {
	switch detail {
	case GeneralTerminalDetailFinishV1, GeneralTerminalDetailOutboxV1,
		GeneralTerminalDetailIdentityV1, GeneralTerminalDetailReservedV1,
		GeneralTerminalDetailPrimaryV1, GeneralTerminalDetailViewV1,
		GeneralTerminalDetailLogV1, GeneralTerminalDetailInventoryV1,
		GeneralTerminalDetailEarlierUnsettledV1, GeneralTerminalDetailFrontierV1,
		GeneralTerminalDetailPrepareV1, GeneralTerminalDetailAppendV1,
		GeneralTerminalDetailReadbackV1, GeneralTerminalDetailExactBundleV1,
		GeneralTerminalDetailUsageSettleV1, GeneralTerminalDetailLivePublicationV1:
		return detail
	default:
		return "unknown"
	}
}
