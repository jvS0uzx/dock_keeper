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

func collectBancosDaListagem(t *testing.T, sess auth.Session, id string) (bool, bool) {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, "/api/servers", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/servers: status %d (%s)", rec.Code, rec.Body.String())
	}
	var lista []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("listagem de servidores: %v", err)
	}
	for _, s := range lista {
		if s["id"] != id {
			continue
		}
		valor, ok := s["collect_bancos"].(bool)
		return valor, ok
	}
	t.Fatalf("servidor %s ausente da listagem", id)
	return false, false
}

func TestServidorNovoListaASondaDeBancosLigada(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-lista", auth.RoleAdmin)
	srv := servidorDeRename(t, "bancos-lista", "203.0.113.86", nil)

	ligada, presente := collectBancosDaListagem(t, sess, srv.ID)
	if !presente {
		t.Fatal("a listagem não expõe collect_bancos")
	}
	if !ligada {
		t.Error("collect_bancos = false para servidor novo, esperado ligado por padrão")
	}
}

func TestPatchDesligaELigaASondaDeBancos(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-patch", auth.RoleAdmin)
	srv := servidorDeRename(t, "bancos-patch", "203.0.113.87", nil)
	t.Cleanup(func() {
		ssh.Manager.Stop(srv.ID)
		database.DB.Where("server_id = ?", srv.ID).Delete(&database.Alert{})
	})
	rota := "/api/servers?id=" + srv.ID

	inst := database.PostgresInstancia{
		ServerID: srv.ID, Porta: 5432, Papel: "primario", Estado: "ativo", ObservadoEm: time.Now().UTC(),
	}
	if err := database.DB.Create(&inst).Error; err != nil {
		t.Fatalf("semear instância: %v", err)
	}

	rec := pedirComSessao(t, http.MethodPatch, rota, `{"collect_bancos":false}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH collect_bancos=false: status %d (%s)", rec.Code, rec.Body.String())
	}
	var resposta map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resposta); err != nil {
		t.Fatalf("resposta do PATCH: %v", err)
	}
	if resposta["collect_bancos"] != false {
		t.Errorf("resposta traz collect_bancos = %v, esperado false", resposta["collect_bancos"])
	}

	var depois database.Server
	if err := database.DB.Where("id = ?", srv.ID).First(&depois).Error; err != nil {
		t.Fatalf("reler servidor: %v", err)
	}
	if depois.CollectBancos {
		t.Fatal("collect_bancos continua true depois do PATCH")
	}
	if ligada, _ := collectBancosDaListagem(t, sess, srv.ID); ligada {
		t.Error("a listagem mostra a sonda ligada depois de desligar")
	}

	prazo := time.Now().Add(5 * time.Second)
	for {
		var restantes int64
		database.DB.Model(&database.PostgresInstancia{}).Where("server_id = ?", srv.ID).Count(&restantes)
		if restantes == 0 {
			break
		}
		if time.Now().After(prazo) {
			t.Fatalf("inventário do servidor continua com %d instância(s) depois de desligar a sonda", restantes)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if rec := pedirComSessao(t, http.MethodPatch, rota, `{"collect_bancos":true}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH collect_bancos=true: status %d (%s)", rec.Code, rec.Body.String())
	}
	if err := database.DB.Where("id = ?", srv.ID).First(&depois).Error; err != nil {
		t.Fatalf("reler servidor: %v", err)
	}
	if !depois.CollectBancos {
		t.Error("collect_bancos = false, esperado true depois de religar")
	}
}

func TestPatchSoComCollectBancosNaoERecusadoComoVazio(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-so", auth.RoleAdmin)
	srv := servidorDeRename(t, "bancos-so", "203.0.113.88", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+srv.ID, `{"collect_bancos":true}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH só com collect_bancos: status %d (%s), esperado 200", rec.Code, rec.Body.String())
	}
}
