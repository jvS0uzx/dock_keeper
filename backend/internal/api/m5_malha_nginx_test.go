package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

type servidorDaMalha struct {
	ID             string  `json:"id"`
	CollectNginx   bool    `json:"collect_nginx"`
	NginxEstado    string  `json:"nginx_estado"`
	NginxMotivo    string  `json:"nginx_motivo"`
	NginxPapel     string  `json:"nginx_papel"`
	NginxChecadoEm *string `json:"nginx_checado_em"`
}

type respostaDaMalha struct {
	Servers        []servidorDaMalha `json:"servers"`
	NginxTopologia []struct {
		ServerID    string `json:"server_id"`
		Bloco       string `json:"bloco"`
		Destino     string `json:"destino"`
		ObservadoEm string `json:"observado_em"`
	} `json:"nginx_topologia"`
}

func lerMalha(t *testing.T, sess auth.Session) respostaDaMalha {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/live", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics/live: status %d (%s)", rec.Code, rec.Body.String())
	}
	var out respostaDaMalha
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta da malha: %v", err)
	}
	return out
}

func TestLiveTrazEstadoEPapelDoNginx(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-estado", auth.RoleAdmin)
	srv := servidorDeRename(t, "malha-estado", "203.0.113.81", nil)

	checado := time.Now().UTC().Truncate(time.Second)
	if err := database.DB.Model(&database.Server{}).Where("id = ?", srv.ID).Updates(map[string]any{
		"nginx_estado":      database.NginxInativo,
		"nginx_motivo":      "Nginx instalado mas parado",
		"nginx_papel":       database.NginxPapelReserva,
		"nginx_checado_em":  checado,
		"nginx_papel_desde": checado,
	}).Error; err != nil {
		t.Fatalf("gravar estado do nginx: %v", err)
	}

	var achado *servidorDaMalha
	for _, s := range lerMalha(t, sess).Servers {
		if s.ID == srv.ID {
			copia := s
			achado = &copia
		}
	}
	if achado == nil {
		t.Fatalf("a resposta não trouxe o servidor %s", srv.ID)
	}
	if achado.NginxEstado != database.NginxInativo {
		t.Errorf("nginx_estado = %q, esperado inativo", achado.NginxEstado)
	}
	if achado.NginxMotivo != "Nginx instalado mas parado" {
		t.Errorf("nginx_motivo = %q, esperado o motivo gravado", achado.NginxMotivo)
	}
	if achado.NginxPapel != database.NginxPapelReserva {
		t.Errorf("nginx_papel = %q, esperado reserva", achado.NginxPapel)
	}
	if achado.NginxChecadoEm == nil {
		t.Errorf("nginx_checado_em veio nulo, esperado a data da sonda")
	}
}

func TestLiveNaoInventaDataDeSondaNaoFeita(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-sem-sonda", auth.RoleAdmin)
	srv := servidorDeRename(t, "malha-sem-sonda", "203.0.113.82", nil)

	for _, s := range lerMalha(t, sess).Servers {
		if s.ID != srv.ID {
			continue
		}
		if s.NginxChecadoEm != nil {
			t.Errorf("nginx_checado_em = %v, esperado null: a sonda nunca rodou", *s.NginxChecadoEm)
		}
		if s.NginxEstado != database.NginxDesconhecido {
			t.Errorf("nginx_estado = %q, esperado desconhecido", s.NginxEstado)
		}
		if s.NginxPapel != database.NginxPapelNenhum {
			t.Errorf("nginx_papel = %q, esperado nenhum", s.NginxPapel)
		}
		return
	}
	t.Fatalf("a resposta não trouxe o servidor %s", srv.ID)
}

func TestLiveTrazATopologiaLidaDoNginx(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-topologia", auth.RoleAdmin)
	srv := servidorDeRename(t, "malha-topologia", "203.0.113.83", nil)

	linha := database.NginxUpstream{
		ServerID: srv.ID, Bloco: "app_backend", Destino: "198.51.100.21:8080",
		ObservadoEm: time.Now().UTC(),
	}
	if err := database.DB.Create(&linha).Error; err != nil {
		t.Fatalf("gravar upstream: %v", err)
	}
	t.Cleanup(func() { database.DB.Where("server_id = ?", srv.ID).Delete(&database.NginxUpstream{}) })

	for _, item := range lerMalha(t, sess).NginxTopologia {
		if item.ServerID != srv.ID {
			continue
		}
		if item.Bloco != "app_backend" || item.Destino != "198.51.100.21:8080" {
			t.Errorf("topologia = %+v, esperado o bloco e o destino gravados", item)
		}
		if item.ObservadoEm == "" {
			t.Errorf("observado_em veio vazio")
		}
		return
	}
	t.Fatalf("a topologia não trouxe o servidor %s", srv.ID)
}

func TestPatchLigaEDesligaACollectNginx(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-patch", auth.RoleAdmin)
	srv := servidorDeRename(t, "malha-patch", "203.0.113.84", nil)
	t.Cleanup(func() {
		ssh.Manager.Stop(srv.ID)
		database.DB.Where("server_id = ?", srv.ID).Delete(&database.Alert{})
	})
	rota := "/api/servers?id=" + srv.ID

	rec := pedirComSessao(t, http.MethodPatch, rota, `{"collect_nginx":true}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH collect_nginx=true: status %d (%s)", rec.Code, rec.Body.String())
	}
	var depois database.Server
	if err := database.DB.Where("id = ?", srv.ID).First(&depois).Error; err != nil {
		t.Fatalf("reler servidor: %v", err)
	}
	if !depois.CollectNginx {
		t.Fatalf("collect_nginx = false depois do PATCH: a coleta continua inalcançável pela tela")
	}

	if rec := pedirComSessao(t, http.MethodPatch, rota, `{"collect_nginx":false}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH collect_nginx=false: status %d (%s)", rec.Code, rec.Body.String())
	}
	if err := database.DB.Where("id = ?", srv.ID).First(&depois).Error; err != nil {
		t.Fatalf("reler servidor: %v", err)
	}
	if depois.CollectNginx {
		t.Errorf("collect_nginx = true, esperado false depois de desligar")
	}
}

func TestPatchSemCampoNenhumContinuaRecusado(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-patch-vazio", auth.RoleAdmin)
	srv := servidorDeRename(t, "malha-patch-vazio", "203.0.113.85", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+srv.ID, `{}`, sess)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH vazio: status %d, esperado 400", rec.Code)
	}
}
