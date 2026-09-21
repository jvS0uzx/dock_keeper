package rules

const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

var severityRank = map[string]int{
	SeverityInfo:     0,
	SeverityWarning:  1,
	SeverityHigh:     2,
	SeverityCritical: 3,
}

func ValidSeverity(s string) bool {
	_, ok := severityRank[s]
	return ok
}

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
