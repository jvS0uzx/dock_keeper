package api

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"
)

func capturarLogDaAPI(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(original) })
	return &buf
}

func TestJanelaDaMalhaNaoPassaDaRetencao(t *testing.T) {
	t.Setenv("LB_MEMBERSHIP_DAYS", "30")
	t.Setenv("METRIC_RETENTION_DAYS", "7")

	buf := capturarLogDaAPI(t)
	if got := janelaDaMalha(); got != 7*24*time.Hour {
		t.Errorf("janela da malha = %s, esperado 168h: além da retenção a tabela já foi podada", got)
	}
	if AvisarJanelaDaMalha(); !strings.Contains(buf.String(), "LB_MEMBERSHIP_DAYS") {
		t.Errorf("o boot não avisou que LB_MEMBERSHIP_DAYS passa da retenção: %q", buf.String())
	}

	t.Setenv("LB_MEMBERSHIP_DAYS", "3")
	if got := janelaDaMalha(); got != 3*24*time.Hour {
		t.Errorf("janela da malha = %s, esperado 72h quando cabe na retenção", got)
	}
}
