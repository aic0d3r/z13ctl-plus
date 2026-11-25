package daemon

// hotplug_test.go — Tests for the keyboard reattach watcher's decision.

import "testing"

func TestHotplugNeedsReopen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		present, held, alive bool
		want                 bool
	}{
		{"attached and held live", true, true, true, false},
		{"attached but never opened (startup failure)", true, false, false, true},
		{"attached but re-enumerated between polls", true, true, false, true},
		{"detached, dead node still held", false, true, false, true},
		{"detached and dropped", false, false, false, false},
	}
	for _, tt := range tests {
		if got := hotplugNeedsReopen(tt.present, tt.held, tt.alive); got != tt.want {
			t.Errorf("%s: got %t, want %t", tt.name, got, tt.want)
		}
	}
}
