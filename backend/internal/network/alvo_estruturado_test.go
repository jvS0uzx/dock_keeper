package network

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestCertificadoVencendoSaiComDiasELimiar(t *testing.T) {
	avisos, _ := capturarAlertasDeSSL(t)

	avisarCertificado(database.Domain{Name: dominioM5}, SSLInfo{Valid: true, DaysLeft: 3})

	if len(*avisos) != 1 {
		t.Fatalf("avisos = %d, esperado exatamente um", len(*avisos))
	}
	e := (*avisos)[0]
	if e.AlvoTipo != database.AlvoTipoServico || e.AlvoNome != dominioM5 {
		t.Errorf("alvo = (%q, %q), esperado o domínio como serviço", e.AlvoTipo, e.AlvoNome)
	}
	if e.Metrica != "certificado_dias" || e.Unidade != "dias" {
		t.Errorf("metrica = %q e unidade = %q, esperado certificado_dias em dias", e.Metrica, e.Unidade)
	}
	if e.Valor == nil || *e.Valor != 3 {
		t.Errorf("Valor = %v, esperado 3 dias restantes", e.Valor)
	}
	if e.Limiar == nil || *e.Limiar != float64(sslWarnDays) {
		t.Errorf("Limiar = %v, esperado %d", e.Limiar, sslWarnDays)
	}
}

func TestCertificadoInvalidoNaoInventaDiasRestantes(t *testing.T) {
	avisos, _ := capturarAlertasDeSSL(t)

	avisarCertificado(database.Domain{Name: dominioM5}, SSLInfo{Valid: false, ErrorMsg: "cadeia incompleta"})

	if len(*avisos) != 1 {
		t.Fatalf("avisos = %d, esperado exatamente um", len(*avisos))
	}
	e := (*avisos)[0]
	if e.AlvoTipo != database.AlvoTipoServico || e.AlvoNome != dominioM5 {
		t.Errorf("alvo = (%q, %q), esperado o domínio como serviço", e.AlvoTipo, e.AlvoNome)
	}
	if e.Metrica != "estado" {
		t.Errorf("Metrica = %q, esperado estado", e.Metrica)
	}
	if e.Valor != nil {
		t.Errorf("Valor = %v, certificado inválido não tem dias restantes observados", e.Valor)
	}
}
