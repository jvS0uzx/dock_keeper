package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

const srvStream = "00000000-0000-0000-0000-0000000000e1"

// Ler /var/log/auth.log de um host de produção é leitura, mas é leitura de dado
// sensível, como root, iniciada por uma pessoa. "Quem leu o log de autenticação
// do servidor X" é a pergunta que a auditoria existe para responder.
func TestAberturaDoStreamDeAuthLogEAuditada(t *testing.T) {
	sede := setupStreamDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/security/auth-log/stream?server_id="+srvStream, nil)
	req = withSession(req, auth.Session{
		UserID: 7, Username: "operador-qa", Role: auth.RoleOperator,
		Accesses: []auth.Access{{SiteID: nil, Role: auth.RoleOperator}},
	})

	testConfig().authLogStreamHandler(rec, req)

	linhas := linhasDe(t, actionAuthLogOpen)
	if len(linhas) != 1 {
		t.Fatalf("linhas gravadas = %d, esperada 1: a abertura do stream não deixou rastro", len(linhas))
	}
	l := linhas[0]
	if l.Result != audit.ResultOK {
		t.Errorf("resultado = %q, esperado %q", l.Result, audit.ResultOK)
	}
	if l.ActorUsername != "operador-qa" {
		t.Errorf("ator = %q, esperado operador-qa", l.ActorUsername)
	}
	if l.TargetID != srvStream {
		t.Errorf("alvo = %q, esperado %s", l.TargetID, srvStream)
	}
	if l.SiteID == nil || *l.SiteID != sede {
		t.Errorf("unidade = %v, esperada %d", l.SiteID, sede)
	}
}

// Recusa por alcance é o sinal mais direto de alguém tentando ler host de
// unidade alheia. lookupServer responde 404 e não 403 para não confirmar
// existência, então sem esta linha a tentativa não deixaria rastro nenhum.
func TestStreamRecusadoPorAlcanceDeixaRastro(t *testing.T) {
	setupStreamDB(t)

	outra := uint(9999)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/security/auth-log/stream?server_id="+srvStream, nil)
	req = withSession(req, auth.Session{
		UserID: 8, Username: "viewer-de-outra-filial", Role: auth.RoleViewer,
		Accesses: []auth.Access{{SiteID: &outra, Role: auth.RoleViewer}},
	})

	testConfig().authLogStreamHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", rec.Code)
	}
	linhas := linhasDe(t, actionAuthLogOpen)
	if len(linhas) != 1 {
		t.Fatalf("linhas gravadas = %d, esperada 1: a recusa não deixou rastro", len(linhas))
	}
	if linhas[0].Result != audit.ResultDenied {
		t.Errorf("resultado = %q, esperado %q", linhas[0].Result, audit.ResultDenied)
	}
}

func setupStreamDB(t *testing.T) uint {
	t.Helper()
	setupAuditAPI(t)

	sede := database.Site{Name: "qa-stream", Code: "qa-stream"}
	database.DB.Where("code = ?", "qa-stream").Delete(&database.Site{})
	if err := database.DB.Create(&sede).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}

	database.DB.Unscoped().Where("id = ?", srvStream).Delete(&database.Server{})
	srv := database.Server{
		ID: srvStream, Name: "host-do-stream", HostIP: "10.91.0.1", Kind: "ssh", SiteID: &sede.ID,
	}
	if err := database.DB.Create(&srv).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}

	t.Cleanup(func() {
		database.DB.Unscoped().Where("id = ?", srvStream).Delete(&database.Server{})
		database.DB.Where("code = ?", "qa-stream").Delete(&database.Site{})
	})
	return sede.ID
}
