package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joaov/vd_stats/internal/audit"
)

func TestAuditActionDerivaONomeDaRota(t *testing.T) {
	casos := []struct {
		metodo string
		rota   string
		quer   string
	}{
		{http.MethodPost, "/api/servers", "server.create"},
		{http.MethodDelete, "/api/servers", "server.delete"},
		{http.MethodPatch, "/api/users", "user.update"},
		{http.MethodPut, "/api/alerts/rules", "alert-rule.update"},
		{http.MethodPost, "/api/sites", "site.create"},
		{http.MethodPatch, "/api/network/host", "network-host.update"},
		{http.MethodPost, "/api/floorplans", "floorplan.create"},
		// Segmento dinâmico: o id não pode virar parte do nome, senão cada
		// planta gera uma ação diferente e o filtro por ação deixa de agrupar.
		{http.MethodPut, "/api/floorplans/12", "floorplan.update"},
		{http.MethodDelete, "/api/floorplans/12/pins", "floorplan.delete"},
		// Rotas cujo último segmento já é o verbo.
		{http.MethodPost, "/api/ssl/recheck", "ssl.recheck"},
		{http.MethodPost, "/api/ssl/recheck-all", "ssl.recheck-all"},
		{http.MethodPost, "/api/network/scan", "network.scan"},
		{http.MethodPost, "/api/auth/login", "auth.login"},
		{http.MethodPost, "/api/auth/logout", "auth.logout"},
		// Rota não cadastrada ainda gera linha, com nome que denuncia a omissão.
		{http.MethodPost, "/api/rota/nova", "desconhecido.create"},
	}
	for _, c := range casos {
		t.Run(c.metodo+" "+c.rota, func(t *testing.T) {
			got := auditAction(httptest.NewRequest(c.metodo, c.rota, nil))
			if got != c.quer {
				t.Errorf("auditAction = %q, esperado %q", got, c.quer)
			}
		})
	}
}

func TestAuditResultForTraduzOStatus(t *testing.T) {
	casos := []struct {
		status int
		quer   string
	}{
		{http.StatusOK, audit.ResultOK},
		{http.StatusCreated, audit.ResultOK},
		{http.StatusNoContent, audit.ResultOK},
		{http.StatusUnauthorized, audit.ResultDenied},
		{http.StatusForbidden, audit.ResultDenied},
		{http.StatusNotFound, audit.ResultDenied},
		{http.StatusTooManyRequests, audit.ResultDenied},
		{http.StatusBadRequest, audit.ResultError},
		{http.StatusMethodNotAllowed, audit.ResultError},
		{http.StatusRequestEntityTooLarge, audit.ResultError},
		{http.StatusInternalServerError, audit.ResultError},
	}
	for _, c := range casos {
		if got := auditResultFor(c.status); got != c.quer {
			t.Errorf("auditResultFor(%d) = %q, esperado %q", c.status, got, c.quer)
		}
	}
}

// O ticket de SSE é credencial de uso único. Se ele entrar no detalhe, a
// auditoria passa a guardar a chave que ela deveria estar protegendo.
func TestDetalheNaoCarregaOTicket(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/x?ticket=SEGREDO&token=SEGREDO&server_id=7", nil)

	d := auditDetail(r, http.StatusOK)

	if d["server_id"] != "7" {
		t.Errorf("server_id não entrou no detalhe: %v", d)
	}
	for chave, valor := range d {
		if s, ok := valor.(string); ok && strings.Contains(s, "SEGREDO") {
			t.Errorf("o detalhe carregou credencial em %q: %v", chave, valor)
		}
	}
	if _, ok := d["ticket"]; ok {
		t.Error("ticket entrou no detalhe")
	}
	if _, ok := d["token"]; ok {
		t.Error("token entrou no detalhe")
	}
}

// startSSE decide se há streaming por type assertion para http.Flusher. Um
// envelope que não a satisfaça faz o painel parar de receber dado em tempo real
// sem erro nenhum, então o envelope precisa repassá-la.
func TestAuditWriterRepassaOFlusher(t *testing.T) {
	aw := &auditWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	if _, ok := http.ResponseWriter(aw).(http.Flusher); !ok {
		t.Fatal("auditWriter não implementa http.Flusher: envolver uma rota de SSE a quebraria")
	}
	if _, ok := startSSE(aw); !ok {
		t.Error("startSSE recusou o auditWriter")
	}
}

func TestAuditWriterGuardaOPrimeiroStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	aw := &auditWriter{ResponseWriter: rec, status: http.StatusOK}

	aw.WriteHeader(http.StatusForbidden)
	if aw.status != http.StatusForbidden {
		t.Errorf("status = %d, esperado %d", aw.status, http.StatusForbidden)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("o status não chegou ao writer de baixo: %d", rec.Code)
	}
}

// Write sem WriteHeader é 200 implícito no net/http, e a auditoria precisa
// registrar 200, não zero.
func TestAuditWriterAssumeDuzentosSemWriteHeader(t *testing.T) {
	aw := &auditWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	if _, err := aw.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if aw.status != http.StatusOK {
		t.Errorf("status = %d, esperado %d", aw.status, http.StatusOK)
	}
}

// gravadas coleta as linhas que o middleware produziria, sem banco: substitui o
// Record por uma captura. Como audit.Record fala direto com o database.DB, o
// teste exercita a cadeia e observa o writer, e a checagem de conteúdo da linha
// fica nos testes de integração abaixo.
func rodaMiddleware(t *testing.T, onlyDenied bool, req *http.Request, status int) *httptest.ResponseRecorder {
	t.Helper()

	cfg := testConfig()
	rec := httptest.NewRecorder()
	h := cfg.audit(onlyDenied)(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, status, map[string]string{"status": "seguiu"})
	})
	h(rec, req)
	return rec
}

// O middleware não pode alterar a resposta: ele observa, não intervém.
func TestAuditNaoAlteraAResposta(t *testing.T) {
	casos := []int{http.StatusOK, http.StatusForbidden, http.StatusInternalServerError}
	for _, status := range casos {
		req := httptest.NewRequest(http.MethodPost, "/api/sites", nil)
		rec := rodaMiddleware(t, auditAll, req, status)
		if rec.Code != status {
			t.Errorf("status = %d, esperado %d: o middleware trocou a resposta", rec.Code, status)
		}
		if !strings.Contains(rec.Body.String(), "seguiu") {
			t.Errorf("corpo = %q, o middleware engoliu a resposta do handler", rec.Body.String())
		}
	}
}

// GET tem que atravessar o middleware sem tocar em nada: é o caminho do polling
// do painel, e qualquer trabalho aqui é multiplicado por todo cliente aberto.
func TestAuditDeixaOGetPassarIntacto(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/live", nil)
	rec := rodaMiddleware(t, auditAll, req, http.StatusOK)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, esperado 200", rec.Code)
	}
}
