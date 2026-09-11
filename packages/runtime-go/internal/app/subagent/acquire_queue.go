package subagent

// AcquireQueuedSignal closes the host-owned queue admission signal at most
// once. Background task startup explicitly transfers responsibility past the
// initiating tool call; every other exit signals from the caller's defer.
type AcquireQueuedSignal struct {
	queued      chan<- struct{}
	signaled    bool
	transferred bool
}

func NewAcquireQueuedSignal(queued chan<- struct{}) *AcquireQueuedSignal {
	return &AcquireQueuedSignal{queued: queued}
}

func (signal *AcquireQueuedSignal) Signal() {
	if signal == nil || signal.queued == nil || signal.signaled {
		return
	}
	close(signal.queued)
	signal.signaled = true
}

func (signal *AcquireQueuedSignal) Transfer() {
	if signal != nil {
		signal.transferred = true
	}
}

func (signal *AcquireQueuedSignal) SignalUnlessTransferred() {
	if signal != nil && !signal.transferred {
		signal.Signal()
	}
}
