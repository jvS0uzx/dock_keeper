package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

type servidorNaResposta struct {
	ID           string   `json:"id"`
	Aliases      []string `json:"aliases"`
	Addresses    []string `json:"addresses"`
	AbsenceAlert *bool    `json:"absence_alert"`
}

func servidorNaLista(t *testing.T, sess auth.Session, rota, id string) servidorNaResposta {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, rota, "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d (%s)", rota, rec.Code, rec.Body.String())
	}
	var lista []servidorNaResposta
	if rota == "/api/metrics/live" {
		var live struct {
			Servers []servidorNaResposta `json:"servers"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &live); err != nil {
			t.Fatalf("GET %s: %v", rota, err)
		}
		lista = live.Servers
	} else if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("GET %s: %v", rota, err)
	}
	for _, s := range lista {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("GET %s não trouxe o servidor %s", rota, id)
	return servidorNaResposta{}
}

func TestAliasesSubstituemAListaManualInteira(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-b4-aliases", auth.RoleAdmin)
	srv := servidorDeRename(t, "m5-b4-aliases", "203.0.113.61", nil)
	t.Cleanup(func() { database.DB.Where("server_id = ?", srv.ID).Delete(&database.ServerAddress{}) })
	database.RegistrarEnderecos(srv.ID, []string{"10.61.0.5"})
	rota := "/api/servers?id=" + srv.ID

	if rec := pedirComSessao(t, http.MethodPatch, rota, `{"aliases":["198.51.100.61","198.51.100.62"]}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH com dois aliases: status %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := pedirComSessao(t, http.MethodPatch, rota, `{"aliases":["198.51.100.62"]}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH com um alias: status %d (%s)", rec.Code, rec.Body.String())
	}

	for _, origem := range []string{"/api/servers", "/api/metrics/live"} {
		s := servidorNaLista(t, sess, origem, srv.ID)
		if !slices.Equal(s.Aliases, []string{"198.51.100.62"}) {
			t.Errorf("%s: aliases = %v, esperado só 198.51.100.62: o alias tirado da lista continua gravado", origem, s.Aliases)
		}
		if !slices.Contains(s.Addresses, "10.61.0.5") || !slices.Contains(s.Addresses, "198.51.100.62") || slices.Contains(s.Addresses, "198.51.100.61") {
			t.Errorf("%s: addresses = %v, esperado o coletado e o alias que ficou", origem, s.Addresses)
		}
	}

	if rec := pedirComSessao(t, http.MethodPatch, rota, `{"aliases":[]}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH com lista vazia: status %d (%s)", rec.Code, rec.Body.String())
	}
	s := servidorNaLista(t, sess, "/api/servers", srv.ID)
	if len(s.Aliases) != 0 || !slices.Contains(s.Addresses, "10.61.0.5") {
		t.Errorf("lista vazia: aliases = %v addresses = %v, esperado nenhum alias e o coletado intacto", s.Aliases, s.Addresses)
	}
}

func TestAusenciaDaEstacaoEConfiguravelPorPatch(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-ausencia-patch", auth.RoleAdmin)
	srv := servidorDeRename(t, "m5-ausencia-patch", "203.0.113.62", nil)
	rota := "/api/servers?id=" + srv.ID

	if s := servidorNaLista(t, sess, "/api/servers", srv.ID); s.AbsenceAlert == nil || *s.AbsenceAlert {
		t.Errorf("absence_alert de servidor novo = %v, esperado false", s.AbsenceAlert)
	}
	if rec := pedirComSessao(t, http.MethodPatch, rota, `{"absence_alert":true}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH absence_alert: status %d (%s)", rec.Code, rec.Body.String())
	}
	for _, origem := range []string{"/api/servers", "/api/metrics/live"} {
		if s := servidorNaLista(t, sess, origem, srv.ID); s.AbsenceAlert == nil || !*s.AbsenceAlert {
			t.Errorf("%s: absence_alert = %v depois do PATCH, esperado true", origem, s.AbsenceAlert)
		}
	}
	if rec := pedirComSessao(t, http.MethodPatch, rota, `{"absence_alert":false}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH absence_alert=false: status %d", rec.Code)
	}
	if s := servidorNaLista(t, sess, "/api/servers", srv.ID); s.AbsenceAlert == nil || *s.AbsenceAlert {
		t.Errorf("absence_alert = %v depois de desligar, esperado false", s.AbsenceAlert)
	}
}
