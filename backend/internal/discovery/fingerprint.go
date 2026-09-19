package discovery

import (
	"strconv"
	"strings"
)

const (
	TypePrinter   = "printer"
	TypeWindows   = "windows"
	TypeNAS       = "nas"
	TypeLinux     = "linux"
	TypeWebDevice = "web-device"
	TypeUnknown   = "unknown"
)

var fingerprints = []struct {
	Port int
	Type string
}{
	{9100, TypePrinter},
	{515, TypePrinter},
	{631, TypePrinter},
	{5000, TypeNAS},
	{3389, TypeWindows},
	{445, TypeWindows},
	{139, TypeWindows},
	{135, TypeWindows},
	{22, TypeLinux},
	{443, TypeWebDevice},
	{80, TypeWebDevice},
}

func ParsePorts(raw string) []int {
	var ports []int
	for _, part := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && n > 0 && n < 65536 {
			ports = append(ports, n)
		}
	}
	return ports
}

func DeviceType(openPorts []int) string {
	open := make(map[int]bool, len(openPorts))
	for _, p := range openPorts {
		open[p] = true
	}

	for _, f := range fingerprints {
		if open[f.Port] {
			return f.Type
		}
	}
	return TypeUnknown
}
