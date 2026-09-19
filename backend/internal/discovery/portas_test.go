package discovery

import (
	"slices"
	"testing"

	"github.com/jvS0uzx/dockkeeper_collector/scan"
)

func TestPortasPadraoSaoAsDoContrato(t *testing.T) {
	contrato := []int{22, 80, 135, 139, 443, 445, 515, 631, 3389, 5000, 8080, 9100}
	if !slices.Equal(scan.DefaultPorts, contrato) {
		t.Errorf("scan.DefaultPorts = %v, contrato %v", scan.DefaultPorts, contrato)
	}
}

func TestClassificacaoUsaAsPortasDoContrato(t *testing.T) {
	if tipo := DeviceType([]int{515, 9100}); tipo != "printer" {
		t.Errorf("DeviceType(impressora) = %q", tipo)
	}
	if tipo := DeviceType([]int{3389}); tipo != "windows" {
		t.Errorf("DeviceType(rdp) = %q", tipo)
	}
}
