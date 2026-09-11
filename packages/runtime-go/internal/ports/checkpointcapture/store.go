package checkpointcapture

type ReconcileRequest struct {
	Draft      map[string]any
	AllowWrite bool
	Publish    bool
}

type Store interface {
	ReconcileCheckpointCapturedEvent(ReconcileRequest) error
}
