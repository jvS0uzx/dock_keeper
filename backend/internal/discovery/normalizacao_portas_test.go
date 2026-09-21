package discovery

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizarPortas(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  []int
		esperado []int
	}{
		{"lista vazia", nil, []int{}},
		{"ordena crescente", []int{443, 22, 80}, []int{22, 80, 443}},
		{"remove duplicata", []int{80, 80, 443, 80}, []int{80, 443}},
		{"descarta fora de faixa", []int{0, -1, 65536, 70000, 22}, []int{22}},
		{"mantem limite da faixa", []int{65535, 1}, []int{1, 65535}},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			obtido := NormalizarPortas(caso.entrada)
			if !slices.Equal(obtido, caso.esperado) {
				t.Errorf("NormalizarPortas(%v) = %v, esperado %v", caso.entrada, obtido, caso.esperado)
			}
		})
	}
}

func TestNormalizarPortasAplicaTetoDepoisDeOrdenar(t *testing.T) {
	entrada := make([]int, 0, 300)
	for p := 300; p >= 1; p-- {
		entrada = append(entrada, p)
	}

	obtido := NormalizarPortas(entrada)
	if len(obtido) != MaxPortasGravadas {
		t.Fatalf("len = %d, esperado %d", len(obtido), MaxPortasGravadas)
	}
	if obtido[0] != 1 {
		t.Errorf("primeira porta = %d, esperado 1", obtido[0])
	}
	if obtido[len(obtido)-1] != MaxPortasGravadas {
		t.Errorf("última porta = %d, esperado %d", obtido[len(obtido)-1], MaxPortasGravadas)
	}
}

func TestJoinPortsNaoEstouraColunaDeTexto(t *testing.T) {
	entrada := make([]int, 0, 1000)
	for p := 1; p <= 1000; p++ {
		entrada = append(entrada, p)
	}

	saida := JoinPorts(entrada)
	if n := len(strings.Split(saida, ",")); n != MaxPortasGravadas {
		t.Fatalf("portas na string = %d, esperado %d", n, MaxPortasGravadas)
	}
	for _, parte := range strings.Split(saida, ",") {
		if _, err := strconv.Atoi(parte); err != nil {
			t.Fatalf("parte %q não é número: %v", parte, err)
		}
	}
}
