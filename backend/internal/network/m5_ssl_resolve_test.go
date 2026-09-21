package network

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const dominioM5 = "m5-ssl.exemplo.test"

func capturarAlertasDeSSL(t *testing.T) (avisos, resolvidos *[]alert.Entrada) {
	t.Helper()

	avisos, resolvidos = &[]alert.Entrada{}, &[]alert.Entrada{}
	avisar, resolver := notifyAlert, resolveAlert
	notifyAlert = func(e alert.Entrada) bool { *avisos = append(*avisos, e); return true }
	resolveAlert = func(e alert.Entrada) bool { *resolvidos = append(*resolvidos, e); return true }
	t.Cleanup(func() { notifyAlert, resolveAlert = avisar, resolver })
	return avisos, resolvidos
}

func chaves(entradas []alert.Entrada) []string {
	saida := make([]string, 0, len(entradas))
	for _, e := range entradas {
		saida = append(saida, e.Key)
	}
	return saida
}

func TestCertificadoValidoDeNovoResolveOsDoisAlertas(t *testing.T) {
	avisos, resolvidos := capturarAlertasDeSSL(t)

	avisarCertificado(database.Domain{Name: dominioM5}, SSLInfo{Valid: true, DaysLeft: 80})

	if len(*avisos) != 0 {
		t.Errorf("certificado válido com 80 dias avisou: %v", chaves(*avisos))
	}
	got := chaves(*resolvidos)
	if len(got) != 2 || got[0] != "ssl_invalid:"+dominioM5 || got[1] != "ssl_expiring:"+dominioM5 {
		t.Errorf("resolvidos = %v, esperado ssl_invalid e ssl_expiring de %s", got, dominioM5)
	}
}

func TestCertificadoPertoDeVencerResolveSoOInvalido(t *testing.T) {
	avisos, resolvidos := capturarAlertasDeSSL(t)

	avisarCertificado(database.Domain{Name: dominioM5}, SSLInfo{Valid: true, DaysLeft: 5})

	if got := chaves(*avisos); len(got) != 1 || got[0] != "ssl_expiring:"+dominioM5 {
		t.Errorf("avisos = %v, esperado ssl_expiring", got)
	}
	if (*avisos)[0].Severity != "high" {
		t.Errorf("severidade = %q, esperado high", (*avisos)[0].Severity)
	}
	if got := chaves(*resolvidos); len(got) != 1 || got[0] != "ssl_invalid:"+dominioM5 {
		t.Errorf("resolvidos = %v, esperado só ssl_invalid: o certificado voltou a valer, mas ainda vence logo", got)
	}
}

func TestCertificadoInvalidoNaoResolveNada(t *testing.T) {
	avisos, resolvidos := capturarAlertasDeSSL(t)

	avisarCertificado(database.Domain{Name: dominioM5}, SSLInfo{Valid: false, ErrorMsg: "expirado"})

	if got := chaves(*avisos); len(got) != 1 || got[0] != "ssl_invalid:"+dominioM5 || (*avisos)[0].Severity != "critical" {
		t.Errorf("avisos = %v, esperado ssl_invalid critical", got)
	}
	if len(*resolvidos) != 0 {
		t.Errorf("certificado inválido resolveu %v", chaves(*resolvidos))
	}
}

func TestAlertaDeCertificadoCarregaAOrigemDoServidor(t *testing.T) {
	setupDominioDB(t)
	avisos, _ := capturarAlertasDeSSL(t)

	unidade := database.Site{Name: "Filial do certificado", Code: "m5-ssl"}
	database.DB.Where("code = ?", unidade.Code).Delete(&database.Site{})
	if err := database.DB.Create(&unidade).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}
	srv := database.Server{Name: "zz-m5-ssl", HostIP: "203.0.113.41", SiteID: &unidade.ID}
	if err := database.DB.Create(&srv).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Unscoped().Where("id = ?", srv.ID).Delete(&database.Server{})
		database.DB.Where("id = ?", unidade.ID).Delete(&database.Site{})
	})

	avisarCertificado(database.Domain{Name: dominioM5, ServerID: &srv.ID}, SSLInfo{Valid: false, ErrorMsg: "expirado"})

	if len(*avisos) != 1 {
		t.Fatalf("%d avisos, esperado 1", len(*avisos))
	}
	e := (*avisos)[0]
	if e.ServerID == nil || *e.ServerID != srv.ID || e.SiteID == nil || *e.SiteID != unidade.ID {
		t.Errorf("origem = servidor %v unidade %v, esperado %s e %d", e.ServerID, e.SiteID, srv.ID, unidade.ID)
	}
}
