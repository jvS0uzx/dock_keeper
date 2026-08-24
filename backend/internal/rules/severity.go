package rules

// Níveis de severidade, do menos para o mais grave. Espelham a escala do
// Zabbix reduzida ao que se usa: um alerta ou é informativo, ou pede atenção,
// ou exige ação, ou é queda de serviço.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// severityRank ordena os níveis para comparação com o mínimo notificável.
var severityRank = map[string]int{
	SeverityInfo:     0,
	SeverityWarning:  1,
	SeverityHigh:     2,
	SeverityCritical: 3,
}

// ValidSeverity diz se o valor é um nível conhecido.
func ValidSeverity(s string) bool {
	_, ok := severityRank[s]
	return ok
}

// Rank devolve a posição do nível; desconhecido vira warning, que é o padrão
// do schema.
func Rank(s string) int {
	if r, ok := severityRank[s]; ok {
		return r
	}
	return severityRank[SeverityWarning]
}

// Prefix é o rótulo textual usado na mensagem do alerta.
//
// Sem emoji de propósito: a mensagem vai para Telegram, log e, futuramente,
// e-mail — prefixo em texto sobrevive a todos.
func Prefix(s string) string {
	switch s {
	case SeverityCritical:
		return "[CRITICO]"
	case SeverityHigh:
		return "[ALERTA]"
	case SeverityInfo:
		return "[INFO]"
	default:
		return "[AVISO]"
	}
}
