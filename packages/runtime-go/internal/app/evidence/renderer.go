package evidence

import domainevidence "analytix.local/runtime-go/internal/domain/evidence"

func RenderFinalAnswer(envelope domainevidence.FinalAnswerEnvelope) (string, error) {
	return domainevidence.RenderFinalAnswer(envelope)
}
