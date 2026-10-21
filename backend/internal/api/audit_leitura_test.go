package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestLeituraSoEntraNaAuditoriaQuandoNegadaPorEscopo(t *testing.T) {
	casos := map[int]bool{
		http.StatusOK:                  false,
		http.StatusBadRequest:          false,
		http.StatusUnauthorized:        false,
		http.StatusTooManyRequests:     false,
		http.StatusInternalServerError: false,
		http.StatusForbidden:           true,
		http.StatusNotFound:            true,
	}
	for status, quer := range casos {
		if got := leituraNegadaPorEscopo(status); got != quer {
			t.Errorf("leituraNegadaPorEscopo(%d) = %v, esperado %v", status, got, quer)
		}
	}
}

func TestAuditWriterDeixaOResponseControllerChegarNoWriterOriginal(t *testing.T) {
	rec := httptest.NewRecorder()
	aw := &auditWriter{ResponseWriter: rec, status: http.StatusOK}
	if aw.Unwrap() != rec {
		t.Fatal("Unwrap não devolveu o writer original: SetWriteDeadline do stream falharia")
	}
}

func servidorDeOutraUnidade(t *testing.T, siteID uint) database.Server {
	t.Helper()

	garantirUnidade(t, &siteID)
	s := database.Server{Name: "qa-leitura-negada", HostIP: "192.0.2.10", SiteID: &siteID}
	if err := database.DB.Create(&s).Error; err != nil {
		t.Fatalf("criar o servidor do teste: %v", err)
	}
	t.Cleanup(func() { database.DB.Unscoped().Delete(&database.Server{}, "id = ?", s.ID) })
	return s
}

func getComSessao(t *testing.T, path string, sess auth.Session) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func TestLeituraDeServidorDeOutraUnidadeDeixaRastro(t *testing.T) {
	setupAuditAPI(t)

	unidadeA, unidadeB := uint(9101), uint(9102)
	sess := sessaoDeTeste(t, "leitor-unidade-a", []auth.Access{{SiteID: &unidadeA, Role: auth.RoleOperator}})
	alheio := servidorDeOutraUnidade(t, unidadeB)

	casos := []struct {
		path, action string
	}{
		{"/api/security/radar?server_id=" + alheio.ID, "security-radar.read"},
		{"/api/containers/logs/stream?container_name=app&server_id=" + alheio.ID, "container-logs.read"},
		{"/api/security/radar?server_id=00000000-0000-0000-0000-000000000000", "security-radar.read"},
	}
	for _, c := range casos {
		limparAuditoria(t)

		rec := getComSessao(t, c.path, sess)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, esperado 404", c.path, rec.Code)
		}

		l := umaLinha(t, c.action)
		if l.Result != audit.ResultDenied {
			t.Errorf("%s: result = %q, esperado denied", c.path, l.Result)
		}
		if l.ActorUsername != "leitor-unidade-a" {
			t.Errorf("%s: actor_username = %q, esperado leitor-unidade-a", c.path, l.ActorUsername)
		}
		if l.TargetID == "" {
			t.Errorf("%s: target_id vazio, o id pedido se perdeu", c.path)
		}
	}
}

func TestLeituraPermitidaNaoEntraNaAuditoria(t *testing.T) {
	setupAuditAPI(t)

	unidadeA := uint(9101)
	sess := sessaoDeTeste(t, "leitor-sem-rastro", []auth.Access{{SiteID: &unidadeA, Role: auth.RoleOperator}})

	rec := getComSessao(t, "/api/sites", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}

	var total int64
	database.DB.Model(&database.AuditLog{}).Count(&total)
	if total != 0 {
		t.Errorf("auditoria tem %d linhas depois de uma leitura permitida, esperado 0", total)
	}
}
