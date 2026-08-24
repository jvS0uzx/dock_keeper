package discovery

import (
	"strconv"
	"strings"
)

// Tipos de equipamento reconhecidos pela varredura. Ficam como constante para
// o painel poder filtrar por um valor estável em vez de texto livre.
const (
	TypePrinter   = "printer"
	TypeWindows   = "windows"
	TypeNAS       = "nas"
	TypeLinux     = "linux"
	TypeWebDevice = "web-device"
	TypeUnknown   = "unknown"
)

// fingerprint associa uma porta ao tipo provável de equipamento. A ordem
// importa: a primeira porta que casar decide, então o sinal mais específico vem
// antes. 9100 (fila de impressão bruta) identifica impressora melhor que 80,
// que qualquer coisa com interface web também abre.
var fingerprints = []struct {
	Port int
	Type string
}{
	{9100, TypePrinter},
	{515, TypePrinter},
	{631, TypePrinter},
	// 5000 antes das portas Windows: um NAS também publica SMB (445), então o
	// SMB sozinho não distingue NAS de estação.
	{5000, TypeNAS},
	{3389, TypeWindows},
	{445, TypeWindows},
	{139, TypeWindows},
	{135, TypeWindows},
	{22, TypeLinux},
	{443, TypeWebDevice},
	{80, TypeWebDevice},
}

// ParsePorts lê a coluna open_ports, que guarda a lista como texto
// ("22,80,443"), e devolve os números.
//
// Existe porque destravar o tipo de um host precisa reinferi-lo na hora, a
// partir das portas já gravadas — esperar a próxima varredura deixaria o campo
// desatualizado por até um ciclo inteiro.
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

// DeviceType infere o tipo de equipamento pelas portas abertas.
//
// É um palpite, não uma identificação: serve para o operador achar a máquina na
// lista, e o painel permite sobrescrever. Sem porta reconhecida devolve
// TypeUnknown em vez de string vazia, para o campo nunca ficar ambíguo entre
// "não sei" e "ainda não varri".
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
