package discovery

import "testing"

func TestJoinPorts(t *testing.T) {
	if got := joinPorts([]int{22, 3389}); got != "22,3389" {
		t.Errorf("joinPorts = %q", got)
	}
	if got := joinPorts(nil); got != "" {
		t.Errorf("joinPorts(nil) = %q", got)
	}
}
