package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestIngestaoGravaEnderecosDeclarados(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-enderecos")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")
	database.DB.Unscoped().Where("name = ?", "estacao-enderecos").Delete(&database.Server{})

	corpo := `{"hostname":"estacao-enderecos","machine_id":"m-enderecos","cpu":12.5,` +
		`"addresses":["100.100.0.2","10.9.0.4","127.0.0.1","nao-e-ip"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(corpo))
	req.Header.Set(headerLegacyToken, "token-enderecos")
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/ingest/metrics: status %d (%s)", rec.Code, rec.Body.String())
	}

	var server database.Server
	if err := database.DB.Where("name = ?", "estacao-enderecos").Take(&server).Error; err != nil {
		t.Fatalf("servidor do agente não foi criado: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("server_id = ?", server.ID).Delete(&database.ServerAddress{})
		database.DB.Where("server_id = ?", server.ID).Delete(&database.MetricServer{})
		database.DB.Unscoped().Where("id = ?", server.ID).Delete(&database.Server{})
	})

	porServidor, err := database.EnderecosPorServidor([]string{server.ID})
	if err != nil {
		t.Fatalf("ler endereços: %v", err)
	}
	guardados := map[string]bool{}
	for _, a := range porServidor[server.ID] {
		guardados[a] = true
	}
	if !guardados["100.100.0.2"] || !guardados["10.9.0.4"] {
		t.Errorf("endereços declarados não foram gravados: %v", porServidor[server.ID])
	}
	if guardados["127.0.0.1"] || guardados["nao-e-ip"] {
		t.Errorf("loopback ou lixo entrou na lista: %v", porServidor[server.ID])
	}
}

func TestLiveDevolveEnderecos(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-enderecos-live", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-live-enderecos", "203.0.113.240", nil)

	database.RegistrarEnderecos(s.ID, []string{"10.4.0.1"})
	database.RegistrarAliases(s.ID, []string{"100.100.0.1"})

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/live", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics/live: status %d", rec.Code)
	}

	var resp struct {
		Servers []struct {
			ID        string   `json:"id"`
			HostIP    string   `json:"host_ip"`
			Addresses []string `json:"addresses"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta do live: %v", err)
	}

	for _, srv := range resp.Servers {
		if srv.ID != s.ID {
			continue
		}
		tem := map[string]bool{}
		for _, a := range srv.Addresses {
			tem[a] = true
		}
		if !tem["10.4.0.1"] || !tem["100.100.0.1"] {
			t.Errorf("live trouxe addresses=%v, esperado coletado e alias juntos", srv.Addresses)
		}
		if !tem[srv.HostIP] {
			t.Errorf("live não incluiu o host_ip %q em addresses=%v", srv.HostIP, srv.Addresses)
		}
		return
	}
	t.Fatalf("servidor %q ausente do live", s.ID)
}

func TestPatchGravaAliasManual(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-alias", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-com-alias", "203.0.113.241", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID,
		`{"aliases":["100.100.0.2","10.5.0.7"]}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH com aliases: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}

	porServidor, err := database.EnderecosPorServidor([]string{s.ID})
	if err != nil {
		t.Fatalf("ler endereços: %v", err)
	}
	if len(porServidor[s.ID]) < 2 {
		t.Errorf("aliases gravados: %v", porServidor[s.ID])
	}
}

func TestPatchAliasInvalidoRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-alias-invalido", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-alias-ruim", "203.0.113.242", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID,
		`{"aliases":["100.100.0.2","nao-e-ip"]}`, sess)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("alias inválido: status %d, esperado 400 (%s)", rec.Code, rec.Body.String())
	}

	porServidor, _ := database.EnderecosPorServidor([]string{s.ID})
	if len(porServidor[s.ID]) != 0 {
		t.Errorf("gravou %v mesmo com um alias inválido no mesmo pedido", porServidor[s.ID])
	}
}
