package discovery

import "testing"

func TestDeviceType(t *testing.T) {
	cases := []struct {
		nome  string
		ports []int
		want  string
	}{
		{"impressora de rede", []int{80, 443, 9100}, TypePrinter},
		{"impressora IPP", []int{631, 80}, TypePrinter},
		{"estacao windows", []int{135, 139, 445, 3389}, TypeWindows},
		{"linux com ssh", []int{22}, TypeLinux},
		{"nas", []int{5000, 445}, TypeNAS},
		{"so interface web", []int{80, 443}, TypeWebDevice},
		{"nada reconhecido", []int{12345}, TypeUnknown},
		{"sem portas", nil, TypeUnknown},
	}

	for _, c := range cases {
		if got := DeviceType(c.ports); got != c.want {
			t.Errorf("%s: DeviceType(%v) = %q, esperado %q", c.nome, c.ports, got, c.want)
		}
	}
}

// 9100 identifica impressora melhor que 80, que qualquer equipamento com
// interface web também abre. A ordem da lista precisa refletir isso.
func TestDeviceTypePrioridade(t *testing.T) {
	if got := DeviceType([]int{80, 443, 445, 9100}); got != TypePrinter {
		t.Errorf("impressora com SMB foi classificada como %q", got)
	}
	if got := DeviceType([]int{22, 445, 3389}); got != TypeWindows {
		t.Errorf("Windows com SSH foi classificado como %q", got)
	}
}
