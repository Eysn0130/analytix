package reportpublication

import (
	"bytes"
	"encoding/json"
	"errors"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

type ReportDeliveryOutcomeKindV1 string

const (
	ReportDeliveryOutcomeProjectedV1 ReportDeliveryOutcomeKindV1 = "projected"
	ReportDeliveryOutcomeRejectedV1  ReportDeliveryOutcomeKindV1 = "rejected"
)

// ReportDeliveryOutcomeV1 is intentionally not a second wire envelope. The
// shared CAS stores the canonical signed projection or rejection bytes
// directly, preserving existing projection files while giving both outcomes
// one stable no-replace slot.
type ReportDeliveryOutcomeV1 struct {
	Kind       ReportDeliveryOutcomeKindV1
	Projection *ReportDeliveryProjectionV1
	Rejection  *ReportDeliveryRejectionV1
}

func ProjectedReportDeliveryOutcomeV1(projection ReportDeliveryProjectionV1) (ReportDeliveryOutcomeV1, error) {
	outcome := ReportDeliveryOutcomeV1{Kind: ReportDeliveryOutcomeProjectedV1, Projection: &projection}
	return outcome, ValidateReportDeliveryOutcomeV1(outcome)
}

func RejectedReportDeliveryOutcomeV1(rejection ReportDeliveryRejectionV1) (ReportDeliveryOutcomeV1, error) {
	outcome := ReportDeliveryOutcomeV1{Kind: ReportDeliveryOutcomeRejectedV1, Rejection: &rejection}
	return outcome, ValidateReportDeliveryOutcomeV1(outcome)
}

func ValidateReportDeliveryOutcomeV1(outcome ReportDeliveryOutcomeV1) error {
	switch outcome.Kind {
	case ReportDeliveryOutcomeProjectedV1:
		if outcome.Projection == nil || outcome.Rejection != nil || ValidateReportDeliveryProjectionV1(*outcome.Projection) != nil {
			return errors.New("projected report delivery outcome is invalid")
		}
	case ReportDeliveryOutcomeRejectedV1:
		if outcome.Rejection == nil || outcome.Projection != nil || ValidateReportDeliveryRejectionV1(*outcome.Rejection) != nil {
			return errors.New("rejected report delivery outcome is invalid")
		}
	default:
		return errors.New("report delivery outcome kind is invalid")
	}
	return nil
}

func ValidateReportDeliveryOutcomeCompletionV1(outcome ReportDeliveryOutcomeV1, completion ReportStageCompletionV1) error {
	if err := ValidateReportDeliveryOutcomeV1(outcome); err != nil {
		return err
	}
	switch outcome.Kind {
	case ReportDeliveryOutcomeProjectedV1:
		return ValidateReportDeliveryProjectionCompletionV1(*outcome.Projection, completion)
	case ReportDeliveryOutcomeRejectedV1:
		return ValidateReportDeliveryRejectionCompletionV1(*outcome.Rejection, completion)
	default:
		return errors.New("report delivery outcome kind is invalid")
	}
}

func ReportDeliveryOutcomeID(outcome ReportDeliveryOutcomeV1) string {
	switch outcome.Kind {
	case ReportDeliveryOutcomeProjectedV1:
		if outcome.Projection != nil {
			return outcome.Projection.DeliveryID
		}
	case ReportDeliveryOutcomeRejectedV1:
		if outcome.Rejection != nil {
			return outcome.Rejection.DeliveryID
		}
	}
	return ""
}

func ReportDeliveryOutcomeCompletionID(outcome ReportDeliveryOutcomeV1) string {
	switch outcome.Kind {
	case ReportDeliveryOutcomeProjectedV1:
		if outcome.Projection != nil {
			return outcome.Projection.CompletionID
		}
	case ReportDeliveryOutcomeRejectedV1:
		if outcome.Rejection != nil {
			return outcome.Rejection.CompletionID
		}
	}
	return ""
}

func ReportDeliveryOutcomeRecordDigest(outcome ReportDeliveryOutcomeV1) string {
	switch outcome.Kind {
	case ReportDeliveryOutcomeProjectedV1:
		if outcome.Projection != nil {
			return outcome.Projection.RecordDigest
		}
	case ReportDeliveryOutcomeRejectedV1:
		if outcome.Rejection != nil {
			return outcome.Rejection.RecordDigest
		}
	}
	return ""
}

func ReportDeliveryOutcomeV1Bytes(outcome ReportDeliveryOutcomeV1) ([]byte, error) {
	if err := ValidateReportDeliveryOutcomeV1(outcome); err != nil {
		return nil, err
	}
	if outcome.Kind == ReportDeliveryOutcomeProjectedV1 {
		return ReportDeliveryProjectionV1Bytes(*outcome.Projection)
	}
	return ReportDeliveryRejectionV1Bytes(*outcome.Rejection)
}

func ParseReportDeliveryOutcomeV1(body []byte) (ReportDeliveryOutcomeV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 16, MaxTokens: 1024, MaxStringBytes: 128 << 10,
	}); err != nil {
		return ReportDeliveryOutcomeV1{}, err
	}
	var discriminator struct {
		Purpose string `json:"purpose"`
	}
	if err := json.Unmarshal(body, &discriminator); err != nil {
		return ReportDeliveryOutcomeV1{}, err
	}
	var outcome ReportDeliveryOutcomeV1
	switch discriminator.Purpose {
	case ReportDeliveryProjectionPurpose:
		projection, err := ParseReportDeliveryProjectionV1(body)
		if err != nil {
			return ReportDeliveryOutcomeV1{}, err
		}
		outcome, err = ProjectedReportDeliveryOutcomeV1(projection)
		if err != nil {
			return ReportDeliveryOutcomeV1{}, err
		}
	case ReportDeliveryRejectionPurpose:
		rejection, err := ParseReportDeliveryRejectionV1(body)
		if err != nil {
			return ReportDeliveryOutcomeV1{}, err
		}
		outcome, err = RejectedReportDeliveryOutcomeV1(rejection)
		if err != nil {
			return ReportDeliveryOutcomeV1{}, err
		}
	default:
		return ReportDeliveryOutcomeV1{}, errors.New("report delivery outcome purpose is unknown")
	}
	canonical, err := ReportDeliveryOutcomeV1Bytes(outcome)
	if err != nil || !bytes.Equal(canonical, body) {
		return ReportDeliveryOutcomeV1{}, errors.New("report delivery outcome is not canonical")
	}
	return outcome, nil
}
