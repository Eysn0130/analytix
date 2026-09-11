package liveevents

// BroadcastBundleNoPrefix enqueues every delivery unit for each subscriber or
// disconnects that subscriber before enqueueing anything. The caller owns the
// subscriber map lock for the full operation.
func BroadcastBundleNoPrefix(
	subscribers map[chan map[string]any]struct{},
	units []map[string]any,
	clone func(map[string]any) map[string]any,
) {
	if len(units) == 0 || clone == nil {
		return
	}
	ready := make([]chan map[string]any, 0, len(subscribers))
	for ch := range subscribers {
		if cap(ch)-len(ch) < len(units) {
			delete(subscribers, ch)
			close(ch)
			continue
		}
		ready = append(ready, ch)
	}
	for _, ch := range ready {
		for _, unit := range units {
			ch <- clone(unit)
		}
	}
}
