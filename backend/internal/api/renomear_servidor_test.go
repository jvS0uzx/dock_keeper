package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

func servidorDeRename(t *testing.T, nome, ip string, siteID *uint) database.Server {
	t.Helper()

	database.DB.Unscoped().Where("host_ip = ?", ip).Delete(&database.Server{})
	s := database.Server{Name: nome, HostIP: ip, User: "root", Port: 22, SiteID: siteID}
	if err := database.DB.Create(&s).Error; err != nil {
		t.Fatalf("criar servidor %s: %v", nome, err)
	}
	t.Cleanup(func() {
		ssh.Manager.Stop(s.ID)
		database.DB.Unscoped().Where("host_ip = ?", ip).Delete(&database.Server{})
	})
	return s
}

func unidadeDeRename(t *testing.T, code string) uint {
	t.Helper()

	database.DB.Where("code = ?", code).Delete(&database.Site{})
	site := database.Site{Name: code, Code: code}
	if err := database.DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade %s: %v", code, err)
	}
	t.Cleanup(func() { database.DB.Where("code = ?", code).Delete(&database.Site{}) })
	return site.ID
}

func TestPatchRenomeiaServidor(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-rename", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-antiga", "203.0.113.210", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID, `{"name":"Node Cascavel"}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/servers: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}

	var devolvido database.Server
	if err := json.Unmarshal(rec.Body.Bytes(), &devolvido); err != nil {
		t.Fatalf("resposta do PATCH: %v", err)
	}
	if devolvido.Name != "Node Cascavel" {
		t.Errorf("resposta trouxe name=%q, esperado %q", devolvido.Name, "Node Cascavel")
	}

	var noBanco database.Server
	if err := database.DB.Where("id = ?", s.ID).Take(&noBanco).Error; err != nil {
		t.Fatalf("reler servidor: %v", err)
	}
	if noBanco.Name != "Node Cascavel" {
		t.Errorf("banco tem name=%q, esperado %q", noBanco.Name, "Node Cascavel")
	}
	if noBanco.HostIP != "203.0.113.210" {
		t.Errorf("host_ip mudou para %q; ele é a identidade da coleta", noBanco.HostIP)
	}
}

func TestPatchAceitaUsuarioEPorta(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-rename-porta", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-porta", "203.0.113.211", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID,
		`{"name":"vps-porta","user":"dockkeeper-monitor","port":2222}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH com user e port: status %d (%s)", rec.Code, rec.Body.String())
	}

	var noBanco database.Server
	database.DB.Where("id = ?", s.ID).Take(&noBanco)
	if noBanco.User != "dockkeeper-monitor" || noBanco.Port != 2222 {
		t.Errorf("banco tem user=%q port=%d, esperado dockkeeper-monitor e 2222", noBanco.User, noBanco.Port)
	}
}

func TestPatchNomeInvalidoRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-rename-invalido", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-nome-ok", "203.0.113.212", nil)

	casos := map[string]string{
		"vazio":          `{"name":""}`,
		"so espaco":      `{"name":"   "}`,
		"acima de 64":    `{"name":"nome-gigante-nome-gigante-nome-gigante-nome-gigante-nome-gigante-123"}`,
		"corpo invalido": `{`,
	}
	for caso, corpo := range casos {
		rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID, corpo, sess)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, esperado 400 (%s)", caso, rec.Code, rec.Body.String())
		}
	}

	var noBanco database.Server
	database.DB.Where("id = ?", s.ID).Take(&noBanco)
	if noBanco.Name != "vps-nome-ok" {
		t.Errorf("nome mudou para %q depois de pedido inválido", noBanco.Name)
	}
}

func TestPatchNomeRepetidoNaUnidadeRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-rename-dup", auth.RoleAdmin)
	unidade := unidadeDeRename(t, "rename-dup")

	servidorDeRename(t, "matriz-01", "203.0.113.213", &unidade)
	outro := servidorDeRename(t, "matriz-02", "203.0.113.214", &unidade)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+outro.ID, `{"name":"matriz-01"}`, sess)
	if rec.Code != http.StatusConflict {
		t.Fatalf("nome repetido na unidade: status %d, esperado 409 (%s)", rec.Code, rec.Body.String())
	}

	var noBanco database.Server
	database.DB.Where("id = ?", outro.ID).Take(&noBanco)
	if noBanco.Name != "matriz-02" {
		t.Errorf("nome virou %q mesmo com o 409", noBanco.Name)
	}
}

func TestPatchServidorInexistenteResponde404(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-rename-404", auth.RoleAdmin)

	rec := pedirComSessao(t, http.MethodPatch,
		"/api/servers?id=00000000-0000-0000-0000-0000000000ee", `{"name":"qualquer"}`, sess)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("servidor inexistente: status %d, esperado 404", rec.Code)
	}
}

func TestRenomearNaoDerrubaAColeta(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-rename-coleta", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-coletando", "203.0.113.215", nil)

	ssh.Manager.Start(ssh.Target{ID: s.ID, Name: s.Name, Host: s.HostIP, User: s.User, Port: s.Port})
	if !ssh.Manager.Rodando(s.ID) {
		t.Fatalf("a coleta não subiu para o servidor de teste")
	}

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID, `{"name":"vps-renomeada"}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: status %d (%s)", rec.Code, rec.Body.String())
	}
	if !ssh.Manager.Rodando(s.ID) {
		t.Errorf("a coleta caiu depois do rename; o Manager acompanha por id e não deveria reiniciar")
	}
}

func TestRenomearGeraAuditoria(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-rename-audit", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-auditada", "203.0.113.216", nil)

	pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID, `{"name":"vps-auditada-2"}`, sess)

	var linhas int64
	database.DB.Model(&database.AuditLog{}).
		Where("action = ? AND target_id = ?", "server.update", s.ID).
		Count(&linhas)
	if linhas != 1 {
		t.Errorf("auditoria do rename: %d linhas, esperado 1", linhas)
	}
}
