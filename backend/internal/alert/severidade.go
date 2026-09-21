package alert

import (
	"os"
	"strings"
)

const severidadeMinimaPadrao = "warning"

var ordemDeSeveridade = map[string]int{
	"info":     0,
	"warning":  1,
	"high":     2,
	"critical": 3,
}

func abaixoDoMinimo(severidade string) bool {
	minimo, ok := ordemDeSeveridade[strings.ToLower(strings.TrimSpace(os.Getenv("ALERT_MIN_SEVERITY")))]
	if !ok {
		minimo = ordemDeSeveridade[severidadeMinimaPadrao]
	}
	ordem, ok := ordemDeSeveridade[severidade]
	if !ok {
		ordem = ordemDeSeveridade[severidadeMinimaPadrao]
	}
	return ordem < minimo
}
