package discovery

import "testing"

// ParsePorts existe para reinferir o tipo quando o operador destrava um host,
// lendo a coluna open_ports que guarda a lista como texto.
func TestParsePorts(t *testing.T) {
	casos := map[string][]int{
		"22,80,443":    {22, 80, 443},
		" 22 , 3389 ":  {22, 3389},
		"":             nil,
		"abc":          nil,
		"0,70000,-1":   nil,
		"22,abc,,3389": {22, 3389},
		"9100":         {9100},
	}

	for entrada, esperado := range casos {
		obtido := ParsePorts(entrada)
		if len(obtido) != len(esperado) {
			t.Errorf("ParsePorts(%q) = %v, esperado %v", entrada, obtido, esperado)
			continue
		}
		for i := range esperado {
			if obtido[i] != esperado[i] {
				t.Errorf("ParsePorts(%q) = %v, esperado %v", entrada, obtido, esperado)
				break
			}
		}
	}
}

// Destravar um host precisa devolvê-lo ao mesmo tipo que a varredura inferiria,
// e é este par de funções que faz isso sem esperar o próximo ciclo.
func TestParsePortsAlimentaDeviceType(t *testing.T) {
	casos := map[string]string{
		"80,443,9100": TypePrinter,
		"135,139,445": TypeWindows,
		"22":          TypeLinux,
		"5000,445":    TypeNAS,
		"":            TypeUnknown,
		"12345":       TypeUnknown,
	}

	for portas, esperado := range casos {
		if obtido := DeviceType(ParsePorts(portas)); obtido != esperado {
			t.Errorf("portas %q: tipo = %q, esperado %q", portas, obtido, esperado)
		}
	}
}
