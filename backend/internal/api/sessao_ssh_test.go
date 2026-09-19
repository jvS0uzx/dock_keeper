package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

func ocuparSessoes(t *testing.T, alvo ssh.Target, n int) {
	t.Helper()
	for range n {
		release, err := ssh.AcquireSession(alvo)
		if err != nil {
			t.Fatalf("ocupar sessão: %v", err)
		}
		t.Cleanup(release)
	}
}

func TestTerceiraSessaoSSHRecebe503SemAbrirConexao(t *testing.T) {
	setupStreamDB(t)
	t.Setenv("SSH_MAX_SESSIONS_PER_HOST", "2")
	ocuparSessoes(t, ssh.Target{ID: srvStream}, 2)

	sessao := auth.Session{
		UserID: 7, Username: "operador-qa", Role: auth.RoleOperator,
		Accesses: []auth.Access{{SiteID: nil, Role: auth.RoleOperator}},
	}
	rotas := map[string]http.HandlerFunc{
		"/api/security/radar?server_id=" + srvStream:          testConfig().securityRadarHandler,
		"/api/security/authlog/stream?server_id=" + srvStream: testConfig().authLogStreamHandler,
	}
	for rota, handler := range rotas {
		rec := httptest.NewRecorder()
		handler(rec, withSession(httptest.NewRequest(http.MethodGet, rota, nil), sessao))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s com o limite ocupado: status %d, esperado 503", rota, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "limite de sessões SSH simultâneas") {
			t.Errorf("%s: corpo %q não explica o limite", rota, rec.Body.String())
		}
	}
}
