package discovery

import "testing"

func TestJoinPorts(t *testing.T) {
	if got := JoinPorts([]int{22, 3389}); got != "22,3389" {
		t.Errorf("JoinPorts = %q", got)
	}
	if got := JoinPorts(nil); got != "" {
		t.Errorf("JoinPorts(nil) = %q", got)
	}
}
