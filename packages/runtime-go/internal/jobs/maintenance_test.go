package jobs

import "testing"

func TestRuntimeIdleForMaintenanceRejectsUnsettledChildWork(t *testing.T) {
	for _, test := range []struct {
		name   string
		record Record
		idle   bool
	}{
		{"settled", Record{Status: "completed", CompletionDeliveryStatus: "delivered"}, true},
		{"queued", Record{Status: "queued"}, false},
		{"running", Record{Status: "running"}, false},
		{"pending delivery", Record{Status: "completed", CompletionDeliveryStatus: "pending"}, false},
		{"delivery not classified", Record{Status: "completed", CompletionDeliveryID: "delivery-1"}, false},
		{"auto continue pending", Record{Status: "completed", AutoContinueParent: true}, false},
		{"auto continue starting", Record{Status: "completed", AutoContinueParent: true, AutoContinueStatus: "starting"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &Manager{jobs: []Record{test.record}}
			if got := manager.RuntimeIdleForMaintenance(); got != test.idle {
				t.Fatalf("runtime idle = %t, want %t", got, test.idle)
			}
		})
	}
}
